import { useCallback, useEffect, useState } from "react"

import type { PostKind } from "../types.ts"

/** What the post archive shows: a `YYYY-MM` month, account, and optional kind. */
export interface PostsView {
  /** Null shows the newest month holding posts. */
  month: string | null
  /** An X handle or Bilibili mid; null shows every account. */
  account: string | null
  /** Optional category filter ("all" | "post" | "reply" | "repost"). */
  kind?: PostKind | null
}

const monthPattern = /^\d{4}-(0[1-9]|1[0-2])$/u
const accountPattern = /^[A-Za-z0-9_]{1,30}$/u
const kindValues = new Set<PostKind>(["all", "post", "reply", "repost"])

/** The view a query names; anything malformed falls back to its default. */
export function postsView(search: string): PostsView {
  const query = new URLSearchParams(search)
  const month = query.get("month")
  const account = query.get("account")
  const kind = query.get("kind") as PostKind | null
  const view: PostsView = {
    month: month && monthPattern.test(month) ? month : null,
    account: account && accountPattern.test(account) ? account : null,
  }
  if (kind && kindValues.has(kind)) view.kind = kind
  return view
}

/** The address of a view of the post archive. */
export function postsHref(view: Partial<PostsView> = {}) {
  const query = new URLSearchParams()
  if (view.month) query.set("month", view.month)
  if (view.account) query.set("account", view.account)
  if (view.kind && view.kind !== "all" && kindValues.has(view.kind))
    query.set("kind", view.kind)
  const search = query.toString()
  return search ? `/posts?${search}` : "/posts"
}

function newEntry() {
  return `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`
}

function isPostsPath(pathname: string) {
  return pathname.replace(/\/+$/u, "") === "/posts"
}

/**
 * The view follows the address: choosing a month or an account pushes an
 * entry, so back and forward step through what was browsed.
 */
export function usePostsLocation() {
  const [view, setView] = useState(() => postsView(window.location.search))

  useEffect(() => {
    const onPopState = () => {
      if (isPostsPath(window.location.pathname))
        setView(postsView(window.location.search))
    }
    window.addEventListener("popstate", onPopState)
    return () => window.removeEventListener("popstate", onPopState)
  }, [])

  const show = useCallback((next: PostsView) => {
    const href = postsHref(next)
    if (`${window.location.pathname}${window.location.search}` !== href) {
      // The site's navigation restores this entry's scroll on the way back.
      window.history.replaceState(
        { ...window.history.state, kanoScroll: window.scrollY },
        "",
      )
      window.history.pushState({ kanoEntry: newEntry() }, "", href)
    }
    setView(next)
  }, [])

  return [view, show] as const
}
