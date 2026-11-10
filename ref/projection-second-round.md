# Dashboard Projection: Second-Round Plan

## Purpose

The first projection pass makes the backend authoritative for the public board's
shared decisions. Dashboard, calendar, archive, and MCP reads expose versioned
projection fields, and the frontend consumes those fields while retaining an
additive fallback for retained snapshots that predate projection version 1.

The second round removes that compatibility debt, makes the contract easier to
validate, and lets the browser refresh from the server's exact transition time.
It does not change source synchronization, SQLite persistence, or localization
rules.

## Current contract

`projection.version` is `1`. Event and video `view` objects are read-time,
locale-neutral projections and are not stored in SQLite.

### Event view

- `state`: `upcoming`, `live`, `open`, `done`, `pending`, `cancelled`, or
  `cancellation_review`.
- `typeCode`: `stream`, `member`, `event`, `release`, `video`, or `sale`, or
  `null` when the stored type is outside the public set.
- `activityCode`: `live`, `game`, `talk`, `song`, or `video`, or `null`.
- `activityContext`: `stream`, `work`, or `""` when no activity is present.
- `isWork`: `true` only for `release` and `video` event types.
- `calendarEntries[].saleMark`: `opens`, `closes`, or `null`. A sale deadline
  has only a closing entry. `timePrecision` is `exact`, `approximate`, or
  `unknown`.

State meanings are stable: `upcoming` has not started, `live` is on air,
`open` is an active sale, `done` has ended, `pending` needs confirmation,
`cancelled` is definitive cancellation, and `cancellation_review` is
suspected cancellation awaiting confirmation.

### Video view

- `phase`: `upcoming`, `live`, `started`, `past`, or `cancelled`.
- `formCode`: `short`, `upcoming`, `stream`, `archive`, `premiere`, `video`, or
  `null`.
- `shelfGroup`: `reservation`, `work`, `archive`, `short`, or `""` when the
  video is omitted from the shelf.
- `latestEligible` and `focusEligible`: booleans.
- `tagContext`: `stream` or `work`.

### Dashboard projection

- `nextTransitionAt`: the next known ISO instant that can change a time-based
  decision, or `null`.
- `focus.eventIds` and `focus.videoIds`: ordered candidate IDs.
- `spotlight.mode`: `next` or `latest`; the whole object is `null` when there is
  no candidate or latest video.
- `sales`: at most three sale IDs in display order.
- `shelf.leadId`: the lead video ID or `null`; `leadIsFeatured` says whether the
  operator featured it. The four shelf arrays are `reservations`, `works`,
  `archives`, and `shorts`.

All codes cross the API boundary unchanged. The frontend translates them and
never derives a replacement value when a version-1 projection is present.

## Second-round work

1. **Contract hardening.** Add fixture coverage for every enum value, null
   branch, sale deadline, cancellation-review state, and projection mode. Keep
   the public API document and TypeScript types in lockstep with those fixtures.
2. **Remove compatibility derivation.** After the supported snapshot retention
   window no longer contains pre-projection payloads, remove the legacy branch
   from `deriveDashboardAt`, the fallback ordering helpers, and the additive
   defaulting used only for missing projection fields. Keep an explicit version
   guard so an unsupported future version fails closed and requests a fresh
   snapshot.
3. **Use exact refresh deadlines.** Schedule the dashboard revalidation at
   `projection.nextTransitionAt` when it is in the future. Keep revision
   events, visibility/focus wakeups, the one-second minimum gap, and the
   periodic safety poll for clocks, missed events, and old snapshots.
4. **Split frontend architecture documentation.** Keep the public projection
   contract in `docs/architecture/public-api.md`. Split request/cache behavior,
   rendering composition, and compatibility removal into focused sections or
   documents so one page does not repeat the same rules.
5. **Keep health locale-neutral.** `/api/health` and MCP health reads return
   English base status text or stable codes only. They never return localized
   Chinese or Japanese copy; UI translations remain a frontend concern.

## Exit criteria

The second round is complete when the browser's normal path requires
`projection.version === 1`, no active board code reconstructs focus, spotlight,
shelf, event state, video phase, or form from raw fields, refresh timers honor
`nextTransitionAt`, all projection enums have fixture coverage, and the public
health contract is locale-neutral.

The SQLite schema remains unchanged because these values are computed from the
latest snapshot at read time. No source fetch or model request is introduced
by this work.
