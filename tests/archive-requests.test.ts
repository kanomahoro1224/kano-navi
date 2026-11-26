import assert from "node:assert/strict"
import path from "node:path"
import test, { type TestContext } from "node:test"
import { act, createElement } from "react"
import { render, type Rendered } from "./dom.ts"
import { createServer } from "vite"

type ProbeProps = {
  request: unknown
  revision?: number | null
  snapshotVersion?: number
}

test("month archives page by revision, follow the board and cancel on close", async (t) => {
  // Load the actual hook with its normal frontend imports, without the runtime
  // Vite plugin, .env loading, a listener or the operator database.
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  try {
    const { useArchive, usePostArchiveIndex, timelineStartPage } =
      await vite.ssrLoadModule("/src/dashboard/use-dashboard.ts")
    await vite.close()
    const setup = async (context: TestContext, initial: ProbeProps) => {
      const previous = (globalThis as any).IS_REACT_ACT_ENVIRONMENT
      ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
      const requests: {
        url: string
        signal: AbortSignal
        resolve: (response: Response) => void
        reject: (error: Error) => void
      }[] = []
      context.mock.method(
        globalThis,
        "fetch",
        (url: RequestInfo | URL, options: { signal: AbortSignal }) => {
          return new Promise<Response>((resolve, reject) =>
            requests.push({
              url: String(url),
              signal: options.signal,
              resolve,
              reject,
            }),
          )
        },
      )
      let value: ReturnType<typeof useArchive>
      let root: Rendered
      function Probe({ request, revision, snapshotVersion }: ProbeProps) {
        value = useArchive(request, revision, snapshotVersion)
        return null
      }
      await act(async () => {
        root = render(createElement(Probe, initial))
      })
      const page = (
        page: number,
        revision: number,
        items: string[],
        hasNext = false,
      ) => ({
        items: items.map((id) => ({ id })),
        adjacent: { previous: null, next: null },
        total: 9,
        page,
        hasNext,
        revision,
      })
      const respond = async (index: number, payload: any) =>
        act(async () =>
          requests[index].resolve({
            ok: true,
            status: 200,
            json: async () => payload,
          } as Response),
        )
      const rerender = async (props: ProbeProps) =>
        act(async () => root.update(createElement(Probe, props)))
      // Leaves the page and opens it again, as a return to it does.
      const remount = async (props: ProbeProps) =>
        act(async () => {
          root.unmount()
          root = render(createElement(Probe, props))
        })
      const reject = async (index: number) =>
        act(async () => requests[index].reject(new Error("Synthetic outage")))
      const unmount = async () => {
        await act(async () => root.unmount())
        ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previous
      }
      const ids = () =>
        value.archive?.items.map((item: { id: string }) => item.id).join(",")
      return {
        requests,
        page,
        respond,
        rerender,
        remount,
        reject,
        unmount,
        ids,
        current: () => value,
      }
    }
    const october = new Date(2026, 9, 1)

    for (const kind of ["schedule", "videos"]) {
      await t.test(
        `${kind} reload replaces expired live states across every loaded page at the same revision`,
        async (context) => {
          const request = { kind, monthStart: october }
          const fixture = await setup(context, {
            request,
            revision: 4,
            snapshotVersion: 1,
          })
          const page = (number: number, liveStatus: string | null) => ({
            ...fixture.page(number, 4, [], number === 1),
            items: [{ id: `event-${number}`, liveStatus }],
          })
          const states = () =>
            fixture.current().archive.items.map((item: any) => item.liveStatus)
          try {
            await fixture.respond(0, page(1, "live"))
            assert.equal(fixture.requests.length, 1)
            await act(async () => void fixture.current().loadMore())
            await fixture.respond(1, page(2, "upcoming"))
            await act(async () => void fixture.current().reload())
            await fixture.respond(2, page(1, null))
            assert.deepEqual(states(), ["live", "upcoming"])
            assert.match(fixture.requests[3].url, /[?&]page=2(&|$)/)
            await fixture.respond(3, page(2, null))
            assert.deepEqual(states(), [null, null])
            assert.equal(fixture.current().archive.page, 2)
            assert.equal(fixture.current().archive.revision, 4)
          } finally {
            await fixture.unmount()
          }
        },
      )
    }

    await t.test(
      "a board snapshot during archive pagination waits, then refreshes the loaded prefix",
      async (context) => {
        const request = { kind: "schedule", monthStart: october }
        const fixture = await setup(context, {
          request,
          revision: 4,
          snapshotVersion: 1,
        })
        try {
          await fixture.respond(0, fixture.page(1, 4, ["old-a"], true))
          await act(async () => void fixture.current().loadMore())
          await fixture.rerender({ request, revision: 4, snapshotVersion: 2 })
          assert.equal(fixture.requests.length, 2)
          assert.equal(fixture.requests[1].signal.aborted, false)
          await fixture.respond(1, fixture.page(2, 4, ["old-b"]))
          assert.equal(fixture.requests.length, 3)
          assert.match(fixture.requests[2].url, /&page=1$/)
          await fixture.respond(2, fixture.page(1, 4, ["new-a"], true))
          assert.equal(fixture.ids(), "old-a,old-b")
          await fixture.respond(3, fixture.page(2, 4, ["new-b"]))
          assert.equal(fixture.ids(), "new-a,new-b")
          assert.equal(fixture.requests.length, 4)
        } finally {
          await fixture.unmount()
        }
      },
    )

    await t.test(
      "failed time refreshes retain the known prefix and retry on the next board snapshot",
      async (context) => {
        const request = { kind: "videos", monthStart: october }
        const fixture = await setup(context, {
          request,
          revision: 4,
          snapshotVersion: 1,
        })
        try {
          await fixture.respond(0, fixture.page(1, 4, ["known"], true))
          await act(async () => void fixture.current().loadMore())
          await fixture.respond(1, fixture.page(2, 4, ["also-known"]))
          await fixture.rerender({ request, revision: 4, snapshotVersion: 2 })
          await fixture.respond(2, fixture.page(1, 4, ["partial"], true))
          await fixture.reject(3)
          assert.equal(fixture.ids(), "known,also-known")
          assert.match(fixture.current().error.message, /Synthetic outage/)
          assert.equal(fixture.requests.length, 4)
          await fixture.rerender({ request, revision: 4, snapshotVersion: 3 })
          assert.equal(fixture.requests.length, 5)
          await fixture.respond(4, fixture.page(1, 4, ["recovered"], true))
          await fixture.respond(5, fixture.page(2, 4, ["recovered-too"]))
          assert.equal(fixture.ids(), "recovered,recovered-too")
          assert.equal(fixture.current().error, null)
          await fixture.rerender({ request, revision: 4, snapshotVersion: 4 })
          await fixture.rerender({
            request: null,
            revision: 4,
            snapshotVersion: 4,
          })
          assert.equal(fixture.requests[6].signal.aborted, true)
          await fixture.respond(6, fixture.page(1, 4, ["late"]))
          assert.equal(fixture.current().archive, null)
          assert.equal(fixture.requests.length, 7)
        } finally {
          await fixture.unmount()
        }
      },
    )

    await t.test(
      "post archives still follow stored revisions rather than time refreshes",
      async (context) => {
        const request = { kind: "posts", monthStart: october }
        const fixture = await setup(context, {
          request,
          revision: 4,
          snapshotVersion: 1,
        })
        try {
          await fixture.respond(0, fixture.page(1, 4, ["post"]))
          await fixture.rerender({ request, revision: 4, snapshotVersion: 2 })
          assert.equal(fixture.requests.length, 1)
          await fixture.rerender({ request, revision: 5, snapshotVersion: 2 })
          assert.equal(fixture.requests.length, 2)
          await fixture.respond(1, fixture.page(1, 5, ["updated-post"]))
          assert.equal(fixture.ids(), "updated-post")
        } finally {
          await fixture.unmount()
        }
      },
    )

    await t.test(
      "a post timeline reopens on the run it loaded and follows the index revision",
      async (context) => {
        const request = {
          kind: "posts",
          monthStart: october,
          account: "memory_a",
        }
        const fixture = await setup(context, { request, revision: 1 })
        try {
          await fixture.respond(0, fixture.page(1, 1, ["a"], true))
          await act(async () => void fixture.current().loadMore())
          await fixture.respond(1, fixture.page(2, 1, ["b"], true))
          // Back on the page before its index arrives: the loaded run, no read.
          await fixture.remount({ request })
          assert.equal(fixture.ids(), "a,b")
          assert.equal(fixture.current().isLoading, false)
          assert.equal(fixture.requests.length, 2)
          // The index names the run's revision: it is current.
          await fixture.rerender({ request, revision: 1 })
          assert.equal(fixture.requests.length, 2)
          // Another revision rereads the whole run.
          await fixture.remount({ request })
          await fixture.rerender({ request, revision: 2 })
          assert.match(fixture.requests[2].url, /[?&]page=1&/)
          assert.equal(fixture.ids(), "a,b")
          await fixture.respond(2, fixture.page(1, 2, ["a2"], true))
          await fixture.respond(3, fixture.page(2, 2, ["b2"], true))
          assert.equal(fixture.ids(), "a2,b2")
          // A failed index read rereads a remembered run once.
          await fixture.remount({ request })
          await fixture.rerender({ request, revision: null })
          assert.equal(fixture.requests.length, 5)
          await fixture.respond(4, fixture.page(1, 2, ["a3"], true))
          await fixture.respond(5, fixture.page(2, 2, ["b3"], true))
          assert.equal(fixture.ids(), "a3,b3")
          assert.equal(fixture.requests.length, 6)
        } finally {
          await fixture.unmount()
        }
      },
    )

    await t.test(
      "the newest post page waits in storage for the next visit",
      async (context) => {
        const stored = {
          kind: "posts",
          monthStart: october,
          account: "stored_a",
        }
        const other = { ...stored, account: "stored_b" }
        window.localStorage.removeItem("kano-post-archive-v1")
        const fixture = await setup(context, { request: stored })
        try {
          await fixture.respond(0, fixture.page(1, 3, ["new"], true))
          await act(async () => void fixture.current().loadMore())
          await fixture.respond(1, fixture.page(2, 3, ["older"]))
          // Another timeline replaces the one this document remembers.
          await fixture.rerender({ request: other })
          await fixture.respond(2, fixture.page(1, 3, ["b"]))
          // The browser kept the first page alone.
          await fixture.remount({ request: stored })
          assert.equal(fixture.ids(), "new")
          assert.equal(fixture.current().archive.page, 1)
          assert.equal(fixture.current().archive.hasNext, true)
          assert.equal(fixture.requests.length, 3)
          await fixture.rerender({ request: stored, revision: 3 })
          assert.equal(fixture.requests.length, 3)
          await act(async () => void fixture.current().loadMore())
          assert.match(fixture.requests[3].url, /[?&]page=2&/)
          // A timeline from a chosen month is not stored.
          const chosen = { ...stored, anchor: new Date(2026, 8, 1) }
          await fixture.rerender({ request: chosen, revision: 3 })
          await fixture.respond(4, fixture.page(1, 3, ["sept"]))
          const kept = JSON.parse(
            window.localStorage.getItem("kano-post-archive-v1") || "{}",
          )
          assert.deepEqual(
            kept.timelines.map((entry: { key: string }) => entry.key),
            ["posts:until::stored_b:", "posts:until::stored_a:"],
          )
        } finally {
          await fixture.unmount()
        }
      },
    )

    await t.test(
      "a post timeline read before its index arrives is not read again for it",
      async (context) => {
        const request = { kind: "posts", monthStart: october, account: "early" }
        const fixture = await setup(context, { request })
        try {
          await fixture.respond(0, fixture.page(1, 7, ["a"]))
          await fixture.rerender({ request, revision: 7 })
          assert.equal(fixture.requests.length, 1)
          // One written in between is.
          await fixture.rerender({ request, revision: 8 })
          assert.equal(fixture.requests.length, 2)
        } finally {
          await fixture.unmount()
        }
      },
    )

    await t.test(
      "a later page from another revision rereads the loaded pages instead of appending",
      async (context) => {
        const request = {
          kind: "posts",
          monthStart: october,
          account: "kano_2525",
        }
        const fixture = await setup(context, { request, revision: 1 })
        try {
          assert.equal(
            fixture.requests[0].url,
            "/api/posts?page=1&pageSize=30&account=kano_2525",
          )
          assert.equal(fixture.current().isLoading, true)
          await fixture.respond(0, fixture.page(1, 1, ["a"], true))
          assert.equal(fixture.ids(), "a")
          await act(async () => void fixture.current().loadMore())
          assert.match(fixture.requests[1].url, /[?&]page=2&/)
          await fixture.respond(1, fixture.page(2, 2, ["b"]))
          // Page two moved under a write: reread page one of revision 2.
          assert.match(fixture.requests[2].url, /[?&]page=1&/)
          await fixture.respond(2, fixture.page(1, 2, ["x"], true))
          assert.equal(fixture.ids(), "x")
          await act(async () => void fixture.current().loadMore())
          await fixture.respond(3, fixture.page(2, 2, ["y"]))
          assert.equal(fixture.ids(), "x,y")
          assert.equal(fixture.current().archive.hasNext, false)
          assert.equal(fixture.current().isLoading, false)
        } finally {
          await fixture.unmount()
        }
      },
    )

    await t.test(
      "the schedule reads its Monday-first grid and follows the board revision",
      async (context) => {
        const request = { kind: "schedule", monthStart: october }
        const fixture = await setup(context, { request, revision: 4 })
        try {
          assert.equal(
            fixture.requests[0].url,
            "/api/calendar?from=2026-09-28&to=2026-11-01&page=1",
          )
          await fixture.respond(0, fixture.page(1, 4, ["a"], true))
          await act(async () => void fixture.current().loadMore())
          await fixture.respond(1, fixture.page(2, 4, ["b"]))
          assert.equal(fixture.ids(), "a,b")
          // An unchanged revision reads nothing; a new one rereads both pages.
          await fixture.rerender({ request, revision: 4 })
          assert.equal(fixture.requests.length, 2)
          await fixture.rerender({ request, revision: 5 })
          assert.match(fixture.requests[2].url, /&page=1$/)
          assert.equal(fixture.ids(), "a,b")
          await fixture.respond(2, fixture.page(1, 5, ["c"], true))
          assert.match(fixture.requests[3].url, /&page=2$/)
          await fixture.respond(3, fixture.page(2, 5, ["d"]))
          assert.equal(fixture.ids(), "c,d")
          assert.equal(fixture.current().archive.revision, 5)
        } finally {
          await fixture.unmount()
        }
      },
    )

    await t.test(
      "changing the video timeline's start or closing cancels the read and ignores late pages",
      async (context) => {
        const latest = { kind: "videos", monthStart: october, anchor: null }
        const fixture = await setup(context, { request: latest, revision: 1 })
        try {
          // The timeline reads every month from the newest video.
          assert.equal(
            fixture.requests[0].url,
            "/api/videos?page=1&pageSize=30",
          )
          // Its month is only what scrolled into view: no new read.
          await fixture.rerender({
            request: { ...latest, monthStart: new Date(2026, 8, 1) },
            revision: 1,
          })
          assert.equal(fixture.requests.length, 1)
          // A start month reads it and everything older, through its last day.
          const september = {
            ...latest,
            anchor: new Date(2026, 8, 1),
            category: "game",
          }
          await fixture.rerender({ request: september, revision: 1 })
          assert.equal(fixture.requests[0].signal.aborted, true)
          assert.equal(
            fixture.requests[1].url,
            "/api/videos?page=1&pageSize=30&until=2026-09-30&category=game",
          )
          await fixture.respond(0, fixture.page(1, 1, ["late"]))
          assert.equal(fixture.current().archive, null)
          await fixture.respond(1, fixture.page(1, 1, ["sept"]))
          assert.equal(fixture.ids(), "sept")
          await fixture.rerender({ request: null, revision: 2 })
          assert.equal(fixture.current().archive, null)
          assert.equal(fixture.current().isLoading, false)
          assert.equal(fixture.requests.length, 2)
        } finally {
          await fixture.unmount()
        }
      },
    )

    await t.test(
      "a timeline from a chosen month starts at its page and loads newer pages above",
      async (context) => {
        const request = {
          kind: "videos",
          monthStart: october,
          anchor: new Date(2026, 8, 1),
          startPage: 3,
        }
        const fixture = await setup(context, { request, revision: 1 })
        const first = () => fixture.current().archive.firstPage
        try {
          // The page holding the month, without an end date.
          assert.equal(
            fixture.requests[0].url,
            "/api/videos?page=3&pageSize=30",
          )
          // It reads the page after it too, so the month can reach the top.
          await fixture.respond(0, fixture.page(3, 1, ["c"], true))
          assert.match(fixture.requests[1].url, /[?&]page=4&/)
          await fixture.respond(1, fixture.page(4, 1, ["d"]))
          assert.equal(fixture.ids(), "c,d")
          assert.equal(first(), 3)
          await act(async () => void fixture.current().loadNewer())
          assert.match(fixture.requests[2].url, /[?&]page=2&/)
          await fixture.respond(2, fixture.page(2, 1, ["b"], true))
          assert.equal(fixture.ids(), "b,c,d")
          assert.equal(first(), 2)
          assert.equal(fixture.current().archive.page, 4)
          // A new revision rereads the whole loaded run, from its first page.
          await fixture.rerender({ request, revision: 2 })
          assert.match(fixture.requests[3].url, /[?&]page=2&/)
          await fixture.respond(3, fixture.page(2, 2, ["b2"], true))
          await fixture.respond(4, fixture.page(3, 2, ["c2"], true))
          await fixture.respond(5, fixture.page(4, 2, ["d2"]))
          assert.equal(fixture.ids(), "b2,c2,d2")
          assert.equal(first(), 2)
          // A newer page from another revision rereads the whole run from
          // that newer page instead of joining, so it still arrives.
          await act(async () => void fixture.current().loadNewer())
          await fixture.respond(6, fixture.page(1, 3, ["a3"], true))
          assert.match(fixture.requests[7].url, /[?&]page=1&/)
          await fixture.respond(7, fixture.page(1, 3, ["a3"], true))
          await fixture.respond(8, fixture.page(2, 3, ["b3"], true))
          await fixture.respond(9, fixture.page(3, 3, ["c3"], true))
          await fixture.respond(10, fixture.page(4, 3, ["d3"]))
          assert.equal(fixture.ids(), "a3,b3,c3,d3")
          assert.equal(first(), 1)
          assert.equal(fixture.current().archive.page, 4)
          // Nothing is newer than page one.
          await act(async () => void fixture.current().loadNewer())
          assert.equal(fixture.requests.length, 11)
        } finally {
          await fixture.unmount()
        }
      },
    )

    await t.test(
      "the post month index shows the last one known until this visit's read",
      async (context) => {
        const previous = (globalThis as any).IS_REACT_ACT_ENVIRONMENT
        ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
        const answers: ((response: Response) => void)[] = []
        context.mock.method(
          globalThis,
          "fetch",
          () => new Promise<Response>((resolve) => answers.push(resolve)),
        )
        const index = (revision: number) => ({
          months: [{ month: "2026-10", total: 2, accountCounts: { a: 2 } }],
          revision,
          accounts: { x: ["a"], bilibili: [] },
          accountAvatars: {},
        })
        const answer = async (at: number, payload: unknown, ok = true) =>
          act(async () =>
            answers[at]({
              ok,
              status: ok ? 200 : 503,
              json: async () => payload,
            } as Response),
          )
        let value: any
        function Probe() {
          value = usePostArchiveIndex()
          return null
        }
        let root!: Rendered
        try {
          await act(async () => {
            root = render(createElement(Probe))
          })
          assert.equal(value.revision, undefined)
          await answer(0, index(5))
          assert.equal(value.revision, 5)
          assert.deepEqual(value.index.accounts, ["a"])
          // Back on the page: the known index at once, its revision unread.
          await act(async () => {
            root.unmount()
            root = render(createElement(Probe))
          })
          assert.equal(value.index.revision, 5)
          assert.equal(value.revision, undefined)
          // A failed read keeps the known index and names no revision.
          await answer(1, null, false)
          assert.equal(value.index.revision, 5)
          assert.equal(value.revision, null)
          assert.match(value.error.message, /API 503/)
          await act(async () => value.retry())
          await answer(2, index(6))
          assert.equal(value.revision, 6)
          assert.equal(value.error, null)
        } finally {
          await act(async () => root?.unmount())
          ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previous
        }
      },
    )

    await t.test(
      "a month's timeline page counts every video of the later months",
      () => {
        const months = [
          { month: "2026-09", total: 10, counts: { game: 4 } },
          { month: "2026-10", total: 40, counts: { game: 30, talk: 10 } },
          { month: "2026-11", total: 5, counts: { game: 2, song: 3 } },
        ]
        assert.equal(timelineStartPage(months, "2026-11", null), 1)
        assert.equal(timelineStartPage(months, "2026-10", null), 1)
        // 45 newer videos: the month starts on the second page of thirty.
        assert.equal(timelineStartPage(months, "2026-09", null), 2)
        assert.equal(timelineStartPage(months, "2026-09", "game"), 2)
        assert.equal(timelineStartPage(months, "2026-09", "talk"), 1)
        // A month without videos starts where the next older one would.
        assert.equal(timelineStartPage(months, "2026-08", null), 2)
      },
    )
  } finally {
    await vite.close()
  }
})
