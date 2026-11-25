# Public API

Public reads, feeds, media and MCP read a prepared SQLite snapshot. None of
them fetches X, YouTube or a model provider. The service that hosts them is
described in [Backend](backend.md).

Media addresses below are written as this site's root-relative routes
(`/media/...`, and the dashboard's `/assets/...` fallback images). With
[`CDN_PUBLIC_BASE`](../operations/cdn.md) set, every public HTTP read and MCP
tool returns those same routes with the CDN base in front instead, such as
`https://cdn.example.invalid/media/<id>?v=<sha256>`; nothing else in a body
changes, and its ETag is computed over the body as sent. Without it they stay
root-relative.

- [Public Snapshot Contract](public-api/snapshot.md): open when you change /api/dashboard fields or revision updates.
- [Board Projection and Views](public-api/board-projection.md): open when you change the spotlight, video shelf, or event and video views.
- [Bounded Calendar and Archives](public-api/calendar-and-archives.md): open when you change calendar or archive endpoints.
- [Journey and Works Collections](public-api/journey.md): open when you change the Journey API or its media.
- [Works](public-api/works.md): open when you change the Works API or uploads.
- [LINE BLOG Archive](public-api/lineblog.md): open when you change the LINE BLOG archive API.
- [Feeds, MCP and Other Endpoints](public-api/feeds-and-endpoints.md): open when you change feeds, /mcp, the WebSub callback, push endpoints, Bambino ingest, or the key directory.

## Related Docs

- [Backend](backend.md)
- [Admin API](admin-api.md)
- [Data model](data-model.md)
- [Web Bot Auth](web-bot-auth.md)
- [Media cache](media-cache.md)
- [Deployment security](../operations/security.md)
