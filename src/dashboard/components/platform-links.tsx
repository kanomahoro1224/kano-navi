import type { Translator } from "@/types"
import { ArrowUpRight, Radio } from "lucide-react"

import { BilibiliLogo, ExternalLink, YouTubeLogo } from "@/site/links"
import type { VideoLink } from "../video-groups"

const platformNames: Record<string, string> = {
  youtube: "YouTube",
  bilibili: "bilibili",
}

/** A link's name: its platform, or the host of a link elsewhere. */
function linkName(link: VideoLink) {
  if (platformNames[link.source]) return platformNames[link.source]
  try {
    return new URL(link.url).hostname.replace(/^www\./u, "")
  } catch {
    return link.source
  }
}

function PlatformMark({ source }: { source: string }) {
  if (source === "youtube") return <YouTubeLogo aria-hidden="true" />
  if (source === "bilibili") return <BilibiliLogo aria-hidden="true" />
  return <ArrowUpRight aria-hidden="true" />
}

/**
 * One link per platform, in `videoLinks` order: the card's own link first,
 * then the other uploads of the same work, such as its bilibili copy. An
 * entry's links (`eventLinks`) mark a platform that is live now.
 */
export function PlatformLinks({
  links,
  title,
  variant,
  t,
}: {
  links: VideoLink[]
  title: string
  variant: "pill" | "chip"
  t: Translator
}) {
  return (
    <span className="platform-links">
      {links.map((link, index) => {
        const name = linkName(link)
        const className =
          variant === "pill"
            ? `pill-button${index === 0 ? " is-solid" : ""}`
            : "platform-chip"
        const live = link.live ? ` (${t("schedule.status.live")})` : ""
        return (
          <ExternalLink
            key={link.url}
            className={`${className}${link.live ? " is-live" : ""}`}
            href={link.url}
            aria-label={`${t("videos.watchOn", { platform: name })}: ${title}${live}`}
          >
            <PlatformMark source={link.source} />
            {name}
            {link.live ? (
              <Radio className="platform-live" aria-hidden="true" />
            ) : null}
          </ExternalLink>
        )
      })}
    </span>
  )
}

/**
 * A cover's mark for the platforms a work is on (`videoPlatforms`). It only
 * signals; the links follow the title, so a tap on the cover still opens the
 * card's link.
 */
export function PlatformBadge({ platforms }: { platforms: string[] }) {
  if (!platforms.length) return null
  return (
    <span className="platform-badge" aria-hidden="true">
      {platforms.map((source, index) => (
        <PlatformMark key={`${source}-${index}`} source={source} />
      ))}
    </span>
  )
}
