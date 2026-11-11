import { useId, useState, type ComponentType, type ReactNode } from "react"
import { ChevronDown, CircleHelp, type LucideIcon } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { IconButton, Modal } from "@/admin/ui"
import { cn } from "@/lib/utils"

/** A question-mark button that opens a short explanation in a dialog. */
export function HelpButton({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const { t } = useAppSettings()
  const [open, setOpen] = useState(false)
  return (
    <>
      <IconButton
        icon={CircleHelp}
        label={t("admin.help.about", { topic: title })}
        className="adm-help-button"
        onClick={() => setOpen(true)}
      />
      <Modal open={open} title={title} onClose={() => setOpen(false)}>
        <div className="adm-help">{children}</div>
      </Modal>
    </>
  )
}

/**
 * A field whose explanation, when it has one, sits behind a question mark
 * beside its label. The label is its own element, so pressing the question
 * mark never focuses the control.
 */
export function HelpField({
  label,
  help,
  hint,
  children,
}: {
  label: string
  help?: ReactNode
  hint?: ReactNode
  children: (id: string) => ReactNode
}) {
  const id = useId()
  return (
    <div className="adm-field">
      <span className="adm-field-label adm-label-help">
        <label htmlFor={id}>{label}</label>
        {help ? <HelpButton title={label}>{help}</HelpButton> : null}
      </span>
      {children(id)}
      {hint ? <span className="adm-field-hint">{hint}</span> : null}
    </div>
  )
}

/**
 * A folded section: a header row that opens onto its settings. Controls in
 * the header, such as a quick switch or a status badge, work while it is
 * closed, and pressing anywhere else on the header opens or closes it. The
 * body stays mounted, so unsaved edits survive folding.
 */
export function Fold({
  icon: FoldIcon,
  title,
  level = 3,
  help,
  hint,
  controls,
  className,
  children,
}: {
  icon: LucideIcon | ComponentType<{ "aria-hidden"?: boolean }>
  title: string
  /** 4 for a fold inside a panel, such as a confirmation step. */
  level?: 3 | 4
  help?: ReactNode
  hint?: ReactNode
  controls?: ReactNode
  className?: string
  children: ReactNode
}) {
  const [open, setOpen] = useState(false)
  const id = useId()
  const Heading = level === 4 ? "h4" : "h3"
  return (
    <section
      className={cn("adm-fold", open && "is-open", className)}
      aria-labelledby={`${id}-title`}
    >
      <header
        className="adm-fold-head"
        onClick={(event) => {
          // The title button, the help and the controls handle their own.
          if (
            !(event.target as Element).closest(
              "button, a, input, label, select",
            )
          )
            setOpen(!open)
        }}
      >
        <span className="adm-fold-mark">
          <FoldIcon aria-hidden />
        </span>
        <span className="adm-fold-title">
          <span className="adm-fold-name">
            <Heading id={`${id}-title`}>
              <button
                type="button"
                aria-expanded={open}
                aria-controls={`${id}-body`}
                onClick={() => setOpen(!open)}
              >
                {title}
              </button>
            </Heading>
            {help ? <HelpButton title={title}>{help}</HelpButton> : null}
          </span>
          {hint ? <small>{hint}</small> : null}
        </span>
        {controls ? (
          <span className="adm-fold-controls">{controls}</span>
        ) : null}
        <ChevronDown aria-hidden className="adm-fold-chevron" />
      </header>
      <div id={`${id}-body`} className="adm-fold-body" hidden={!open}>
        {children}
      </div>
    </section>
  )
}
