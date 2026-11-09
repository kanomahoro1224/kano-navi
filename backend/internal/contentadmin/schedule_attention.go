package contentadmin

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// attentionLimit bounds each kind of item the schedule attention reads.
const attentionLimit = 100

var japanZone = time.FixedZone("JST", 9*3600)

// attentionWeek is one Monday-to-Sunday week with what in it needs a look:
// the schedule images whose dates start in it and the entries that start in
// it.
type attentionWeek struct {
	WeekStart string              `json:"weekStart"`
	Images    []scheduleAsset     `json:"images"`
	Events    []sqlite.AdminEvent `json:"events"`
}

// scheduleAttention is the Review queue's schedule work. Undated images have
// no week; Truncated says a kind had more than attentionLimit items.
type scheduleAttention struct {
	Today     string          `json:"today"`
	Weeks     []attentionWeek `json:"weeks"`
	Undated   []scheduleAsset `json:"undated"`
	Truncated bool            `json:"truncated"`
}

// mondayOf is the Monday on or before the date day, or "" for no date.
func mondayOf(day string) string {
	date, ok := calendarDate(day)
	if !ok {
		return ""
	}
	return date.AddDate(0, 0, -((int(date.Weekday()) + 6) % 7)).Format("2006-01-02")
}

// readScheduleAttention groups the pending schedule images and the entries
// that need a look (see sqlite.EventAttentionSQL) by week, oldest first.
func (m *Module) readScheduleAttention(ctx context.Context) (scheduleAttention, error) {
	today := m.now().In(japanZone).Format("2006-01-02")
	out := scheduleAttention{Today: today, Weeks: []attentionWeek{}, Undated: []scheduleAsset{}}
	images, err := m.listScheduleAssets(ctx, attentionLimit+1, "", "", true)
	if err != nil {
		return out, err
	}
	if len(images) > attentionLimit {
		images, out.Truncated = images[:attentionLimit], true
	}
	events, err := m.store.ListAdminEvents(ctx, sqlite.AdminEventFilter{
		PageSize: attentionLimit, Ascending: true, Merge: "visible", Today: today, Attention: true,
	})
	if err != nil {
		return out, err
	}
	if events.Total > len(events.Items) {
		out.Truncated = true
	}

	weeks := map[string]*attentionWeek{}
	week := func(start string) *attentionWeek {
		if weeks[start] == nil {
			weeks[start] = &attentionWeek{WeekStart: start, Images: []scheduleAsset{}, Events: []sqlite.AdminEvent{}}
		}
		return weeks[start]
	}
	for _, image := range images {
		start := ""
		if image.PeriodStart != nil {
			start = mondayOf(*image.PeriodStart)
		}
		if start == "" {
			out.Undated = append(out.Undated, image)
			continue
		}
		group := week(start)
		group.Images = append(group.Images, image)
	}
	for _, event := range events.Items {
		if start := mondayOf(event.StartsOn); start != "" {
			group := week(start)
			group.Events = append(group.Events, event)
		}
	}
	for _, group := range weeks {
		out.Weeks = append(out.Weeks, *group)
	}
	sort.Slice(out.Weeks, func(i, j int) bool { return out.Weeks[i].WeekStart < out.Weeks[j].WeekStart })
	return out, nil
}

func (m *Module) scheduleAttentionRoute(w http.ResponseWriter, r *http.Request) {
	attention, err := m.readScheduleAttention(r.Context())
	if err != nil {
		writeFailure(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, attention)
}
