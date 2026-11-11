package sqlite

import (
	"sort"
	"strings"
	"time"
)

const (
	projectionVersion       = 1
	projectionLiveGrace     = 3 * time.Hour
	projectionLiveYield     = 30 * time.Minute
	projectionSaleLookahead = 7 * 24 * time.Hour
)

var projectionEventTags = map[string]bool{
	"live": true, "game": true, "talk": true, "song": true, "video": true,
}

var projectionEventTypes = map[string]bool{
	"stream": true, "member": true, "event": true, "release": true, "video": true, "sale": true,
}

// applyEventView computes the public event semantics used by every reader.
// It intentionally returns codes and dates, never localized copy.
func applyEventView(event *DashboardEvent, now time.Time) {
	if event == nil {
		return
	}
	typeCode := strings.ToLower(strings.TrimSpace(valueOrEmpty(event.EventType)))
	if !projectionEventTypes[typeCode] {
		typeCode = ""
	}
	tag := strings.ToLower(strings.TrimSpace(valueOrEmpty(event.Tag)))
	if !projectionEventTags[tag] {
		tag = ""
	}
	if (tag == "live" && (typeCode == "stream" || typeCode == "member")) ||
		(tag == "video" && typeCode == "video") {
		tag = ""
	}
	isWork := typeCode == "release" || typeCode == "video"
	context := ""
	if tag != "" {
		if isWork {
			context = "work"
		} else {
			context = "stream"
		}
	}
	var typePtr, tagPtr *string
	if typeCode != "" && (tag == "" || typeCode != "stream") {
		typePtr = strPtr(typeCode)
	}
	if tag != "" {
		tagPtr = strPtr(tag)
	}
	event.View = EventView{
		State:           eventProjectionState(*event, now),
		TypeCode:        typePtr,
		ActivityCode:    tagPtr,
		ActivityContext: context,
		IsWork:          isWork,
		CalendarEntries: eventCalendarEntries(*event),
	}
}

// ProjectEventView applies the public event rules to a standalone event read.
// It lets bounded readers that use a different query path share the dashboard
// contract without exposing the internal projection helpers.
func ProjectEventView(event DashboardEvent, now time.Time) EventView {
	applyEventView(&event, now)
	return event.View
}

