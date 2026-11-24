package export

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"meal-bot/internal/storage"
)

func TestBuildMealsCSV(t *testing.T) {
	loc := time.FixedZone("UTC+2", 2*3600)
	meals := []storage.Meal{
		{
			ID:            1,
			UserID:        42,
			RecordedAt:    time.Date(2026, 11, 24, 8, 15, 30, 0, time.UTC),
			SatietyBefore: 3,
			SatietyAfter:  8,
		},
		{
			ID:            2,
			UserID:        42,
			RecordedAt:    time.Date(2026, 11, 24, 18, 0, 0, 0, time.UTC),
			SatietyBefore: 6,
			SatietyAfter:  6,
		},
		{
			ID:            3,
			UserID:        42,
			RecordedAt:    time.Date(2026, 11, 24, 22, 30, 0, 0, time.UTC),
			SatietyBefore: 7,
			SatietyAfter:  5,
		},
	}

	raw, err := BuildMealsCSV(meals, loc)
	if err != nil {
		t.Fatalf("BuildMealsCSV: %v", err)
	}

	records, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
	if err != nil {
		t.Fatalf("parse generated CSV: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("len(records) = %d, want 4", len(records))
	}

	wantHeader := []string{"id", "date", "time", "recorded_at", "satiety_before", "satiety_after", "delta"}
	for i, col := range wantHeader {
		if records[0][i] != col {
			t.Fatalf("header[%d] = %q, want %q", i, records[0][i], col)
		}
	}

	// Row 1: 08:15:30 UTC -> 10:15:30 +02:00, delta +5
	if records[1][1] != "2026-11-24" || records[1][2] != "10:15:30" || records[1][6] != "5" {
		t.Fatalf("unexpected row 1: %v", records[1])
	}
	// Row 2: delta 0
	if records[2][6] != "0" {
		t.Fatalf("row 2 delta = %q, want 0", records[2][6])
	}
	// Row 3: 22:30:00 UTC -> next local day 2026-11-25 00:30:00, delta -2
	if records[3][1] != "2026-11-25" || records[3][2] != "00:30:00" || records[3][6] != "-2" {
		t.Fatalf("unexpected row 3: %v", records[3])
	}
}

func TestParseDateRangeValid(t *testing.T) {
	loc := time.FixedZone("UTC+2", 2*3600)

	cases := []struct {
		input       string
		wantFrom    time.Time
		wantTo      time.Time
		wantFile    string
		wantDisplay string
	}{
		{
			input:       "01.11.2026 - 24.11.2026",
			wantFrom:    time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
			wantTo:      time.Date(2026, 11, 25, 0, 0, 0, 0, loc),
			wantFile:    "2026-11-01_to_2026-11-24",
			wantDisplay: "01.11.2026 — 24.11.2026",
		},
		{
			input:       "01.11.2026-24.11.2026",
			wantFrom:    time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
			wantTo:      time.Date(2026, 11, 25, 0, 0, 0, 0, loc),
			wantFile:    "2026-11-01_to_2026-11-24",
			wantDisplay: "01.11.2026 — 24.11.2026",
		},
		{
			input:       "2026-11-01 - 2026-11-24",
			wantFrom:    time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
			wantTo:      time.Date(2026, 11, 25, 0, 0, 0, 0, loc),
			wantFile:    "2026-11-01_to_2026-11-24",
			wantDisplay: "01.11.2026 — 24.11.2026",
		},
		{
			input:       "2026-11-01..2026-11-24",
			wantFrom:    time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
			wantTo:      time.Date(2026, 11, 25, 0, 0, 0, 0, loc),
			wantFile:    "2026-11-01_to_2026-11-24",
			wantDisplay: "01.11.2026 — 24.11.2026",
		},
		{
			input:       "01.11.2026 24.11.2026",
			wantFrom:    time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
			wantTo:      time.Date(2026, 11, 25, 0, 0, 0, 0, loc),
			wantFile:    "2026-11-01_to_2026-11-24",
			wantDisplay: "01.11.2026 — 24.11.2026",
		},
		{
			input:       "24.11.2026",
			wantFrom:    time.Date(2026, 11, 24, 0, 0, 0, 0, loc),
			wantTo:      time.Date(2026, 11, 25, 0, 0, 0, 0, loc),
			wantFile:    "2026-11-24",
			wantDisplay: "24.11.2026",
		},
		{
			input:       "  2026-11-24  ",
			wantFrom:    time.Date(2026, 11, 24, 0, 0, 0, 0, loc),
			wantTo:      time.Date(2026, 11, 25, 0, 0, 0, 0, loc),
			wantFile:    "2026-11-24",
			wantDisplay: "24.11.2026",
		},
	}

	for _, tc := range cases {
		from, to, fileLabel, displayLabel, err := ParseDateRange(tc.input, loc)
		if err != nil {
			t.Fatalf("ParseDateRange(%q) error = %v", tc.input, err)
		}
		if !from.Equal(tc.wantFrom) || !to.Equal(tc.wantTo) {
			t.Fatalf("ParseDateRange(%q) = [%v, %v), want [%v, %v)", tc.input, from, to, tc.wantFrom, tc.wantTo)
		}
		if fileLabel != tc.wantFile || displayLabel != tc.wantDisplay {
			t.Fatalf("ParseDateRange(%q) labels = (%q, %q), want (%q, %q)", tc.input, fileLabel, displayLabel, tc.wantFile, tc.wantDisplay)
		}
	}
}

func TestParseDateRangeInvalid(t *testing.T) {
	loc := time.UTC
	invalid := []string{
		"",
		"   ",
		"24.11.2026 - 01.11.2026", // inverted
		"31.02.2026",              // impossible calendar date
		"24.11.2026 extra",        // trailing garbage
		"yesterday",
	}
	for _, in := range invalid {
		if _, _, _, _, err := ParseDateRange(in, loc); err == nil {
			t.Errorf("ParseDateRange(%q): expected error, got nil", in)
		}
	}
}
