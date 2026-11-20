---
name: release
description: Prepare a kano-navi release when the user asks (version bump, release note from unreleased.md, validation, and the release commit).
---

# Prepare a Release

Run only when the user asks for a release and names the version, or approves
the one you propose from `package.json`. Never invent a version or tag. The
rules are in [Commit and Release](../../../docs/development/commit-and-release.md).

1. Set the version in `package.json` and `package-lock.json`
   ([Version source](../../../docs/development/commit-and-release.md#version-source)).
2. Move the area blocks of `docs/history/unreleased.md` into
   `docs/history/<tag>.md`, add the upgrade notes and summary above them in
   the order and format of
   [Release notes](../../../docs/development/commit-and-release.md#release-notes),
   leave `unreleased.md` with only its heading, and add the page to
   `docs/history/index.md`.
3. Update version examples in `.env.example` and the
   [Docker deployment guide](../../../docs/operations/docker.md).
4. Run `make ci` and `make sensitive-check`, review the findings and the diff.
5. Commit as `chore(release): prepare <tag> release` when the user asks. Push
   the commit and the tag only when the user asks; the Release workflow then
   verifies the source as described in
   [Release steps](../../../docs/development/commit-and-release.md#release-steps).
