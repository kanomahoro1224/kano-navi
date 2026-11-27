import { useCallback, useEffect, useMemo, useState } from "react"

import type { ArchiveMonth, ArchiveSnapshot, Post } from "@/types"
import { createRequestScope } from "../lib/polling"
import { readScopedJson } from "./api"
import {
  type ArchiveRequest,
  monthIndexes,
  readArchiveMonth,
  readArchivePage,
} from "./archive-query"
import { localDateKey } from "./format"
import { STORED_READ_MAX_AGE_MS } from "./snapshot-cache"

/** The post archive page's index: its months and the accounts it filters. */
export interface PostArchiveIndex {
  months: ArchiveMonth[]
  /** X handles: the board's accounts, then the archive-only ones. */
  accounts: string[]
  bilibiliAccounts: string[]
  accountAvatars: Record<string, string | null>
  revision: number
}

function stringList(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : []
}

/** A post month index answer as the page uses it, or an error. */
function readPostArchiveIndex(result: any): PostArchiveIndex {
  if (!Array.isArray(result?.months) || !Number.isInteger(result.revision))
    throw new Error("Invalid month index")
  const accounts = result.accounts ?? {}
  return {
    months: (result.months as unknown[])
      .map((month) => readArchiveMonth(month, monthIndexes.posts.counts))
      .filter((month): month is ArchiveMonth => month !== null),
    accounts: stringList(accounts.x),
    bilibiliAccounts: stringList(accounts.bilibili),
    accountAvatars:
      result.accountAvatars && typeof result.accountAvatars === "object"
        ? result.accountAvatars
        : {},
    revision: result.revision,
  }
}

/**
 * The post archive page's last reads, so a return to the page shows them at
 * once and reads them again only when the month index names another
 * revision. This document keeps the index and the last timeline as loaded;
 * localStorage keeps the index and the newest page of the last few accounts'
 * timelines from their newest post, for a week. A change to the shape of
 * either answer takes a new key.
 */
const POST_ARCHIVE_CACHE_KEY = "kano-post-archive-v1"
const POST_ARCHIVE_STORED_TIMELINES = 3

/** Where a post timeline starts, as the archive page asked for it. */
export interface PostTimelineStart {
  account: string | null
  postKind?: string | null
  /** A `YYYY-MM` month, or null for the newest post. */
  anchor: string | null
  startPage?: number
}

const postArchiveMemory: {
  index: PostArchiveIndex | null
  indices: Record<string, PostArchiveIndex>
  timeline: {
    key: string
    start: PostTimelineStart
    snapshot: ArchiveSnapshot<unknown>
  } | null
} = { index: null, indices: {}, timeline: null }

type StoredEntry = { savedAt: number; value: unknown }
type StoredPostArchive = {
  index?: StoredEntry
  indices?: Record<string, StoredEntry>
  timelines: (StoredEntry & { key: string })[]
}

function readStoredPostArchive(): StoredPostArchive {
  try {
    const stored = JSON.parse(
      window.localStorage.getItem(POST_ARCHIVE_CACHE_KEY) || "null",
    )
    return {
      index: stored?.index,
      indices:
        stored?.indices && typeof stored.indices === "object"
          ? stored.indices
          : undefined,
      timelines: Array.isArray(stored?.timelines) ? stored.timelines : [],
    }
  } catch {
    return { timelines: [] }
  }
}

function writeStoredPostArchive(
  change: (stored: StoredPostArchive) => StoredPostArchive,
) {
  try {
    window.localStorage.setItem(
      POST_ARCHIVE_CACHE_KEY,
      JSON.stringify(change(readStoredPostArchive())),
    )
  } catch {
    // Without storage the next visit reads the page as it opens.
  }
}

/** A stored value under a week old that `read` accepts, or null. */
function storedValue<T>(
  entry: StoredEntry | undefined,
  read: (value: unknown) => T,
) {
  const age = Date.now() - (entry?.savedAt ?? Number.NaN)
  if (!(age >= 0 && age <= STORED_READ_MAX_AGE_MS)) return null
  try {
    return read(entry!.value)
  } catch {
    return null
  }
}

function restoredPostIndex(postKind?: string | null) {
  if (!postKind) {
    return (
      postArchiveMemory.index ??
      storedValue(readStoredPostArchive().index, readPostArchiveIndex)
    )
  }
  return (
    postArchiveMemory.indices[postKind] ??
    storedValue(
      readStoredPostArchive().indices?.[postKind],
      readPostArchiveIndex,
    )
  )
}

