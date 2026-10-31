package push

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// Category groups the update kinds a device can switch off independently. The
// App's three settings toggles map one-to-one onto these values.
type Category string

const (
	CategoryLive  Category = "live"
	CategoryVideo Category = "video"
	CategoryPost  Category = "post"
)

// Update is one normalized change worth notifying about. It carries text only:
// the wording per language is chosen here, and the platform's own markup never
// reaches the shade.
type Update struct {
	// Platform is x, youtube or bilibili.
	Platform string
	// Kind is the BambinoStatus update kind, e.g. post, video, live_start.
	Kind string
	// Author is the display name shown before the action verb.
	Author string
	// Title is the video or stream title when the platform has one.
	Title string
	// Text is the post body, used when there is no title.
	Text string
	// URL is the original page, opened if the App cannot route the tap.
	URL string
}

// Category reports which switch governs this update.
func (u Update) Category() Category {
	switch u.Kind {
	case "live_schedule", "live_start", "live_end", "live_update":
		return CategoryLive
	case "video", "shorts":
		return CategoryVideo
	default:
		return CategoryPost
	}
}

// Dispatcher delivers updates to every registered device that accepts their
// category, in that device's language.
type Dispatcher struct {
	client *Client
	store  *sqlite.Store
}

// NewDispatcher returns nil when push is off, so callers can hold the value
// without checking configuration at every update.
func NewDispatcher(client *Client, store *sqlite.Store) *Dispatcher {
	if !client.Enabled() || store == nil {
		return nil
	}
	return &Dispatcher{client: client, store: store}
}

// maxFanout bounds how many notifications one ingest batch may produce. A
// first sync can carry dozens of updates at once; delivering each would fill
// the shade and burn the daily quota in one burst.
const maxFanout = 8

// Dispatch sends one notification per update, skipping devices whose switches
// exclude it. Failures are logged through the returned error; the caller must
// never let them fail the ingest itself.
func (d *Dispatcher) Dispatch(ctx context.Context, updates []Update) error {
	if d == nil || len(updates) == 0 {
		return nil
	}
	devices, err := d.store.PushDevices(ctx)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return nil
	}
	// One row per author and category: a creator publishing three posts in a
	// minute should produce one shade entry, not three.
	selected := dedupeByAuthor(updates, maxFanout)
	var stale []string
	var failures []string
	for _, update := range selected {
		category := update.Category()
		byLocale := map[string][]string{}
		for _, device := range devices {
			if !accepts(device, category) {
				continue
			}
			byLocale[normalizeLocale(device.Locale)] = append(byLocale[normalizeLocale(device.Locale)], device.Token)
		}
		for locale, tokens := range byLocale {
			title, body := wording(update, locale)
			rejected, sendErr := d.client.Send(ctx, Notification{
				Title:  title,
				Body:   body,
				Route:  routeFor(update),
				URL:    update.URL,
				Notify: notifyID(update),
			}, tokens)
			stale = append(stale, rejected...)
			if sendErr != nil {
				failures = append(failures, sendErr.Error())
			}
		}
	}
	if len(stale) > 0 {
		_ = d.store.DeletePushDevices(ctx, stale)
	}
	if len(failures) > 0 {
		return fmt.Errorf("push dispatch: %s", failures[0])
	}
	return nil
}

// accepts applies a device's master switch and its category switch.
func accepts(device sqlite.PushDevice, category Category) bool {
	if !device.Enabled {
		return false
	}
	switch category {
	case CategoryLive:
		return device.NotifyLive
	case CategoryVideo:
		return device.NotifyVideo
	default:
		return device.NotifyPost
	}
}

// dedupeByAuthor keeps the newest update per creator and category, then caps
// the batch.
func dedupeByAuthor(updates []Update, limit int) []Update {
	seen := map[string]bool{}
	selected := make([]Update, 0, limit)
	for _, update := range updates {
		key := update.Platform + "\x1f" + update.Author + "\x1f" + string(update.Category())
		if seen[key] {
			continue
		}
		seen[key] = true
		selected = append(selected, update)
		if len(selected) >= limit {
			break
		}
	}
	return selected
}

// routeFor picks the App screen: live events belong to the recent feed, while
// posts and uploads open the dynamic wall on the platform's own tab. The App
// reads "feed:<platform>" and switches both the tab and the platform.
func routeFor(update Update) string {
	if update.Category() == CategoryLive {
		return "recent"
	}
	platform := strings.ToLower(strings.TrimSpace(update.Platform))
	// BambinoStatus calls the platform twitter; the App's tab id is x.
	if platform == "twitter" {
		platform = "x"
	}
	if platform == "" {
		return "feed"
	}
	return "feed:" + platform
}

// notifyID is stable per creator and category, so a new update replaces the
// previous entry from the same creator instead of stacking.
func notifyID(update Update) int {
	digest := fnv.New32a()
	_, _ = digest.Write([]byte(update.Platform + "\x1f" + update.Author + "\x1f" + string(update.Category())))
	return int(digest.Sum32() & 0x7FFFFFFF)
}

// normalizeLocale maps the App's locale strings onto the three wordings this
// service maintains.
func normalizeLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	switch {
	case strings.HasPrefix(locale, "ja"):
		return "ja"
	case strings.HasPrefix(locale, "zh-hant"), strings.HasPrefix(locale, "zh-tw"), strings.HasPrefix(locale, "zh-hk"):
		return "zh-Hant"
	case strings.HasPrefix(locale, "zh"):
		return "zh"
	default:
		return "en"
	}
}

