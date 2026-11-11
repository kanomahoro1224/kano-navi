import type { ReactNode } from "react"
import { useMemo, useState } from "react"
import { ImageOff, Play } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { mediaSrcSet, mediaVariantUrl } from "@/lib/media"
import { ExternalLink, YouTubeLogo } from "@/site/links"
import { MediaDialog, stepIndex } from "@/site/media-dialog"
import {
  articleHref,
  isEmoji,
  meaningfulAlt,
  media,
  mediaUrl,
  postDate,
  postImages,
} from "./content"
import type {
  LineblogBlock,
  LineblogImage,
  LineblogInline,
  LineblogPost,
} from "./types"
import {
  type Translation,
  hasTranslation,
  useOwnTranslation,
  TranslationMark,
} from "./translation"
import { articleLink } from "./navigation"

/** How wide each kind of picture shows (lineblog.css); viewers keep originals. */
export const SIZES = {
  article: "(max-width: 660px) calc(100vw - 32px), 600px",
  card: "(max-width: 560px) 76px, 112px",
  video: "(max-width: 720px) calc(100vw - 32px), 660px",
  profile: "(max-width: 880px) 160px, 240px",
  recent: "60px",
}

/** A poster takes one address; this width suits the article column. */
const POSTER_WIDTH = 960

// ---------------------------------------------------------------------------
// Post content
// ---------------------------------------------------------------------------

interface ContentContext {
  openImage: (image: LineblogImage) => void
}

function ContentLink({
  href,
  children,
}: {
  href: string
  children: ReactNode
}) {
  const local = articleHref(href)
  if (local) return <a href={local}>{children}</a>
  return <ExternalLink href={href}>{children}</ExternalLink>
}

function MissingImage({
  image,
  message,
}: {
  image: LineblogImage
  message: string
}) {
  const { t } = useAppSettings()
  const source = image.remote ?? image.source
  return (
    <span className="lb-missing">
      <ImageOff aria-hidden="true" />
      <span>{meaningfulAlt(image.alt) ?? message}</span>
      {meaningfulAlt(image.alt) ? <small>{message}</small> : null}
      {source ? (
        <ExternalLink href={source} className="lb-missing-link">
          {t("lineblog.originalImage")}
        </ExternalLink>
      ) : null}
    </span>
  )
}

function ArchiveImage({
  image,
  context,
  sizes = SIZES.article,
}: {
  image: LineblogImage
  context: ContentContext
  sizes?: string
}) {
  const { t } = useAppSettings()
  const [failed, setFailed] = useState(false)
  const item = image.media ? media.get(image.media) : undefined
  if (!item) {
    return (
      <MissingImage image={image} message={t("lineblog.imageNotArchived")} />
    )
  }
  if (failed) {
    return (
      <MissingImage image={image} message={t("lineblog.imageUnavailable")} />
    )
  }
  const alt = meaningfulAlt(image.alt)
  if (isEmoji(item.name)) {
    return (
      <img
        className="lb-emoji"
        src={mediaUrl(item.name)}
        alt={alt ?? ""}
        width={item.width}
        height={item.height}
        loading="lazy"
        decoding="async"
        onError={() => setFailed(true)}
      />
    )
  }
  return (
    <button
      type="button"
      className="lb-picture media-opener"
      onClick={() => context.openImage(image)}
      aria-label={alt ?? t("lineblog.openImage")}
    >
      <img
        src={mediaUrl(item.name)}
        srcSet={mediaSrcSet(mediaUrl(item.name), item.width)}
        sizes={sizes}
        alt={alt ?? t("lineblog.photo")}
        width={item.width}
        height={item.height}
        loading="lazy"
        decoding="async"
        onError={() => setFailed(true)}
      />
    </button>
  )
}

function Inlines({
  content,
  context,
}: {
  content: LineblogInline[]
  context: ContentContext
}) {
  return content.map((inline, index) => {
    if (inline.type === "text") return inline.text
    if (inline.type === "link") {
      return (
        <ContentLink key={index} href={inline.href}>
          <Inlines content={inline.content} context={context} />
        </ContentLink>
      )
    }
    return <ArchiveImage key={index} image={inline} context={context} />
  })
}

