import type { ReactNode } from "react"
import { useEffect, useRef } from "react"

/**
 * Native modal sheet for one record in full: a post's wording or a schedule
 * entry. Escape and the backdrop close it, its `.sheet-dialog-close` button
 * takes focus, and focus returns to the opener without scrolling the page.
 * It may open above the month dialog, so it locks page scroll with its own
 * body class.
 */
export function SheetDialog({
  label,
  onClose,
  children,
}: {
  label: string
  onClose: () => void
  children: ReactNode
}) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return undefined
    const previous = document.activeElement as HTMLElement | null
    if (!dialog.open) dialog.showModal?.()
    dialog.querySelector<HTMLElement>(".sheet-dialog-close")?.focus()
    document.body.classList.add("has-detail-sheet")
    return () => {
      if (dialog.open) dialog.close()
      document.body.classList.remove("has-detail-sheet")
      previous?.focus?.({ preventScroll: true })
    }
  }, [])
  return (
    <dialog
      ref={dialogRef}
      className="sheet-dialog"
      aria-label={label}
      onCancel={(event) => {
        // React passes the event on to a month dialog beneath, which would
        // close too.
        event.preventDefault()
        event.stopPropagation()
        onClose()
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      {children}
    </dialog>
  )
}
