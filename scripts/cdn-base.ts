import net from "node:net"

/**
 * Normalizes CDN_PUBLIC_BASE for the build exactly as backend/internal/config
 * CDNBase does at startup, so the bundle, the server's
 * `<meta name="kano-cdn-base">` and the API name one spelling of the base: an
 * http or https URL with a lowercase host, an optional port and an optional
 * path prefix of plain segments, without a trailing slash. Empty stays empty.
 * Anything else throws, so a bad value fails `vite build` rather than the
 * deployed server.
 */
export function normalizeCdnBase(raw: string): string {
  const value = raw.trim()
  if (value === "") return ""
  const invalid = new Error("CDN_PUBLIC_BASE must be an http or https base URL")
  const parts = /^(https?):\/\/([^/?#]*)([^?#]*)$/i.exec(value)
  if (!parts) throw invalid
  const [, scheme, authority, rawPath] = parts
  const host = normalizedAuthority(authority)
  const path = basePath(rawPath)
  if (host === null || path === null) throw invalid
  return `${scheme.toLowerCase()}://${host}${path}`
}

/** A lowercase DNS name or IP address with an optional canonical port. */
function normalizedAuthority(authority: string): string | null {
  if (authority.includes("@") || authority.endsWith(":")) return null
  const parts = /^(\[[^\]]*\]|[^:[\]]*)(?::([0-9]+))?$/.exec(authority)
  if (!parts) return null
  const [, rawHost, port] = parts
  let host = rawHost.toLowerCase()
  if (host.startsWith("[")) {
    if (!net.isIPv6(host.slice(1, -1))) return null
  } else if (!net.isIPv4(host) && !dnsName(host)) {
    return null
  }
  if (port !== undefined) {
    const number = Number(port)
    if (number < 1 || number > 65535 || String(number) !== port) return null
    host += `:${port}`
  }
  return host
}

/** Letters, digits and inner hyphens in dot-separated labels. */
function dnsName(host: string): boolean {
  if (host === "" || host.length > 253) return false
  return host
    .split(".")
    .every(
      (label) =>
        label.length > 0 &&
        label.length <= 63 &&
        /^[a-z0-9-]+$/.test(label) &&
        !label.startsWith("-") &&
        !label.endsWith("-"),
    )
}

/** The path prefix without its trailing slash, or null when it is not plain. */
function basePath(raw: string): string | null {
  if (raw !== "" && !raw.startsWith("/")) return null
  const path = raw.endsWith("/") ? raw.slice(0, -1) : raw
  if (path === "") return ""
  for (const segment of path.slice(1).split("/")) {
    if (segment === "" || segment === "." || segment === "..") return null
    if (!/^[A-Za-z0-9._~-]+$/.test(segment)) return null
  }
  return path
}
