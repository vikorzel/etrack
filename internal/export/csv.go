package export

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"meal-bot/internal/storage"
)

var (
	singleDatePattern = regexp.MustCompile(`^(\d{2}\.\d{2}\.\d{4}|\d{4}-\d{2}-\d{2})$`)
	rangeDatePattern  = regexp.MustCompile(`^(\d{2}\.\d{2}\.\d{4}|\d{4}-\d{2}-\d{2})(?:\s*(?:-|–|—|\.\.)\s*|\s+)(\d{2}\.\d{2}\.\d{4}|\d{4}-\d{2}-\d{2})$`)

	ErrEmptyRange         = errors.New("empty date range")
	ErrInvalidRangeFormat = errors.New("invalid date or range format")
	ErrInvalidStartDate   = errors.New("invalid start date")
	ErrInvalidEndDate     = errors.New("invalid end date")
	ErrEndBeforeStart     = errors.New("end date is before start date")
)

// InvalidDateError reports an unparseable start or end date token.
type InvalidDateError struct {
	Sentinel error
	Token    string
	Err      error
}

func (e *InvalidDateError) Error() string {
	return fmt.Sprintf("%s %q: %v", e.Sentinel.Error(), e.Token, e.Err)
}

func (e *InvalidDateError) Unwrap() []error {
	return []error{e.Sentinel, e.Err}
}

// BuildMealsCSV formats meal records as UTF-8 CSV using loc for date/time display.
func BuildMealsCSV(meals []storage.Meal, loc *time.Location) ([]byte, error) {
	if loc == nil {
		loc = time.UTC
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	header := []string{"id", "date", "time", "recorded_at", "satiety_before", "satiety_after", "delta"}
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("write csv header: %w", err)
	}

	for _, m := range meals {
		t := m.RecordedAt.In(loc)
		row := []string{
			strconv.FormatInt(m.ID, 10),
			t.Format("2006-01-02"),
			t.Format("15:04:05"),
			t.Format(time.RFC3339),
			strconv.Itoa(m.SatietyBefore),
			strconv.Itoa(m.SatietyAfter),
			strconv.Itoa(m.SatietyAfter - m.SatietyBefore),
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("write csv row %d: %w", m.ID, err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flush csv: %w", err)
	}
	return buf.Bytes(), nil
}

// ParseDateRange parses a single date or two-date range (DD.MM.YYYY or YYYY-MM-DD)
// in loc and returns a half-open [from, to) window plus filename/display labels.
func ParseDateRange(input string, loc *time.Location) (from time.Time, to time.Time, fileLabel string, displayLabel string, err error) {
	if loc == nil {
		loc = time.UTC
	}

	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return time.Time{}, time.Time{}, "", "", ErrEmptyRange
	}

	var startToken, endToken string
	if m := singleDatePattern.FindStringSubmatch(trimmed); m != nil {
		startToken = m[1]
		endToken = m[1]
	} else if m := rangeDatePattern.FindStringSubmatch(trimmed); m != nil {
		startToken = m[1]
		endToken = m[2]
	} else {
		return time.Time{}, time.Time{}, "", "", ErrInvalidRangeFormat
	}

	start, err := parseDateToken(startToken, loc)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", &InvalidDateError{
			Sentinel: ErrInvalidStartDate,
			Token:    startToken,
			Err:      err,
		}
	}
	end, err := parseDateToken(endToken, loc)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", &InvalidDateError{
			Sentinel: ErrInvalidEndDate,
			Token:    endToken,
			Err:      err,
		}
	}

	if end.Before(start) {
		return time.Time{}, time.Time{}, "", "", ErrEndBeforeStart
	}

	from = start
	to = end.AddDate(0, 0, 1)

	if start.Equal(end) {
		fileLabel = start.Format("2006-01-02")
		displayLabel = start.Format("02.01.2006")
	} else {
		fileLabel = start.Format("2006-01-02") + "_to_" + end.Format("2006-01-02")
		displayLabel = start.Format("02.01.2006") + " — " + end.Format("02.01.2006")
	}
	return from, to, fileLabel, displayLabel, nil
}

func parseDateToken(token string, loc *time.Location) (time.Time, error) {
	if strings.Contains(token, ".") {
		return time.ParseInLocation("02.01.2006", token, loc)
	}
	return time.ParseInLocation("2006-01-02", token, loc)
}