// wording renders the title and body in one language. The title reads
// "{author} 在 {platform} {action}" so the shade tells both who posted and
// where, without the user having to open the App first.
func wording(update Update, locale string) (string, string) {
	return title(update, locale), detail(update)
}

// title assembles the first line. Each language keeps its own word order:
// Chinese puts the platform after "在", Japanese before the verb with "で",
// English trails the platform. A missing author or platform is dropped whole
// so no stray particle or doubled space survives.
func title(update Update, locale string) string {
	author := strings.TrimSpace(update.Author)
	platform := platformLabel(update.Platform, locale)
	action := headline(update.Kind, locale)
	switch locale {
	case "ja":
		// 鹿乃がYouTubeでライブ配信を始めました
		place := ""
		if platform != "" {
			place = platform + "で"
		}
		return connect(author, "が", place, "", action, "")
	case "en":
		// Kano posted a new video on YouTube
		place := ""
		if platform != "" {
			place = " on " + platform
		}
		return connect(author, " ", action, place)
	default:
		// 鹿乃 在 B站 投稿了新视频~
		place := ""
		if platform != "" {
			place = "在 " + platform
		}
		return connect(author, " ", place, " ", action, "")
	}
}

// connect joins value/glue pairs, skipping empty values together with their
// glue.
func connect(pairs ...string) string {
	var builder strings.Builder
	for index := 0; index+1 < len(pairs); index += 2 {
		value := strings.TrimSpace(pairs[index])
		if value == "" {
			continue
		}
		builder.WriteString(value)
		builder.WriteString(pairs[index+1])
	}
	return strings.TrimSpace(builder.String())
}

// platformLabel is the short name shown in the title. Chinese keeps the
// colloquial "B站" and "YT"; Japanese and English spell the services out.
func platformLabel(platform, locale string) string {
	name := strings.ToLower(strings.TrimSpace(platform))
	switch name {
	case "twitter", "x":
		return "X"
	case "youtube":
		if locale == "ja" || locale == "en" {
			return "YouTube"
		}
		return "YT"
	case "bilibili":
		if locale == "ja" || locale == "en" {
			return "Bilibili"
		}
		return "B站"
	default:
		return strings.TrimSpace(platform)
	}
}

// headline picks the action phrase for the language.
func headline(kind, locale string) string {
	switch locale {
	case "ja":
		return headlineJa(kind)
	case "zh-Hant":
		return headlineHant(kind)
	case "en":
		return headlineEn(kind)
	default:
		return headlineHans(kind)
	}
}

func headlineHans(kind string) string {
	switch kind {
	case "repost":
		return "转推了动态~"
	case "quote":
		return "引用了一条动态~"
	case "live_schedule":
		return "发布了直播预告~"
	case "live_start":
		return "的直播开始啦~"
	case "live_end":
		return "的直播结束了。"
	case "live_update":
		return "修改了直播间信息"
	case "shorts":
		return "投稿了短视频~"
	case "video":
		return "投稿了新视频~"
	default:
		return "的动态更新啦~"
	}
}

func headlineHant(kind string) string {
	switch kind {
	case "repost":
		return "轉推了動態~"
	case "quote":
		return "引用了一條動態~"
	case "live_schedule":
		return "發布了直播預告~"
	case "live_start":
		return "的直播開始啦~"
	case "live_end":
		return "的直播結束了。"
	case "live_update":
		return "修改了直播間資訊"
	case "shorts":
		return "投稿了短影音~"
	case "video":
		return "投稿了新影片~"
	default:
		return "的動態更新啦~"
	}
}

func headlineJa(kind string) string {
	switch kind {
	case "repost":
		return "リポストしました"
	case "quote":
		return "引用ポストしました"
	case "live_schedule":
		return "ライブ配信を予定しています"
	case "live_start":
		return "ライブ配信を始めました"
	case "live_end":
		return "ライブ配信が終了しました"
	case "live_update":
		return "ライブ配信情報を更新しました"
	case "shorts":
		return "ショート動画を投稿しました"
	case "video":
		return "新しい動画を投稿しました"
	default:
		return "ポストしました"
	}
}

func headlineEn(kind string) string {
	switch kind {
	case "repost":
		return "reposted"
	case "quote":
		return "quoted a post"
	case "live_schedule":
		return "scheduled a live stream"
	case "live_start":
		return "went live"
	case "live_end":
		return "ended the stream"
	case "live_update":
		return "updated the stream details"
	case "shorts":
		return "posted a Short"
	case "video":
		return "posted a new video"
	default:
		return "posted"
	}
}

// detail is the second line: the video or stream title, else the post body.
func detail(update Update) string {
	if title := strings.TrimSpace(update.Title); title != "" {
		return oneLine(title, 120)
	}
	return oneLine(update.Text, 120)
}

// oneLine flattens whitespace so a multi-line body cannot stretch the shade
// entry past its two-line limit.
func oneLine(value string, limit int) string {
	fields := strings.Fields(value)
	flattened := strings.Join(fields, " ")
	if flattened == "" {
		return ""
	}
	return clip(flattened, limit)
}
