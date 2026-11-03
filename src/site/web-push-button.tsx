import { Bell, BellRing, X } from "lucide-react"
import { useEffect, useState } from "react"

import type { Locale, Translator } from "@/types"
import {
  currentWebPushSubscription,
  disableWebPush,
  enableWebPush,
  readWebPushSettings,
  updateWebPushSettings,
  webPushAvailability,
  webPushConfigured,
  type WebPushSettings,
  type WebPushStatus,
} from "@/site/web-push"

/**
 * The bell in the X section's header. A click subscribes the browser (the
 * permission prompt must belong to the gesture); the sheet behind it carries
 * the three category switches and the way out.
 */
export function WebPushButton({
  t,
  locale,
}: {
  t: Translator
  locale: Locale
}) {
  const [status, setStatus] = useState<WebPushStatus>("checking")
  const [configured, setConfigured] = useState(false)
  const [open, setOpen] = useState(false)
  const [settings, setSettings] = useState<WebPushSettings>(() =>
    readWebPushSettings(),
  )
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    void (async () => {
      const availability = webPushAvailability()
      if (availability === "unsupported") {
        if (!cancelled) setStatus("unsupported")
        return
      }
      const [enabled, subscription] = await Promise.all([
        webPushConfigured(),
        currentWebPushSubscription(),
      ])
      if (cancelled) return
      setConfigured(enabled)
      if (!enabled) {
        setStatus("unsupported")
        return
      }
      if (availability === "blocked") {
        setStatus("blocked")
        return
      }
      if (availability === "ios") {
        setStatus("ios")
        return
      }
      setStatus(subscription ? "subscribed" : "available")
    })()
    return () => {
      cancelled = true
    }
  }, [])

  async function toggle() {
    if (busy) return
    if (status === "blocked" || status === "ios" || status === "subscribed") {
      setOpen((value) => !value)
      return
    }
    setBusy(true)
    setFailed(false)
    try {
      await enableWebPush(settings, locale)
      setStatus("subscribed")
      setOpen(true)
    } catch (error) {
      setStatus(webPushAvailability() === "blocked" ? "blocked" : status)
      setFailed(true)
      setOpen(true)
    } finally {
      setBusy(false)
    }
  }

  async function change(key: keyof WebPushSettings, value: boolean) {
    const next = { ...settings, [key]: value }
    setSettings(next)
    setFailed(false)
    try {
      await updateWebPushSettings(next, locale)
      setStatus("subscribed")
    } catch (error) {
      setFailed(true)
    }
  }

  async function turnOff() {
    setBusy(true)
    setFailed(false)
    try {
      await disableWebPush()
      setStatus("available")
      setOpen(false)
    } catch (error) {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }

  if (!configured || status === "checking" || status === "unsupported") {
    return null
  }

  const label = status === "subscribed" ? t("feed.notifyOn") : t("feed.notify")
  const hint =
    status === "blocked"
      ? t("feed.notifyBlocked")
      : status === "ios"
        ? t("feed.notifyIos")
        : null

  return (
    <div className="web-push">
      <button
        type="button"
        className="pill-button is-collapsible"
        aria-expanded={open}
        aria-label={label}
        title={label}
        disabled={busy}
        onClick={() => void toggle()}
      >
        {status === "subscribed" ? (
          <BellRing aria-hidden="true" />
        ) : (
          <Bell aria-hidden="true" />
        )}
        <span className="pill-label">{label}</span>
      </button>
      {open ? (
        <div
          className="web-push-sheet"
          role="dialog"
          aria-label={t("feed.notifyTitle")}
        >
          <div className="web-push-head">
            <p>{t("feed.notifyTitle")}</p>
            <button
              type="button"
              className="web-push-close"
              aria-label={t("feed.notifyClose")}
              onClick={() => setOpen(false)}
            >
              <X aria-hidden="true" />
            </button>
          </div>
          {hint ? <p className="web-push-hint">{hint}</p> : null}
          {failed ? (
            <p className="web-push-hint">{t("feed.notifyFailed")}</p>
          ) : null}
          {status === "subscribed" ? (
            <>
              <p className="web-push-hint">{t("feed.notifyHint")}</p>
              {(
                [
                  ["live", t("feed.notifyLive")],
                  ["video", t("feed.notifyVideo")],
                  ["post", t("feed.notifyPost")],
                ] as const
              ).map(([key, text]) => (
                <label className="web-push-switch" key={key}>
                  <span>{text}</span>
                  <input
                    type="checkbox"
                    checked={settings[key]}
                    onChange={(event) => void change(key, event.target.checked)}
                  />
                </label>
              ))}
              <button
                type="button"
                className="web-push-off"
                disabled={busy}
                onClick={() => void turnOff()}
              >
                {t("feed.notifyOff")}
              </button>
            </>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
