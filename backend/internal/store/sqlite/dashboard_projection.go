package sqlite

import (
	"sort"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/japantime"
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
	return japantime.Date(now)
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
	end, hasEnd := projectionEventEnd(event)
	confirmed := projectionConfirmed(event.LiveStatus)
	// A linked broadcast confirmed on air is live, even before the entry's
	// scheduled start or on a date-only entry.
	if confirmed == "live" {
		return !hasEnd || end.After(now)
	}
	// A date-only entry has no time to have gone on air at.
	if event.StartsAt == nil {
		return false
	}
	start, ok := projectionEventStart(event)
	if !ok || start.After(now) || confirmed != "" {
		return false
	}
	if hasEnd {
		return end.After(now)
	}
	return isStream && start.After(now.Add(-projectionLiveGrace))
}

func eventProjectionState(event DashboardEvent, now time.Time) string {
	// A cancelled status or an operator's confirmation is definitive; a
	// model's suspicion only asks for review.
	if event.Cancelled() {
		return "cancelled"
	}
	if event.CancellationStatus == "llm_suspected" {
		return "cancellation_review"
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
	if eventProjectionOngoing(event, now) {
		return "ongoing"
	}
	return "done"
}

// eventProjectionOngoing reports an entry without a broadcast on air that is
// between its start time and its end, such as an offline event.
func eventProjectionOngoing(event DashboardEvent, now time.Time) bool {
	if event.StartsAt == nil || SaleEvent(event.EventType) || projectionConfirmed(event.LiveStatus) == "ended" {
		return false
	}
	start, ok := projectionEventStart(event)
	end, hasEnd := projectionEventEnd(event)
	return ok && hasEnd && !start.After(now) && end.After(now)
}

func projectionEventStart(event DashboardEvent) (time.Time, bool) {
	if event.StartsAt != nil {
		value, err := time.Parse(time.RFC3339, *event.StartsAt)
		return value, err == nil
	}
	value, err := japantime.ParseLocal("2006-01-02T15:04:05.999", event.StartsOn+"T23:59:59.999")
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
	// A deadline has a close date and no known opening: the close date is
	// the start date and there is no start time. A sale whose close is not
	// known shows its opening alone.
	deadline := event.EndsOn != nil && closeDay == event.StartsOn && event.StartsAt == nil
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
	value, _ := japantime.ParseDate(event.StartsOn)
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
	value, _ := japantime.ParseLocal("2006-01-02T15:04:05.999", day+"T23:59:59.999")
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
		// Only a stream keeps the live grace after its start, as its phase
		// does; a premiere without an end or a confirmed broadcast has aired.
		if videoKind(video) == "stream" {
			return start.After(now.Add(-projectionLiveGrace))
		}
		return start.After(now)
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

func projectionNewestSort(videos []Video) {
	sort.SliceStable(videos, func(i, j int) bool {
		a, b := videoSortTime(videos[i]), videoSortTime(videos[j])
		return b.Before(a) || b.Equal(a) && videos[i].ID < videos[j].ID
	})
}

// projectionFocusSort orders focus videos by start, earliest first. A
// waiting room without a start follows every scheduled video: its
// publication time says nothing about when it goes on air.
func projectionFocusSort(videos []Video) {
	sort.SliceStable(videos, func(i, j int) bool {
		_, scheduledI := videoStart(videos[i])
		_, scheduledJ := videoStart(videos[j])
		if scheduledI != scheduledJ {
			return scheduledI
		}
		a, b := videoSortTime(videos[i]), videoSortTime(videos[j])
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
		// A started entry without an end, or an ongoing one, yields to a
		// later entry about to start.
		if !start.After(now) && (!hasEnd || events[index].View.State == "ongoing") && projectionConfirmed(events[index].LiveStatus) != "live" {
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

// projectionLeadingVideo picks the next stream from the ordered focus
// videos: a video confirmed on air first, then the scheduled videos with
// the live yield, and a waiting room without a start only when no scheduled
// video leads.
func projectionLeadingVideo(videos []Video, now time.Time) *Video {
	for index := range videos {
		if projectionConfirmed(videos[index].LiveStatus) == "live" {
			copy := videos[index]
			return &copy
		}
	}
	var unscheduled *Video
	for index := range videos {
		start, ok := videoStart(videos[index])
		if !ok {
			if videos[index].IsUpcoming && unscheduled == nil {
				unscheduled = &videos[index]
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
	if unscheduled != nil {
		copy := *unscheduled
		return &copy
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
	// ongoing is an event without a broadcast between its start and its end.
	ongoing bool
}

func projectionLeadingCandidate(candidates []projectionCandidate, now time.Time) *projectionCandidate {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].start.Before(candidates[j].start) || candidates[i].start.Equal(candidates[j].start) && candidateID(candidates[i]) < candidateID(candidates[j])
	})
	for index := range candidates {
		candidate := candidates[index]
		if !candidate.start.After(now) && (!candidate.hasEnd || candidate.ongoing) && !candidate.live {
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
				// A date-only entry changes when its day begins and ends.
				parsed, err := japantime.ParseDate(event.StartsOn)
				start, ok = parsed, err == nil
				consider(start.AddDate(0, 0, 1), ok)
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
		video := &dashboard.Videos[index]
		applyVideoView(video, now)
		// The Featured video may be the latest video even as a Short.
		if dashboard.Meta.FeaturedVideoID != nil && video.ID == *dashboard.Meta.FeaturedVideoID && videoShort(*video) {
			video.View.LatestEligible = video.View.Phase != "upcoming" && video.View.Phase != "cancelled" && !gone(video.Availability)
		}
	}

	focusEvents := make([]DashboardEvent, 0, len(dashboard.Events))
	for _, event := range dashboard.Events {
		if event.View.State != "cancelled" && event.View.State != "cancellation_review" && !SaleEvent(event.EventType) && (eventProjectionUpcoming(event, now) || eventProjectionLive(event, now) || event.View.State == "ongoing") {
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
	projectionFocusSort(focusVideos)

	nextEvent := projectionLeadingEvent(focusEvents, now)
	nextVideo := projectionLeadingVideo(focusVideos, now)
	dashboard.Summary.NextEvent = nextEvent
	dashboard.Summary.NextStream = nextVideo

	latest := make([]Video, 0, len(dashboard.Videos))
	for _, video := range dashboard.Videos {
		if video.View.LatestEligible {
			latest = append(latest, video)
		}
	}
	projectionNewestSort(latest)
	if len(latest) > 0 {
		copy := latest[0]
		dashboard.Summary.LatestVideo = &copy
	} else {
		dashboard.Summary.LatestVideo = nil
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
				candidates = append(candidates, projectionCandidate{event: event, start: start, end: end, hasEnd: hasEnd, live: projectionLiveLink(*event), ongoing: event.View.State == "ongoing"})
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
	// A started entry is on air unless it is an ongoing event without a
	// broadcast; a date-only entry starts only at the end of its day.
	isLive := lead.live || !lead.start.After(now) && !lead.ongoing
	spotlight := &DashboardSpotlight{Mode: "next", LiveNow: lead.live && lead.event == nil && lead.video == nil, IsLive: isLive}
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
