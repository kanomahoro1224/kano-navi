import { useState } from "react"

import { useAppSettings } from "@/app-settings"
import { isVisitorRegionPath, useMainlandSite } from "@/lib/visitor-region"
import "./visitor-region-toast.css"

const SESSION_KEY = "kano-visitor-region-notice"

function wasHandled() {
  try {
    return window.sessionStorage.getItem(SESSION_KEY) === "handled"
  } catch {
    return false
  }
}

function rememberHandled() {
  try {
    window.sessionStorage.setItem(SESSION_KEY, "handled")
  } catch {
    // A blocked storage area should not prevent the visitor from dismissing
    // the notice for the current render.
  }
}

export function VisitorRegionToast() {
  const { t } = useAppSettings()
  const [offered] = useState(
    () => isVisitorRegionPath(window.location.pathname) && !wasHandled(),
  )
  const [dismissed, setDismissed] = useState(false)
  const site = useMainlandSite(offered)
  const config = site?.mainlandChina && !dismissed ? site : null

  if (!config) return null

  const marker = "__VISITOR_REGION_PROVIDER__"
  const message = t("visitorRegion.message", { provider: marker })
  const markerAt = message.indexOf(marker)
  const messageBefore = markerAt < 0 ? message : message.slice(0, markerAt)
  const messageAfter =
    markerAt < 0 ? "" : message.slice(markerAt + marker.length)

  const dismiss = () => {
    rememberHandled()
    setDismissed(true)
  }

  return (
    <aside className="visitor-region-toast" role="status" aria-live="polite">
      <div className="visitor-region-toast-copy">
        <strong>{t("visitorRegion.title")}</strong>
        <p>
          {messageBefore}
          <a
            className="visitor-region-toast-provider-link"
            href={config.providerUrl}
            target="_blank"
            rel="noopener noreferrer"
            onClick={rememberHandled}
          >
            {config.providerName}
          </a>
          {messageAfter}
        </p>
      </div>
      <div className="visitor-region-toast-actions">
        <a
          href={config.mirrorUrl}
          target="_blank"
          rel="noopener noreferrer"
          onClick={rememberHandled}
        >
          {t("visitorRegion.visit")}
        </a>
        <button type="button" onClick={dismiss}>
          {t("visitorRegion.dismiss")}
        </button>
      </div>
    </aside>
  )
}
