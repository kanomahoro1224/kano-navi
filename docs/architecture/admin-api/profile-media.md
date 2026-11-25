# Profile Media and Account Avatars

Selecting profile media and account avatars.

## Profile media

| Method and path                     | Body                         | Result                      |
| ----------------------------------- | ---------------------------- | --------------------------- |
| `GET /profile-media`                |                              | `{items, active}`           |
| `POST /profile-media`               | `{source, sourceUrl, slot?}` | `201 {item, items, active}` |
| `POST /profile-media/discover`      | `{source, slot?, handle?}`   | discovery result            |
| `POST /profile-media/clear`         | `{slot}`                     | `{items, active}`           |
| `POST /profile-media/upload?slot=`  | raw image body (`slot?`)     | `201 {item, items, active}` |
| `GET /profile-media/{id}/preview`   |                              | image bytes (ready only)    |
| `POST /profile-media/{id}/download` |                              | `{item, items, active}`     |
| `POST /profile-media/{id}/select`   | `{slot?}`                    | `{item, items, active}`     |
| `DELETE /profile-media/{id}`        |                              | `{items, active}`           |

`source` is `x` or `youtube`, and `slot` is `avatar` or `banner`. When `source`
is `x`, an optional `handle` selects that X profile URL instead of the default
configured `profiles.x_url`, and discovery falls back to `api.fxtwitter.com` if
the X profile page yields no URLs. A candidate's `slot` is the slot it is active
for, or otherwise the one that found it; an added or uploaded candidate without
one starts as an avatar. A source URL is one candidate whichever slot finds it.
Discovery without a slot records the avatar and the banner the profile page
names, and its result then has no `slot`. Selection uses the given slot, or the
candidate's own, and moves the candidate there; the slot it leaves falls back to
the built-in image. `POST /profile-media/clear` clears the active image for
the given `slot` (`avatar` or `banner`) so the slot returns to the built-in
image while keeping its candidates. Deleting the active candidate falls back to
the built-in image, an unknown ID answers `404 profile_media_not_found`, and a
later discovery may record a deleted URL again. Files follow
[Selected Profile Media](../media-cache/lifecycle.md#selected-profile-media).

## Account avatars

| Method and path                                                    | Body                     | Result                   |
| ------------------------------------------------------------------ | ------------------------ | ------------------------ |
| `GET /account-avatars`                                             |                          | `{items}`                |
| `POST /account-avatars/{source}/{handle}/discover`                 |                          | `{item, items}`          |
| `POST /account-avatars/{source}/{handle}/url`                      | `{sourceUrl, activate?}` | `201 {item, items}`      |
| `POST /account-avatars/{source}/{handle}/upload`                   | raw image body           | `201 {item, items}`      |
| `PUT /account-avatars/{source}/{handle}/mode`                      | `{mode}`                 | `{item, items}`          |
| `POST /account-avatars/{source}/{handle}/apply-profile`            | `{candidateId?}`         | `{item, items, profile}` |
| `GET /account-avatars/{source}/{handle}/candidates/{id}/preview`   |                          | image bytes (ready only) |
| `POST /account-avatars/{source}/{handle}/candidates/{id}/download` |                          | `{item, items}`          |
| `POST /account-avatars/{source}/{handle}/candidates/{id}/select`   |                          | `{item, items}`          |
| `DELETE /account-avatars/{source}/{handle}/candidates/{id}`        |                          | `{item, items}`          |

Each item tracks a tracked X account (`source` `x`, key `accountKey`) across
configured `X_HANDLES` (`own`), `x_related_accounts` (`related`), and stored
avatar rows (`other`). `mode` is `auto` (latest synced or discovered image wins),
`manual` (operator selection stays locked across syncs), or `hidden` (public
board shows initials and omits an avatar URL). `discover` checks public X
endpoints (`fxtwitter` / Twitter) for the account's latest avatar; `apply-profile`
copies an own account's ready avatar into the global site `avatar` slot in
`profile_media`.
