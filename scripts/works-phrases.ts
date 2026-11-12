import type { WorksPhrase, WorksText } from "../src/works/types.ts"
import type { WorksPhraseCode } from "../src/works/works-phrases.ts"

/*
 * The collection writes its descriptive fields in Japanese with Simplified
 * Chinese notes. Packaging keeps names as written (people, works, stores,
 * platforms, editions and bonus items) and turns the generic words around
 * them into phrase codes, which the page translates (src/works/works-phrases.ts).
 * Wording left in Chinese after that stops packaging, so a new note needs a
 * phrase before it is published.
 */

type Entry = {
  pattern: RegExp
  /** The phrase this wording becomes; without one it is kept as written. */
  phrase?: WorksPhraseCode
  /** The capture group holding the name the phrase wraps. */
  value?: number
  /** The capture group holding the phrase's number. */
  count?: number
}

const name = (pattern: RegExp): Entry => ({ pattern })
const phrase = (
  pattern: RegExp,
  code: WorksPhraseCode,
  groups: { value?: number; count?: number } = {},
): Entry => ({ pattern, phrase: code, ...groups })

/**
 * The collection's Chinese spellings of Japanese names, read before phrases:
 * edition names, the artwork of an anime license and bonus items.
 */
const spellings: [RegExp, string][] = [
  [/歌手盘/gu, "アーティスト盤"],
  [/生产/gu, "生産"],
  [/绘本付/gu, "絵本付"],
  [/版権绘/gu, "版権絵"],
  [/盘(?!面)/gu, "盤"],
  [/明信片/gu, "ポストカード"],
  [/立牌/gu, "アクリルスタンド"],
  [/钥匙扣/gu, "アクリルキーホルダー"],
  [/，/gu, "、"],
]

/** Names that hold a phrase's words: limited editions and Chinese platforms. */
const names = [
  name(/(?:初回生産|初回|絵本付生産|アニメイト|とらのあな)?限定盤/uy),
  name(/QQ音乐|酷狗音乐|酷我音乐/uy),
]

/**
 * The descriptive fields of releases and editions: captions, edition names,
 * formats, packaging, bonuses, stores, disc names, labels and prices.
 * Longer wording comes before the shorter wording it holds.
 */
