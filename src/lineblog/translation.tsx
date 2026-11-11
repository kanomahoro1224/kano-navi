import { useState } from "react"
import { Languages } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { cn } from "@/lib/utils"
import { type LineblogDesign } from "./designs"
import type { LineblogPost } from "./types"

/** The translation each content language shows; Japanese shows none. */
export const TRANSLATIONS = {
  "zh-CN": {
    blocks: "zh",
    title: "titleZh",
    profile: "profileTextZh",
    lang: "zh-CN",
    menu: { home: "首页", site: "鹿乃官方网站" },
  },
  en: {
    blocks: "en",
    title: "titleEn",
    profile: "profileTextEn",
    lang: "en",
    menu: { home: "Home", site: "Kano Official Site" },
  },
} as const

export type Translation = (typeof TRANSLATIONS)[keyof typeof TRANSLATIONS]

/** Whether a post carries any of the translation's lines. */
export function hasTranslation(post: LineblogPost, translation: Translation) {
  return (
    Boolean(post[translation.title]) ||
    post.blocks.some(
      (block) => block.type === "p" && Boolean(block[translation.blocks]),
    )
  )
}

/**
 * The same mark as a translated video title: AI, lit while the translation
 * lines show and off while they are hidden. The menu bar's mark switches
 * every article; an article's own mark switches that article until the menu
 * bar's is used.
 */
export function TranslationMark({
  on,
  all = false,
  onToggle,
}: {
  on: boolean
  all?: boolean
  onToggle: () => void
}) {
  const { t } = useAppSettings()
  const action = t(on ? "translation.showOriginal" : "lineblog.translationOn")
  const scope = all ? `${t("lineblog.allArticles")} · ` : ""
  return (
    <button
      type="button"
      className={cn("translation-mark", on && "is-active")}
      title={`${scope}${t("translation.machineHint")} · ${action}`}
      aria-label={`${scope}${t("translation.machine")}`}
      aria-pressed={on}
      onClick={onToggle}
    >
      <Languages aria-hidden="true" />
      {t("translation.machineShort")}
    </button>
  )
}

/**
 * The own translation switch of an article, the profile or the recent posts:
 * it holds until the menu bar's switch is used.
 */
export function useOwnTranslation(
  translationOn: boolean,
  translationRound: number,
) {
  const [own, setOwn] = useState<{ round: number; on: boolean } | null>(null)
  const on = own?.round === translationRound ? own.on : translationOn
  return [on, () => setOwn({ round: translationRound, on: !on })] as const
}

/**
 * A menu-bar link's Japanese label and, while the menu bar's switch shows
 * translations, the archive's own translation after it, without an AI mark.
 */
export function MenuLabel({
  label,
  item,
  translation,
}: {
  label: string
  item: keyof Translation["menu"]
  translation: Translation | null
}) {
  return (
    <>
      {label}
      {translation ? (
        <small className="lb-menu-translation" lang={translation.lang}>
          {translation.menu[item]}
        </small>
      ) : null}
    </>
  )
}

/** The blog's profile text, followed by its translation and the AI mark. */
export function ProfileText({
  design,
  offered,
  translationOn,
  translationRound,
  className,
}: {
  design: LineblogDesign
  offered: Translation | null
  translationOn: boolean
  /** Counts uses of the menu bar's switch. */
  translationRound: number
  className: string
}) {
  const [on, toggle] = useOwnTranslation(translationOn, translationRound)
  return (
    <>
      <p className={className} lang="ja">
        {design.profileText}
      </p>
      {offered ? (
        <p className={cn(className, `${className}-translation`)}>
          {on ? (
            <span lang={offered.lang}>{design[offered.profile]} </span>
          ) : null}
          <TranslationMark on={on} onToggle={toggle} />
        </p>
      ) : null}
    </>
  )
}
