package push

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

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
	// ID is the post identity, which ties a deferred notification to its
	// stored translations. Other categories leave it empty.
	ID string
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
	// Bodies holds translated bodies by locale, filled only for a deferred
	// post whose translation arrived after the Japanese notification.
	Bodies map[string]string
	// DurationSeconds is a finished stream's length; a live_end shows it as
	// the body. Zero while unknown.
	DurationSeconds int
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
	// web carries the same updates to browsers that subscribed through the
	// site. It is optional: a deployment with only Huawei push leaves it nil.
	web   *WebSender
	store *sqlite.Store
	// flushMu serializes pending flushes: the translate step and the ingest
	// catch-up can both fire, and a second runner would resend the same rows.
	flushMu sync.Mutex
}

// Option configures a Dispatcher without changing the Huawei call sites.
type Option func(*Dispatcher)

// WithWeb adds the browser transport beside Huawei Push Kit.
func WithWeb(sender *WebSender) Option {
	return func(d *Dispatcher) { d.web = sender }
}

// NewDispatcher returns nil when no transport is configured, so callers can
// hold the value without checking configuration at every update.
func NewDispatcher(client *Client, store *sqlite.Store, options ...Option) *Dispatcher {
	if store == nil {
		return nil
	}
	dispatcher := &Dispatcher{client: client, store: store}
	for _, option := range options {
		option(dispatcher)
	}
	if !client.Enabled() && !dispatcher.web.Enabled() {
		return nil
	}
	return dispatcher
}

// maxFanout bounds how many notifications one ingest batch may produce. A
// first sync can carry dozens of updates at once; delivering each would fill
// the shade and burn the daily quota in one burst.
const maxFanout = 8

// pendingGrace bounds how long a post waits for its translation. The translate
// queue normally settles an ingested post within its run; a queue that is off
// or keeps failing must not hold the notification forever.
const pendingGrace = 10 * time.Minute

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
	return d.deliver(ctx, devices, updates, nil, nil)
}

// DispatchPosts sends the batch to the devices. A post's wording can be
// translated, so it goes to the devices that read Japanese now and is
// remembered for every other language; the translate step flushes the
// remembered rows once it settles that wording. Everything else — a video's
// title, a live room's title — arrives in the platform's own words, so it is
// sent at once in every language instead of waiting for a translation that
// would never come.
func (d *Dispatcher) DispatchPosts(ctx context.Context, updates []Update) error {
	if d == nil || len(updates) == 0 {
		return nil
	}
	posts := make([]Update, 0, len(updates))
	immediate := make([]Update, 0, len(updates))
	for _, update := range updates {
		if update.Category() == CategoryPost {
			posts = append(posts, update)
		} else {
			immediate = append(immediate, update)
		}
	}
	if len(posts) == 0 {
		devices, err := d.store.PushDevices(ctx)
		if err != nil {
			return err
		}
		return d.deliver(ctx, devices, immediate, nil, nil)
	}
	log.Printf("kano-go: push posts deferred=%d immediate=%d", len(posts), len(immediate))
	createdAt := sqlite.ISOTime(time.Now())
	items := make([]sqlite.PendingPush, 0, len(posts))
	for _, update := range posts {
		items = append(items, sqlite.PendingPush{
			PostID:    strings.TrimSpace(update.ID),
			Platform:  update.Platform,
			Kind:      update.Kind,
			Author:    update.Author,
			Title:     update.Title,
			Text:      update.Text,
			URL:       update.URL,
			CreatedAt: createdAt,
		})
	}
	// The row is the only record of the post, so keep it even when no device
	// is registered yet; a device added before the translation settles still
	// receives the deferred notification. A failed write must not swallow the
	// Japanese notification that needs no translation.
	saveErr := d.store.SavePendingPushes(ctx, items)
	devices, err := d.store.PushDevices(ctx)
	if err != nil {
		return err
	}
	sendErr := d.deliver(ctx, devices, posts, func(locale string) bool { return locale == "ja" }, nil)
	if len(immediate) > 0 {
		if err := d.deliver(ctx, devices, immediate, nil, nil); err != nil && sendErr == nil {
			sendErr = err
		}
	}
	if saveErr != nil {
		return saveErr
	}
	return sendErr
}

