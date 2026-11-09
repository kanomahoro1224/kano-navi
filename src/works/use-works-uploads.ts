import { useEffect, useState } from "react"

import type { Translations } from "../types.ts"

/*
 * What this server knows about the packaged uploads: each one's cover image
 * and stored videos, joined by video ID (GET /api/works/uploads). The list
 * itself always comes from uploads.json; this only adds to it, along with
 * the uploads posted after the list's date.
 */

export type WorksUploadVideo = {
  source: "youtube" | "bilibili"
  id: string
  /** The platform's current title. */
  title: string
  url: string
  publishedAt: string | null
  scheduledAt: string | null
  durationSeconds: number | null
  /** public, unlisted, members, private or removed; null while unknown. */
  availability: string | null
}

export type WorksUploadCover = {
  url: string
  width: number | null
  height: number | null
}

export type WorksUploadState = {
  cover: WorksUploadCover | null
  videos: WorksUploadVideo[]
}

/**
 * An upload posted after the collection date: found by its song tag and
 * form (auto) or added by the site's operator.
 */
export type WorksRecentUpload = {
  id: string
  source: "youtube" | "bilibili"
  /** The shown title, in its own language. */
  title: string
  url: string
  /** When it went up; a premiere's scheduled time. */
  publishedAt: string | null
  cover: WorksUploadCover | null
  origin: "auto" | "operator"
  translations?: Translations
  sourceLanguage?: string | null
  /** Its copies on other sites, such as a Bilibili upload of the video. */
  alternates: { source: string; url: string }[]
}

export type WorksUploadsOverlay = {
  updatedAt: string
  revision: number
  uploads: Record<string, WorksUploadState>
  /** Newest first; absent from an older server. */
  recent?: WorksRecentUpload[]
}

export type WorksUploadsStatus =
  | { state: "loading" }
  | { state: "ready"; overlay: WorksUploadsOverlay }
  | { state: "failed" }

const ENDPOINT = "/api/works/uploads"

let pending: Promise<WorksUploadsOverlay> | null = null

function isOverlay(value: unknown): value is WorksUploadsOverlay {
  if (!value || typeof value !== "object") return false
  const uploads = (value as { uploads?: unknown }).uploads
  return Boolean(uploads) && typeof uploads === "object"
}

function loadOverlay() {
  pending ??= fetch(ENDPOINT, { headers: { Accept: "application/json" } })
    .then(async (response) => {
      if (!response.ok) throw new Error(`works_uploads_${response.status}`)
      const body: unknown = await response.json()
      if (!isOverlay(body)) throw new Error("works_uploads_invalid")
      return body
    })
    .catch((error: unknown) => {
      // A later visit to the section asks again.
      pending = null
      throw error
    })
  return pending
}

/**
 * The overlay, read once per page load. A failed read leaves the packaged
 * list as it is: printed sleeves and the collection's links.
 */
export function useWorksUploads(): WorksUploadsStatus {
  const [status, setStatus] = useState<WorksUploadsStatus>({
    state: "loading",
  })
  useEffect(() => {
    let current = true
    loadOverlay().then(
      (overlay) => {
        if (current) setStatus({ state: "ready", overlay })
      },
      () => {
        if (current) setStatus({ state: "failed" })
      },
    )
    return () => {
      current = false
    }
  }, [])
  return status
}

const recentSources = new Set(["youtube", "bilibili"])

/** The recent uploads a page can show: well-formed ones, newest first. */
export function recentUploads(overlay: WorksUploadsOverlay | undefined) {
  return (overlay?.recent ?? []).filter(
    (item) =>
      item &&
      typeof item.id === "string" &&
      typeof item.title === "string" &&
      typeof item.url === "string" &&
      recentSources.has(item.source),
  )
}

/** The sites a recent upload is on: its own, then its copies, once each. */
export function recentLinks(item: WorksRecentUpload) {
  const links: { platform: "youtube" | "bilibili"; href: string }[] = [
    { platform: item.source, href: item.url },
  ]
  for (const alternate of item.alternates ?? []) {
    if (
      recentSources.has(alternate.source) &&
      !links.some((link) => link.href === alternate.url)
    )
      links.push({
        platform: alternate.source as "youtube" | "bilibili",
        href: alternate.url,
      })
  }
  return links
}

const japanDays = new Intl.DateTimeFormat("en", {
  timeZone: "Asia/Tokyo",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
})

/** An ISO instant's Japan date as YYYY-MM-DD, or "" when it has none. */
export function japanDay(value: string | null | undefined) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  const parts = japanDays.formatToParts(date)
  const part = (type: string) => parts.find((item) => item.type === type)?.value
  return `${part("year")}-${part("month")}-${part("day")}`
}

/** Whether nobody can watch a video any more. */
export function videoGone(video: WorksUploadVideo) {
  return video.availability === "removed" || video.availability === "private"
}

/** m:ss or h:mm:ss of a length in seconds. */
export function durationLabel(seconds: number) {
  const whole = Math.max(0, Math.round(seconds))
  const hours = Math.floor(whole / 3600)
  const minutes = Math.floor((whole % 3600) / 60)
  const rest = String(whole % 60).padStart(2, "0")
  return hours
    ? `${hours}:${String(minutes).padStart(2, "0")}:${rest}`
    : `${minutes}:${rest}`
}
