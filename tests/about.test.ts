import assert from "node:assert/strict"
import path from "node:path"
import test from "node:test"
import { act, createElement } from "react"
import { createServer } from "vite"
import { browser, click, render, type Rendered } from "./dom.ts"

test("About lists the apps with their unsigned platforms and opens the other models", async () => {
  const vite = await createServer({
    configFile: false,
    envDir: false,
    server: { middlewareMode: true, watch: null, ws: false },
    optimizeDeps: { noDiscovery: true, include: [] },
    resolve: { alias: { "@": path.resolve("src") } },
  })
  const previousAct = (globalThis as any).IS_REACT_ACT_ENVIRONMENT
  ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
  let root: Rendered | undefined
  try {
    const [
      { AboutApp },
      { aboutMessages },
      { AppSettingsProvider, PageMessagesProvider },
    ] = await Promise.all([
      vite.ssrLoadModule("/src/about/about-app.tsx"),
      vite.ssrLoadModule("/src/about/about-i18n.ts"),
      vite.ssrLoadModule("/src/app-settings.tsx"),
    ])
    await act(async () => {
      root = render(
        createElement(
          AppSettingsProvider,
          null,
          createElement(
            PageMessagesProvider,
            { messages: aboutMessages },
            createElement(AboutApp),
          ),
        ),
      )
    })
    const page = root!.container

    // The apps follow the title, the fans' app first, each with its maker,
    // platforms (with their lowest system version where known) and link; the
    // unsigned platforms share one frame that names them unsigned.
    const apps = [
      ...page.querySelectorAll(".about-section-apps .about-app"),
    ].map((app) => ({
      name: app.querySelector("h3")!.textContent,
      maker: app.querySelector(".about-app-name p")!.textContent,
      signed: [...app.querySelectorAll(".about-app-platforms > li.tag")].map(
        (chip) => chip.textContent,
      ),
      unsigned: [...app.querySelectorAll(".about-app-unsigned li")].map(
        (chip) => chip.textContent,
      ),
      mark: app.querySelector(".about-app-unsigned > span")?.textContent,
      link: app.querySelector("a.about-app-action")!.getAttribute("href"),
    }))
    assert.deepEqual(apps, [
      {
        name: "Kano Fans’ Treasure (小鹿包们的宝物)",
        maker: "By Lu KeKe (鹿可可)",
        signed: ["Android", "HarmonyOS 5+", "Windows"],
        unsigned: ["iOS 13+"],
        mark: "Unsigned",
        link: "https://dl.xiaolubao.com",
      },
      {
        name: "Kano Navi for Apple",
        maker: "By yexca",
        signed: [],
        unsigned: ["iOS 26+", "iPadOS 26+", "macOS 26+"],
        mark: "Unsigned",
        link: "https://github.com/yexca/kano-navi-apple",
      },
    ])
    // A contributor with a separate name shows the login after it.
    assert.deepEqual(
      [...page.querySelectorAll(".about-contributors a")].map((link) => [
        link.textContent,
        link.getAttribute("href"),
      ]),
      [
        ["Lu Ke (kanomahoro1224)", "https://github.com/kanomahoro1224"],
        ["ZhFuwe", "https://github.com/ZhFuwe"],
      ],
    )

    // The LINE BLOG credit links its curator and the project in its wording.
    const lineblog = [
      ...page.querySelectorAll(".about-section-credits .about-card"),
    ].find((card) => card.querySelector("h3")!.textContent === "LINE BLOG")!
    assert.deepEqual(
      [...lineblog.querySelectorAll("a")].map((link) => [
        link.textContent,
        link.getAttribute("href"),
      ]),
      [
        [
          "Kano_LINE_Blog_Recovery",
          "https://github.com/ZhFuwe/Kano_LINE_Blog_Recovery",
        ],
        ["ZhFuwe", "https://github.com/ZhFuwe"],
      ],
    )
    assert.match(
      lineblog.textContent || "",
      /from the Kano_LINE_Blog_Recovery project by ZhFuwe\./u,
    )

    // Three models show; "+4" opens the rest and Escape closes them.
    const models = page.querySelector(".about-chips.is-mono")!
    assert.deepEqual(
      [...models.querySelectorAll(":scope > li:not(.is-more)")].map(
        (item) => item.textContent,
      ),
      ["Claude Opus 5.5", "Gemini 4 Hydrogen", "GPT-6.2-Sol"],
    )
    const more = models.querySelector("button.about-more-button")!
    const panel = models.querySelector(".about-more-panel")!
    assert.equal(more.textContent, "+4")
    assert.equal(more.getAttribute("aria-label"), "4 more models")
    assert.equal(panel.hasAttribute("hidden"), true)
    await click(more)
    assert.equal(more.getAttribute("aria-expanded"), "true")
    assert.equal(panel.hasAttribute("hidden"), false)
    assert.deepEqual(
      [...panel.querySelectorAll("li")].map((item) => item.textContent),
      ["GPT-6.1-Sol", "Gemini 4 Carbon", "GPT-6-Astra", "GPT-5.6-Sol"],
    )
    await act(async () => {
      ;(browser.document as unknown as Document).dispatchEvent(
        new (browser.KeyboardEvent as unknown as typeof KeyboardEvent)(
          "keydown",
          { key: "Escape" },
        ),
      )
    })
    assert.equal(more.getAttribute("aria-expanded"), "false")
    assert.equal(panel.hasAttribute("hidden"), true)

    // Each locale names the apps and the maker in its own way.
    assert.equal(aboutMessages.ja["about.app.treasure"], "鹿友たちの宝物")
    assert.equal(aboutMessages["zh-CN"]["about.app.treasure"], "小鹿包们的宝物")
    for (const locale of ["zh-CN", "ja"] as const)
      assert.equal(aboutMessages[locale]["about.name.lukeke"], "鹿可可")
  } finally {
    await act(async () => root?.unmount())
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = previousAct
    await vite.close()
  }
})
