# Deployments

How an instance receives the archive media.

The image carries the articles' text, index, comments and media catalog, so
an instance needs only the media files in `data/lineblog/`, which belongs to
the ignored `data/` directory. A missing file shows an unavailable state on
the page while keeping the original links; without the design art the header
and profile keep a plain background.

The archive's `media/` already uses the names packaging gives the media, so
an instance takes its files as they are, with no packaging and no Node. In the
directory that holds `docker-compose.yml`, clone the archive (about 1 GiB):

```bash
git clone --depth 1 https://github.com/yexca/Kano_LINE_Blog_Recovery.git
```

`media/` holds every file of the media catalog, plus a few archive
files that no article references and the page never links. Put it in place
in one of two ways:

- **Mount it read-only.** The server only reads `/app/data/lineblog/` in the
  container (resized copies go to the variant cache under `data/cache/`), so
  the clone's `media/` can stay where it is. Add a
  `docker-compose.override.yml` beside `docker-compose.yml`, which Compose
  reads with it:

  ```yaml
  services:
    kano:
      volumes:
        - ./Kano_LINE_Blog_Recovery/media:/app/data/lineblog:ro
  ```

  Run `docker compose up -d` to recreate the container with the mount. A
  later `git pull` in the clone updates the media in place.

- **Copy it into `./data`.** The `./data` mount already maps
  `./data/lineblog/` to the container, so no restart is needed, and the
  clone can be removed afterwards:

  ```bash
  mkdir -p data/lineblog && cp Kano_LINE_Blog_Recovery/media/* data/lineblog/
  ```

A picture of the archive answers once the media are in place:

```bash
curl -I http://localhost:7657/media/lineblog/941e5735900c.jpg
```

A `200` with `Content-Type: image/jpeg` confirms it; a `404` means the file
is not where the container looks.

Then give the pages the smaller copies they request, once for the whole
archive (five entries per image: a copy, or a marker that the original serves
that width). Until an image has them, its thumbnails load the original. The
copies are named after the originals' bytes, so copies made anywhere apply
wherever the originals are. They come one of four ways:

- **Make them from the console.** `/admin` → Site → Image copies shows how
  many archive, history and cached media images have their copies; Make
  missing copies queues a
  [copies run](../../architecture/media-cache/resized-copies.md#copies-run) of up to five
  minutes. Run it again until LINE BLOG archive reads all copies made.

- **Make them in the container.** The
  [variants command](../../architecture/media-cache/resized-copies.md#variants-command) works
  two images at a time and can be stopped and run again:

  ```bash
  docker compose run --rm kano variants
  ```

- **Install a copies archive.** A `lineblog-variants.tar` built as in
  [Copies Archive](packaging.md#copies-archive) holds the copies of every archive image
  under `data/cache/variants/`, with its checksum in
  `lineblog-variants.tar.sha256`. Installing it is optional and replaces
  running the command. In the directory that holds `docker-compose.yml`,
  check the archive and extract it over `./data`:

  ```bash
  sha256sum -c lineblog-variants.tar.sha256
  ```

  ```bash
  tar -xf lineblog-variants.tar
  ```

  The server opens copies per request, so no restart is needed, and hashes
  each archive image once itself
  ([File Hashes](../../architecture/media-cache/resized-copies.md#file-hashes)). Until it has,
  the console counts those images as without copies; a copies run reads
  their hashes without decoding them.

- **Make them on a workstation**, in a checkout whose `data/lineblog/` holds
  the media, after `npm run build` has built the site into `dist/`:

  ```bash
  npm run variants -- -out variants-out
  ```

  Running it again skips the images that are done. Copy the contents of
  `variants-out/cache/variants/` into the server's `./data/cache/variants/`.
