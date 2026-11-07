package bilibili

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Room is the account's live room as the batch status endpoint reports it.
type Room struct {
	RoomID string
	Title  string
	// LiveStatus is 0 off air, 1 live and 2 a rotation of recordings, which
	// is not a broadcast.
	LiveStatus int
	// LiveTime is the actual start of the current broadcast, RFC 3339 in
	// UTC, or "" when the room is not live.
	LiveTime string
}

// RoomLive is the only status that is a broadcast.
const RoomLive = 1

// RoomURL is the public link of a live room. The room ID is learned from
// each live check, never configured.
func RoomURL(roomID string) string { return "https://live.bilibili.com/" + roomID }

var roomPattern = mid

// ParseLiveStatus reads get_status_info_by_uids for one account.
func ParseLiveStatus(data json.RawMessage, account string) (Room, error) {
	var rooms map[string]struct {
		Title      string  `json:"title"`
		RoomID     flexInt `json:"room_id"`
		LiveStatus *int    `json:"live_status"`
		LiveTime   flexInt `json:"live_time"`
	}
	if json.Unmarshal(data, &rooms) != nil {
		return Room{}, &Error{"bilibili_invalid_response"}
	}
	r, ok := rooms[account]
	room := Room{RoomID: strconv.FormatInt(int64(r.RoomID), 10), Title: strings.TrimSpace(r.Title)}
	if !ok || r.LiveStatus == nil || !roomPattern.MatchString(room.RoomID) {
		return Room{}, &Error{"bilibili_invalid_response"}
	}
	room.LiveStatus = *r.LiveStatus
	if runes := []rune(room.Title); len(runes) > 200 {
		room.Title = string(runes[:200])
	}
	if room.LiveStatus == RoomLive && r.LiveTime > 0 {
		room.LiveTime = time.Unix(int64(r.LiveTime), 0).UTC().Format(time.RFC3339)
	}
	return room, nil
}

// LiveStatus asks api.live.bilibili.com about the account's room in one
// request, without any cookie.
func (c *Client) LiveStatus(ctx context.Context, account string) (Room, error) {
	if !mid.MatchString(account) {
		return Room{}, &Error{"bilibili_invalid_account"}
	}
	body, err := c.fetch(ctx, liveHost, "/room/v1/Room/get_status_info_by_uids", "uids%5B%5D="+account, "https://live.bilibili.com/", "application/json", nil, false)
	if err != nil {
		return Room{}, err
	}
	data, err := envelope(body)
	if err != nil {
		return Room{}, err
	}
	return ParseLiveStatus(data, account)
}
