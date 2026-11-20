# Sources and Schedule Extraction

How each external source is read, which limits bound it, and how fetched posts become schedule entries, translations and quoted or reposted content. Only server workflows run these reads; page loads never do.

- [X](sources/x.md): open when you change X timeline reads, FxTwitter or X API use.
- [X Related Accounts and Account Exports](sources/x-accounts.md): open when you change related accounts or X account-export imports.
- [YouTube](sources/youtube.md): open when you change YouTube reads or Data API use.
- [YouTube Video Details](sources/youtube-videos.md): open when you change localized titles, Shorts, availability, or follow-up checks.
- [YouTube Live Status and WebSub](sources/youtube-live.md): open when you change the live-status probe or WebSub push handling.
- [Bilibili](sources/bilibili.md): open when you change Bilibili uploads, the live room, or login handling.
- [Bilibili Dynamics and Forwards](sources/bilibili-dynamics.md): open when you change Bilibili dynamics or forwards.
- [Official Shop and Works Cover Lookups](sources/shop.md): open when you change shop reads or Works cover lookups.
- [Source Request Limits](sources/request-limits.md): open when you change per-source request limits or budgets.
- [Schedule Extraction Pipeline](sources/extraction.md): open when you change schedule extraction or its failure backoff.
- [Time Zones](sources/time-zones.md): open when you change how source times are interpreted.
- [Schedule Images](sources/schedule-images.md): open when you change schedule image handling.
- [LLM Provider Catalog](sources/llm-providers.md): open when you change providers, models, or routing.
- [Quoted Posts](sources/quoted-posts.md): open when you change quotes, reposts, or source refreshes.
- [Translation](sources/translation.md): open when you change translation.

## Related Docs

- [Workflows and manual fetches](workflows.md)
- [Event precedence](data-model/event-precedence.md)
- [Configuration](../operations/configuration.md)
- [Secure development](../development/security.md)
