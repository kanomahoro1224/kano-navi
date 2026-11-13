import path from "node:path"
import { defineConfig, loadEnv } from "vite"
import react from "@vitejs/plugin-react"
import tailwindcss from "@tailwindcss/vite"
import { precompress } from "./scripts/vite-precompress.ts"
import { pagePreload } from "./scripts/vite-page-preload.ts"

export default defineConfig(({ command, mode }) => {
  const environment = loadEnv(mode, process.cwd(), "")
  // Optional CDN: the build addresses its own assets at CDN_PUBLIC_BASE
  // (backend/internal/cdn rewrites the rest of index.html). A development
  // server always serves "/", since the same variable may name the deployed
  // site's CDN.
  const cdnBase =
    command === "build"
      ? (environment.CDN_PUBLIC_BASE || "").trim().replace(/\/+$/, "")
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
