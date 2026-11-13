# CDN

`CDN_PUBLIC_BASE` moves the site's static files and cacheable media to an
optional CDN. Empty — the default — keeps every address on this origin. Any
HTTP(S) host works: Cloudflare, Alibaba Cloud CDN, Tencent Cloud EdgeOne or a
self-hosted nginx cache. The application only builds the addresses; it does not
configure or contact a provider.

## What the Switch Changes

The frontend build reads `CDN_PUBLIC_BASE` and writes its content-hashed
JavaScript, CSS, fonts and bundled images to that base (`base` in
`vite.config.ts`). The Go service reads the same variable at startup and:

- rewrites the remaining root-relative references in `index.html` (icons and
  the share-tag banner; the web app manifest stays at the origin so its
  `start_url` and `scope` keep matching the site) and adds
  `<meta name="kano-cdn-base">` for the media paths the frontend builds from its
  packaged catalogs;
- returns every public media address — dashboard posts and schedule images,
  the post and video archives, the calendar, the works uploads — as
  `CDN_PUBLIC_BASE/media/...` instead of `/media/...`.

Both parts are needed for full offload. A build without the variable still
serves its entry chunk and CSS from the origin, and lazy chunks always follow
the build, so build with `CDN_PUBLIC_BASE` set (`npm run build`, or
`docker build --build-arg CDN_PUBLIC_BASE=...`) and keep it in the runtime
environment. Turning the switch off restores the origin addresses; no data
changes and no migration is needed.

`PUBLIC_ORIGIN` stays the site's own origin: share links, `og:url` and the
Atom feed's self link keep pointing at the site, not at the CDN.

## Origin Contract

The CDN pulls from the site's public origin, so the paths it caches are the
ones the service already serves. Cache these:

| Path                                           | Origin `Cache-Control`                  | Notes                                                        |
| ---------------------------------------------- | --------------------------------------- | ------------------------------------------------------------ |
| `/assets/<hash>.js`                            | `public, max-age=31536000, immutable`   | Manifest-listed build files; the hash changes with the bytes |
| `/assets/<hash>.css`                           | `public, max-age=31536000, immutable`   | Same                                                         |
| `/assets/*` (other)                            | `public, max-age=0, must-revalidate`    | Icons and the bundled banner; small and long-lived           |
| `/media/<id>?v=<hash>`                         | `public, max-age=31536000, immutable`   | Cached source media; `v` is the content hash                 |
| `/media/<id>`                                  | `public, max-age=0, must-revalidate`    | Without a version; ETag revalidation                         |
| `/media/{lineblog,works,journey}/<hash>.<ext>` | immutable once the bytes match the name | Archive images and videos                                    |
| `/manifest.webmanifest`, `/robots.txt`         | `public, max-age=0, must-revalidate`    | Small fixed files                                            |

Keep the query string in the cache key: `?v=` selects a kept earlier version
and `?w=` selects a resized copy (320, 640, 960, 1280 or 1920). Do **not**
strip query parameters on `/media/`.

Never cache `/api/*` (including `/api/dashboard/events`, a long-lived
event stream), `/admin`, `/mcp`, `/internal/*` or `/healthz`. `index.html` and
the SPA fallback routes are deliberately `must-revalidate`; caching them at the
edge would pin visitors to a build whose hashed chunks may already be gone.
Cache only the paths above — not "everything".

The service answers ranges for media and sets `Vary: Accept-Encoding` on the
text it serves, with precompressed `.br`/`.gz` siblings. A CDN may pass those
through or compress at the edge, but it must not recompress a body that
already carries `Content-Encoding`, and it should honor `Vary` (or normalize
compression at the edge). Public responses carry no cookies or credentials,
so the asset host needs no cookie forwarding and should not receive any.

## CORS and HTTPS

The page is served from the site origin and its assets from the CDN, so the
asset host is a different origin. That requires:

- `Access-Control-Allow-Origin` for the site origin (or `*`) on `/assets/`.
  Module scripts, stylesheets and fonts are fetched in CORS mode
  (`crossorigin`); images are not, but allowing them is harmless.
- HTTPS on the CDN host when the site is HTTPS. An `http://` CDN base in an
  `https://` page is blocked as mixed content. `CDN_PUBLIC_BASE` accepts both
  schemes, so this is a deployment choice rather than a validated rule.

No cookies are needed, so a separate asset hostname (rather than the site's
own) also avoids sending the site's cookies to the CDN.

## Recommended Setup

1. Create a pull-zone/CDN hostname, for example `cdn.example.invalid`, with the
   site origin (`https://kano.example.invalid`, port 443) as its origin, and
   `Host: kano.example.invalid` as the origin host if the provider asks (the Go
   service does not check the Host header for public reads).
2. Add a cache rule for `/assets/*` and `/media/*` that respects the origin's
   `Cache-Control` (or sets a one-year edge TTL for `/assets/` and passes
   `/media/` through). Cloudflare's default file-extension cache would not
   cache extension-less `/media/<hash>` paths, so a path-based rule is needed
   there.
3. Bypass the cache for `/api/*`, `/admin*`, `/mcp*` and `/internal/*`.
4. Issue an HTTPS certificate for the CDN hostname and enable HTTP/2 or
   HTTP/3.
5. Set the variable where both stages see it:

   ```bash
   # Build the frontend with the CDN base.
   CDN_PUBLIC_BASE=https://cdn.example.invalid npm run build
   # Run the service with the same value.
   CDN_PUBLIC_BASE=https://cdn.example.invalid kano-go serve -static dist
   ```

   With Docker, pass it as a build argument and as a runtime variable:

   ```bash
   docker build --build-arg CDN_PUBLIC_BASE=https://cdn.example.invalid -t kano-navi:local .
   ```

   For the published image, the build argument is fixed at image build time;
   a deployment that wants its own CDN rebuilds the image, or accepts that
   lazy chunks stay on the origin while the runtime switch still moves media.

6. If the origin firewall restricts access, allow the provider's origin addresses
   (Cloudflare publishes its ranges; Alibaba Cloud and Tencent Cloud list
   theirs in their consoles).

The origin needs no nginx change beyond what it already does for the site: the
CDN requests the same public paths, and the Go listener serves them with the
headers above. Keep `proxy_set_header Host $host` and pass `Range` and
`Accept-Encoding` through unchanged.

## Verification

```bash
# 1. The build addresses its own files at the CDN.
grep -o 'https://cdn.example.invalid/assets/[^"]*' dist/index.html | head

# 2. The running service advertises the base and rewrites media addresses.
curl -s http://127.0.0.1:7657/ | grep kano-cdn-base
curl -s 'http://127.0.0.1:7657/api/dashboard?days=1' | grep -o '"url":"https://cdn.example.invalid/media/[^"]*"' | head

# 3. The CDN serves an asset with the origin's cache headers and CORS.
curl -sI https://cdn.example.invalid/assets/$(ls dist/assets | grep -m1 '\.js$') \
  | grep -i 'cache-control\|access-control-allow-origin'

# 4. A media range request passes through.
curl -sI -H 'Range: bytes=0-1023' https://cdn.example.invalid/media/<id>?v=<hash>
```

In the browser, the network panel shows no `/assets/` or `/media/` requests to
the site origin; only `/api/*` calls and the HTML itself stay there. Unset the
variable, rebuild and restart: every address returns to the origin.
