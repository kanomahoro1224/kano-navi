import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import { dashboardMessages } from "../src/dashboard/dashboard-i18n.ts"
import { browser, click, render, type Rendered } from "./dom.ts"

const translation = (text: string) => ({ text, origin: "machine" as const })

/** Loads modules through Vite, so the `@/` imports of the views resolve. */
async function withVite(run: (load: (id: string) => Promise<any>) => unknown) {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  try {
    await run((id) => vite.ssrLoadModule(id))
  } finally {
    await vite.close()
  }
}

test("Japanese readers see a Japanese translation only of wording not in Japanese", async () => {
  await withVite(async (load) => {
    const { pickTranslation } = await load(
      "/src/components/translated-text.tsx",
    )
    const translations = {
      en: translation("Synthetic English"),
      ja: translation("合成の日本語"),
      "zh-CN": translation("合成中文"),
    }
    assert.equal(
      pickTranslation(translations, "ja", "zh")?.text,
      "合成の日本語",
    )
    assert.equal(
      pickTranslation(translations, "ja", null)?.text,
      "合成の日本語",
    )
    assert.equal(pickTranslation(translations, "ja", "ja"), null)
    assert.equal(pickTranslation(translations, "ja", "ja-JP"), null)
    assert.equal(pickTranslation({ en: translations.en }, "ja", "zh"), null)
    assert.equal(pickTranslation({ ja: translation("") }, "ja", "zh"), null)
    // English and Chinese readers keep their own locale's translation.
    assert.equal(
      pickTranslation(translations, "en", "ja")?.text,
      "Synthetic English",
    )
    assert.equal(pickTranslation(translations, "zh-CN", "ja")?.text, "合成中文")
  })
})

test("the translation switch marks where a translation came from", async () => {
  await withVite(async (load) => {
    const { translationMarkKind } = await load(
      "/src/components/translated-text.tsx",
    )
    const mark = (origin: string, platform?: string) =>
      translationMarkKind({ text: "Synthetic", origin, platform })
    assert.equal(mark("machine"), "ai")
    assert.equal(mark("manual"), "icon")
    assert.equal(mark("platform", "youtube"), "youtube")
    assert.equal(mark("platform", "bilibili"), "bilibili")
    // A platform without a mark shows the translate icon alone.
    assert.equal(mark("platform", "example"), "icon")
    assert.equal(mark("platform", "constructor"), "icon")
  })
})

test("the post wall's title names no single platform", () => {
  assert.equal(dashboardMessages.en["feed.title"], "Recent Posts")
  for (const locale of ["en", "ja", "zh-CN"] as const)
    assert.doesNotMatch(dashboardMessages[locale]["feed.title"], /\bX\b/u)
})

