import type { ArchiveMonth, Locale, Translator } from "@/types"
import type { ReactNode } from "react"

import { BilibiliLogo } from "@/site/links"
import {
  AccountAvatar,
  accountAvatar,
  PostAccountLabel,
  postAccounts,
} from "@/dashboard/components/post-wall"
import { accountSpan } from "./timeline"

function spanText(first: string | null, last: string | null) {
  if (!first || !last) return null
  return first === last ? first : `${first}–${last}`
}

/**
 * The archive's accounts as cards, All first: each with its portrait, the
 * years its posts span and how many there are. Choosing one filters the
 * timeline. The page places them above the timeline or in its rail.
 */
export function AccountBand({
  placement,
  accounts,
  bilibiliAccounts,
  months,
  avatars,
  kanoAvatar,
  account,
  setAccount,
  locale,
  t,
}: {
  placement: "top" | "rail"
  accounts: string[]
  bilibiliAccounts: string[]
  months: readonly ArchiveMonth[]
  avatars: Record<string, string | null>
  /** The site portrait, shown for All. */
  kanoAvatar: string
  account: string | null
  setAccount: (account: string | null) => void
  locale: Locale
  t: Translator
}) {
  const number = new Intl.NumberFormat(locale)
  const all = accountSpan(months, null)
  const cards = [
    {
      key: "",
      handle: null as string | null,
      name: t("common.all") as ReactNode,
      portrait: (
        <AccountAvatar url={kanoAvatar} handle="kano" className="is-avatar" />
      ),
      span: all,
    },
    ...postAccounts(accounts, bilibiliAccounts).map((item) => {
      const url = accountAvatar(
        item.bilibili ? "bilibili" : "x",
        item.handle,
        avatars,
      )
      return {
        key: item.handle,
        handle: item.handle,
        name: <PostAccountLabel account={item} />,
        portrait:
          item.bilibili && !url ? (
            <span className="posts-account-mark" aria-hidden="true">
              <BilibiliLogo />
            </span>
          ) : (
            <AccountAvatar
              key={url || ""}
              url={url || null}
              handle={item.handle}
              className="is-avatar"
            />
          ),
        span: accountSpan(months, item.handle.toLowerCase()),
      }
    }),
  ]
  return (
    <div
      className={`posts-accounts is-${placement}`}
      role="group"
      aria-label={t("feed.filter")}
    >
      {cards.map((card) => {
        const years = spanText(card.span.first, card.span.last)
        return (
          <button
            type="button"
            key={card.key}
            className="posts-account"
            aria-pressed={
              card.handle
                ? account?.toLowerCase() === card.handle.toLowerCase()
                : !account
            }
            onClick={() => setAccount(card.handle)}
          >
            {card.portrait}
            <span className="posts-account-text">
              <span className="posts-account-name">{card.name}</span>
              <span className="posts-account-meta">
                {years ? <span>{years}</span> : null}
                <span>{number.format(card.span.total)}</span>
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}
