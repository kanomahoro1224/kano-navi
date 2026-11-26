import { POST_WINDOW_DAYS } from "./content"
import { readJsonWithin } from "./api"
import {
  hasSupportedProjection,
  type SupportedSnapshot,
} from "./snapshot-cache"

/** Revision polling, used only while the revision stream is unavailable. */
const REVISION_POLL_MS = 30000
/** First retry after a failed read; it doubles up to MAX_RETRY_MS. */
const RETRY_MS = 8000
const REQUEST_TIMEOUT_MS = 15000
const MAX_RETRY_MS = 60000
/** Time-derived projections such as liveStatus expire without a revision. */
const SNAPSHOT_MS = 60000
/**
 * A snapshot is read this long after its projection's next transition, so
 * the server's clock has passed it too.
 */
const TRANSITION_MARGIN_MS = 1000
/** The shortest gap between two reads that no change or wake started. */
const MIN_GAP_MS = 1000
/** Pushed changes spread their snapshot reads over this window. */
const PUSH_JITTER_MS = 3000
/** Stream errors without a message in between before polling takes over. */
const STREAM_FAILURES = 3
/** How long polling stands in before the stream is tried again. */
const STREAM_RETRY_MS = 300000
const REVISION_STREAM_URL = "/api/dashboard/events"

export type LoadResult = { ok: true } | { ok: false; error: Error }

/** What the snapshot coordinator reports to, and keeps current. */
export interface SnapshotSyncHandlers {
  /** A snapshot read starts; `hasLoaded` says whether one was read before. */
  onReadStart(hasLoaded: boolean): void
  /** A snapshot with the supported projection arrived. */
  onSnapshot(payload: SupportedSnapshot): void
  /** A snapshot read failed; the known snapshot stays. */
  onReadFailed(error: Error): void
  /** The board's calendar, which follows the snapshot and its revision. */
  calendar: {
    /**
     * A full snapshot at `revision` was installed; `later` when it replaced
     * an earlier one, which also refreshes time-based calendar fields.
     */
    afterSnapshot(revision: number | null, later: boolean): void
    /** A revision that left the snapshot current; rereads a stale calendar. */
    followRevision(revision: number): void
  }
}

/**
 * Reads the stored dashboard snapshot and follows the durable revision
 * counter, so MCP or admin changes appear without a manual reload. The
 * server pushes revision changes over an EventSource stream while the page
 * is visible; when the stream is unavailable this polls the revision
 * instead. Nothing here reaches an external platform. `stop` closes the
 * stream, aborts the read and removes every timer and listener.
 */
