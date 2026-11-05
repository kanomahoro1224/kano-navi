import type { Translator } from "@/types"
import { Search } from "lucide-react"

export type Order = "newest" | "oldest"

/** Newest or oldest first; every section starts with the newest. */
export function OrderToggle({
  order,
  onChange,
  t,
}: {
  order: Order
  onChange: (order: Order) => void
  t: Translator
}) {
  return (
    <div className="segmented" role="group" aria-label={t("works.order")}>
      {(["newest", "oldest"] as const).map((item) => (
        <button
          key={item}
          type="button"
          aria-pressed={order === item}
          onClick={() => onChange(item)}
        >
          {t(`works.order.${item}`)}
        </button>
      ))}
    </div>
  )
}

/** A filter's choices, each with how many records it keeps. */
export function FilterButtons<T extends string>({
  label,
  options,
  value,
  onChange,
  name,
  count,
  t,
}: {
  label: string
  options: readonly T[]
  value: T
  onChange: (value: T) => void
  /** The message key of an option other than "all". */
  name: (option: T) => string
  count: (option: T) => number
  t: Translator
}) {
  return (
    <div className="segmented" role="group" aria-label={label}>
      {options.map((option) => (
        <button
          key={option}
          type="button"
          aria-pressed={value === option}
          onClick={() => onChange(option)}
        >
          {option === "all" ? t("common.all") : t(name(option))}
          <small>{count(option)}</small>
        </button>
      ))}
    </div>
  )
}

export function SearchBox({
  value,
  onChange,
  label,
}: {
  value: string
  onChange: (value: string) => void
  label: string
}) {
  return (
    <label className="works-search">
      <Search aria-hidden="true" />
      <span className="sr-only">{label}</span>
      <input
        type="search"
        value={value}
        placeholder={label}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  )
}