/** The post timeline this document or browser last loaded under `key`. */
export function restoredPostTimeline(
  key: string,
): ArchiveSnapshot<unknown> | null {
  if (postArchiveMemory.timeline?.key === key)
    return postArchiveMemory.timeline.snapshot
  const stored = readStoredPostArchive().timelines.find(
    (entry) => entry.key === key,
  )
  const page = storedValue(stored, readArchivePage)
  return page && { ...page, firstPage: 1 }
}

/**
 * Keeps a post timeline's run, and in localStorage its first page as read
 * when the timeline starts from the newest post.
 */
export function rememberPostTimeline(
  key: string,
  request: ArchiveRequest,
  snapshot: ArchiveSnapshot<unknown>,
  firstPage: ArchiveSnapshot<unknown> | null,
) {
  const start = {
    account: request.account ?? null,
    postKind: request.postKind ?? null,
    anchor: request.anchor ? localDateKey(request.anchor).slice(0, 7) : null,
    startPage: request.startPage,
  }
  postArchiveMemory.timeline = { key, start, snapshot }
  if (!firstPage || start.anchor) return
  writeStoredPostArchive((stored) => ({
    ...stored,
    timelines: [
      { key, savedAt: Date.now(), value: firstPage },
      ...stored.timelines.filter((entry) => entry.key !== key),
    ].slice(0, POST_ARCHIVE_STORED_TIMELINES),
  }))
}

/**
 * The post timeline this document read last, where it started and what it
 * loaded, so a return to the archive page can resume it.
 */
export function rememberedPostTimeline() {
  const timeline = postArchiveMemory.timeline
  return (
    timeline && {
      start: timeline.start,
      snapshot: timeline.snapshot as ArchiveSnapshot<Post>,
    }
  )
}

/**
 * Reads the post month index once for the archive page, with the accounts
 * the page filters by, showing the last known index until it arrives.
 * `revision` is the read's revision once it arrives, null after a failed
 * read and undefined before. `retry` reads it again after a failure.
 */
export function usePostArchiveIndex(postKind?: string | null) {
  const scope = useMemo(() => createRequestScope(), [])
  const [attempt, setAttempt] = useState(0)
  const [state, setState] = useState<{
    kind: string | null
    index: PostArchiveIndex | null
    error: Error | null
    read: boolean
  }>(() => ({
    kind: postKind ?? null,
    index: restoredPostIndex(postKind),
    error: null,
    read: false,
  }))

  useEffect(() => {
    scope.activate()
    return () => scope.dispose()
  }, [scope])

  useEffect(() => {
    const kindKey = postKind ?? null
    setState({
      kind: kindKey,
      index: restoredPostIndex(kindKey),
      error: null,
      read: false,
    })
    const url = kindKey
      ? `${monthIndexes.posts.url}?kind=${encodeURIComponent(kindKey)}`
      : monthIndexes.posts.url
    scope
      .run("index", async (signal) => {
        const result = await readScopedJson(url, signal)
        return { result, index: readPostArchiveIndex(result) }
      })
      .then(
        ({ result, index }) => {
          if (!kindKey) {
            postArchiveMemory.index = index
            writeStoredPostArchive((stored) => ({
              ...stored,
              index: { savedAt: Date.now(), value: result },
            }))
          } else {
            postArchiveMemory.indices[kindKey] = index
            writeStoredPostArchive((stored) => ({
              ...stored,
              indices: {
                ...stored.indices,
                [kindKey]: { savedAt: Date.now(), value: result },
              },
            }))
          }
          setState({ kind: kindKey, index, error: null, read: true })
        },
        (caught: unknown) => {
          if (caught instanceof Error && caught.name === "AbortError") return
          setState((value) => ({
            ...value,
            error: caught instanceof Error ? caught : new Error(String(caught)),
          }))
        },
      )
    return () => scope.cancelKey("index")
  }, [attempt, postKind, scope])

  const retry = useCallback(() => setAttempt((value) => value + 1), [])
  const revision =
    state.read && state.index
      ? state.index.revision
      : state.error
        ? null
        : undefined
  return { index: state.index, error: state.error, revision, retry }
}
