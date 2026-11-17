# Unreleased

<details>
<summary><strong>Public pages</strong></summary>

- The board's header drops its sync-state dot, so the header keeps the same
  layout when moving between pages. The profile card's platform icons still
  mark an out-of-date source, and an unreachable API still shows its alert
  with a retry.

</details>

<details>
<summary><strong>Runtime and development</strong></summary>

- The Release workflow builds the published image without a GitHub Actions
  layer cache, which restored no layer between releases and took about half
  of the image job exporting.

</details>
