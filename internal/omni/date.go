package omni

import (
	"fmt"
	"time"
)

// dateOnly is the YYYY-MM-DD layout; in a write it means 00:00 local time.
const dateOnly = "2006-01-02"

// date is a client-sent date, parsed. Whether it names a whole day or an
// instant is known from construction; the client's text is kept, because the
// scripts and error messages take it verbatim. The zero value is an absent date.
type date struct {
	text    string
	t       time.Time
	dayOnly bool
}

// parseDate parses the accepted forms: YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local
// time) or RFC 3339 with an offset. The empty string is the absent date.
func parseDate(field, s string) (date, error) {
	if s == "" {
		return date{}, nil
	}
	if t, err := time.ParseInLocation(dateOnly, s, time.Local); err == nil {
		return date{text: s, t: t, dayOnly: true}, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local); err == nil {
		return date{text: s, t: t}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return date{text: s, t: t}, nil
	}
	return date{}, fmt.Errorf("%s: %q is not YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS or an RFC 3339 timestamp with offset (Z, +01:00)", field, s)
}

func (d date) IsZero() bool { return d.text == "" }

// bound is the full timestamp d stands for as one end of a range, so the
// scripts compare instants and never expand a date-only value themselves. It
// is the one owner of the rule: a date-only value covers its whole local day,
// so a lower bound starts at 00:00:00 and an upper bound (end) ends at 23:59:59
// of it. An instant is its own bound.
func (d date) bound(end bool) date {
	if !d.dayOnly {
		return d
	}
	y, m, day := d.t.Date()
	if end {
		return date{text: d.text + "T23:59:59", t: time.Date(y, m, day, 23, 59, 59, 0, time.Local)}
	}
	return date{text: d.text + "T00:00:00", t: d.t}
}
