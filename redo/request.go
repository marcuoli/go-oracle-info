package redo

import (
	"time"

	"github.com/marcuoli/go-sqltemplate/reference"
)

// Filter selects reporting INST_ID values. Empty selects all visible instances;
// this is not a claim of complete physical RAC membership.
type Filter struct{ Instances []int64 }

// Request supplies admitted version, instance selection and execution reference.
type Request struct {
	OracleMajor int
	Filter      Filter
	Reference   reference.Reference
}

// HistoryFilter selects a finite database-local DATE interval [From,Until).
// Timezones are not converted: wall-clock fields are used verbatim, to seconds.
// Bounds must be whole seconds in one location, increasing and at most 31 days.
type HistoryFilter struct {
	Instances   []int64
	From, Until time.Time
}

// HistoryRequest admits an explicit local-wall-clock interval and SQL reference.
type HistoryRequest struct {
	OracleMajor int
	Filter      HistoryFilter
	Reference   reference.Reference
}

func historyBounds(filter HistoryFilter) (string, string, error) {
	if filter.From.Year() < 1 || filter.Until.Year() > 9999 || filter.Until.Year() < 1 || filter.From.Year() > 9999 {
		return "", "", ErrInvalidRequest
	}
	if filter.From.IsZero() || filter.Until.IsZero() || filter.From.Nanosecond() != 0 || filter.Until.Nanosecond() != 0 || filter.From.Location().String() != filter.Until.Location().String() {
		return "", "", ErrInvalidRequest
	}
	// Compare wall clocks, not elapsed seconds, so DST cannot widen the admitted interval.
	layout := "2006-01-02T15:04:05"
	from := filter.From.Format(layout)
	until := filter.Until.Format(layout)
	a, e1 := time.Parse(layout, from)
	b, e2 := time.Parse(layout, until)
	if e1 != nil || e2 != nil || !b.After(a) || b.Sub(a) > 31*24*time.Hour {
		return "", "", ErrInvalidRequest
	}
	return from, until, nil
}