func applyVideoView(video *Video, now time.Time) {
	if video == nil {
		return
	}
	phase := videoProjectionPhase(*video, now)
	form := videoProjectionForm(*video, now)
	context := "work"
	if strings.EqualFold(valueOrEmpty(video.Kind), "stream") {
		context = "stream"
	}
	video.View = VideoView{
		Phase:          phase,
		FormCode:       form,
		FocusEligible:  videoProjectionFocus(*video, now),
		TagContext:     context,
		LatestEligible: phase != "upcoming" && phase != "cancelled" && !videoShort(*video) && !gone(video.Availability),
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func projectionNowDate(now time.Time) string {
	return dateInJapan(now)
}

func eventProjectionUpcoming(event DashboardEvent, now time.Time) bool {
	if event.StartsAt != nil {
		start, err := time.Parse(time.RFC3339, *event.StartsAt)
		return err == nil && !start.Before(now)
	}
	return event.StartsOn >= projectionNowDate(now)
}

func projectionConfirmed(status *string) string {
	if status == nil {
		return ""
	}
	switch *status {
	case "live", "upcoming", "ended":
		return *status
	default:
		return ""
	}
}

func eventProjectionLive(event DashboardEvent, now time.Time) bool {
	typeCode := strings.ToLower(strings.TrimSpace(valueOrEmpty(event.EventType)))
	isStream := typeCode == "stream" || typeCode == "member"
	if !isStream && event.Source != "youtube" {
		return false
	}
	start, ok := projectionEventStart(event)
	if !ok || start.After(now) {
		return false
	}
	end, hasEnd := projectionEventEnd(event)
	if confirmed := projectionConfirmed(event.LiveStatus); confirmed != "" {
		return confirmed == "live" && (!hasEnd || end.After(now))
	}
	if hasEnd {
		return end.After(now)
	}
	return isStream && start.After(now.Add(-projectionLiveGrace))
}

func eventProjectionState(event DashboardEvent, now time.Time) string {
	if event.CancellationStatus == "manual_confirmed" {
		return "cancelled"
	}
	if event.CancellationStatus == "llm_suspected" {
		return "cancellation_review"
	}
	if cancelledStatus(event.Status) {
		return "cancelled"
	}
	if SaleEvent(event.EventType) {
		return saleProjectionState(event, now)
	}
	if eventProjectionLive(event, now) {
		return "live"
	}
	if projectionConfirmed(event.LiveStatus) == "upcoming" || eventProjectionUpcoming(event, now) {
		return "upcoming"
	}
	status := strings.ToLower(strings.TrimSpace(valueOrEmpty(event.Status)))
	if event.TimePrecision == "unknown" || status == "pending" || status == "tentative" || status == "unknown" ||
		status == "待确认" || status == "待补充" || status == "確認待ち" || status == "未定" {
		return "pending"
	}
	return "done"
}

func projectionEventStart(event DashboardEvent) (time.Time, bool) {
	if event.StartsAt != nil {
		value, err := time.Parse(time.RFC3339, *event.StartsAt)
		return value, err == nil
	}
	value, err := time.ParseInLocation("2006-01-02T15:04:05.999", event.StartsOn+"T23:59:59.999", time.FixedZone("JST", 9*60*60))
	return value, err == nil
}

func projectionEventEnd(event DashboardEvent) (time.Time, bool) {
	if event.EndsAt == nil {
		return time.Time{}, false
	}
	value, err := time.Parse(time.RFC3339, *event.EndsAt)
	return value, err == nil
}

func projectionEventTime(event DashboardEvent) time.Time {
	value, ok := projectionEventStart(event)
	if !ok {
		return time.Time{}
	}
	return value
}

func eventCalendarEntries(event DashboardEvent) []CalendarEntry {
	if !SaleEvent(event.EventType) {
		return []CalendarEntry{{StartsOn: event.StartsOn, StartsAt: event.StartsAt, TimePrecision: event.TimePrecision}}
	}
	closeDay := event.StartsOn
	if event.EndsOn != nil && *event.EndsOn != "" {
		closeDay = *event.EndsOn
	}
	closePrecision := "unknown"
	if event.EndsAt != nil {
		closePrecision = "exact"
	}
	closingMark := "closes"
	closing := CalendarEntry{SaleMark: &closingMark, StartsOn: closeDay, StartsAt: event.EndsAt, TimePrecision: closePrecision}
	deadline := closeDay == event.StartsOn && event.StartsAt == nil
	if deadline {
		return []CalendarEntry{closing}
	}
	openingMark := "opens"
	opening := CalendarEntry{SaleMark: &openingMark, StartsOn: event.StartsOn, StartsAt: event.StartsAt, TimePrecision: event.TimePrecision}
	if event.EndsOn != nil && *event.EndsOn > event.StartsOn {
		return []CalendarEntry{opening, closing}
	}
	return []CalendarEntry{opening}
}

func saleProjectionOpen(event DashboardEvent) time.Time {
	if event.EndsOn != nil && *event.EndsOn == event.StartsOn && event.StartsAt == nil {
		return time.Time{}
	}
	if event.StartsAt != nil {
		if value, err := time.Parse(time.RFC3339, *event.StartsAt); err == nil {
			return value
		}
	}
	value, _ := time.ParseInLocation("2006-01-02", event.StartsOn, time.FixedZone("JST", 9*60*60))
	return value
}

func saleProjectionClose(event DashboardEvent) time.Time {
	day := event.StartsOn
	if event.EndsOn != nil && *event.EndsOn != "" {
		day = *event.EndsOn
	}
	if event.EndsAt != nil {
		if value, err := time.Parse(time.RFC3339, *event.EndsAt); err == nil {
			return value
		}
	}
	value, _ := time.ParseInLocation("2006-01-02T15:04:05.999", day+"T23:59:59.999", time.FixedZone("JST", 9*60*60))
	return value
}

func saleProjectionState(event DashboardEvent, now time.Time) string {
	if now.Before(saleProjectionOpen(event)) {
		return "upcoming"
	}
	if now.Before(saleProjectionClose(event)) {
		return "open"
	}
	return "done"
}

func videoShort(video Video) bool {
	return video.IsShort != nil && *video.IsShort
}

func videoKind(video Video) string {
	return strings.ToLower(strings.TrimSpace(valueOrEmpty(video.Kind)))
}

func videoStart(video Video) (time.Time, bool) {
	if video.ScheduledAt == nil {
		return time.Time{}, false
	}
	value, err := time.Parse(time.RFC3339, *video.ScheduledAt)
	return value, err == nil
}

func videoEnd(video Video) (time.Time, bool) {
	if video.EndsAt == nil {
		return time.Time{}, false
	}
	value, err := time.Parse(time.RFC3339, *video.EndsAt)
	return value, err == nil
}

func videoProjectionFocus(video Video, now time.Time) bool {
	if video.IsCancelled {
		return false
	}
	start, hasStart := videoStart(video)
	end, hasEnd := videoEnd(video)
	confirmed := projectionConfirmed(video.LiveStatus)
	if confirmed == "live" {
		return !hasEnd || end.After(now)
	}
	if confirmed == "ended" {
		return false
	}
	if hasEnd {
		return end.After(now) && hasStart
	}
	if hasStart {
		return start.After(now.Add(-projectionLiveGrace))
	}
	return video.IsUpcoming
}

func videoProjectionPhase(video Video, now time.Time) string {
	if video.IsCancelled {
		return "cancelled"
	}
	start, hasStart := videoStart(video)
	end, hasEnd := videoEnd(video)
	confirmed := projectionConfirmed(video.LiveStatus)
	if confirmed == "live" {
		if hasEnd && !end.After(now) {
			return "past"
		}
		return "live"
	}
	if confirmed == "upcoming" {
		return "upcoming"
	}
	if confirmed == "ended" {
		return "past"
	}
	if !hasStart {
		if video.IsUpcoming {
			return "upcoming"
		}
		return "past"
	}
	if start.After(now) {
		return "upcoming"
	}
	if hasEnd {
		if end.After(now) {
			return "started"
		}
		return "past"
	}
	if videoKind(video) == "stream" && start.After(now.Add(-projectionLiveGrace)) {
		return "started"
	}
	return "past"
}

func videoProjectionForm(video Video, now time.Time) *string {
	form := ""
	if videoShort(video) {
		form = "short"
	} else {
		switch videoKind(video) {
		case "stream":
			if video.IsUpcoming || projectionConfirmed(video.LiveStatus) == "upcoming" {
				form = "upcoming"
			} else if videoProjectionPhase(video, now) == "started" || videoProjectionPhase(video, now) == "live" {
				form = "stream"
			} else {
				form = "archive"
			}
		case "premiere":
			phase := videoProjectionPhase(video, now)
			if phase == "upcoming" || phase == "live" || phase == "started" {
				form = "premiere"
			}
		default:
			form = "video"
		}
	}
	if form == "" {
		return nil
	}
	return strPtr(form)
}

func videoSortTime(video Video) time.Time {
	if value, ok := videoStart(video); ok {
		return value
	}
	if video.PublishedAt != nil {
		value, err := time.Parse(time.RFC3339, *video.PublishedAt)
		if err == nil {
			return value
		}
	}
	return time.Time{}
}

func projectionEventSort(events []DashboardEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		a, b := projectionEventTime(events[i]), projectionEventTime(events[j])
		return a.Before(b) || a.Equal(b) && events[i].ID < events[j].ID
	})
}

