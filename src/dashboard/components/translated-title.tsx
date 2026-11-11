import { useState, type PointerEvent } from "react"
import type { Translations, Translator, Video } from "@/types"

import {
  TranslationToggle,
  useTranslatedText,
} from "@/components/translated-text"
import { ExternalLink } from "@/site/links"
import { cn } from "@/lib/utils"
import { cleanVideoTitle } from "../format"

/**
 * Whether a card's clamp cuts the wording: the wording clamps itself next to
 * a translation switch, and its card title clamps it otherwise.
 */
function clamped(wording: HTMLElement) {
  return [wording, wording.parentElement].some(
    (box) => box !== null && box.scrollHeight > box.clientHeight + 1,
  )
}

/**
 * A video or schedule title on a card (a ticket, a row): the reader's
 * translation, followed by the translation switch. The title is the
 * card's link (see `followCardLink`), because the switch cannot go inside
 * one; with `onOpen` instead of `href`, the title is the button that opens
 * the card's details. `title` is the source title as the card shows it;
 * `translations` and `sourceLanguage` are its video's or event's. While the
 * card's clamp cuts the wording, hovering it shows the whole wording.
 */
export function TranslatedTitle({
  className,
  title,
  translations,
  sourceLanguage,
  href,
  onOpen,
  t,
}: {
  className: string
  title: string
  translations?: Translations | null
  sourceLanguage?: string | null
  href?: string | null
  /** Opens the card's details; `href` wins when both are given. */
  onOpen?: () => void
  t: Translator
}) {
  const state = useTranslatedText(title, translations, sourceLanguage)
  const text = state.shown ? cleanVideoTitle(state.text) : title
  // Measured on hover, so a resize or a language switch never leaves a stale
  // hint and an uncut title gets none.
  const [hint, setHint] = useState<string>()
  const hover = {
    title: hint,
    onPointerEnter: (event: PointerEvent<HTMLElement>) =>
      setHint(clamped(event.currentTarget) ? text : undefined),
  }
  const wording = href ? (
    <ExternalLink
      className="card-link translated-title-text"
      href={href}
      lang={state.lang}
      {...hover}
    >
      {text}
    </ExternalLink>
  ) : onOpen ? (
    <button
      type="button"
      className="card-link translated-title-text"
      lang={state.lang}
      onClick={onOpen}
      aria-haspopup="dialog"
      {...hover}
    >
      {text}
    </button>
  ) : (
    <span className="translated-title-text" lang={state.lang} {...hover}>
      {text}
    </span>
  )
  return (
    <span className={cn(className, state.available && "translated-title")}>
      {wording}
      <TranslationToggle state={state} original={title} t={t} />
    </span>
  )
}

/**
 * A video title outside any link: the reader's translation as a heading,
 * with the same switch after its last word.
 */
export function TranslatedHeading({
  title,
  video,
  t,
}: {
  title: string
  video: Video
  t: Translator
}) {
  const state = useTranslatedText(
    title,
    video.translations,
    video.sourceLanguage,
  )
  return (
    <h3 className="translated-heading">
      <span lang={state.lang}>{cleanVideoTitle(state.text)}</span>
      <TranslationToggle state={state} original={title} t={t} />
    </h3>
  )
}