test("a Bilibili post names its source, shows its forward and no X-only parts", async () => {
  await withVite(async (load) => {
    const [
      { PostWall },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
    ] = await Promise.all([
      load("/src/dashboard/components/post-wall.tsx"),
      load("/src/app-settings.tsx"),
    ])
    const base = {
      publishedAt: "2026-10-08T12:00:00+09:00",
      mediaUrl: null,
      mediaStatus: "none",
      likes: 12,
      reposts: 3,
      replies: 4,
      translations: {},
      sourceLanguage: "ja",
    }
    const quote = {
      id: "900",
      authorHandle: "quoted_example",
      authorName: "Quoted Example",
      text: "Synthetic quoted wording",
      publishedAt: "2026-10-07T12:00:00+09:00",
      url: "https://x.example.invalid/quoted_example/status/900",
      unavailable: false,
      media: [],
    }
    // The dynamic a forward forwards: its author by mid and display name.
    const forwarded = {
      id: "bilibili-1000000000000000003",
      authorHandle: "42",
      authorName: "Synthetic Agency",
      text: "被转发的合成动态",
      publishedAt: "2026-10-07T12:00:00+09:00",
      url: "https://www.bilibili.example.invalid/opus/1000000000000000003",
      unavailable: false,
      media: [],
      translations: {
        en: { text: "Synthetic forwarded dynamic", origin: "machine" },
      },
      sourceLanguage: "zh",
    }
    const posts = [
      {
        ...base,
        id: "bilibili-1000000000000000001",
        source: "bilibili",
        accountHandle: "100000001",
        accountUrl: "https://space.bilibili.example.invalid/100000001",
        text: "合成动态",
        url: "https://www.bilibili.example.invalid/opus/1000000000000000001",
        media: [],
        quote: forwarded,
      },
      {
        ...base,
        id: "bilibili-1000000000000000002",
        source: "bilibili",
        accountHandle: "100000001",
        accountUrl: "https://space.bilibili.example.invalid/100000001",
        text: "",
        url: "https://www.bilibili.example.invalid/opus/1000000000000000002",
        media: [
          {
            id: "synthetic-image",
            url: "/media/x/synthetic.jpg",
            status: "ready",
            sourceUrl: null,
          },
        ],
        quote: null,
      },
      {
        // A forward of a deleted dynamic, with wording of its own.
        ...base,
        id: "bilibili-1000000000000000004",
        source: "bilibili",
        accountHandle: "100000001",
        accountUrl: "https://space.bilibili.example.invalid/100000001",
        text: "原动态没了",
        url: "https://www.bilibili.example.invalid/opus/1000000000000000004",
        media: [],
        quote: {
          id: "bilibili-1000000000000000005",
          authorHandle: null,
          authorName: null,
          text: null,
          publishedAt: null,
          url: "https://www.bilibili.example.invalid/opus/1000000000000000005",
          unavailable: true,
          media: [],
        },
      },
      {
        // A forward with no wording of its own.
        ...base,
        id: "bilibili-1000000000000000006",
        source: "bilibili",
        accountHandle: "100000001",
        accountUrl: "https://space.bilibili.example.invalid/100000001",
        text: "",
        url: "https://www.bilibili.example.invalid/opus/1000000000000000006",
        media: [],
        quote: forwarded,
      },
      {
        ...base,
        id: "800",
        source: "x",
        accountHandle: "synthetic_account",
        accountUrl: "https://x.example.invalid/synthetic_account",
        text: "Synthetic X post",
        url: "https://x.example.invalid/synthetic_account/status/800",
        media: [],
        quote,
      },
    ]
    function Harness() {
      const { locale, t } = useAppSettings()
      return createElement(PostWall, {
        posts,
        latestPost: posts[0],
        accounts: ["synthetic_account"],
        bilibiliAccounts: ["100000001"],
        avatarUrl: null,
        windowDays: 14,
        isLoading: false,
        locale,
        now: Date.parse("2026-10-08T21:00:00+09:00"),
        t,
        onOpenMedia: () => {},
        archiveHref: () => "/posts",
      })
    }
    // The wall measures its columns and clamps; happy-dom lays nothing out,
    // so the wall keeps one column and every post's wording unclamped.
    const globals = globalThis as any
    const previous = {
      act: globals.IS_REACT_ACT_ENVIRONMENT,
      ResizeObserver: globals.ResizeObserver,
      getComputedStyle: globals.getComputedStyle,
    }
    globals.IS_REACT_ACT_ENVIRONMENT = true
    globals.ResizeObserver = class {
      observe() {}
      disconnect() {}
    }
    globals.getComputedStyle = (element: Element) =>
      browser.getComputedStyle(element as any)
    let root: Rendered | undefined
    try {
      await act(async () => {
        root = render(
          createElement(
            AppSettingsProvider,
            null,
            createElement(
              PageMessagesProvider,
              { messages: dashboardMessages },
              createElement(Harness),
            ),
          ),
        )
      })
      const notes = [...root!.container.querySelectorAll("article.post-note")]
      assert.equal(notes.length, 5)
      const [dynamic, imageOnly, gone, textless, xPost] = notes
      const links = (note: Element) =>
        [...note.querySelectorAll("a")].map((link) => link.getAttribute("href"))

      for (const note of [dynamic, imageOnly, gone, textless]) {
        const handle = note.querySelector(".post-note-handle")!
        assert.equal(handle.textContent, "bilibili")
        assert.ok(handle.querySelector("svg"))
        assert.doesNotMatch(note.textContent || "", /@/u)
        assert.equal(note.querySelector(".post-engagement"), null)
        assert.ok(links(note).every((href) => !/x\.example/u.test(href || "")))
        assert.equal(
          note.querySelector(".post-note-open")!.getAttribute("aria-label"),
          "Open the original post on bilibili",
        )
      }
      assert.deepEqual(links(dynamic), [
        "https://space.bilibili.example.invalid/100000001",
        "https://www.bilibili.example.invalid/opus/1000000000000000001",
        "https://www.bilibili.example.invalid/opus/1000000000000000003",
      ])
      assert.match(dynamic.textContent || "", /合成动态/u)
      // The forwarded dynamic shows by its author's name and the bilibili
      // mark, links to its page and has the translation switch.
      const card = dynamic.querySelector(".post-quote")!
      assert.ok(card.querySelector("svg.post-quote-mark"))
      const name = card.querySelector(".post-quote-name")!
      assert.equal(name.textContent, "Synthetic Agency")
      assert.equal(
        name.getAttribute("aria-label"),
        "Open the forwarded post by Synthetic Agency on bilibili",
      )
      assert.equal(card.querySelector(".post-quote-handle"), null)
      // An English reader sees the forwarded wording translated.
      assert.match(card.textContent || "", /Synthetic forwarded dynamic/u)
      assert.ok(card.querySelector(".translation-mark"))
      assert.equal(imageOnly.querySelector(".post-quote"), null)
      // A deleted dynamic shows the unavailable card under the wording.
      assert.match(
        gone.querySelector(".post-note-text")?.textContent || "",
        /原动态没了/u,
      )
      const unavailable = gone.querySelector(".post-quote.is-unavailable")!
      assert.equal(
        unavailable.getAttribute("href"),
        "https://www.bilibili.example.invalid/opus/1000000000000000005",
      )
      assert.match(unavailable.textContent || "", /Quoted post unavailable/u)
      // No wording of its own: just the quote, without an empty text block.
      assert.equal(textless.querySelector(".post-note-text"), null)
      assert.equal(textless.querySelector(".post-note-more"), null)
      assert.equal(
        textless.querySelector(".post-quote-name")?.textContent,
        "Synthetic Agency",
      )
      assert.equal(textless.querySelector(".post-quote.is-unavailable"), null)
      // An emoji-only dynamic keeps its images without an empty paragraph.
      assert.equal(imageOnly.querySelector(".post-note-text"), null)
      assert.ok(imageOnly.querySelector(".post-note-media img"))

      // An X post keeps its handle, quote and counts.
      assert.equal(
        xPost.querySelector(".post-note-handle")!.textContent,
        "@synthetic_account",
      )
      assert.ok(xPost.querySelector(".post-quote"))
      assert.ok(xPost.querySelector(".post-engagement"))

      // The heading is the title alone, without a source label.
      assert.equal(
        root!.container.querySelector("#feed .section-title .kicker"),
        null,
      )

      // The account filter offers the Bilibili account by its mark, not its
      // mid, without counts, and shows only its posts.
      const filters = [
        ...root!.container.querySelectorAll("#feed .segmented button"),
      ]
      assert.deepEqual(
        filters.map((button) => button.textContent),
        ["All", "@synthetic_account", "bilibili"],
      )
      assert.ok(filters[2].querySelector("svg.account-mark"))
      await click(filters[2])
      assert.equal(filters[2].getAttribute("aria-pressed"), "true")
      assert.equal(
        root!.container.querySelectorAll("article.post-note").length,
        4,
      )
    } finally {
      await act(async () => root?.unmount())
      globals.IS_REACT_ACT_ENVIRONMENT = previous.act
      globals.ResizeObserver = previous.ResizeObserver
      globals.getComputedStyle = previous.getComputedStyle
    }
  })
})

