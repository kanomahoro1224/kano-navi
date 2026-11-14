/**
 * The optional CDN base the server advertises in index.html
 * (`<meta name="kano-cdn-base">`), or "" when this origin serves everything.
 * The build itself reads CDN_PUBLIC_BASE for its own assets; the server writes
 * the normalized base into the page for the media addresses the frontend builds
 * from its packaged catalogs.
 */
export function cdnBase(): string {
  // Outside a page (Node scripts, tests with a stand-in document) there is
  // no advertised base.
  if (
    typeof document === "undefined" ||
    typeof document.querySelector !== "function"
  )
    return ""
  const content =
    document
      .querySelector('meta[name="kano-cdn-base"]')
      ?.getAttribute("content") ?? ""
  return content.trim().replace(/\/+$/, "")
}

/** A root-relative public path, through the CDN when the page advertises one. */
export function cdnUrl(path: string): string {
  const base = cdnBase()
  return base && path.startsWith("/") ? `${base}${path}` : path
}
