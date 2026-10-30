// Package ts formats timestamps and dates for API messages.
package ts

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

const DateLayout = "2006-01-02"

func Time(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func PtrTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return Time(*t)
}

func Date(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format(DateLayout)
}

// ParseDate parses YYYY-MM-DD; an empty string yields a NULL date.
func ParseDate(s string) (pgtype.Date, error) {
	if s == "" {
		return pgtype.Date{}, nil
	}
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}