test("a dynamic's stickers show in its original wording, never in a translation", async () => {
  await withVite(async (load) => {
    const [
      { PostWall },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
    ] = await Promise.all([
      load("/src/dashboard/components/post-wall.tsx"),
      load("/src/app-settings.tsx"),
    ])
    const local = "/media/synthetic-laugh?v=synthetic"
    const base = {
      source: "bilibili",
      accountHandle: "100000001",
      accountUrl: "https://space.bilibili.example.invalid/100000001",
      publishedAt: "2026-10-08T12:00:00+09:00",
      mediaUrl: null,
      mediaStatus: "none",
      media: [],
      quote: null,
    }
    const posts = [
      {
        ...base,
        id: "bilibili-1000000000000000011",
        url: "https://www.bilibili.example.invalid/opus/1000000000000000011",
        text: "早安",
        translations: {
          en: { text: "Good morning", origin: "machine" },
        },
        sourceLanguage: "zh",
        // A picture not cached yet shows its caption; an address that is
        // not a local one is never loaded.
        rich: [
          { text: "早" },
          { emoji: "[笑]", url: local },
          { text: "安" },
          { emoji: "[哭]", url: null },
          { emoji: "[外]", url: "https://i0.hdslb.example.invalid/x.png" },
        ],
        quote: {
          id: "bilibili-1000000000000000013",
          authorHandle: "42",
          authorName: "Synthetic Agency",
          text: "明天",
          publishedAt: "2026-10-07T12:00:00+09:00",
          url: "https://www.bilibili.example.invalid/opus/1000000000000000013",
          unavailable: false,
          media: [],
          translations: {},
          sourceLanguage: "zh",
          rich: [{ text: "明天" }, { emoji: "[赞]", url: local }],
        },
      },
      {
        // Stickers alone: no text and nothing to translate.
        ...base,
        id: "bilibili-1000000000000000012",
        url: "https://www.bilibili.example.invalid/opus/1000000000000000012",
        text: "",
        translations: {},
        sourceLanguage: null,
        rich: [{ emoji: "[笑]", url: local }],
      },
    ]
    function Harness() {
      const { locale, t } = useAppSettings()
      return createElement(PostWall, {
        posts,
        latestPost: posts[0],
        accounts: [],
        bilibiliAccounts: ["100000001"],
        avatarUrl: null,
        windowDays: 14,
        isLoading: false,
        locale,
        now: Date.parse("2026-10-08T21:00:00+09:00"),
        t,
        onOpenMedia: () => {},
        archiveHref: () => "/posts",
      })
    }
    const globals = globalThis as any
    const previous = {
      act: globals.IS_REACT_ACT_ENVIRONMENT,
      ResizeObserver: globals.ResizeObserver,
      getComputedStyle: globals.getComputedStyle,
    }
    globals.IS_REACT_ACT_ENVIRONMENT = true
    globals.ResizeObserver = class {
      observe() {}
      disconnect() {}
    }
    globals.getComputedStyle = (element: Element) =>
      browser.getComputedStyle(element as any)
    let root: Rendered | undefined
    try {
      await act(async () => {
        root = render(
          createElement(
            AppSettingsProvider,
            null,
            createElement(
              PageMessagesProvider,
              { messages: dashboardMessages },
              createElement(Harness),
            ),
          ),
        )
      })
      const [dynamic, stickersOnly] = [
        ...root!.container.querySelectorAll("article.post-note"),
      ]
      const text = dynamic.querySelector(".post-note-text")!
      // An English reader sees the translation, without stickers.
      assert.match(text.textContent || "", /Good morning/u)
      assert.equal(text.querySelector("img"), null)
      await click(text.querySelector(".translation-mark")!)
      const stickers = [...text.querySelectorAll("img.post-sticker")]
      assert.deepEqual(
        stickers.map((img) => [
          img.getAttribute("src"),
          img.getAttribute("alt"),
        ]),
        [[local, "[笑]"]],
      )
      assert.equal(stickers[0].getAttribute("title"), "[笑]")
      assert.match(text.textContent || "", /^早安\[哭\]\[外\]/u)
      // The forwarded dynamic shows its sticker the same way.
      const quoted = dynamic.querySelector(".post-quote-text img.post-sticker")
      assert.equal(quoted?.getAttribute("src"), local)
      assert.equal(quoted?.getAttribute("alt"), "[赞]")
      // A dynamic of stickers alone shows them rather than an empty card.
      assert.equal(
        stickersOnly
          .querySelector(".post-note-text img.post-sticker")
          ?.getAttribute("alt"),
        "[笑]",
      )
      for (const img of root!.container.querySelectorAll("img"))
        assert.doesNotMatch(img.getAttribute("src") || "", /^https?:/u)
    } finally {
      await act(async () => root?.unmount())
      globals.IS_REACT_ACT_ENVIRONMENT = previous.act
      globals.ResizeObserver = previous.ResizeObserver
      globals.getComputedStyle = previous.getComputedStyle
    }
  })
})

