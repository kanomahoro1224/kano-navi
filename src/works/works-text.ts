import type { Translator } from "../types.ts"
import type { WorksText } from "./types.ts"

const wordEnd = /[\p{L}\p{N})」』》]$/u
const wordStart = /^[\p{L}\p{N}(「『《]/u

/**
 * English punctuation around the collection's names: Japanese and Chinese
 * brackets, colons and list marks read as their ASCII forms with spaces.
 */
function latinPunctuation(text: string, capitalize: boolean) {
  const result = text
    .replace(/（/gu, "(")
    .replace(/）/gu, ")")
    .replace(/：\s*/gu, ": ")
    .replace(/[、，]\s*/gu, ", ")
    .replace(/；\s*/gu, "; ")
    .replace(/(?<!\d):(?=[^\s/])/gu, ": ")
    .replace(/([^\s(])\(/gu, "$1 (")
    .replace(/\(\s+/gu, "(")
    .replace(/\s+\)/gu, ")")
    .replace(/\s{2,}/gu, " ")
    .trim()
  return capitalize && /^\p{Ll}/u.test(result)
    ? result.charAt(0).toUpperCase() + result.slice(1)
    : result
}

/**
 * Collection wording in the reader's language: names as the collection
 * writes them, generic words translated. English puts spaces between a
 * phrase and the names beside it and capitalizes a leading phrase, never a
 * name; Japanese and Chinese keep the collection's spacing.
 */
export function worksText(text: WorksText | undefined, t: Translator) {
  if (!text) return ""
  const parts = typeof text === "string" ? [text] : text
  const latin = t("works.text.style") === "latin"
  let result = ""
  for (const part of parts) {
    const piece =
      typeof part === "string"
        ? part
        : t(`works.phrase.${part.phrase}`, {
            value: part.value ?? "",
            count: part.count ?? "",
          })
    if (latin && piece && wordEnd.test(result) && wordStart.test(piece))
      result += " "
    result += piece
  }
  return latin ? latinPunctuation(result, typeof parts[0] !== "string") : result
}
