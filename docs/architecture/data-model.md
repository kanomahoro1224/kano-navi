# Data Model

The SQLite snapshot schema, seeds, and the precedence rules that keep manual edits, locks and tombstones ahead of automatic sync and extraction.

- [SQLite Snapshot](data-model/sqlite-snapshot.md): open when you change tables, columns, or sync state.
- [Seeds, Schema Changes and Persistence](data-model/seeds-and-schema.md): open when you change bootstrap.sql, schema_upgrade.go, seeds, or store ownership.
- [Event Labels, Sales and Platforms](data-model/events.md): open when you change event labels, sale periods, platforms, or stored event text.
- [Event Precedence](data-model/event-precedence.md): open when you change how sync, extraction, or manual edits replace events.
- [Duplicate Merging](data-model/duplicate-merging.md): open when you change event deduplication or merging.
- [Posts](data-model/posts.md): open when you change stored posts or their LLM states.
- [Bilibili Stickers](data-model/bilibili-stickers.md): open when you change Bilibili stickers.
- [Translations](data-model/translations.md): open when you change stored translations.
- [Video Modeling](data-model/video-form.md): open when you change video form or tags.
- [Video Corrections and Merging](data-model/videos.md): open when you change video corrections or video merging.

## Related Docs

- [Sources and extraction](sources.md)
- [Manual source fetches](workflows/manual-fetches.md)
- [Media cache](media-cache.md)
