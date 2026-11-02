import type { LucideIcon } from "lucide-react"
import {
  Clapperboard,
  Gamepad2,
  MessageCircle,
  Mic,
  MonitorPlay,
} from "lucide-react"

import type { EventTag } from "../format"

// Radio already marks an entry on air, so a plain stream takes MonitorPlay.
const tagIcons: Record<EventTag, LucideIcon> = {
  live: MonitorPlay,
  game: Gamepad2,
  talk: MessageCircle,
  song: Mic,
  video: Clapperboard,
}

export function EventTagIcon({ tag }: { tag: EventTag }) {
  const Icon = tagIcons[tag]
  return <Icon aria-hidden="true" />
}