test("a post's video and GIF stills carry their mark and open the post on X", async () => {
  await withVite(async (load) => {
    const [
      { PostWall, PostMediaView },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
    ] = await Promise.all([
      load("/src/dashboard/components/post-wall.tsx"),
      load("/src/app-settings.tsx"),
    ])
    const item = (name: string, kind: string) => ({
      id: `x-${name}`,
      url: `/media/x-${name}?v=synthetic`,
      status: "ready",
      sourceUrl: null,
      kind,
    })
    const post = {
      id: "1900000000000000900",
      source: "x",
      accountHandle: "example",
      url: "https://x.example.invalid/example/status/1900000000000000900",
      text: "clip",
      publishedAt: "2026-10-08T12:00:00+09:00",
      mediaUrl: null,
      mediaStatus: "ready",
      media: [
        item("still", "photo"),
        item("clip", "video"),
        item("loop", "gif"),
      ],
      translations: {},
      sourceLanguage: "ja",
      quote: {
        id: "1900000000000000901",
        authorHandle: "other",
        authorName: "Other",
        text: "quoted",
        publishedAt: "2026-10-07T12:00:00+09:00",
        url: "https://x.example.invalid/other/status/1900000000000000901",
        unavailable: false,
        media: [item("quoted", "video")],
        translations: {},
        sourceLanguage: "ja",
      },
    }
    const opened: { items: any[]; index: number }[] = []
    function Harness() {
      const { locale, t } = useAppSettings()
      return createElement(
        "div",
        null,
        createElement(PostWall, {
          posts: [post],
          latestPost: post,
          accounts: ["example"],
          bilibiliAccounts: [],
          avatarUrl: null,
          windowDays: 14,
          isLoading: false,
          locale,
          now: Date.parse("2026-10-08T21:00:00+09:00"),
          t,
          onOpenMedia: (items: any[], index: number) =>
            opened.push({ items, index }),
          archiveHref: () => "/posts",
        }),
        createElement(
          "div",
          { className: "viewer" },
          createElement(PostMediaView, {
            item: { ...item("clip", "video"), href: post.url },
            t,
          }),
          createElement(PostMediaView, { item: item("still", "photo"), t }),
        ),
      )
    }
    const globals = globalThis as any
    const previous = {
      act: globals.IS_REACT_ACT_ENVIRONMENT,
      ResizeObserver: globals.ResizeObserver,
      getComputedStyle: globals.getComputedStyle,
    }
    globals.IS_REACT_ACT_ENVIRONMENT = true
    globals.ResizeObserver = class {
      observe() {}
      disconnect() {}
    }
    globals.getComputedStyle = (element: Element) =>
      browser.getComputedStyle(element as any)
    let root: Rendered | undefined
    try {
      await act(async () => {
        root = render(
          createElement(
            AppSettingsProvider,
            null,
            createElement(
              PageMessagesProvider,
              { messages: dashboardMessages },
              createElement(Harness),
            ),
          ),
        )
      })
      const note = root!.container.querySelector("article.post-note")!
      const grid = [...note.querySelector(".post-note-media")!.children]
      // The photo opens the viewer; the video and the GIF link to the post.
      assert.deepEqual(
        grid.map((element) => [
          element.tagName,
          element.getAttribute("href"),
          element.getAttribute("target"),
          element.getAttribute("aria-label"),
          element.querySelector(".media-kind-mark")?.className ?? null,
        ]),
        [
          ["BUTTON", null, null, "Open image 1", null],
          [
            "A",
            post.url,
            "_blank",
            "Play video on X 2",
            "media-kind-mark is-video",
          ],
          [
            "A",
            post.url,
            "_blank",
            "Play GIF on X 3",
            "media-kind-mark is-gif",
          ],
        ],
      )
      assert.equal(
        grid[2].querySelector(".media-kind-mark")!.textContent,
        "GIF",
      )
      await click(grid[0])
      // The viewer's set keeps every still, each video linked to its post.
      assert.deepEqual(
        opened.at(-1)!.items.map((entry) => [entry.kind, entry.href ?? null]),
        [
          ["photo", null],
          ["video", post.url],
          ["gif", post.url],
        ],
      )
      assert.equal(opened.at(-1)!.index, 0)
      // A quoted video plays from the quote's page.
      const quoted = note.querySelector(".post-quote-media a")!
      assert.equal(quoted.getAttribute("href"), post.quote.url)
      assert.equal(quoted.getAttribute("aria-label"), "Play video on X 1")
      assert.ok(quoted.querySelector(".media-kind-mark.is-video"))
      // In the viewer, a video's still links to its post; a photo stays an
      // image.
      const viewer = root!.container.querySelector(".viewer")!
      const play = viewer.querySelector("a.media-dialog-play")!
      assert.equal(play.getAttribute("href"), post.url)
      assert.equal(play.getAttribute("aria-label"), "Play video on X")
      assert.equal(viewer.querySelectorAll("a").length, 1)
      assert.equal(viewer.querySelectorAll("img").length, 2)
      // Every board locale names both actions.
      for (const locale of ["en", "ja", "zh-CN"] as const)
        for (const key of ["feed.playVideo", "feed.playGif"] as const)
          assert.ok(dashboardMessages[locale][key], `${locale} ${key}`)
    } finally {
      if (root) await act(async () => root!.unmount())
      globals.IS_REACT_ACT_ENVIRONMENT = previous.act
      globals.ResizeObserver = previous.ResizeObserver
      globals.getComputedStyle = previous.getComputedStyle
    }
  })
})

