# Unreleased

<details>
<summary><strong>Public pages</strong></summary>

- The board's header drops its sync-state dot, so the header keeps the same
  layout when moving between pages. The profile card's platform icons still
  mark an out-of-date source, and an unreachable API still shows its alert
  with a retry.
- The All view of **Streams and works** fills the column beside the lead with
  four earlier works when nothing is coming up, and five items in all beside
  reservations, keeping at least one earlier work. Beyond four reservations,
  a count beside **Coming up** shows the rest in place.
- An open board reads its snapshot again one second after the next time it
  names, such as a stream's start, so **Next up** turns to **On air** within
  about a second instead of within the minute.
- A schedule card on air shows one **On air** label ending in the logos of
  the platforms it is live on, each opening that stream, instead of a
  separate label and a named, marked link per platform, so a stream on
  YouTube and bilibili at once keeps its labels on one line.
- The About page thanks its contributors for contributing to the project
  and lists ZhFuwe among them. GPT-6.2-Sol joins the shown development
  models, and GPT-6.1-Sol moves behind the "+4" button.
- On a phone, an About app card lists its platforms under the name and
  ends with a full-width Download or GitHub button, and a name with a
  Chinese title in parentheses wraps before the parentheses.

</details>

<details>
<summary><strong>Runtime and development</strong></summary>

- The Release workflow builds the published image without a GitHub Actions
  layer cache, which restored no layer between releases and took about half
  of the image job exporting.
- A schedule extraction run reads the inputs of its unsettled posts in
  batches of 200 instead of one row at a time, and reads only the image lists
  of each post's source record. A skip or a cache hit keeps a reprocess
  request an operator makes while the run is going, which the next run
  carries out.
- The repository root keeps fewer files: the release image builds from
  `docker/Dockerfile` (pass `-f docker/Dockerfile` to `docker build`), the
  security policy lives in `.github/`, the Tailwind theme lives in
  `src/index.css`, the Prettier settings live in `package.json`, and the
  scripts and tests type-check through `scripts/tsconfig.json`.

</details>
