// Package japantime is the backend's one source for Japan time. Stored
// instants stay UTC ISO strings; Japan dates (YYYY-MM-DD) and Japan wall-clock
// times are read and written through this package.
package japantime

import "time"

// Zone is Japan Standard Time. Japan has no daylight saving time, so a fixed
// UTC+9 zone matches Asia/Tokyo and needs no tzdata.
var Zone = time.FixedZone("JST", 9*60*60)

// Date is the Japan calendar day of t as YYYY-MM-DD.
func Date(t time.Time) string { return t.In(Zone).Format(time.DateOnly) }

// ParseDate reads a YYYY-MM-DD Japan date as its midnight in Japan.
func ParseDate(value string) (time.Time, error) {
	return time.ParseInLocation(time.DateOnly, value, Zone)
}

// ParseLocal reads a Japan wall-clock time that carries no offset.
func ParseLocal(layout, value string) (time.Time, error) {
	return time.ParseInLocation(layout, value, Zone)
}
