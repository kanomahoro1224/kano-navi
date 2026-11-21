# Admin API

The operator console at `/admin` is the only client of `/api/admin`. The API
is a resource-oriented JSON surface served by `backend/internal/adminapi`,
which owns one route table (Go `http.ServeMux` method patterns), the session
and origin guards, and the shared response conventions. Feature packages
(`llmadmin`, `contentadmin`, `outboundproxy`, `alerts`, YouTube `websub`) contribute route handlers;
they do not route paths themselves.

MCP (`/mcp`) is a separate boundary with its own projections and never reuses
these routes. See [Public API](public-api/feeds-and-endpoints.md#mcp-boundary-mcp).

- [Conventions, Session and Settings](admin-api/overview.md): open when you change admin conventions, the session, the overview, or site settings.
- [Runs and Queue Preview](admin-api/runs.md): open when you change runs or the queue preview.
- [Sources and Outbound Proxies](admin-api/sources.md): open when you change source or proxy endpoints.
- [Web Bot Auth key](admin-api/web-bot-auth-key.md): open when you change the Web Bot Auth key endpoints.
- [Alerts](admin-api/alerts.md): open when you change the alert endpoints.
- [Events and Schedule Images](admin-api/events.md): open when you change event or schedule-image endpoints.
- [Posts and External Posts](admin-api/posts.md): open when you change post or external-post endpoints.
- [Videos](admin-api/videos.md): open when you change video endpoints.
- [Video Merging and Corrections](admin-api/video-merging.md): open when you change video merging or correction endpoints.
- [Profile Media and Account Avatars](admin-api/profile-media.md): open when you change profile media or avatar endpoints.
- [Extraction, Models and Translations](admin-api/models.md): open when you change extraction, model, or translation endpoints.

## Related Docs

- [Backend](backend.md)
- [Frontend](frontend.md)
- [Workflows](workflows.md)
- [Secure development](../development/security.md)
