# Push Delivery

Admin API endpoints for Huawei Push Kit devices and browser Web Push subscriptions.

| Method and path                   | Body                                 | Result                                                                                    |
| --------------------------------- | ------------------------------------ | ----------------------------------------------------------------------------------------- |
| `GET /push`                       |                                      | `{configured, projectId, keyId, subAccount, category, testMode, devices}`                 |
| `GET /push/web`                   |                                      | `{configured, total, enabled, testMode, subscriptions}`                                   |
| `PUT /push/web`                   | `{testMode}`                         | the same browser snapshot                                                                 |
| `POST /push/test`                 | `{deviceId?, transport?, endpoint?}` | `{ok, devices, stale, error}` or `{ok, transport, testMode, subscriptions, stale, error}` |
| `DELETE /push/devices/{deviceId}` |                                      | `204`                                                                                     |
| `DELETE /push/web?endpoint=`      |                                      | `204`                                                                                     |

The App panel reports the AGC service account loaded from
`HUAWEI_PUSH_SERVICE_ACCOUNT` at startup; `configured` is false when that
variable is empty. Device rows carry a masked token only, and a row exists per
App install. A test goes to one device or to every device whose master switch
is on; a token Huawei rejects is retired in the same request. Removing a
device deletes its row, and the App registers again the next time its push
switch is turned on.

The browser panel is its own view, because the two transports share only
wording and category names. It reports whether a VAPID pair is loaded, how many
subscriptions exist and how many still accept updates, and the 20 most
recently seen rows. A row shows a masked endpoint, its locale, user agent,
category switches and consecutive failure count; the console also receives the
full endpoint, which only the remove action uses. A test with
`transport: "web"` goes to every browser or to the one `endpoint`. A push
service that answers `404` or `410` retires the row during a send, and ten
consecutive failures pause it.

Real notifications are sent at `Urgency: high`, which a browser on Windows
turns into an urgent toast: that is the kind Windows still shows while Do Not
Disturb is on. A test is `normal` unless `testMode` is on, so opening the
console and pressing Test does not interrupt anyone. `PUT /push/web` persists
that switch in `app_settings` and the next test reads it.
