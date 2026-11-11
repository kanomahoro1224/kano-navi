import type { ReactNode } from "react"
import { useEffect, useRef } from "react"
import { X } from "lucide-react"

import { useAppSettings } from "@/app-settings"

/** The elements a sheet hands focus back to when it closes. */
const OPENERS = "button, a[href], input, select, textarea, [role='tab']"

/**
 * Native modal sheet for one release in full. Escape, the close button and
 * the backdrop close it; focus returns to the opener without scrolling. The
 * image viewer opens above it from outside its tree, so its Escape never
 * reaches this sheet.
 */
export function WorksSheet({
  label,
  onClose,
  children,
}: {
  label: string
  onClose: () => void
  children: ReactNode
}) {
  const { t } = useAppSettings()
  const dialogRef = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return undefined
    const previous = document.activeElement as HTMLElement | null
    // A tap in Safari can focus the nearest focusable container rather than
    // the button tapped, so only a control gets focus back; a container would
    // show its focus outline around everything in it.
    const control = previous?.matches(OPENERS) ? previous : null
    if (!dialog.open) dialog.showModal?.()
    dialog.querySelector<HTMLElement>(".works-sheet-close")?.focus()
    document.body.classList.add("has-works-sheet")
    return () => {
      if (dialog.open) dialog.close()
      document.body.classList.remove("has-works-sheet")
      if (control) control.focus({ preventScroll: true })
      else if (previous && document.activeElement === previous) previous.blur()
    }
  }, [])
  return (
    <dialog
      ref={dialogRef}
      className="works-sheet"
      aria-label={label}
      onCancel={(event) => {
        event.preventDefault()
        onClose()
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <button
        type="button"
        className="round-button works-sheet-close"
        onClick={onClose}
        aria-label={t("common.close")}
        title={t("common.close")}
      >
        <X aria-hidden="true" />
      </button>
      {children}
    </dialog>
  )
}