// FlushPending notifies the non-Japanese devices of the posts whose
// translation settled, or whose grace period ran out. It leaves rows that are
// still waiting for the model, so the next translate run or catch-up tick
// retries them.
func (d *Dispatcher) FlushPending(ctx context.Context) error {
	if d == nil {
		return nil
	}
	d.flushMu.Lock()
	defer d.flushMu.Unlock()
	items, err := d.store.PendingPushes(ctx, time.Now(), pendingGrace, maxFanout)
	if err != nil {
		return err
	}
	ready := make([]sqlite.PendingPush, 0, len(items))
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if !item.Ready {
			continue
		}
		ready = append(ready, item)
		ids = append(ids, item.PostID)
	}
	if len(ready) == 0 {
		return nil
	}
	sets, err := d.store.PostTranslationSets(ctx, ids)
	if err != nil {
		return err
	}
	devices, err := d.store.PushDevices(ctx)
	if err != nil {
		return err
	}
	updates := make([]Update, 0, len(ready))
	for _, item := range ready {
		update := Update{
			ID:       item.PostID,
			Platform: item.Platform,
			Kind:     item.Kind,
			Author:   item.Author,
			Title:    item.Title,
			Text:     item.Text,
			URL:      item.URL,
			Bodies:   map[string]string{},
		}
		for locale, translation := range sets[item.PostID].Locales {
			update.Bodies[locale] = translation.Text
		}
		updates = append(updates, update)
	}
	// Japanese devices already hold these posts; only the deferred languages
	// are sent now.
	log.Printf("kano-go: push flush rows=%d pending=%d", len(ready), len(items))
	sendErr := d.deliver(ctx, devices, updates, func(locale string) bool { return locale != "ja" }, translatedBody)
	// A failed send is not retried: a later flush would deliver the same post
	// twice to any device the first attempt reached.
	_ = d.store.DeletePendingPushes(ctx, ids)
	return sendErr
}

// deliver groups the accepted devices by language and sends one notification
// per language. allow narrows which languages receive this pass; body
// overrides the second line, which deferred posts fill from their translation.
func (d *Dispatcher) deliver(ctx context.Context, devices []sqlite.PushDevice, updates []Update, allow func(string) bool, body func(Update, string) string) error {
	if len(updates) == 0 {
		return nil
	}
	// Browsers are read once per dispatch, like devices, so a batch of updates
	// does not query the table again for each one.
	var subs []sqlite.WebPushSubscription
	if d.web.Enabled() {
		loaded, err := d.store.WebPushSubscriptions(ctx)
		if err != nil {
			return err
		}
		subs = loaded
	}
	if len(devices) == 0 && len(subs) == 0 {
		log.Printf("kano-go: push skipped: no registered device or browser")
		return nil
	}
	// One row per author and category: a creator publishing three posts in a
	// minute should produce one shade entry, not three.
	selected := dedupeByAuthor(updates, maxFanout)
	log.Printf("kano-go: push dispatch updates=%d selected=%d devices=%d browsers=%d kit=%t", len(updates), len(selected), len(devices), len(subs), d.client.Enabled())
	var stale []string
	var failures []string
	sent := 0
	started := time.Now()
	for _, update := range selected {
		category := update.Category()
		byLocale := map[string][]string{}
		for _, device := range devices {
			if !accepts(device, category) {
				continue
			}
			locale := normalizeLocale(device.Locale)
			if allow != nil && !allow(locale) {
				continue
			}
			byLocale[locale] = append(byLocale[locale], device.Token)
		}
		for locale, tokens := range byLocale {
			if !d.client.Enabled() {
				log.Printf("kano-go: push kit disabled, dropping kind=%s id=%s locale=%s targets=%d", update.Kind, update.ID, locale, len(tokens))
				continue
			}
			title, detailText := wording(update, locale)
			if body != nil {
				detailText = body(update, locale)
			}
			rejected, sendErr := d.client.Send(ctx, Notification{
				Title:  title,
				Body:   detailText,
				Route:  routeFor(update),
				URL:    update.URL,
				Notify: notifyID(update),
			}, tokens)
			stale = append(stale, rejected...)
			switch {
			case sendErr != nil:
				failures = append(failures, sendErr.Error())
				log.Printf("kano-go: push kit failed kind=%s category=%s locale=%s targets=%d title=%q body=%q err=%v", update.Kind, category, locale, len(tokens), title, detailText, sendErr)
			default:
				sent++
				log.Printf("kano-go: push kit sent kind=%s category=%s locale=%s targets=%d stale=%d title=%q body=%q", update.Kind, category, locale, len(tokens), len(rejected), title, detailText)
			}
		}
		if len(subs) > 0 {
			if webErr := d.deliverWeb(ctx, subs, update, allow, body); webErr != nil {
				failures = append(failures, webErr.Error())
			}
		}
	}
	if len(stale) > 0 {
		_ = d.store.DeletePushDevices(ctx, stale)
		log.Printf("kano-go: push kit dropped %d unregistered token(s)", len(stale))
	}
	log.Printf("kano-go: push done batches=%d stale=%d failures=%d took_ms=%d", sent, len(stale), len(failures), time.Since(started).Milliseconds())
	if len(failures) > 0 {
		return fmt.Errorf("push dispatch: %s", failures[0])
	}
	return nil
}

