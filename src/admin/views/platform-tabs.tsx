import {
  useId,
  useRef,
  useState,
  type ComponentType,
  type KeyboardEvent,
  type ReactNode,
} from "react"
import { Layers } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { stepIcons } from "@/admin/views/run-parts"
import { cn } from "@/lib/utils"

export type Platform = "all" | "x" | "youtube" | "bilibili"

const platformIcons: Record<
  Platform,
  ComponentType<{ "aria-hidden"?: boolean }>
> = {
  all: Layers,
  x: stepIcons.x,
  youtube: stepIcons.youtube,
  bilibili: stepIcons.bilibili,
}

/**
 * The platform switch at the top of Posts, External posts and Videos: one
 * icon tab per platform above a tab panel. The arrow keys, Home and End move
 * between the tabs and select the one they reach; only the selected tab is in
 * the Tab order.
 */
export function PlatformTabs<T extends Platform>({
  platforms,
  value,
  onSelect,
  children,
}: {
  platforms: readonly T[]
  value: T
  onSelect: (platform: T) => void
  children: ReactNode
}) {
  const { t } = useAppSettings()
  const id = useId()
  const list = useRef<HTMLDivElement>(null)
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const index = platforms.indexOf(value)
    const last = platforms.length - 1
    const next =
      event.key === "ArrowRight"
        ? index === last
          ? 0
          : index + 1
        : event.key === "ArrowLeft"
          ? index <= 0
            ? last
            : index - 1
          : event.key === "Home"
            ? 0
            : event.key === "End"
              ? last
              : -1
    if (next < 0) return
    event.preventDefault()
    list.current
      ?.querySelectorAll<HTMLButtonElement>('[role="tab"]')
      [next]?.focus()
    if (next !== index) onSelect(platforms[next])
  }
  return (
    <>
      <div
        ref={list}
        className="adm-tabs is-bare adm-source-tabs"
        role="tablist"
        aria-label={t("admin.platform")}
        onKeyDown={onKeyDown}
      >
        {platforms.map((platform) => {
          const Icon = platformIcons[platform as Platform]
          const selected = platform === value
          return (
            <button
              key={platform}
              id={`${id}-${platform}`}
              type="button"
              role="tab"
              aria-selected={selected}
              aria-controls={selected ? `${id}-panel` : undefined}
              tabIndex={selected ? 0 : -1}
              className={cn(selected && "is-active")}
              onClick={() => onSelect(platform)}
            >
              <span className="adm-source-icon">
                <Icon aria-hidden />
              </span>
              <strong>{t(`admin.platform.${platform}`)}</strong>
            </button>
          )
        })}
      </div>
      <div
        id={`${id}-panel`}
        role="tabpanel"
        className="adm-platform-panel"
        aria-labelledby={`${id}-${value}`}
      >
        {children}
      </div>
    </>
  )
}

/**
 * A list page that starts again at 1 whenever its scope (the platform, a tab
 * or the filters) changes, without a second request for the old page.
 */
export function useScopedPage(scope: string) {
  const [state, setState] = useState({ scope, page: 1 })
  const page = state.scope === scope ? state.page : 1
  return [page, (next: number) => setState({ scope, page: next })] as const
}