test("a promoted repost names the own accounts that reposted it", async () => {
  await withVite(async (load) => {
    const [
      { PostWall, repostingAccounts },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
    ] = await Promise.all([
      load("/src/dashboard/components/post-wall.tsx"),
      load("/src/app-settings.tsx"),
    ])
    const base = {
      source: "x",
      mediaUrl: null,
      mediaStatus: "none",
      media: [],
      translations: {},
      sourceLanguage: "en",
      quote: null,
    }
    const reposted = {
      ...base,
      id: "1900000000000000950",
      accountHandle: "composer_example",
      accountUrl: "https://x.example.invalid/composer_example",
      url: "https://x.example.invalid/composer_example/status/1900000000000000950",
      text: "Synthetic song release",
      publishedAt: "2026-10-08T12:00:00+09:00",
      promoted: true,
      repostedBy: ["_SyntheticB", "kano_example"],
    }
    const own = {
      ...base,
      id: "1900000000000000951",
      accountHandle: "kano_example",
      accountUrl: "https://x.example.invalid/kano_example",
      url: "https://x.example.invalid/kano_example/status/1900000000000000951",
      text: "Synthetic own post",
      publishedAt: "2026-10-08T11:00:00+09:00",
      promoted: false,
      repostedBy: [],
    }
    function Harness() {
      const { locale, t } = useAppSettings()
      return createElement(PostWall, {
        posts: [reposted, own],
        latestPost: own,
        accounts: ["kano_example", "_SyntheticB"],
        bilibiliAccounts: [],
        avatarUrl: null,
        windowDays: 3,
        isLoading: false,
        locale,
        now: Date.parse("2026-10-08T21:00:00+09:00"),
        t,
        onOpenMedia: () => {},
        archiveHref: () => "/posts",
      })
    }
    const globals = globalThis as any
    const previous = {
      act: globals.IS_REACT_ACT_ENVIRONMENT,
      ResizeObserver: globals.ResizeObserver,
      getComputedStyle: globals.getComputedStyle,
    }
    globals.IS_REACT_ACT_ENVIRONMENT = true
    globals.ResizeObserver = class {
      observe() {}
      disconnect() {}
    }
    globals.getComputedStyle = (element: Element) =>
      browser.getComputedStyle(element as any)
    let root: Rendered | undefined
    try {
      await act(async () => {
        root = render(
          createElement(
            AppSettingsProvider,
            null,
            createElement(
              PageMessagesProvider,
              { messages: dashboardMessages },
              createElement(Harness),
            ),
          ),
        )
      })
      const notes = [...root!.container.querySelectorAll("article.post-note")]
      const marks = notes.map(
        (note) =>
          note.querySelector(".post-note-reposted")?.textContent ?? null,
      )
      // The repost shows its author and names both reposting accounts in
      // their order; an own post carries no mark.
      assert.deepEqual(marks, [
        "Reposted by @_SyntheticB and @kano_example",
        null,
      ])
      assert.equal(
        notes[0].querySelector(".post-note-handle")!.textContent,
        "@composer_example",
      )
    } finally {
      if (root) await act(async () => root!.unmount())
      globals.IS_REACT_ACT_ENVIRONMENT = previous.act
      globals.ResizeObserver = previous.ResizeObserver
      globals.getComputedStyle = previous.getComputedStyle
    }
    // Every board locale names the accounts in its own list form.
    for (const locale of ["en", "ja", "zh-CN"] as const)
      assert.match(
        dashboardMessages[locale]["feed.repostedBy"],
        /\{accounts\}/u,
      )
    assert.equal(repostingAccounts(["kano_example"], "ja"), "@kano_example")
    for (const locale of ["ja", "zh-CN"] as const)
      assert.equal(
        repostingAccounts(["_SyntheticB", "kano_example"], locale),
        new Intl.ListFormat(locale, { type: "conjunction" }).format([
          "@_SyntheticB",
          "@kano_example",
        ]),
      )
  })
})

