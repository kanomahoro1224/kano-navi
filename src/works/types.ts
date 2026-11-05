/*
 * The /works page data, written by scripts/package-works.ts from the
 * maintainer's kano_official reference collection. Titles, credits, edition
 * names, bonuses and image captions keep the collection's wording, in Japanese
 * or Simplified Chinese; the collection's own checking notes are not
 * published. Interface labels come from codes.
 */

/** A cover image in data/works/, named by its bytes' SHA-256. */
export type WorksCover = {
  /** Media name: the first 12 hex digits of its SHA-256 and the extension. */
  media: string
  width: number
  height: number
  /** What the image shows, in the collection's Simplified Chinese. */
  caption?: string
  /** The page the file was taken from, when recorded and public. */
  source?: string
  /** The site the file was taken from, when recorded without a page. */
  sourceLabel?: string
  /** Who scanned, photographed or gave a file from their own copy. */
  credit?: WorksCredit
  /**
   * The release's own artwork in a file from a store, database or collector,
   * not the publisher's site.
   */
  thirdParty: boolean
}

/**
 * A file from a person's own copy: a scan, a photo, or a file a fan gave
 * without a public page. A named person may link to their profile.
 */
export type WorksCredit = {
  role: "scan" | "photo" | "fan"
  name?: string
  url?: string
}

/** A packaged image with no caption or provenance of its own. */
export type WorksImage = Pick<WorksCover, "media" | "width" | "height">

export type WorksTrack = {
  disc: number
  /** The disc's name, such as "DVD" or "Side-A", when the collection gives one. */
  discLabel?: string
  no: number
  title: string
  credit: string
  /** A tie-in or version note, such as the anime a song opens. */
  note: string
  /** Where the same recording was posted, when it was. */
  listen?: UploadVideos
}

/** Kano's own releases: a solo work or a work of a unit she belongs to. */
export type PersonalFormat = "album" | "single" | "ep" | "doujin"

/** A bonus one or more stores gave with a purchase. */
export type StoreBonus = {
  store: string
  item: string
  images: WorksCover[]
}

/** One version of a release on sale, such as a limited or standard edition. */
export type PersonalEdition = {
  name: string
  /** Japan date when this edition came out after the release date. */
  date: string | null
  /** Announced and then withdrawn before sale. */
  cancelled: boolean
  catalog: string
  jan: string
  price: string
  /** The physical or digital format, such as "CD+DVD". */
  format: string
  packaging: string
  covers: WorksCover[]
  /** What comes in the package beyond the discs. */
  bonuses: string[]
  bonusImages: WorksCover[]
  /** Store bonuses given with this edition. */
  storeBonuses: StoreBonus[]
  /** The edition's full tracklist, shared discs included. */
  tracks: WorksTrack[]
  /** Photos of the edition's own discs, by disc number. */
  discs: DiscImage[]
  /** No disc was pressed (a download, or withdrawn): its cover is printed on one. */
  printedDisc: boolean
}

/** A photo or scan of one disc of a release, by its disc number. */
export type DiscImage = {
  disc: number
  image: WorksCover
}

export type PersonalRelease = {
  /** Japan date of release and a hash of the title. */
  id: string
  /** Japan date (YYYY-MM-DD) of the first release. */
  date: string
  title: string
  format: PersonalFormat
  /** Released for download or streaming, with a CD or not. */
  digital: boolean
  /** Every edition was withdrawn before sale. */
  cancelled: boolean
  /** The unit's name for a unit work. */
  unit: string | null
  label: string
  /** The release's catalog numbers; each edition carries its own. */
  catalog: string
  covers: WorksCover[]
  /** A photo or scan of the disc itself, when the collection has one. */
  disc: WorksCover | null
  /** The record shows the cover printed on a disc instead of a drawn one. */
  printedDisc: boolean
  /** A figure cut out of the cover on a transparent ground, when there is one. */
  cutout: WorksImage | null
  /** The tracklist every edition shares. */
  tracks: WorksTrack[]
  /** Photos of the discs every edition shares, by disc number. */
  discs: DiscImage[]
  /** Empty for a release sold in one plain version. */
  editions: PersonalEdition[]
  /** Store bonuses for any edition. */
  storeBonuses: StoreBonus[]
}

/** What Kano did on a work released under someone else's name. */
export type ParticipationRole =
  "vocal" | "lyrics" | "composition" | "voice" | "guest"

export type ParticipationForm =
  "digital" | "bonus" | "drama" | "cdr" | "boxSet" | "notForSale" | "promo"

/** A music database's credit for Kano on a track, beyond singing. */
export type KanoCredit =
  "lyrics" | "composition" | "arrangement" | "animation" | "other"

export type ParticipationTrack = Omit<WorksTrack, "note"> & {
  /** Kano sang, wrote or otherwise took part in this track. */
  kano: boolean
  kanoCredits: KanoCredit[]
}

export type ParticipationRelease = {
  id: string
  date: string
  title: string
  roles: ParticipationRole[]
  forms: ParticipationForm[]
  /** The group she is a member of, when it released the work. */
  unit: string | null
  label: string
  event: string
  catalog: string
  /** The page the record follows. */
  source: string
  cover: WorksCover | null
  tracks: ParticipationTrack[]
}

export type UploadKind =
  | "cover"
  | "original"
  | "mv"
  | "project"
  | "compilation"
  | "game"
  | "composition"

/** The videos of one upload, by platform. */
export type UploadVideos = Pick<
  Upload,
  "niconico" | "youtube" | "youtubeMembers" | "bilibili"
>

/** One recording uploaded to one or more video platforms. */
export type Upload = {
  id: string
  date: string
  song: string
  originalArtist: string
  kind: UploadKind
  with: string
  niconico: string | null
  youtube: string[]
  /** YouTube videos only channel members can watch. */
  youtubeMembers: string[]
  bilibili: string | null
  /** The bilibili title when it differs from the song. */
  bilibiliTitle: string
  /** The albums that include this recording, as the collection names them. */
  albums: string[]
  /** The songs a compilation holds. */
  tracks: string[]
  /** A video thumbnail the maintainer saved, when there is one. */
  cover: WorksCover | null
}

export type WorksMedia = {
  name: string
  mimeType: string
  byteSize: number
  sha256: string
  width: number
  height: number
}