const releasePhrases: Entry[] = [
  ...names,
  // Phrases that wrap a name or a number.
  phrase(/([^（）]+?)等图案/uy, "designFor", { value: 1 }),
  phrase(/与\s*([^（）]+?)\s*同时购买/uy, "boughtWith", { value: 1 }),
  phrase(/与\s*([^（）]+?)\s*相同/uy, "sameAs", { value: 1 }),
  phrase(/上记各店与\s*([^（）]+)/uy, "aboveStoresAnd", { value: 1 }),
  phrase(/(?<=（)含\s*([^（）]+)/uy, "including", { value: 1 }),
  phrase(/随机\s*(\d+)\s*种之一/uy, "oneOfRandom", { count: 1 }),
  phrase(/(\d+)\s*个组/uy, "setOf", { count: 1 }),
  phrase(/专属语音\s*(\d+)\s*段/uy, "exclusiveVoice", { count: 1 }),
  phrase(/购买\s*(\d+)\s*张即解锁/uy, "unlockedByBuying", { count: 1 }),
  phrase(
    /([^\s：:、]+?)专属\s*3D\s*形象电子签名照\s*(\d+)\s*张/uy,
    "signedAvatarPhotos",
    { value: 1, count: 2 },
  ),
  phrase(/([^\s：:、]+?)粉丝勋章/uy, "fanBadge", { value: 1 }),
  // What an image shows.
  phrase(/中文翻唱数字专辑封面/uy, "chineseDigitalAlbumCover"),
  phrase(/同人专辑封面/uy, "doujinAlbumCover"),
  phrase(/专辑封面/uy, "albumCover"),
  phrase(/单曲封面/uy, "singleCover"),
  phrase(/先行配信封面/uy, "earlyDigitalCover"),
  phrase(/配信封面/uy, "digitalCover"),
  phrase(/插画封面/uy, "illustratedCover"),
  phrase(/CD 封面尺寸/uy, "cdCoverSize"),
  phrase(/封面・盘面/uy, "coverAndDisc"),
  phrase(/封面/uy, "cover"),
  phrase(/粉红单色印刷盘面/uy, "pinkPrintedDisc"),
  phrase(/盘面/uy, "disc"),
  phrase(/外装/uy, "outerPackage"),
  phrase(/内容/uy, "contents"),
  phrase(/展开图/uy, "spread"),
  phrase(/曲目/uy, "tracklist"),
  phrase(/样图/uy, "sample"),
  phrase(/图案/uy, "design"),
  // Bonuses.
  phrase(/先着购入特典一览/uy, "earlyPurchaseBonusList"),
  phrase(/店铺特典一览/uy, "storeBonusList"),
  phrase(/店铺特典/uy, "storeBonus"),
  phrase(/特典一览/uy, "bonusList"),
  phrase(/特典情报/uy, "bonusInfo"),
  phrase(/封入特典/uy, "packedBonus"),
  phrase(/限定特典/uy, "exclusiveBonus"),
  phrase(/メーカー特典/uy, "makerBonus"),
  phrase(/連動購入特典/uy, "linkedPurchaseBonus"),
  phrase(/连动特典/uy, "linkedBonus"),
  phrase(/电子特典/uy, "digitalBonus"),
  phrase(/特典/uy, "bonus"),
  phrase(/同梱/uy, "bundled"),
  phrase(/初回生産分封入/uy, "firstPressPacked"),
  phrase(/原定封入/uy, "plannedPacked"),
  phrase(/带本人直筆サイン/uy, "withAutograph"),
  phrase(/绘本朗读/uy, "pictureBookReading"),
  phrase(/[绘絵]本/uy, "pictureBook"),
  phrase(/原作/uy, "originalStory"),
  phrase(/原曲/uy, "original"),
  phrase(/插画/uy, "illustration"),
  phrase(/手机游戏/uy, "mobileGame"),
  phrase(/用序列码/uy, "serialCode"),
  phrase(/Android 专用/uy, "androidOnly"),
  phrase(/收录曲的新录音/uy, "newRecordingOf"),
  phrase(/电子手写信/uy, "digitalLetter"),
  phrase(/惊喜写真/uy, "surprisePhotos"),
  // Packaging and formats.
  phrase(/セット商品/uy, "set"),
  phrase(/外加/uy, "plus"),
  phrase(/白纸套/uy, "whiteSleeve"),
  phrase(/本身就是包装/uy, "isPackaging"),
  phrase(/约/uy, "approx"),
  phrase(/带支脚/uy, "withStand"),
  phrase(/周边/uy, "goods"),
  phrase(/TV ?アニメ/uy, "tvAnime"),
  // Editions, formats and digital items.
  phrase(/配信版/uy, "digitalEdition"),
  phrase(/先行配信/uy, "earlyDigital"),
  phrase(/配信/uy, "digital"),
  phrase(/数字专辑/uy, "digitalAlbum"),
  phrase(/单曲/uy, "single"),
  phrase(/附赠/uy, "giveaway"),
  phrase(/TV ?(?:サイズ|size)/iuy, "tvSize"),
  phrase(/instrumental/uy, "instrumental"),
  phrase(/オリジナル盤/uy, "originalSide"),
  phrase(/カバー盤/uy, "coverSide"),
  phrase(/限定/uy, "exclusive"),
  // Stores.
  phrase(/全国対象店舗?/uy, "participatingStoresNationwide"),
  phrase(/対象店舗/uy, "participatingStores"),
  phrase(/対象販売店/uy, "participatingRetailers"),
  phrase(/対象法人/uy, "participatingCompanies"),
  phrase(/応援店/uy, "supportingStores"),
  phrase(/上记以外的店铺/uy, "otherStores"),
  phrase(/与/uy, "and"),
  // Labels, catalogs and prices.
  phrase(/同人サークル/uy, "doujinCircle"),
  phrase(/self-released/uy, "selfReleased"),
  phrase(/cancelled|发售中止/uy, "cancelled"),
  phrase(/\+税/uy, "plusTax"),
]