export function startSnapshotSync(handlers: SnapshotSyncHandlers) {
  const { calendar } = handlers
  let active = true
  let loadedAt = 0
  // When the loaded snapshot's read started, in this browser's clock.
  let snapshotStartedAt = 0
  // The loaded projection's next time-based change in this browser's clock,
  // or 0 without one.
  let transitionAt = 0
  let hasLoaded = false
  let revision: number | null = null
  let needsSnapshot = true
  let failures = 0
  let generation = 0
  let timer: number | undefined
  let nextCheckAt = 0
  let lastStartedAt = 0
  let flight: {
    kind: "snapshot" | "revision"
    controller: AbortController
    promise: Promise<LoadResult>
  } | null = null
  // The revision stream, while open or reconnecting. Without it the
  // coordinator polls the revision; pollUntil holds polling after the stream
  // failed.
  let stream: EventSource | null = null
  let streamErrors = 0
  let pollUntil = 0
  let pushTimer: number | undefined
  // A change pushed during a snapshot read is checked when the read ends.
  let pushedRevision: number | null = null
  let pushPending = false

  const clearTimer = () => {
    if (timer !== undefined) window.clearTimeout(timer)
    timer = undefined
  }
  const retryDelay = () =>
    failures
      ? Math.min(MAX_RETRY_MS, RETRY_MS * 2 ** (failures - 1))
      : stream
        ? SNAPSHOT_MS
        : REVISION_POLL_MS
  // The loaded snapshot is due again after a minute, or just after its next
  // transition when that comes first. A transition this browser's clock had
  // already passed when the read started is left to the minute, so a clock
  // ahead of the server's cannot repeat the read; one passed while the
  // snapshot was in transit is read again at once.
  const snapshotDueAt = () => {
    const due = loadedAt + SNAPSHOT_MS
    if (transitionAt <= snapshotStartedAt) return due
    return Math.min(
      due,
      Math.max(transitionAt + TRANSITION_MARGIN_MS, lastStartedAt + MIN_GAP_MS),
    )
  }
  const schedule = (delay = retryDelay()) => {
    clearTimer()
    if (hasLoaded && !needsSnapshot && failures === 0)
      delay = Math.min(delay, Math.max(0, snapshotDueAt() - Date.now()))
    nextCheckAt = Date.now() + delay
    if (active && document.visibilityState !== "hidden")
      timer = window.setTimeout(check, delay)
  }

  const clearPush = () => {
    if (pushTimer !== undefined) window.clearTimeout(pushTimer)
    pushTimer = undefined
  }
  // Every open board hears a change at once; a random delay spreads their
  // snapshot reads.
  const pushSnapshot = () => {
    if (pushTimer !== undefined) return
    pushTimer = window.setTimeout(() => {
      pushTimer = undefined
      if (!active || document.visibilityState === "hidden") return
      if (flight?.kind === "snapshot") pushPending = true
      else void start("snapshot")
    }, Math.random() * PUSH_JITTER_MS)
  }
  const onPushedRevision = (value: number) => {
    pushedRevision = value
    if (flight?.kind === "snapshot") pushPending = true
    else if (revision == null || value !== revision) pushSnapshot()
    else calendar.followRevision(value)
  }
  const closeStream = () => {
    stream?.close()
    stream = null
    clearPush()
  }
  const openStream = () => {
    if (
      stream ||
      typeof EventSource !== "function" ||
      !active ||
      document.visibilityState === "hidden" ||
      Date.now() < pollUntil
    )
      return
    const source = new EventSource(REVISION_STREAM_URL)
    stream = source
    source.onmessage = (event) => {
      if (stream !== source) return
      streamErrors = 0
      const value = Number(event.data)
      if (Number.isSafeInteger(value)) onPushedRevision(value)
    }
    source.onerror = () => {
      if (stream !== source) return
      streamErrors++
      // EventSource reconnects by itself after a dropped connection, and
      // the reconnected stream resends the current revision. A refused
      // stream stays closed; that or repeated drops hand over to polling.
      if (
        source.readyState === EventSource.CLOSED ||
        streamErrors >= STREAM_FAILURES
      ) {
        closeStream()
        streamErrors = 0
        pollUntil = Date.now() + STREAM_RETRY_MS
        if (!flight) schedule()
      }
    }
  }

  const start = (kind: "snapshot" | "revision"): Promise<LoadResult> => {
    if (flight?.kind === "snapshot" || flight?.kind === kind)
      return flight.promise
    // Manual refresh supersedes a revision read. Its eventual completion is
    // guarded by generation, even if its body was already being decoded.
    flight?.controller.abort(new Error("Request superseded"))
    clearTimer()
    const currentGeneration = ++generation
    const controller = new AbortController()
    lastStartedAt = Date.now()
    const startedAt = lastStartedAt
    if (kind === "snapshot") handlers.onReadStart(hasLoaded)
    const isCurrent = () => active && generation === currentGeneration
    const promise = (async (): Promise<LoadResult> => {
      let changed = false
      try {
        const payload = await readJsonWithin(
          kind === "snapshot"
            ? `/api/dashboard?days=${POST_WINDOW_DAYS}`
            : `/api/dashboard/revision?since=${encodeURIComponent(String(revision ?? ""))}`,
          controller,
          REQUEST_TIMEOUT_MS,
        )
        if (!isCurrent())
          return { ok: false, error: new Error("Request superseded") }
        if (kind === "snapshot") {
          // An unsupported projection fails like any read: the known
          // snapshot stays and a fresh one is read after the backoff.
          if (!hasSupportedProjection(payload))
            throw new Error("Unsupported snapshot projection")
          handlers.onSnapshot(payload)
          revision = payload.meta?.revision ?? null
          const later = hasLoaded
          hasLoaded = true
          loadedAt = Date.now()
          snapshotStartedAt = startedAt
          const transition = Date.parse(
            payload.projection?.nextTransitionAt ?? "",
          )
          transitionAt = Number.isFinite(transition) ? transition : 0
          needsSnapshot = false
          // A later full snapshot also refreshes time-based calendar fields.
          // If a page is in flight, the next revision check consumes this.
          calendar.afterSnapshot(revision, later)
        } else {
          changed = revision == null || Number(payload.revision) !== revision
          if (!changed) calendar.followRevision(Number(payload.revision))
        }
        failures = 0
        return { ok: true }
      } catch (caught) {
        const error =
          caught instanceof Error ? caught : new Error(String(caught))
        if (isCurrent()) {
          failures = Math.min(5, failures + 1)
          if (kind === "snapshot") {
            needsSnapshot = true
            handlers.onReadFailed(error)
          }
        }
        return { ok: false, error }
      } finally {
        if (isCurrent()) {
          flight = null
          if (changed) void start("snapshot")
          else schedule()
          if (kind === "snapshot" && pushPending) {
            pushPending = false
            if (pushedRevision != null && pushedRevision !== revision)
              pushSnapshot()
            else if (pushedRevision != null)
              calendar.followRevision(pushedRevision)
          }
        }
      }
    })()
    flight = { kind, controller, promise }
    return promise
  }

  function check() {
    if (!active || document.visibilityState === "hidden" || flight) return
    openStream()
    if (needsSnapshot || revision == null || Date.now() >= snapshotDueAt())
      void start("snapshot")
    // The stream reports changes; between them only the snapshot's
    // revalidation is due.
    else if (stream) schedule()
    else void start("revision")
  }
  const wake = () => {
    clearTimer()
    if (!active) return
    // A hidden page holds no stream; a visible one reconnects, and the
    // stream's first message is the current revision.
    if (document.visibilityState === "hidden") {
      closeStream()
      return
    }
    openStream()
    if (flight) return
    // Visibility + focus often arrive together. Coalesce them, and preserve
    // failure backoff so repeated focus cannot turn an outage into a storm.
    const due = failures ? nextCheckAt : lastStartedAt + MIN_GAP_MS
    timer = window.setTimeout(check, Math.max(0, due - Date.now()))
  }

  void start("snapshot")
  openStream()
  document.addEventListener("visibilitychange", wake)
  window.addEventListener("focus", wake)
  return {
    /** Reads the snapshot now, sharing a snapshot read already in flight. */
    reload: () => start("snapshot"),
    stop() {
      active = false
      generation++
      clearTimer()
      closeStream()
      flight?.controller.abort(new Error("Page unmounted"))
      document.removeEventListener("visibilitychange", wake)
      window.removeEventListener("focus", wake)
    },
  }
}
