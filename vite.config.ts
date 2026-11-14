import path from "node:path"
import { defineConfig, loadEnv } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { precompress } from "./scripts/vite-precompress.ts"
import { pagePreload } from "./scripts/vite-page-preload.ts"
import { normalizeCdnBase } from "./scripts/cdn-base.ts"

export default defineConfig(({ command, mode }) => {
  const environment = loadEnv(mode, process.cwd(), "")
  // Optional CDN: the build addresses its own assets at CDN_PUBLIC_BASE
  // (backend/internal/cdn rewrites the rest of index.html). A development
  // server always serves "/", since the same variable may name the deployed
  // site's CDN.
  // The value is validated and normalized like the server's (lowercase host,
  // no trailing slash), so the bundle and the API name the same base, and a
  // bad value fails the build.
  const cdnBase =
    command === "build"
      ? normalizeCdnBase(environment.CDN_PUBLIC_BASE || "")
      : ""
  const base = cdnBase ? `${cdnBase}/` : "/"
  const port = Number(environment.PORT || 7657)
  // The Go backend owns /api, /media and /mcp. Vite only serves the frontend
  // and forwards those paths with the browser's Host, so the admin origin
  // guard sees the same origin the page was loaded from.
  const apiPort = Number(environment.KANO_API_PORT || 7658)
  const backend = process.env.KANO_API_ORIGIN || `http://127.0.0.1:${apiPort}`
  const proxy = { "^/(?:api|media|mcp)(?:/|$)": { target: backend } }
  // The watcher covers the frontend sources only. Runtime data, local
  // reference material, the Go module, build output and local tool state
  // (agent worktrees included) hold tens of thousands of files; crawling them
  // at startup holds every module request back until the crawl ends.
  const ignored = [
    "**/data/**",
    "**/ref/**",
    "**/backend/**",
    "**/dist/**",
    "**/.claude/**",
    "**/.codegraph/**",
  ]
  // A Windows or macOS checkout mounted into the development container sends
  // no file events, so the container polls the sources instead.
  const watch =
    environment.KANO_WATCH_POLLING === "1"
      ? { usePolling: true, interval: 500, binaryInterval: 1000, ignored }
      : { ignored }
  return {
    base,
    build: { manifest: true },
    plugins: [
      react(),
      tailwindcss(),
      // The web app manifest stays at this origin even with a CDN base:
      // browsers resolve its start_url and scope against the manifest's own
      // address and ignore them when that is another origin.
      // Vite moves root-relative url() references to public files in CSS
      // (such as the LINE BLOG icons) to the base. With a CDN base, fail the
      // build if one were left at "/", where a prefixed base would miss it.
      {
        name: "kano-cdn-css-urls",
        apply: "build",
        // After vite:css-post, which emits the CSS in its own generateBundle
        // when cssCodeSplit is off.
        enforce: "post",
        generateBundle(_options, bundle) {
          if (!cdnBase) return
          for (const file of Object.values(bundle)) {
            if (file.type !== "asset" || !file.fileName.endsWith(".css"))
              continue
            const source =
              typeof file.source === "string"
                ? file.source
                : new TextDecoder().decode(file.source)
            const left = source.match(/url\(\s*["']?\/(?!\/)[^)]*\)/)
            if (left) {
              this.error(
                `${file.fileName} still names ${left[0]} at the origin root while CDN_PUBLIC_BASE is set`,
              )
            }
          }
        },
      },
      {
        name: "kano-manifest-origin",
        enforce: "post",
        transformIndexHtml: (html) =>
          cdnBase
            ? html.replaceAll(
                `href="${base}manifest.webmanifest"`,
                'href="/manifest.webmanifest"',
              )
            : html,
      },
      // The modules src/main.tsx imports for the board.
      pagePreload({
        "/": [
          "src/dashboard/dashboard-app.tsx",
          "src/dashboard/dashboard-i18n.ts",
        ],
      }),
      // Runs after the bundle is written, so index.html already holds the
      // preload script.
      precompress(),
    ],
    server: { port, strictPort: true, proxy, watch },
    preview: { port, strictPort: true, proxy },
    resolve: {
      alias: {
        "@": path.resolve(process.cwd(), "./src"),
      },
    },
  }
})
