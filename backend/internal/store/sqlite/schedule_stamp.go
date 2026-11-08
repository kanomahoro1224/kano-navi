package sqlite

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	moderncsqlite "modernc.org/sqlite"
)

// ScheduleMediaKeys are the raw_json properties a post declares its images
// under, in the order the schedule extractor reads them.
var ScheduleMediaKeys = []string{"mediaUrls", "media_urls", "mediaURLs"}

// ScheduleQuote is the part of a stored quote projection the schedule
// extractor reads.
type ScheduleQuote struct {
	AuthorHandle string `json:"authorHandle"`
	AuthorName   string `json:"authorName"`
	Text         string `json:"text"`
	Unavailable  bool   `json:"unavailable"`
}

// DecodeScheduleQuote reads a stored quote projection; ok is false when the
// text is not one.
func DecodeScheduleQuote(text string) (quote ScheduleQuote, ok bool) {
	return quote, json.Unmarshal([]byte(text), &quote) == nil
}

// ScheduleInputStampSQL is the input stamp of the shown post aliased alias
// for one detection stage. It binds two parameters, the detection type and
// the extractor version, and covers every input of the extraction identity
// and the image check: the source, text, publication time, URL, declared
// and linked images with their cache state, and the shown quote.
func ScheduleInputStampSQL(alias string) string {
	return fmt.Sprintf(`schedule_input_stamp(?, ?, %[1]s.source, %[1]s.text, %[1]s.published_at, %[1]s.url,
		%[1]s.media_url, %[1]s.raw_json, CASE WHEN %[1]s.quote_hidden_at IS NULL THEN %[1]s.quote_json END,
		(SELECT json_group_array(json_array(ml.media_id, ma.status, ma.sha256, ma.mime_type, ma.cache_path) ORDER BY ml.position, ml.id)
			FROM media_links ml LEFT JOIN media_assets ma ON ma.id = ml.media_id
			WHERE ml.owner_type = 'post' AND ml.owner_id = %[1]s.id AND ml.role = 'post-image'))`, alias)
}

const scheduleStampArgs = 10

// scheduleInputStamp hashes the stamp arguments. Each one is written with
// its type and length, so no two argument lists share an encoding. The
// stored source record and the quote are read the way the extractor reads
// them, so every difference the extractor sees changes the stamp; a record
// that is not a JSON object is hashed whole.
func scheduleInputStamp(args []driver.Value) (string, error) {
	if len(args) != scheduleStampArgs {
		return "", fmt.Errorf("schedule_input_stamp takes %d arguments", scheduleStampArgs)
	}
	hash := sha256.New()
	write := func(tag byte, data []byte) {
		var header [9]byte
		header[0] = tag
		binary.BigEndian.PutUint64(header[1:], uint64(len(data)))
		hash.Write(header[:])
		hash.Write(data)
	}
	value := func(arg driver.Value) {
		switch typed := arg.(type) {
		case nil:
			write('n', nil)
		case string:
			write('s', []byte(typed))
		case []byte:
			write('s', typed)
		case int64:
			write('i', binary.BigEndian.AppendUint64(nil, uint64(typed)))
		case float64:
			write('f', binary.BigEndian.AppendUint64(nil, math.Float64bits(typed)))
		case bool:
			if typed {
				write('b', []byte{1})
			} else {
				write('b', []byte{0})
			}
		default:
			write('s', []byte(fmt.Sprint(typed)))
		}
	}
	text := func(arg driver.Value) (string, bool) {
		switch typed := arg.(type) {
		case string:
			return typed, true
		case []byte:
			return string(typed), true
		}
		return "", false
	}
	write('v', []byte("schedule-input-stamp/1"))
	const raw, quote = 7, 8
	for i, arg := range args {
		switch i {
		case raw:
			// The declared image lists, the last spelling of each property
			// winning as it does for the extractor.
			stored, isText := text(arg)
			var members map[string]json.RawMessage
			if !isText || stored == "" {
				value(arg)
				continue
			}
			if json.Unmarshal([]byte(stored), &members) != nil {
				write('r', []byte(stored))
				continue
			}
			write('o', nil)
			for _, key := range ScheduleMediaKeys {
				if member, ok := members[key]; ok {
					write('k', []byte(key))
					write('j', member)
				}
			}
		case quote:
			stored, isText := text(arg)
			if !isText || stored == "" {
				value(arg)
				continue
			}
			decoded, ok := DecodeScheduleQuote(stored)
			if !ok {
				write('q', nil)
				continue
			}
			write('Q', nil)
			value(decoded.AuthorHandle)
			value(decoded.AuthorName)
			value(decoded.Text)
			value(decoded.Unavailable)
		default:
			value(arg)
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// schedule_input_stamp lets the candidate query compare a post's stored
// settled stamp with its current input without reading the post into Go.
func init() {
	if err := moderncsqlite.RegisterDeterministicScalarFunction("schedule_input_stamp", scheduleStampArgs, func(_ *moderncsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		return scheduleInputStamp(args)
	}); err != nil {
		panic(err)
	}
}
