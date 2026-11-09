import { useAppSettings } from "@/app-settings"
import { cn } from "@/lib/utils"
import { postDate } from "./content"
import type { LineblogComment } from "./types"

/** The old comment form's face ratings, from 1 (worst) to 5 (best). */
const RATING_FACES = ["😣", "😟", "😐", "🙂", "😊"]

function CommentRating({
  rating,
}: {
  rating: NonNullable<LineblogComment["rating"]>
}) {
  const { t } = useAppSettings()
  return (
    <span
      className={cn("lb-comment-rating", `is-${rating.icon}`)}
      role="img"
      aria-label={t("lineblog.commentRating", { value: rating.value })}
    >
      {rating.icon === "star"
        ? "★".repeat(rating.value) + "☆".repeat(5 - rating.value)
        : RATING_FACES[rating.value - 1]}
    </span>
  )
}

/**
 * The comment list an archived copy of the article page kept, newest first,
 * in the commenters' own words; their blog links are not kept.
 */
export function Comments({ comments }: { comments: LineblogComment[] }) {
  const { t } = useAppSettings()
  if (!comments.length) return null
  return (
    <section className="lb-comments" aria-labelledby="lb-comments-title">
      <h2 className="lb-comments-title" id="lb-comments-title">
        {t("lineblog.comments")}
      </h2>
      <p className="lb-comments-note">{t("lineblog.commentsNote")}</p>
      <ol className="lb-comment-list" lang="ja">
        {comments.map((comment) => (
          <li
            key={comment.number}
            className={cn("lb-comment", comment.rating && "has-rating")}
          >
            <p className="lb-comment-head">
              <span className="lb-comment-author">
                {comment.number}.{comment.author}
              </span>
              <time className="lb-comment-date" dateTime={comment.posted}>
                {postDate(comment.posted)}
              </time>
            </p>
            {comment.rating ? <CommentRating rating={comment.rating} /> : null}
            {comment.body ? (
              <p className="lb-comment-body">{comment.body}</p>
            ) : null}
          </li>
        ))}
      </ol>
    </section>
  )
}

/**
 * The template's comment list: a count beside the comment icon, then each
 * comment's number and name, its text and its time, newest first.
 */
export function SmartphoneComments({
  comments,
}: {
  comments: LineblogComment[]
}) {
  const { t } = useAppSettings()
  if (!comments.length) return null
  return (
    <section
      className="lbl-comments"
      id="lb-comments"
      aria-labelledby="lbl-comments-title"
    >
      <h2 className="lbl-comments-title" id="lbl-comments-title">
        <span className="lbl-comment-icon" aria-hidden="true" />
        <span className="sr-only">{t("lineblog.comments")}</span>
        <span className="lbl-comments-count">{comments.length}</span>
      </h2>
      <p className="lbl-comments-note">{t("lineblog.commentsNote")}</p>
      <ol className="lbl-comment-list" lang="ja">
        {comments.map((comment) => (
          <li
            key={comment.number}
            className={cn("lbl-comment", comment.rating && "has-rating")}
          >
            <p className="lbl-comment-author">
              <span className="lbl-comment-id">{comment.number}.</span>
              {comment.author}
            </p>
            {comment.body ? (
              <p className="lbl-comment-body">{comment.body}</p>
            ) : null}
            <time className="lbl-comment-time" dateTime={comment.posted}>
              {postDate(comment.posted)}
            </time>
            {comment.rating ? <CommentRating rating={comment.rating} /> : null}
          </li>
        ))}
      </ol>
    </section>
  )
}
