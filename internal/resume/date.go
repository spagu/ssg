package resume

import (
	"regexp"
	"time"
)

// Date is an ISO 8601 date at the precision the author knew it: a year
// ("2019"), a month ("2019-06") or a day ("2019-06-15").
type Date string

// datePattern is the schema's own pattern for a date.
var datePattern = regexp.MustCompile(`^([1-2][0-9]{3}-[0-1][0-9]-[0-3][0-9]|[1-2][0-9]{3}-[0-1][0-9]|[1-2][0-9]{3})$`)

// dateLayouts are the Go layouts for each precision, by string length.
var dateLayouts = map[int]string{4: "2006", 7: "2006-01", 10: "2006-01-02"}

// Valid reports whether the date matches the schema and names a real day
// (no month 13, no 31 April).
func (d Date) Valid() bool {
	_, ok := d.Time()
	return ok
}

// Time is the first instant the date covers, in UTC.
func (d Date) Time() (time.Time, bool) {
	s := string(d)
	if !datePattern.MatchString(s) {
		return time.Time{}, false
	}
	t, err := time.Parse(dateLayouts[len(s)], s)
	return t, err == nil
}

// Before reports whether d ends before other begins, compared at the
// coarser of the two precisions: "2020" is not before "2020-06".
func (d Date) Before(other Date) bool {
	a, b := string(d), string(other)
	n := min(len(a), len(b))
	return a[:n] < b[:n]
}