test("PostWall resolves per-account, hidden, promoted, and fallback avatars", async () => {
  await withVite(async (load) => {
    const [
      { PostWall },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
    ] = await Promise.all([
      load("/src/dashboard/components/post-wall.tsx"),
      load("/src/app-settings.tsx"),
    ])
    const makePost = (
      id: string,
      accountHandle: string,
      promoted: boolean,
      publishedAt: string,
    ) => ({
      id,
      source: "x",
      accountHandle,
      accountUrl: `https://x.example.invalid/${accountHandle}`,
      url: `https://x.example.invalid/${accountHandle}/status/${id}`,
      text: `post ${id}`,
      publishedAt,
      mediaUrl: null,
      mediaStatus: "ready",
      media: [],
      translations: {},
      sourceLanguage: "ja",
      promoted,
    })
    const posts = [
      makePost("1", "kano_example", false, "2026-10-08T12:00:00+09:00"),
      makePost("2", "_SyntheticB", false, "2026-10-08T11:00:00+09:00"),
      makePost("3", "_FallbackOwn", false, "2026-10-08T10:00:00+09:00"),
      makePost("4", "_HiddenOwn", false, "2026-10-08T09:00:00+09:00"),
      makePost("5", "RelatedCached", true, "2026-10-08T08:00:00+09:00"),
      makePost("6", "UnsetRelated", true, "2026-10-08T07:00:00+09:00"),
    ]
    const accountAvatars = {
      "x:kano_example": "/media/account-avatar/aaa?v=1",
      "x:_syntheticb": "/media/account-avatar/bbb?v=2",
      "x:_hiddenown": null,
      "x:relatedcached": "/media/account-avatar/ccc?v=3",
    }
    function Harness() {
      const { locale, t } = useAppSettings()
      return createElement(PostWall, {
        posts,
        latestPost: posts[0],
        accounts: ["kano_example", "_SyntheticB", "_FallbackOwn", "_HiddenOwn"],
        bilibiliAccounts: [],
        avatarUrl: "/media/profile/site?v=0",
        accountAvatars,
        windowDays: 14,
        isLoading: false,
        locale,
        now: Date.parse("2026-10-08T21:00:00+09:00"),
        t,
        onOpenMedia: () => {},
        archiveHref: () => "/posts",
      })
    }
    const globals = globalThis as any
    const previous = {
      act: globals.IS_REACT_ACT_ENVIRONMENT,
      ResizeObserver: globals.ResizeObserver,
      getComputedStyle: globals.getComputedStyle,
    }
    globals.IS_REACT_ACT_ENVIRONMENT = true
    globals.ResizeObserver = class {
      observe() {}
      disconnect() {}
    }
    globals.getComputedStyle = (element: Element) =>
      browser.getComputedStyle(element as any)
    let root: Rendered | undefined
    try {
      await act(async () => {
        root = render(
          createElement(
            AppSettingsProvider,
            null,
            createElement(
              PageMessagesProvider,
              { messages: dashboardMessages },
              createElement(Harness),
            ),
          ),
        )
      })
      const heads = [
        ...root!.container.querySelectorAll(
          "article.post-note .post-note-head",
        ),
      ].map((head) => {
        const img = head.querySelector("img")
        const initial = head.querySelector(".post-note-initial")
        return {
          src: img ? img.getAttribute("src") : null,
          initial: initial ? initial.textContent : null,
        }
      })
      assert.deepEqual(heads, [
        { src: "/media/account-avatar/aaa?v=1", initial: null },
        { src: "/media/account-avatar/bbb?v=2", initial: null },
        { src: "/media/profile/site?v=0", initial: null },
        { src: null, initial: "H" },
        { src: "/media/account-avatar/ccc?v=3", initial: null },
        { src: null, initial: "U" },
      ])
      // A portrait that fails to load gives way to the handle's initial.
      const broken = root!.container.querySelector(
        "article.post-note .post-note-head img",
      )!
      await act(async () => {
        broken.dispatchEvent(
          new (browser.Event as unknown as typeof Event)("error"),
        )
      })
      const first = root!.container.querySelector(
        "article.post-note .post-note-head",
      )!
      assert.equal(first.querySelector("img"), null)
      assert.equal(first.querySelector(".post-note-initial")!.textContent, "k")
    } finally {
      if (root) await act(async () => root!.unmount())
      globals.IS_REACT_ACT_ENVIRONMENT = previous.act
      globals.ResizeObserver = previous.ResizeObserver
      globals.getComputedStyle = previous.getComputedStyle
    }
  })
})