/** Track notes and credits. */
const trackPhrases: Entry[] = [
  ...names,
  phrase(/([^\s(（/]+?)とのコラボ/uy, "collabWith", { value: 1 }),
  phrase(/([^\s(（/]+?)が後にセルフカバー/uy, "laterSelfCover", { value: 1 }),
  phrase(/([^\s(（/]+?)が(\d{4})年にセルフカバー/uy, "selfCoverIn", {
    value: 1,
    count: 2,
  }),
  phrase(/(\d{4})年(\d{1,2})月度/uy, "monthOf", { value: 1, count: 2 }),
  // Credits.
  phrase(/作詞・作曲・編曲/uy, "lyricsMusicArrangement"),
  phrase(/作詞・作曲/uy, "lyricsMusic"),
  phrase(/作曲・編曲/uy, "musicArrangement"),
  phrase(/作詞/uy, "lyrics"),
  phrase(/作曲/uy, "music"),
  phrase(/編曲/uy, "arrangement"),
  phrase(/原作・朗読/uy, "storyNarration"),
  phrase(/原曲/uy, "original"),
  phrase(/イラスト/uy, "illustration"),
  phrase(/古筝/uy, "guzheng"),
  // Notes.
  phrase(/TV ?アニメ/uy, "tvAnime"),
  phrase(/ゲーム/uy, "game"),
  phrase(/主題歌/uy, "themeSong"),
  phrase(/アコースティック/uy, "acoustic"),
  phrase(/同人音源リマスター/uy, "doujinRemaster"),
  phrase(/ボーナス・?トラック|bonus track/uy, "bonusTrack"),
  phrase(/カバー/uy, "coverSong"),
  phrase(/仅 CD/uy, "cdOnly"),
  phrase(/未発表新曲/uy, "unreleasedNewSong"),
  phrase(/未発表曲/uy, "unreleasedSong"),
  phrase(/新曲/uy, "newSong"),
  phrase(/ボカロ版初公開/uy, "firstVocaloidRelease"),
  phrase(/歌唱/uy, "vocals"),
  phrase(/独唱/uy, "solo"),
  phrase(/日本語/uy, "japanese"),
  phrase(/表題曲/uy, "titleTrack"),
  phrase(/オリジナル/uy, "originalSong"),
  phrase(/楽曲/uy, "song"),
  phrase(/(?<![A-Za-z])live(?![A-Za-z])/uy, "live"),
]

/** Characters only Simplified Chinese writes, which no name here holds. */
const chineseOnly =
  /[专丝乐买产仅动勋单发图带开张录惊戏报时样电盘码种签红约纸组绘览记语读购赠辑边连钥铺锁页们这为给听说该应]/u

/** A quoted title, which is a name and is kept as written. */
const quoted = /「[^」]*」|『[^』]*』|《[^》]*》/uy

function spelled(text: string) {
  return spellings.reduce(
    (result, [pattern, replacement]) => result.replace(pattern, replacement),
    text,
  )
}

function checkWording(text: string, source: string) {
  if (chineseOnly.test(text))
    throw new Error(`Collection wording without a phrase: ${source}`)
}

function toText(source: string, entries: Entry[], check: boolean): WorksText {
  const text = spelled(source)
  const parts: (string | WorksPhrase)[] = []
  let kept = ""
  const keep = (value: string, isName = false) => {
    if (check && !isName) checkWording(value, source)
    kept += value
  }
  let at = 0
  scan: while (at < text.length) {
    quoted.lastIndex = at
    const title = quoted.exec(text)
    if (title) {
      keep(title[0], true)
      at += title[0].length
      continue
    }
    for (const entry of entries) {
      entry.pattern.lastIndex = at
      const match = entry.pattern.exec(text)
      if (!match || !match[0]) continue
      if (!entry.phrase) keep(match[0], true)
      else {
        if (kept) parts.push(kept)
        kept = ""
        const value = entry.value ? match[entry.value].trim() : undefined
        if (value && check) checkWording(value.replace(quoted, ""), source)
        parts.push({
          phrase: entry.phrase,
          ...(value ? { value } : {}),
          ...(entry.count ? { count: Number(match[entry.count]) } : {}),
        })
      }
      at += match[0].length
      continue scan
    }
    keep(text[at])
    at += 1
  }
  if (kept) parts.push(kept)
  return parts.every((part) => typeof part === "string")
    ? parts.join("")
    : parts
}

/**
 * A release or edition field as published: names as written, the generic
 * words as phrases. Chinese wording without a phrase stops packaging, except
 * in a field that is a name, such as a label.
 */
export function releaseText(source: string, { check = true } = {}) {
  return toText(source, releasePhrases, check)
}

/**
 * A track note or credit as published. Credits name Chinese artists, so only
 * notes are checked for Chinese wording.
 */
export function trackText(source: string, { check = true } = {}) {
  return toText(source, trackPhrases, check)
}

/** Whether a text is the collection's Chinese commentary rather than a name. */
export function isChineseWording(text: string) {
  return chineseOnly.test(text)
}
