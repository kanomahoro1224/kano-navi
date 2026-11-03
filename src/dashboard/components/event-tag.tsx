import type { Translator, Video } from "@/types"
import type { LucideIcon } from "lucide-react"
import {
  Ban,
  Clapperboard,
  Gamepad2,
  Lock,
  MessageCircle,
  MicVocal,
  MonitorPlay,
  Music2,
} from "lucide-react"

import { videoTag, videoTagLabel, type EventTag } from "../format"

// Radio already marks an entry on air, so a plain stream takes MonitorPlay.
const tagIcons: Record<EventTag, LucideIcon> = {
  live: MonitorPlay,
  game: Gamepad2,
  talk: MessageCircle,
  song: MicVocal,
  video: Clapperboard,
}

/**
 * A label's icon. A song is a singing stream by default; `work` marks a cover,
 * original or music video, which takes a note instead of a microphone.
 */
export function EventTagIcon({
  tag,
  work = false,
}: {
  tag: EventTag
  work?: boolean
}) {
  const Icon = work && tag === "song" ? Music2 : tagIcons[tag]
  return <Icon aria-hidden="true" />
}

/** A video's tag chip, with the schedule's icon; nothing without a tag. */
export function VideoTagChip({ video, t }: { video: Video; t: Translator }) {
  const tag = videoTag(video)
  if (!tag) return null
  return (
    <span className="tag video-tag">
      <EventTagIcon tag={tag} work={video.kind !== "stream"} />
      {videoTagLabel(video, t)}
    </span>
  )
}

/**
 * Who can watch a video, when not everyone can: members only, private or
 * removed. Nothing for a public or unlisted video, or one not yet checked.
 */
export function VideoAvailabilityChip({
  video,
  t,
}: {
  video: Video
  t: Translator
}) {
  const availability = video.availability
  if (
    availability !== "members" &&
    availability !== "private" &&
    availability !== "removed"
  )
    return null
  const Icon = availability === "removed" ? Ban : Lock
  return (
    <span className="tag video-availability">
      <Icon aria-hidden="true" />
      {t(`videos.availability.${availability}`)}
    </span>
  )
}
