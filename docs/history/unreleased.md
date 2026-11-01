# Unreleased

<details>
<summary><strong>Operator console</strong></summary>

- A Proxies page sets up forward proxies for the server's own requests: an
  ordered queue tried one proxy at a time, a choice of which areas use it
  (X, YouTube, Bilibili, media downloads and model providers, each with all
  or selected proxies), an optional direct fallback, and a per-proxy test.
  yt-dlp reads follow the YouTube area, trying up to three proxies.
  Proxy passwords are stored sealed with `LLM_SECRETS_KEY`.
- The Review queue's failed, uncertain and queued posts show their
  publication time as `YYYY-MM-DD HH:mm` in Japan time, the same in every
  console language.

</details>
