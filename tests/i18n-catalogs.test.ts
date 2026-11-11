import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import { aboutMessages } from "../src/about/about-i18n.ts"
import { adminMessages } from "../src/admin-i18n.ts"
import { dashboardMessages } from "../src/dashboard/dashboard-i18n.ts"
import { historyMessages } from "../src/history/history-i18n.ts"
import { lineblogMessages } from "../src/lineblog/lineblog-i18n.ts"
import { worksMessages } from "../src/works/works-i18n.ts"

const locales = ["en", "ja", "zh-CN"] as const

function sameKeys(
  name: string,
  catalog: Record<string, Record<string, unknown>>,
) {
  const sets = locales.map((locale) => new Set(Object.keys(catalog[locale])))
  const all = new Set(sets.flatMap((keys) => [...keys]))
  for (const [index, locale] of locales.entries()) {
    const missing = [...all].filter((key) => !sets[index].has(key))
    assert.deepEqual(missing, [], `${name} ${locale}`)
  }
}

test("every interface catalog has the same keys in each locale", () => {
  sameKeys("admin", adminMessages)
  sameKeys("dashboard", dashboardMessages)
  sameKeys("history", historyMessages)
  sameKeys("works", worksMessages)
  sameKeys("about", aboutMessages)
  sameKeys("lineblog", lineblogMessages)
  sameKeys("shell", shellMessages(readFileSync("src/i18n.ts", "utf8")))
})

/** Keys of the shell catalog, which stays private to the translator. */
function shellMessages(source: string) {
  const sections: Record<string, Record<string, true>> = {}
  let locale = ""
  for (const line of source.split("\n")) {
    const header = line.match(/^  (?:"(zh-CN)"|(ja|en)): \{$/)
    if (header) {
      locale = header[1] || header[2]
      sections[locale] = {}
      continue
    }
    if (!locale) continue
    if (line === "  },") {
      locale = ""
      continue
    }
    const key = line.match(/^    "([^"]+)":/)
    if (key) sections[locale][key[1]] = true
  }
  return sections
}