func projectionVideoSort(videos []Video, newest bool) {
	sort.SliceStable(videos, func(i, j int) bool {
		a, b := videoSortTime(videos[i]), videoSortTime(videos[j])
		if newest {
			return b.Before(a) || b.Equal(a) && videos[i].ID < videos[j].ID
		}
		return a.Before(b) || a.Equal(b) && videos[i].ID < videos[j].ID
	})
}

func projectionLeadingEvent(events []DashboardEvent, now time.Time) *DashboardEvent {
	for index := range events {
		start, ok := projectionEventStart(events[index])
		if !ok {
			continue
		}
		end, hasEnd := projectionEventEnd(events[index])
		if !start.After(now) && !hasEnd && projectionConfirmed(events[index].LiveStatus) != "live" {
			yields := false
			for _, later := range events[index+1:] {
				next, valid := projectionEventStart(later)
				if valid && next.After(start) && next.Sub(now) <= projectionLiveYield {
					yields = true
					break
				}
			}
			if yields {
				continue
			}
		}
		if !hasEnd || end.After(now) || start.After(now) || projectionConfirmed(events[index].LiveStatus) == "live" {
			copy := events[index]
			return &copy
		}
	}
	return nil
}

func projectionLeadingVideo(videos []Video, now time.Time) *Video {
	for index := range videos {
		start, ok := videoStart(videos[index])
		if !ok {
			if videos[index].IsUpcoming {
				copy := videos[index]
				return &copy
			}
			continue
		}
		end, hasEnd := videoEnd(videos[index])
		if !start.After(now) && !hasEnd && projectionConfirmed(videos[index].LiveStatus) != "live" {
			yields := false
			for _, later := range videos[index+1:] {
				next, valid := videoStart(later)
				if valid && next.After(start) && next.Sub(now) <= projectionLiveYield {
					yields = true
					break
				}
			}
			if yields {
				continue
			}
		}
		if !hasEnd || end.After(now) || start.After(now) || projectionConfirmed(videos[index].LiveStatus) == "live" {
			copy := videos[index]
			return &copy
		}
	}
	return nil
}

