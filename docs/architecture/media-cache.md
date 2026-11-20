# Media Cache

How source, profile and packaged media are stored, registered, downloaded and served.

- [Layout, Model and HTTP Contract](media-cache/layout-and-model.md): open when you change the media layout, media tables, /media serving, or fixed files.
- [Lifecycle](media-cache/lifecycle.md): open when you change downloads, step bounds, profile media, or avatars.
- [Packaged Media](media-cache/packaged-media.md): open when you change packaged archive or Works upload media.
- [Resized Copies](media-cache/resized-copies.md): open when you change image variants or copies.

## Purpose

Platform media is runtime data, not a source-controlled frontend asset. X and
YouTube URLs can expire, change, or become rate-limited, and loading them from
the browser would make every page view a remote request. The media layer gives
each discovered remote URL a stable database identity and downloads it during
server-side synchronization.
