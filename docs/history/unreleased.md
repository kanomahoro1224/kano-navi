# Unreleased

> [!NOTE]
> LINE BLOG thumbnails load the full images until their smaller copies
> exist. Make them once after upgrading from `/admin` → Site → Make missing
> copies, with `docker compose run --rm kano variants`, or install a prepared
> `lineblog-variants.tar` copies archive, as described in
> [LINE BLOG deployments](../development/lineblog-archive.md#deployments).

<details>
<summary><strong>Public pages</strong></summary>

- Opening the LINE BLOG page or the board no longer makes the server resize
  images while visitors wait, so a page full of pictures cannot exhaust a
  small server's memory. A request gets the stored smaller copy or, until one
  exists, the original. Images and their copies are streamed from disk, and
  each file is checked against its hash once instead of on every request.
- The board's first section no longer jumps when the page opens. A return
  visit shows what the device last saw at once and updates it in place, and
  a first visit's placeholder has the ticket's shape.
- About credits 鹿可可, who provides and independently deploys the mainland
  China server site, and links to it. When the operator enables the mirror,
  the Simplified Chinese footer keeps a link to it after its notice is
  dismissed. While the mirror is off, the visitor region response leaves out
  its provider and addresses.

</details>

<details>
<summary><strong>Operator console</strong></summary>

- A Proxies page sets up forward proxies for the server's own requests: an
  ordered queue tried one proxy at a time, a choice of which areas use it
  (X, YouTube, Bilibili, media downloads and model providers, each with all
  or selected proxies), an optional direct fallback, and a per-proxy test.
  yt-dlp reads follow the YouTube area, trying up to three proxies.
  Proxy passwords are stored sealed with `LLM_SECRETS_KEY`.
- Console dates read `YYYY-MM-DD` and times `YYYY-MM-DD HH:mm` in Japan
  time, the same in every console language.
- The Review queue's failed posts are split into Extraction failed and
  Translation failed, and each row names the step that failed with its
  error. A failed extraction can be dismissed, which stops its retries until
  the post or the model queue changes; a failed translation can be queued
  again.
- The Schedule page's images open on Active, which leaves out images
  labelled Not a schedule; those move to their own tab.
- A schedule image without dates covers the Monday-to-Sunday weeks of the
  events extracted from its post, so an approved board reaches the board
  and calendar for its week without a manual correction. A corrected period
  still wins.
- The Schedule calendar lists the month's schedule images below the grid,
  and each image card shows Show in calendar, which opens its month.
- Schedule image cards show the source post's time and wording, and Correct
  dates shows the image and the whole post beside the form.
- An X post's dialog on the Posts page can mark one of its images as the
  schedule image, with a reason and optional dates. Synchronization keeps
  the picked image, and the post list shows a Schedule image badge.
- The Review queue, Schedule, Profile and Proxies pages drop their
  explanatory page descriptions.
- The Site page shows how many LINE BLOG, Journey and cached source images
  have their smaller copies, and Make missing copies runs a copies job of up
  to five minutes on the shared queue. The counts list file names only, are
  read when the page opens and after each run, and Check again lists anew.

</details>

<details>
<summary><strong>Synchronization and extraction</strong></summary>

- The media step makes the smaller copies of newly downloaded images, and of
  older ones that have none, up to 100 images and two minutes per run. It
  reports them as `resized`.

</details>

<details>
<summary><strong>Runtime and development</strong></summary>

- `kano-go variants` (`npm run variants`) makes the smaller copies of the
  LINE BLOG archive and history images ahead of time. It resumes where it
  stopped and can write to another data directory, so the copies can be made
  on a workstation and copied to the server.
- One core makes the copies for the media step, the command and the
  console's copies run, by group (`lineblog`, `history`, `media`) and within
  optional file and time bounds, and reports each group's originals, ready
  images and copies per width without reading the originals.
- Making copies holds at most two originals in memory at once and scales
  each original once by averaging the area each target pixel covers, which
  needs no buffer larger than the copy; narrower widths are scaled from the
  widest copy, and every copy is written as soon as it is encoded.
- Production Docker Compose limits the container to 1 GiB (`KANO_MEM_LIMIT`)
  and sets the Go runtime's soft limit `GOMEMLIMIT` to 700 MiB.

</details>