func projectionLiveLink(event DashboardEvent) bool {
	if event.LiveStatus != nil && *event.LiveStatus == "live" {
		return true
	}
	for _, link := range event.Links {
		if link.LiveStatus != nil && *link.LiveStatus == "live" {
			return true
		}
	}
	return false
}

type projectionCandidate struct {
	event  *DashboardEvent
	video  *Video
	live   bool
	start  time.Time
	end    time.Time
	hasEnd bool
}

func projectionLeadingCandidate(candidates []projectionCandidate, now time.Time) *projectionCandidate {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].start.Before(candidates[j].start) || candidates[i].start.Equal(candidates[j].start) && candidateID(candidates[i]) < candidateID(candidates[j])
	})
	for index := range candidates {
		candidate := candidates[index]
		if !candidate.start.After(now) && !candidate.hasEnd && !candidate.live {
			yields := false
			for _, later := range candidates[index+1:] {
				if later.start.After(candidate.start) && later.start.Sub(now) <= projectionLiveYield {
					yields = true
					break
				}
			}
			if yields {
				continue
			}
		}
		return &candidate
	}
	return nil
}

func candidateID(candidate projectionCandidate) string {
	if candidate.video != nil {
		return candidate.video.ID
	}
	if candidate.event != nil {
		return candidate.event.ID
	}
	return ""
}

func projectionFutureTransition(events []DashboardEvent, videos []Video, now time.Time) *string {
	var next time.Time
	consider := func(value time.Time, ok bool) {
		if !ok || !value.After(now) || !next.IsZero() && !value.Before(next) {
			return
		}
		next = value
	}
	for _, event := range events {
		if SaleEvent(event.EventType) {
			open := saleProjectionOpen(event)
			consider(open, !open.IsZero())
			consider(saleProjectionClose(event), true)
		} else {
			start, ok := projectionEventStart(event)
			if event.StartsAt == nil {
				parsed, err := time.ParseInLocation("2006-01-02", event.StartsOn, time.FixedZone("JST", 9*60*60))
				start, ok = parsed, err == nil
			}
			consider(start, ok)
			if ok && event.StartsAt != nil && (strings.EqualFold(valueOrEmpty(event.EventType), "stream") || strings.EqualFold(valueOrEmpty(event.EventType), "member")) && event.EndsAt == nil {
				consider(start.Add(projectionLiveGrace), true)
			}
		}
		end, ok := projectionEventEnd(event)
		consider(end, ok)
	}
	for _, video := range videos {
		start, ok := videoStart(video)
		consider(start, ok)
		end, hasEnd := videoEnd(video)
		if ok && !hasEnd && videoKind(video) == "stream" {
			consider(start.Add(projectionLiveGrace), true)
		}
		consider(end, hasEnd)
	}
	if next.IsZero() {
		return nil
	}
	value := next.UTC().Format(time.RFC3339Nano)
	return &value
}

