import { useState, type ComponentType, type SVGProps } from "react"
import { Languages } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { cn } from "@/lib/utils"
import { BilibiliLogo, YouTubeLogo } from "@/site/links"
import type { Locale, Translation, Translations, Translator } from "@/types"

/** The lang attribute of wording in a detected source language. */
const sourceLangs: Record<string, string> = {
  ja: "ja",
  en: "en",
  zh: "zh",
  ko: "ko",
}

/** The marks of the platforms whose translations readers see. */
const platformLogos: Record<string, ComponentType<SVGProps<SVGSVGElement>>> = {
  youtube: YouTubeLogo,
  bilibili: BilibiliLogo,
}

/** The platform of a platform translation the switch has a mark for. */
function knownPlatform(translation: Translation) {
  const platform = translation.platform || ""
  return Object.hasOwn(platformLogos, platform) ? platform : null
}

/** Whether a detected source language (ja, ja-JP, ...) is Japanese. */
function isJapanese(language: string | undefined | null) {
  return (language || "").toLowerCase().split("-")[0] === "ja"
}

/**
 * The translation a reader of the content locale sees instead of the source
 * wording, or null: untranslated wording keeps the source, and Japanese
 * readers see a Japanese translation only of wording not written in Japanese.
 */
export function pickTranslation(
  translations: Translations | undefined | null,
  locale: Locale,
  sourceLanguage?: string | null,
): Translation | null {
  if (locale === "ja" && isJapanese(sourceLanguage)) return null
  const translation = translations?.[locale]
  return translation?.text ? translation : null
}

/** Names where a translation came from: AI, a platform or the editor. */
export function translationOriginLabel(
  translation: Translation,
  t: Translator,
): string {
  if (translation.origin === "machine") return t("translation.machine")
  if (translation.origin === "manual") return t("translation.manual")
  return t(`translation.platform.${knownPlatform(translation) || "other"}`)
}

/**
 * What follows the switch's translate icon: "AI" for a model's translation,
 * the platform's mark for a platform's, and nothing for the editor's, which
 * shows the translate icon alone.
 */
export function translationMarkKind(
  translation: Translation,
): "ai" | "youtube" | "bilibili" | "icon" {
  if (translation.origin === "machine") return "ai"
  if (translation.origin === "manual") return "icon"
  const platform = knownPlatform(translation)
  return platform === "youtube" || platform === "bilibili" ? platform : "icon"
}

export interface TranslatedTextState {
  /** The wording to show. */
  text: string
  /** The translation shown, or null while the source shows. */
  shown: Translation | null
  /** The translation this reader can switch to, shown or not. */
  available: Translation | null
  /** The lang attribute of the wording shown. */
  lang: string | undefined
  toggle: () => void
}

/**
 * Shows the translation in the reader's content language by default and lets
 * them switch back.
 */
export function useTranslatedText(
  original: string,
  translations: Translations | undefined | null,
  sourceLanguage: string | undefined | null,
): TranslatedTextState {
  const { contentLocale } = useAppSettings()
  const [showOriginal, setShowOriginal] = useState(false)
  const available = pickTranslation(translations, contentLocale, sourceLanguage)
  const shown = available && !showOriginal ? available : null
  return {
    text: shown ? shown.text : original,
    shown,
    available,
    lang: shown ? contentLocale : sourceLangs[sourceLanguage || ""],
    toggle: () => setShowOriginal((value) => !value),
  }
}

/**
 * The switch after translated wording: a translate icon followed by its
 * origin's mark (translationMarkKind), lit while the translation shows.
 * Selecting it shows the source wording and turns it off; selecting it again
 * brings the translation back. Its tooltip and accessible name spell out
 * where the translation came from (AI, a platform or the editor), and a
 * title's tooltip previews the source wording.
 */
export function TranslationToggle({
  state,
  original,
  t,
  className,
}: {
  state: TranslatedTextState
  /** Short source wording, such as a title, to preview in the tooltip. */
  original?: string
  t: Translator
  className?: string
}) {
  const { available, shown } = state
  if (!available) return null
  const origin = translationOriginLabel(available, t)
  const kind = translationMarkKind(available)
  const PlatformLogo =
    kind === "youtube" || kind === "bilibili" ? platformLogos[kind] : null
  const hint = [
    origin,
    available.origin === "machine" ? t("translation.machineHint") : null,
    shown && original
      ? t("translation.originalIs", { text: original })
      : t(shown ? "translation.showOriginal" : "translation.showTranslation"),
  ]
    .filter(Boolean)
    .join(" · ")
  return (
    <button
      type="button"
      className={cn(
        "translation-mark",
        kind !== "ai" && "is-icon",
        shown && "is-active",
        className,
      )}
      title={hint}
      aria-label={origin}
      aria-pressed={Boolean(shown)}
      onClick={state.toggle}
    >
      <Languages aria-hidden="true" />
      {kind === "ai" ? t("translation.machineShort") : null}
      {PlatformLogo ? <PlatformLogo aria-hidden="true" /> : null}
    </button>
  )
}
