# Unreleased

> [!WARNING]
> **v0.5.0 images cannot make HTTPS requests from the server.** Their runtime
> has no CA certificates, so X, the YouTube Data API and feed, model
> providers and media downloads fail as `*_unavailable` or `*_failed`; only
> yt-dlp still reads. Pull this release's image. Until then, mount the host's
> bundle read-only at `/etc/ssl/certs/ca-certificates.crt` in the container.

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- The Docker image carries the system CA certificates again, so source reads,
  model calls and media downloads verify TLS. The production smoke fails an
  image without them.

</details>

<details>
<summary><strong>Runtime and development</strong></summary>

- Frontend tests render with React DOM into a happy-dom window instead of the
  deprecated `react-test-renderer`, so `npm run test:frontend` no longer
  prints its deprecation warning.

</details>