func applyDashboardProjection(dashboard *Dashboard, now time.Time) {
	if dashboard == nil {
		return
	}
	for index := range dashboard.Events {
		applyEventView(&dashboard.Events[index], now)
	}
	for index := range dashboard.Videos {
		applyVideoView(&dashboard.Videos[index], now)
	}

	focusEvents := make([]DashboardEvent, 0, len(dashboard.Events))
	for _, event := range dashboard.Events {
		if event.View.State != "cancelled" && event.View.State != "cancellation_review" && !SaleEvent(event.EventType) && (eventProjectionUpcoming(event, now) || eventProjectionLive(event, now)) {
			focusEvents = append(focusEvents, event)
		}
	}
	projectionEventSort(focusEvents)
	focusVideos := make([]Video, 0, len(dashboard.Videos))
	for _, video := range dashboard.Videos {
		if video.View.FocusEligible {
			focusVideos = append(focusVideos, video)
		}
	}
	projectionVideoSort(focusVideos, false)

	nextEvent := projectionLeadingEvent(focusEvents, now)
	nextVideo := projectionLeadingVideo(focusVideos, now)
	dashboard.Summary.NextEvent = nextEvent
	dashboard.Summary.NextStream = nextVideo

	latest := make([]Video, 0, len(dashboard.Videos))
	for _, video := range dashboard.Videos {
		video.View.LatestEligible = video.View.Phase != "upcoming" && video.View.Phase != "cancelled" && (!videoShort(video) || dashboard.Meta.FeaturedVideoID != nil && video.ID == *dashboard.Meta.FeaturedVideoID) && !gone(video.Availability)
		if video.View.LatestEligible {
			latest = append(latest, video)
		}
	}
	projectionVideoSort(latest, true)
	if len(latest) > 0 {
		copy := latest[0]
		dashboard.Summary.LatestVideo = &copy
	} else {
		dashboard.Summary.LatestVideo = nil
	}

	for index := range dashboard.Events {
		for _, candidate := range focusEvents {
			if candidate.ID == dashboard.Events[index].ID {
				dashboard.Events[index].View = candidate.View
				break
			}
		}
	}
	for index := range dashboard.Videos {
		for _, candidate := range focusVideos {
			if candidate.ID == dashboard.Videos[index].ID {
				dashboard.Videos[index].View = candidate.View
				break
			}
		}
	}

	shelf := DashboardShelf{Reservations: []string{}, Works: []string{}, Archives: []string{}, Shorts: []string{}}
	featuredID := dashboard.Meta.FeaturedVideoID
	var featured *Video
	for index := range dashboard.Videos {
		video := &dashboard.Videos[index]
		if featuredID != nil && video.ID == *featuredID {
			copy := *video
			featured = &copy
			continue
		}
		phase := video.View.Phase
		if phase == "cancelled" {
			start, ok := videoStart(*video)
			if ok && start.After(now) {
				shelf.Reservations = append(shelf.Reservations, video.ID)
			}
			continue
		}
		if phase != "past" {
			shelf.Reservations = append(shelf.Reservations, video.ID)
		} else if videoShort(*video) {
			shelf.Shorts = append(shelf.Shorts, video.ID)
		} else if videoKind(*video) == "stream" {
			shelf.Archives = append(shelf.Archives, video.ID)
		} else {
			shelf.Works = append(shelf.Works, video.ID)
		}
	}
	orderIDs := func(ids []string, newest bool) {
		sort.SliceStable(ids, func(i, j int) bool {
			var a, b Video
			for _, video := range dashboard.Videos {
				if video.ID == ids[i] {
					a = video
				}
				if video.ID == ids[j] {
					b = video
				}
			}
			ta, tb := videoSortTime(a), videoSortTime(b)
			if newest {
				return tb.Before(ta) || tb.Equal(ta) && a.ID < b.ID
			}
			return ta.Before(tb) || ta.Equal(tb) && a.ID < b.ID
		})
	}
	orderIDs(shelf.Reservations, false)
	orderIDs(shelf.Works, true)
	orderIDs(shelf.Archives, true)
	orderIDs(shelf.Shorts, true)
	shelf.LeadIsFeatured = featured != nil
	if featured != nil {
		shelf.LeadID = strPtr(featured.ID)
		removeID(&shelf.Works, featured.ID)
	} else {
		lead := ""
		for _, id := range shelf.Works {
			video := findVideo(dashboard.Videos, id)
			if video != nil && !gone(video.Availability) && (videoKind(*video) == "premiere" || strings.EqualFold(valueOrEmpty(video.Tag), "song")) {
				lead = id
				break
			}
		}
		if lead == "" {
			for _, id := range shelf.Works {
				video := findVideo(dashboard.Videos, id)
				if video != nil && !gone(video.Availability) {
					lead = id
					break
				}
			}
		}
		if lead != "" {
			shelf.LeadID = strPtr(lead)
			removeID(&shelf.Works, lead)
		}
	}
	for index := range dashboard.Videos {
		dashboard.Videos[index].View.ShelfGroup = ""
		for _, id := range shelf.Reservations {
			if id == dashboard.Videos[index].ID {
				dashboard.Videos[index].View.ShelfGroup = "reservation"
			}
		}
		for _, id := range shelf.Works {
			if id == dashboard.Videos[index].ID {
				dashboard.Videos[index].View.ShelfGroup = "work"
			}
		}
		for _, id := range shelf.Archives {
			if id == dashboard.Videos[index].ID {
				dashboard.Videos[index].View.ShelfGroup = "archive"
			}
		}
		for _, id := range shelf.Shorts {
			if id == dashboard.Videos[index].ID {
				dashboard.Videos[index].View.ShelfGroup = "short"
			}
		}
		if dashboard.Videos[index].View.ShelfGroup == "" && featuredID != nil && dashboard.Videos[index].ID == *featuredID {
			dashboard.Videos[index].View.ShelfGroup = projectionShelfGroup(dashboard.Videos[index], now)
		}
		if dashboard.Videos[index].View.ShelfGroup == "" && shelf.LeadID != nil && dashboard.Videos[index].ID == *shelf.LeadID {
			dashboard.Videos[index].View.ShelfGroup = "work"
		}
	}

	projection := DashboardProjection{
		Version:          projectionVersion,
		NextTransitionAt: projectionFutureTransition(dashboard.Events, dashboard.Videos, now),
		Focus:            DashboardFocusProjection{EventIDs: idsOfEvents(focusEvents), VideoIDs: idsOfVideos(focusVideos)},
		Sales:            projectionSales(dashboard.Events, now),
		Shelf:            shelf,
	}
	projection.Spotlight = projectionSpotlight(dashboard, now)
	dashboard.Projection = projection
}

