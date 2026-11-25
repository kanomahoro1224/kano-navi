import type { Dashboard, Locale, Profile, Translator } from "@/types"
import {
  BookOpen,
  Globe,
  Hash,
  House,
  ListMusic,
  Radio,
  ShoppingBag,
  Sparkles,
} from "lucide-react"

import { SectionTitle } from "@/site/site-shell"
import { BilibiliLogo, ExternalLink, XLogo, YouTubeLogo } from "@/site/links"
import { hashtags } from "../content"
import { profileLinkUrls } from "../profile-links"
import { localizeSeedText } from "../seed-copy"

function resourceIcon(url = "") {
  if (/\/\/(?:www\.)?(?:x|twitter)\.com\//u.test(url)) return XLogo
  if (/youtube\.com/u.test(url)) return YouTubeLogo
  if (/bilibili\.com/u.test(url)) return BilibiliLogo
  if (/twitcasting\.tv/u.test(url)) return Radio
  if (/nicovideo\.jp/u.test(url)) return ListMusic
  if (/\/\/shop\./u.test(url)) return ShoppingBag
  if (/milpr\.com/u.test(url)) return Sparkles
  if (/amebaownd\.com/u.test(url)) return House
  if (/wikipedia\.org/u.test(url)) return BookOpen
  return Globe
}

function hostOf(url: string) {
  try {
    return new URL(url).hostname.replace(/^www\./u, "")
  } catch {
    return ""
  }
}

/**
 * The board's closing row: the resources the profile card does not already
 * link, each a chip whose tooltip is its detail, then the common hashtags.
 */
export function LinkDirectory({
  profile,
  resources,
  locale,
  t,
}: {
  profile: Pick<Profile, "xUrl" | "youtubeUrl">
  resources: Dashboard["resources"]
  locale: Locale
  t: Translator
}) {
  const onCard = profileLinkUrls(profile, resources)
  const links = resources.filter((resource) => !onCard.has(resource.url))
  return (
    <section className="section links" id="links" aria-labelledby="links-title">
      <SectionTitle id="links-title" title={t("directory.title")} />
      <div className="links-layout">
        <ul className="link-chips">
          {links.map((resource) => {
            const Icon = resourceIcon(resource.url)
            return (
              <li key={resource.id}>
                <ExternalLink
                  className="pill-button"
                  href={resource.url}
                  title={
                    localizeSeedText(resource.detail, locale) ||
                    hostOf(resource.url)
                  }
                >
                  <Icon aria-hidden="true" />
                  {localizeSeedText(resource.title, locale)}
                </ExternalLink>
              </li>
            )
          })}
        </ul>

        <div className="hashtags">
          <span className="hashtags-label">
            <Hash aria-hidden="true" />
            {t("directory.tags")}
          </span>
          {hashtags.map(({ tag, url }) => (
            <ExternalLink key={tag} className="hashtag" href={url}>
              #{tag}
            </ExternalLink>
          ))}
        </div>
      </div>
    </section>
  )
}
