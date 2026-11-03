import type { Locale } from "@/types"

/** What a reader can switch off; the three categories the App also has. */
export interface WebPushSettings {
  live: boolean
  video: boolean
  post: boolean
}

/** Why the button is in the state it is in. */
export type WebPushStatus =
  "checking" | "unsupported" | "blocked" | "ios" | "available" | "subscribed"

const SETTINGS_KEY = "kano.webPush.settings"
const DEFAULT_SETTINGS: WebPushSettings = {
  live: true,
  video: true,
  post: true,
}
const SERVICE_WORKER_URL = "/sw.js"

export function readWebPushSettings(): WebPushSettings {
  try {
    const raw = localStorage.getItem(SETTINGS_KEY)
    if (!raw) return { ...DEFAULT_SETTINGS }
    const stored = JSON.parse(raw) as Partial<WebPushSettings>
    return {
      live: stored.live !== false,
      video: stored.video !== false,
      post: stored.post !== false,
    }
  } catch (error) {
    return { ...DEFAULT_SETTINGS }
  }
}

function writeWebPushSettings(settings: WebPushSettings): void {
  try {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings))
  } catch (error) {
    // A private window may refuse storage; the server still has the switches.
  }
}

/** Browsers only deliver Web Push on iOS after the site is added to the Home
 * Screen, and there is no API to ask whether that happened. */
function isIosWithoutInstall(): boolean {
  if (typeof navigator === "undefined") return false
  const ios = /iPad|iPhone|iPod/.test(navigator.userAgent)
  if (!ios) return false
  const standalone = (navigator as Navigator & { standalone?: boolean })
    .standalone
  return standalone !== true
}

/** Whether this browser can subscribe at all, before asking the server. */
export function webPushAvailability(): WebPushStatus {
  if (typeof window === "undefined") return "unsupported"
  if (!("serviceWorker" in navigator) || !("PushManager" in window)) {
    return "unsupported"
  }
  if (typeof Notification === "undefined") return "unsupported"
  if (Notification.permission === "denied") return "blocked"
  if (isIosWithoutInstall()) return "ios"
  return Notification.permission === "granted" ? "subscribed" : "available"
}

/** The worker this browser already registered, without fetching one: a
 * reader who never subscribes should not download it. */
async function activeRegistration(): Promise<ServiceWorkerRegistration | null> {
  try {
    return (await navigator.serviceWorker.getRegistration("/")) ?? null
  } catch (error) {
    return null
  }
}

async function ensureRegistration(): Promise<ServiceWorkerRegistration> {
  const existing = await activeRegistration()
  if (existing) return existing
  return navigator.serviceWorker.register(SERVICE_WORKER_URL, { scope: "/" })
}

/** The subscription this browser already holds, if any. */
export async function currentWebPushSubscription(): Promise<PushSubscription | null> {
  if (webPushAvailability() === "unsupported") return null
  try {
    const active = await activeRegistration()
    if (!active) return null
    return await active.pushManager.getSubscription()
  } catch (error) {
    return null
  }
}

/** Whether the server has Web Push configured; a deployment without keys
 * answers `enabled:false`, and the button stays hidden. */
export async function webPushConfigured(): Promise<boolean> {
  try {
    const response = await fetch("/api/push/web/key", { cache: "no-store" })
    if (!response.ok) return false
    const payload = (await response.json()) as {
      enabled?: boolean
      publicKey?: string
    }
    return payload.enabled === true && Boolean(payload.publicKey)
  } catch (error) {
    return false
  }
}

function decodeKey(value: string): Uint8Array<ArrayBuffer> {
  const padding = "=".repeat((4 - (value.length % 4)) % 4)
  const base64 = (value + padding).replace(/-/g, "+").replace(/_/g, "/")
  const raw = atob(base64)
  const bytes = new Uint8Array(raw.length)
  for (let index = 0; index < raw.length; index += 1) {
    bytes[index] = raw.charCodeAt(index)
  }
  return bytes
}

function browserLocale(locale: Locale): string {
  if (locale === "ja") return "ja"
  if (locale === "en") return "en"
  return "zh-CN"
}

async function applicationServerKey(): Promise<Uint8Array<ArrayBuffer>> {
  const response = await fetch("/api/push/web/key", { cache: "no-store" })
  if (!response.ok) throw new Error("web push is unavailable")
  const payload = (await response.json()) as {
    enabled?: boolean
    publicKey?: string
  }
  if (!payload.enabled || !payload.publicKey) {
    throw new Error("web push is unavailable")
  }
  return decodeKey(payload.publicKey)
}

async function saveSubscription(
  subscription: PushSubscription,
  settings: WebPushSettings,
  locale: Locale,
): Promise<void> {
  const p256dh = subscription.getKey("p256dh")
  const auth = subscription.getKey("auth")
  if (!p256dh || !auth) throw new Error("subscription has no keys")
  const encode = (bytes: ArrayBuffer) =>
    btoa(String.fromCharCode(...new Uint8Array(bytes)))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "")
  const response = await fetch("/api/push/web/subscribe", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      endpoint: subscription.endpoint,
      keys: { p256dh: encode(p256dh), auth: encode(auth) },
      locale: browserLocale(locale),
      live: settings.live,
      video: settings.video,
      post: settings.post,
    }),
  })
  if (!response.ok) throw new Error("subscription was rejected")
}

/** Ask for permission and subscribe. Must run inside a click: Safari ignores a
 * permission prompt that is not tied to a user gesture. */
export async function enableWebPush(
  settings: WebPushSettings,
  locale: Locale,
): Promise<void> {
  const active = await ensureRegistration()
  await navigator.serviceWorker.ready
  const permission = await Notification.requestPermission()
  if (permission !== "granted") throw new Error("permission was not granted")
  const existing = await active.pushManager.getSubscription()
  const subscription =
    existing ??
    (await active.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: await applicationServerKey(),
    }))
  await saveSubscription(subscription, settings, locale)
  writeWebPushSettings(settings)
}

/** Push changed switches to the server; subscribing first if the browser has
 * no subscription yet. */
export async function updateWebPushSettings(
  settings: WebPushSettings,
  locale: Locale,
): Promise<void> {
  const subscription = await currentWebPushSubscription()
  if (!subscription) {
    await enableWebPush(settings, locale)
    return
  }
  await saveSubscription(subscription, settings, locale)
  writeWebPushSettings(settings)
}

/** Drop the browser subscription and the stored row. */
export async function disableWebPush(): Promise<void> {
  const subscription = await currentWebPushSubscription()
  if (!subscription) return
  const endpoint = subscription.endpoint
  try {
    await subscription.unsubscribe()
  } finally {
    await fetch("/api/push/web/unsubscribe", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ endpoint }),
    }).catch(() => undefined)
  }
}