func projectionShelfGroup(video Video, now time.Time) string {
	if video.View.Phase == "cancelled" {
		start, ok := videoStart(video)
		if !ok || !start.After(now) {
			return ""
		}
	}
	if video.View.Phase != "past" {
		return "reservation"
	}
	if videoShort(video) {
		return "short"
	}
	if videoKind(video) == "stream" {
		return "archive"
	}
	return "work"
}

func idsOfEvents(events []DashboardEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}
func idsOfVideos(videos []Video) []string {
	ids := make([]string, 0, len(videos))
	for _, video := range videos {
		ids = append(ids, video.ID)
	}
	return ids
}

func projectionSales(events []DashboardEvent, now time.Time) []string {
	listed := make([]DashboardEvent, 0)
	for _, event := range events {
		if !SaleEvent(event.EventType) || event.View.State == "cancelled" || event.View.State == "cancellation_review" {
			continue
		}
		open := saleProjectionOpen(event)
		if event.View.State == "open" || event.View.State == "upcoming" && !open.IsZero() && !open.After(now.Add(projectionSaleLookahead)) {
			listed = append(listed, event)
		}
	}
	sort.SliceStable(listed, func(i, j int) bool {
		openI, openJ := listed[i].View.State == "open", listed[j].View.State == "open"
		if openI != openJ {
			return openI
		}
		a, b := saleProjectionClose(listed[i]), saleProjectionClose(listed[j])
		if !openI {
			a, b = saleProjectionOpen(listed[i]), saleProjectionOpen(listed[j])
		}
		return a.Before(b) || a.Equal(b) && listed[i].ID < listed[j].ID
	})
	ids := make([]string, 0, len(listed))
	for _, event := range listed {
		ids = append(ids, event.ID)
	}
	if len(ids) > 3 {
		ids = ids[:3]
	}
	return ids
}