test("a lone image keeps its shape within the board's bounds", async () => {
  await withVite(async (load) => {
    const { loneImageFrame, handleInitial } = await load(
      "/src/dashboard/components/post-wall.tsx",
    )
    // Within the bounds the frame is the image's own shape, uncropped.
    assert.deepEqual(loneImageFrame({ width: 1600, height: 900 }, 1), {
      ratio: 1600 / 900,
      letterboxed: false,
    })
    // A tall image stops at a square on the board and at 3:4 in full; a
    // very wide one at 2:1. Past the bounds it sits whole on its own blur.
    assert.deepEqual(loneImageFrame({ width: 900, height: 1600 }, 1), {
      ratio: 1,
      letterboxed: true,
    })
    assert.deepEqual(loneImageFrame({ width: 900, height: 1600 }, 3 / 4), {
      ratio: 3 / 4,
      letterboxed: true,
    })
    assert.deepEqual(loneImageFrame({ width: 3000, height: 1000 }, 1), {
      ratio: 2,
      letterboxed: true,
    })
    // An image of unknown size keeps the shared 16:10 frame.
    assert.equal(loneImageFrame({ width: null, height: null }, 1), null)
    assert.equal(handleInitial("@_SyntheticB"), "S")
    assert.equal(handleInitial("__"), "_")
    assert.equal(handleInitial(""), "X")
  })
})