/** A paragraph of only pictures is a centred figure row, as on the blog. */
function isFigureRow(content: LineblogInline[]) {
  return content.every(
    (inline) =>
      (inline.type === "image" && !isEmoji(inline.media)) ||
      (inline.type === "text" && inline.text.trim() === ""),
  )
}

/** A small note under pictures added after the recovery archive. */
function RecoveryNote({ content }: { content: LineblogInline[] }) {
  const { t } = useAppSettings()
  const kind = content.find(
    (inline): inline is LineblogImage =>
      inline.type === "image" && inline.recovery !== undefined,
  )?.recovery
  if (!kind) return null
  return (
    <p className="lb-recovery-note">
      <small>
        {t(kind === "stand-in" ? "lineblog.standIn" : "lineblog.tweetPictures")}
      </small>
    </p>
  )
}

function YoutubeBlock({
  block,
}: {
  block: Extract<LineblogBlock, { type: "youtube" }>
}) {
  const { t } = useAppSettings()
  const [failed, setFailed] = useState(false)
  const video = block.video && media.get(block.video)
  const cover = block.cover && media.get(block.cover)
  return (
    <figure className="lb-video">
      {video && !failed ? (
        <video
          controls
          preload={cover ? "none" : "metadata"}
          poster={
            cover
              ? mediaVariantUrl(mediaUrl(cover.name), POSTER_WIDTH)
              : undefined
          }
          src={mediaUrl(video.name)}
          onError={() => setFailed(true)}
        />
      ) : (
        <div className="lb-video-missing">
          {cover && !failed ? (
            <img
              src={mediaUrl(cover.name)}
              srcSet={mediaSrcSet(mediaUrl(cover.name), cover.width)}
              sizes={SIZES.video}
              alt=""
              loading="lazy"
            />
          ) : null}
          <p>
            <Play aria-hidden="true" />
            {t("lineblog.videoUnavailable")}
          </p>
        </div>
      )}
      <figcaption>
        <ExternalLink href={block.watch} className="lb-youtube-link">
          <YouTubeLogo aria-hidden="true" />
          {t("lineblog.watchOnYouTube")}
        </ExternalLink>
        {block.archive ? (
          <ExternalLink href={block.archive}>
            {t("lineblog.capture")}
          </ExternalLink>
        ) : null}
      </figcaption>
    </figure>
  )
}

function CardBlock({
  block,
  context,
}: {
  block: Extract<LineblogBlock, { type: "card" }>
  context: ContentContext
}) {
  const { t } = useAppSettings()
  const host = (() => {
    try {
      return new URL(block.url).hostname.replace(/^www\./u, "")
    } catch {
      return block.url
    }
  })()
  return (
    <aside className="lb-card">
      {block.image?.media && media.has(block.image.media) ? (
        <div className="lb-card-image">
          <ArchiveImage
            image={block.image}
            context={context}
            sizes={SIZES.card}
          />
        </div>
      ) : null}
      <div className="lb-card-copy">
        <ContentLink href={block.url}>
          <strong>{block.title}</strong>
        </ContentLink>
        {block.description ? <p>{block.description}</p> : null}
        <small>
          {host}
          {block.archive ? (
            <>
              {" · "}
              <ExternalLink href={block.archive}>
                {t("lineblog.capture")}
              </ExternalLink>
            </>
          ) : null}
        </small>
      </div>
    </aside>
  )
}

function Block({
  block,
  context,
  translation,
}: {
  block: LineblogBlock
  context: ContentContext
  translation: Translation | null
}) {
  switch (block.type) {
    case "p": {
      const translated = translation && block[translation.blocks]
      return (
        <>
          {isFigureRow(block.content) ? (
            <div className="lb-figures">
              <Inlines content={block.content} context={context} />
            </div>
          ) : (
            <p>
              <Inlines content={block.content} context={context} />
            </p>
          )}
          <RecoveryNote content={block.content} />
          {translated
            ? translated.map((paragraph, index) => (
                <p
                  key={index}
                  className="lb-translation"
                  lang={translation.lang}
                >
                  <Inlines content={paragraph} context={context} />
                </p>
              ))
            : null}
        </>
      )
    }
    case "quote":
      return (
        <blockquote>
          <Inlines content={block.content} context={context} />
        </blockquote>
      )
    case "hr":
      return <hr />
    case "video": {
      const video = media.get(block.video)
      return video ? (
        <figure className="lb-video">
          <video controls preload="metadata" src={mediaUrl(video.name)} />
        </figure>
      ) : null
    }
    case "youtube":
      return <YoutubeBlock block={block} />
    case "card":
      return <CardBlock block={block} context={context} />
  }
}