// maxWebPushFailures is how many consecutive transport failures retire a
// browser. 404 and 410 delete the row at once; this limit covers a push
// service that keeps failing without saying the subscription is gone.
const maxWebPushFailures = 10

// webPushConcurrency bounds how many browsers are addressed at once. A large
// fanout must not hold the ingest for one slow push service.
const webPushConcurrency = 4

// endpointHost names a browser's push service without logging the endpoint
// itself: that URL is a capability, so only its host is safe to print.
func endpointHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "-"
	}
	return parsed.Host
}

// deliverWeb sends one update to every browser that accepts its category, in
// that browser's language. A gone subscription is retired; a failed one is
// counted and retried by a later update.
func (d *Dispatcher) deliverWeb(ctx context.Context, subs []sqlite.WebPushSubscription, update Update, allow func(string) bool, body func(Update, string) string) error {
	category := update.Category()
	byLocale := map[string][]sqlite.WebPushSubscription{}
	for _, sub := range subs {
		if !acceptsWeb(sub, category) {
			continue
		}
		locale := normalizeLocale(sub.Locale)
		if allow != nil && !allow(locale) {
			continue
		}
		byLocale[locale] = append(byLocale[locale], sub)
	}
	if len(byLocale) == 0 {
		log.Printf("kano-go: web push skipped kind=%s category=%s: no browser accepts it", update.Kind, category)
		return nil
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var stale []string
	var firstErr error
	slots := make(chan struct{}, webPushConcurrency)
	for locale, group := range byLocale {
		title, detailText := wording(update, locale)
		if body != nil {
			detailText = body(update, locale)
		}
		payload := WebPayload{Title: title, Body: detailText, URL: update.URL, Tag: string(category)}
		for _, sub := range group {
			wg.Add(1)
			slots <- struct{}{}
			go func(sub sqlite.WebPushSubscription) {
				defer wg.Done()
				defer func() { <-slots }()
				sendErr := d.web.Send(ctx, sub, payload)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case errors.Is(sendErr, ErrWebPushGone):
					stale = append(stale, sub.Endpoint)
					log.Printf("kano-go: web push gone kind=%s category=%s locale=%s host=%s title=%q body=%q", update.Kind, category, locale, endpointHost(sub.Endpoint), payload.Title, payload.Body)
				case sendErr != nil:
					_ = d.store.RecordWebPushFailure(ctx, sub.Endpoint, maxWebPushFailures)
					if firstErr == nil {
						firstErr = sendErr
					}
					log.Printf("kano-go: web push failed kind=%s category=%s locale=%s host=%s title=%q body=%q err=%v", update.Kind, category, locale, endpointHost(sub.Endpoint), payload.Title, payload.Body, sendErr)
				default:
					_ = d.store.ClearWebPushFailures(ctx, sub.Endpoint)
					log.Printf("kano-go: web push sent kind=%s category=%s locale=%s host=%s title=%q body=%q", update.Kind, category, locale, endpointHost(sub.Endpoint), payload.Title, payload.Body)
				}
			}(sub)
		}
	}
	wg.Wait()
	for _, endpoint := range stale {
		_ = d.store.DeleteWebPushSubscription(ctx, endpoint)
	}
	return firstErr
}