test("a post counts its images past four and a board quote shows one thumbnail", async () => {
  await withVite(async (load) => {
    const [
      { PostWall, PostArchive },
      { AppSettingsProvider, PageMessagesProvider, useAppSettings },
    ] = await Promise.all([
      load("/src/dashboard/components/post-wall.tsx"),
      load("/src/app-settings.tsx"),
    ])
    const item = (name: string, width = 1200, height = 1200) => ({
      id: `x-${name}`,
      url: `/media/x-${name}?v=synthetic`,
      status: "ready",
      sourceUrl: null,
      kind: "photo",
      width,
      height,
    })
    const post = {
      id: "1900000000000000980",
      source: "x",
      accountHandle: "kano_example",
      url: "https://x.example.invalid/kano_example/status/1900000000000000980",
      text: "five images",
      publishedAt: "2026-10-08T12:00:00+09:00",
      mediaUrl: null,
      mediaStatus: "ready",
      media: ["a", "b", "c", "d", "e"].map((name) => item(name)),
      translations: {},
      sourceLanguage: "ja",
      quote: {
        id: "1900000000000000981",
        authorHandle: "Related_Example",
        authorName: "Related",
        text: "quoted with images",
        publishedAt: "2026-10-07T12:00:00+09:00",
        url: "https://x.example.invalid/Related_Example/status/1900000000000000981",
        unavailable: false,
        media: [item("q1", 900, 1600), item("q2"), item("q3")],
        translations: {},
        sourceLanguage: "ja",
      },
    }
    const accountAvatars = {
      "x:related_example": "/media/account-avatar/ddd?v=4",
    }
    const opened: { items: any[]; index: number }[] = []
    function Harness() {
      const { locale, t } = useAppSettings()
      const common = {
        accountAvatars,
        avatarUrl: null,
        locale,
        now: Date.parse("2026-10-08T21:00:00+09:00"),
        t,
        onOpenMedia: (items: any[], index: number) =>
          opened.push({ items, index }),
      }
      return createElement(
        "div",
        null,
        createElement(PostWall, {
          ...common,
          posts: [post],
          latestPost: post,
          accounts: ["kano_example"],
          bilibiliAccounts: [],
          windowDays: 14,
          isLoading: false,
          archiveHref: () => "/posts",
        }),
        createElement(
          "div",
          { className: "archive" },
          createElement(PostArchive, {
            ...common,
            archive: { items: [post], hasNext: false, adjacent: null },
            layout: "list",
            isLoading: false,
            error: null,
            onRetry: () => {},
            onLoadMore: () => {},
            onJump: () => {},
          }),
        ),
      )
    }
    const globals = globalThis as any
    const previous = {
      act: globals.IS_REACT_ACT_ENVIRONMENT,
      ResizeObserver: globals.ResizeObserver,
      getComputedStyle: globals.getComputedStyle,
    }
    globals.IS_REACT_ACT_ENVIRONMENT = true
    globals.ResizeObserver = class {
      observe() {}
      disconnect() {}
    }
    globals.getComputedStyle = (element: Element) =>
      browser.getComputedStyle(element as any)
    let root: Rendered | undefined
    try {
      await act(async () => {
        root = render(
          createElement(
            AppSettingsProvider,
            null,
            createElement(
              PageMessagesProvider,
              { messages: dashboardMessages },
              createElement(Harness),
            ),
          ),
        )
      })
      const note = root!.container.querySelector("#feed article.post-note")!
      const grid = note.querySelector(":scope > .post-note-media")!
      // Four tiles; the fourth counts the fifth and opens the viewer there.
      assert.equal(grid.className, "post-note-media count-4")
      assert.equal(grid.children.length, 4)
      assert.equal(grid.querySelector(".post-media-more")!.textContent, "+1")
      await click(grid.children[3])
      assert.equal(opened.at(-1)!.items.length, 5)
      assert.equal(opened.at(-1)!.index, 3)
      // The board quote shows its author's avatar and one thumbnail
      // counting the other two, which opens the viewer at the first.
      const quote = note.querySelector(".post-quote")!
      assert.ok(quote.classList.contains("has-thumb"))
      assert.equal(
        quote.querySelector(".post-quote-avatar")!.getAttribute("src"),
        "/media/account-avatar/ddd?v=4",
      )
      const thumb = quote.querySelectorAll(".post-quote-media.is-thumb button")
      assert.equal(thumb.length, 1)
      assert.equal(
        thumb[0].querySelector(".post-quote-count")!.textContent,
        "+2",
      )
      await click(thumb[0])
      assert.equal(opened.at(-1)!.items.length, 3)
      assert.equal(opened.at(-1)!.index, 0)
      // In the archive the quote shows its images as a post's.
      const archived = root!.container.querySelector(".archive .post-quote")!
      assert.ok(!archived.classList.contains("has-thumb"))
      assert.equal(
        archived.querySelector(".post-note-media")!.className,
        "post-note-media count-3",
      )
    } finally {
      if (root) await act(async () => root!.unmount())
      globals.IS_REACT_ACT_ENVIRONMENT = previous.act
      globals.ResizeObserver = previous.ResizeObserver
      globals.getComputedStyle = previous.getComputedStyle
    }
  })
})