function sourceHost(address: string) {
  try {
    return new URL(address).hostname
  } catch {
    return address
  }
}

/**
 * The site a picture's archived bytes came from, as the viewer's small print
 * rather than a link: most of those addresses no longer open.
 */
function SourceNote({ source }: { source: string }) {
  const { t } = useAppSettings()
  return <>{t("lineblog.recoveredFrom", { host: sourceHost(source) })}</>
}

/** Whether a picture has a caption or a link for the viewer's panel. */
function hasImageDetails(image: LineblogImage) {
  return Boolean(meaningfulAlt(image.alt) || image.href)
}

/** The viewer's panel for a picture with a caption or a link. */
function ImageDetails({ image }: { image: LineblogImage }) {
  const { t } = useAppSettings()
  const alt = meaningfulAlt(image.alt)
  return (
    <div className="lb-image-details">
      {alt ? <p>{alt}</p> : null}
      {image.href ? (
        <ExternalLink href={image.href}>
          {t("lineblog.linkInPost")}
        </ExternalLink>
      ) : null}
    </div>
  )
}

export function Article({
  post,
  single,
  offered,
  translationOn,
  translationRound,
}: {
  post: LineblogPost
  single: boolean
  offered: Translation | null
  translationOn: boolean
  /** Counts uses of the menu bar's switch. */
  translationRound: number
}) {
  const { t } = useAppSettings()
  const images = useMemo(() => postImages(post.blocks), [post])
  const translated = offered && hasTranslation(post, offered) ? offered : null
  const [on, toggle] = useOwnTranslation(translationOn, translationRound)
  const translation = on ? translated : null
  const translatedTitle = translation && post[translation.title]
  const [open, setOpen] = useState(-1)
  const context = useMemo<ContentContext>(
    () => ({ openImage: (image) => setOpen(images.indexOf(image)) }),
    [images],
  )
  const Title = single ? "h1" : "h2"
  const titleId = `lb-title-${post.id}`
  return (
    <article className="lb-article" aria-labelledby={titleId}>
      <header className="lb-article-header">
        <Title className="lb-article-title" id={titleId} lang="ja">
          {single ? post.title : <a href={articleLink(post)}>{post.title}</a>}
        </Title>
        {translatedTitle && translatedTitle !== post.title ? (
          <p
            className="lb-translation lb-title-translation"
            lang={translation.lang}
          >
            {translatedTitle}
          </p>
        ) : null}
        <p className="lb-article-date">
          <time dateTime={post.published}>{postDate(post.published)}</time>
          {translated ? <TranslationMark on={on} onToggle={toggle} /> : null}
        </p>
      </header>
      <div className="lb-article-body" lang="ja">
        {post.blocks.map((block, index) => (
          <Block
            key={index}
            block={block}
            context={context}
            translation={translation}
          />
        ))}
      </div>
      <footer className="lb-article-footer">
        <span>{t("lineblog.provenance")}</span>
        <ExternalLink href={post.url}>
          {t("lineblog.originalAddress")}
        </ExternalLink>
        <ExternalLink href={post.archive}>{t("lineblog.capture")}</ExternalLink>
      </footer>
      <MediaDialog
        items={open >= 0 ? images : null}
        index={Math.max(open, 0)}
        label={t("lineblog.imageViewer")}
        onClose={() => setOpen(-1)}
        onStep={(offset) =>
          setOpen((index) => stepIndex(index, offset, images.length))
        }
        renderImage={(image) => {
          const item = image.media ? media.get(image.media) : undefined
          return item ? (
            <img
              src={mediaUrl(item.name)}
              alt={meaningfulAlt(image.alt) ?? t("lineblog.photo")}
              width={item.width}
              height={item.height}
            />
          ) : null
        }}
        renderDetails={(image) =>
          hasImageDetails(image) ? <ImageDetails image={image} /> : null
        }
        renderNote={(image) =>
          image.source ? <SourceNote source={image.source} /> : null
        }
      />
    </article>
  )
}