func projectionSpotlight(dashboard *Dashboard, now time.Time) *DashboardSpotlight {
	candidates := make([]projectionCandidate, 0, 3)
	if dashboard.LiveNow != nil {
		start, _ := time.Parse(time.RFC3339, dashboard.LiveNow.StartedAt)
		candidates = append(candidates, projectionCandidate{live: true, start: start})
	}
	if dashboard.Summary.NextStream != nil && dashboard.Summary.NextStream.ScheduledAt != nil {
		video := dashboard.Summary.NextStream
		start, ok := videoStart(*video)
		if ok {
			end, hasEnd := videoEnd(*video)
			candidates = append(candidates, projectionCandidate{video: video, start: start, end: end, hasEnd: hasEnd, live: projectionConfirmed(video.LiveStatus) == "live"})
		}
	}
	if dashboard.Summary.NextEvent != nil {
		event := dashboard.Summary.NextEvent
		paired := false
		if dashboard.Summary.NextStream != nil && event.URL != nil && dashboard.Summary.NextStream.URL != "" && streamIdentity(*event.URL) == streamIdentity(dashboard.Summary.NextStream.URL) {
			paired = true
		}
		if !paired {
			start, ok := projectionEventStart(*event)
			if ok {
				end, hasEnd := projectionEventEnd(*event)
				candidates = append(candidates, projectionCandidate{event: event, start: start, end: end, hasEnd: hasEnd, live: projectionLiveLink(*event)})
			}
		} else if len(candidates) > 0 {
			for index := range candidates {
				if candidates[index].video != nil {
					candidates[index].event = event
					candidates[index].live = candidates[index].live || projectionLiveLink(*event)
				}
			}
		}
	}
	lead := projectionLeadingCandidate(candidates, now)
	if lead == nil {
		if dashboard.Summary.LatestVideo == nil {
			return nil
		}
		return &DashboardSpotlight{Mode: "latest", VideoID: strPtr(dashboard.Summary.LatestVideo.ID)}
	}
	spotlight := &DashboardSpotlight{Mode: "next", LiveNow: lead.live && lead.event == nil && lead.video == nil, IsLive: lead.live || !lead.start.After(now)}
	if lead.event != nil {
		spotlight.EventID = strPtr(lead.event.ID)
	}
	if lead.video != nil {
		spotlight.VideoID = strPtr(lead.video.ID)
	}
	if lead.event != nil && lead.video != nil && strings.EqualFold(valueOrEmpty(lead.event.EventType), "member") && (lead.video.Availability == nil || *lead.video.Availability != "members" && *lead.video.Availability != "private" && *lead.video.Availability != "removed") {
		activity := lead.video.Tag != nil || lead.video != nil && (lead.event.View.ActivityCode != nil && (*lead.event.View.ActivityCode == "game" || *lead.event.View.ActivityCode == "talk" || *lead.event.View.ActivityCode == "song"))
		spotlight.MembersChip = activity
	}
	if lead.video == nil || lead.video.ThumbnailURL == nil {
		latest := dashboard.Summary.LatestVideo
		if latest != nil && (lead.video == nil || latest.URL != lead.video.URL) && (lead.event == nil || lead.event.URL == nil || latest.URL != *lead.event.URL) {
			spotlight.CompanionVideoID = strPtr(latest.ID)
		}
	}
	return spotlight
}

func findVideo(videos []Video, id string) *Video {
	for index := range videos {
		if videos[index].ID == id {
			return &videos[index]
		}
	}
	return nil
}
func removeID(ids *[]string, target string) {
	for index, id := range *ids {
		if id == target {
			*ids = append((*ids)[:index], (*ids)[index+1:]...)
			return
		}
	}
}
