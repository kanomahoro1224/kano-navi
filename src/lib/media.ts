// Smaller copies of images. The server makes them ahead at a fixed list of
// widths (backend/internal/mediavariant) for cached source media, LINE BLOG
// archive images, Works covers and Journey images, and keeps the original
// untouched; full-image viewers keep loading the original URL.

import { cdnBase } from "./cdn.ts"

/** The only widths the server accepts in a media URL's `w` parameter. */
export const MEDIA_VARIANT_WIDTHS = [320, 640, 960, 1280, 1920] as const

/**
 * Matches an image URL with smaller copies: a cached asset (`/media/<id>`), a
 * LINE BLOG archive image, a Works cover or a Journey image. Profile media,
 * videos and bundled assets have none.
 */
const CACHED_MEDIA =
  /^\/media\/(?:[a-f0-9]{64}|(?:lineblog|works|journey)\/[a-f0-9]{12}\.(?:jpg|png))(?:\?|$)/

/**
 * The URL without the advertised CDN base, so a media address still matches
 * the routes below whichever host serves it.
 */
function originPath(url: string): string {
  const base = cdnBase()
  return base && url.startsWith(`${base}/`) ? url.slice(base.length) : url
}

/** Whether url names a cached media route, on this origin or on the CDN. */
export function isMediaUrl(url: string | null | undefined): boolean {
  return !!url && originPath(url).startsWith("/media/")
}

/** The URL of a smaller copy, or the URL itself when it has none. */
export function mediaVariantUrl(url: string, width: number): string {
  if (!CACHED_MEDIA.test(originPath(url))) return url
  return `${url}${url.includes("?") ? "&" : "?"}w=${width}`
}

/**
 * A srcset for an image with smaller copies. With a known original width it
 * lists the smaller copies and then the original; without one it lists every
 * copy, and the server answers a width the image cannot shrink to with the
 * original. Other URLs (bundled assets, profile media, missing images) get
 * none.
 */
export function mediaSrcSet(
  url: string | null | undefined,
  originalWidth?: number | null,
): string | undefined {
  if (!url || !CACHED_MEDIA.test(originPath(url))) return undefined
  const known =
    typeof originalWidth === "number" && originalWidth > 0
      ? originalWidth
      : null
  const widths = MEDIA_VARIANT_WIDTHS.filter(
    (width) => known === null || width < known,
  )
  const candidates = widths.map(
    (width) => `${mediaVariantUrl(url, width)} ${width}w`,
  )
  if (known !== null) candidates.push(`${url} ${known}w`)
  return candidates.length ? candidates.join(", ") : undefined
}
