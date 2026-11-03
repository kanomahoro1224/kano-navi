/* Kano Navi service worker: browser notifications only.
 *
 * It deliberately has no fetch handler. Adding one would put this file in
 * charge of the SPA's assets and their cache headers, which it has no reason
 * to touch.
 */

function urlBase64ToUint8Array(base64String) {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4)
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/")
  const raw = atob(base64)
  const output = new Uint8Array(raw.length)
  for (let index = 0; index < raw.length; index += 1) {
    output[index] = raw.charCodeAt(index)
  }
  return output
}

function subscriptionKeys(subscription) {
  const p256dh = subscription.getKey("p256dh")
  const auth = subscription.getKey("auth")
  if (!p256dh || !auth) return null
  return {
    p256dh: btoa(String.fromCharCode.apply(null, new Uint8Array(p256dh)))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, ""),
    auth: btoa(String.fromCharCode.apply(null, new Uint8Array(auth)))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, ""),
  }
}

self.addEventListener("push", (event) => {
  let payload = {}
  try {
    payload = event.data ? event.data.json() : {}
  } catch (error) {
    payload = { body: event.data ? event.data.text() : "" }
  }
  const title = payload.title || "Kano Navi"
  const options = {
    body: payload.body || "",
    icon: payload.icon || "/assets/kano-avatar.jpg",
    badge: "/assets/icons/icon-192.png",
    data: { url: payload.url || "/" },
  }
  if (payload.tag) options.tag = payload.tag
  event.waitUntil(self.registration.showNotification(title, options))
})

self.addEventListener("notificationclick", (event) => {
  event.notification.close()
  const target = (event.notification.data && event.notification.data.url) || "/"
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({
        type: "window",
        includeUncontrolled: true,
      })
      for (const client of windows) {
        if (!client.url.startsWith(self.location.origin)) continue
        await client.focus()
        if (target.startsWith(self.location.origin)) {
          await client.navigate(target)
        } else {
          await self.clients.openWindow(target)
        }
        return
      }
      await self.clients.openWindow(target)
    })(),
  )
})

// A browser rotates an endpoint on its own; re-register before the old one
// starts answering 410, so the reader keeps receiving notifications.
self.addEventListener("pushsubscriptionchange", (event) => {
  event.waitUntil(
    (async () => {
      let subscription = event.newSubscription
      if (!subscription) {
        const response = await fetch("/api/push/web/key", {
          cache: "no-store",
        })
        const key = await response.json()
        if (!key || !key.enabled) return
        subscription = await self.registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: urlBase64ToUint8Array(key.publicKey),
        })
      }
      const keys = subscriptionKeys(subscription)
      if (!keys) return
      await fetch("/api/push/web/subscribe", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          endpoint: subscription.endpoint,
          keys,
          locale: navigator.language,
        }),
      })
    })(),
  )
})