// translatedBody picks the deferred notification's second line. Traditional
// Chinese readers receive the Simplified translation, which is the only
// Chinese body the machine step produces; a missing or failed translation
// falls back to the post's own wording.
func translatedBody(update Update, locale string) string {
	// A finished stream's body is its length, never a translation.
	if update.Kind == "live_end" {
		return liveEndDetail(update, locale)
	}
	// The wordings use zh for Simplified and zh-Hant for Traditional; the
	// machine step stores one Chinese body under zh-CN.
	target := locale
	if target == "zh" || target == "zh-Hant" {
		target = "zh-CN"
	}
	if text := strings.TrimSpace(update.Bodies[target]); text != "" {
		return oneLine(text, 120)
	}
	if detailText := detail(update, locale); detailText != "" {
		return detailText
	}
	return fallbackDetail(update.Kind, locale)
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

// TestNotification is the message the console sends to prove delivery on one
// device. It carries no update and opens the Recent screen.
func TestNotification(locale string) Notification {
	title, body := testWording(normalizeLocale(locale))
	return Notification{Title: title, Body: body, Route: "recent", Notify: 1}
}

func testWording(locale string) (string, string) {
	switch locale {
	case "zh-Hant":
		return "推送測試", "這台裝置的華為推送鏈路正常"
	case "ja":
		return "テスト通知", "この端末へのプッシュ配信は正常です"
	case "en":
		return "Test notification", "Huawei Push Kit is delivering to this device."
	default:
		return "推送测试", "这台设备的华为推送链路正常"
	}
}

// wording renders the title and body in one language. The title reads
// "{author} 在 {platform} {action}" so the shade tells both who posted and
// where, without the user having to open the App first.
func wording(update Update, locale string) (string, string) {
	line := title(update, locale)
	detailText := detail(update, locale)
	if detailText == "" {
		// Push Kit refuses a notification without a body (80100003), so an
		// update that carries no title and no text still says what happened.
		detailText = fallbackDetail(update.Kind, locale)
	}
	return line, detailText
}

// fallbackDetail is the second line of an update with no wording of its own,
// such as a live room that changed its cover.
func fallbackDetail(kind, locale string) string {
	switch locale {
	case "ja":
		if kind == "live_update" {
			return "配信ルームの情報が更新されました"
		}
		return "詳細はアプリでご確認ください"
	case "en":
		if kind == "live_update" {
			return "The live room details changed"
		}
		return "Open the app for details"
	case "zh-Hant":
		if kind == "live_update" {
			return "直播間資訊有更新"
		}
		return "詳情請在 App 中查看"
	default:
		if kind == "live_update" {
			return "直播间信息有更新"
		}
		return "详情请在 App 中查看"
	}
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

// detail is the second line: a finished stream's length, else the video or
// stream title, else the post body. Empty leaves the caller's fallback.
func detail(update Update, locale string) string {
	if update.Kind == "live_end" {
		return durationText(update.DurationSeconds, locale)
	}
	if title := strings.TrimSpace(update.Title); title != "" {
		return oneLine(title, 120)
	}
	return oneLine(update.Text, 120)
}

// liveEndDetail is a finished stream's body: its length, or the fallback
// while the length is unknown.
func liveEndDetail(update Update, locale string) string {
	if text := durationText(update.DurationSeconds, locale); text != "" {
		return text
	}
	return fallbackDetail(update.Kind, locale)
}

// durationText spells a stream's length in the reader's language with hours,
// minutes and seconds all present: hours unpadded, minutes and seconds two
// digits ("直播总时长：2小时05分钟08秒"). A length of zero or less is unknown and
// gives no text.
func durationText(seconds int, locale string) string {
	if seconds <= 0 {
		return ""
	}
	h, m, sec := seconds/3600, seconds%3600/60, seconds%60
	switch locale {
	case "ja":
		return fmt.Sprintf("配信時間：%d時間%02d分%02d秒", h, m, sec)
	case "en":
		return fmt.Sprintf("Total stream time: %dh %02dm %02ds", h, m, sec)
	case "zh-Hant":
		return fmt.Sprintf("直播總時長：%d小時%02d分鐘%02d秒", h, m, sec)
	default:
		return fmt.Sprintf("直播总时长：%d小时%02d分钟%02d秒", h, m, sec)
	}
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
