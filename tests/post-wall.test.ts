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
        onOpenArchive: () => {},
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

      // The section names both sources while the wall holds a dynamic.
      const kicker = root!.container.querySelector("#feed .kicker")
      assert.match(kicker?.textContent || "", /@synthetic_account · +bilibili/u)

      // The account filter offers the Bilibili account by its mark, not its
      // mid, and shows only its posts.
      const filters = [
        ...root!.container.querySelectorAll("#feed .segmented button"),
      ]
      assert.deepEqual(
        filters.map((button) => button.textContent),
        ["All5", "@synthetic_account1", "bilibili4"],
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
        onOpenArchive: () => {},
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
