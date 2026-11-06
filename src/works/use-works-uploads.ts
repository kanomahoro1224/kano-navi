import { useEffect, useState } from "react"

/*
 * What this server knows about the packaged uploads: each one's cover image
 * and stored videos, joined by video ID (GET /api/works/uploads). The list
 * itself always comes from uploads.json; this only adds to it.
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

export type WorksUploadsOverlay = {
  updatedAt: string
  revision: number
  uploads: Record<string, WorksUploadState>
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

/** The Japan month (YYYY-MM) a stored video's time falls in. */
export function japanMonth(instant: string) {
  const time = new Date(instant)
  if (Number.isNaN(time.getTime())) return null
  return new Date(time.getTime() + 9 * 3600 * 1000).toISOString().slice(0, 7)
}

/**
 * The board's video archive: at a month, or of one category from the
 * current month.
 */
export function videoArchiveHref({
  month,
  category,
}: {
  month?: string
  category?: "song"
}) {
  const query = new URLSearchParams({ archive: "videos" })
  if (month) query.set("month", month)
  if (category) query.set("category", category)
  return `/?${query.toString()}`
}
