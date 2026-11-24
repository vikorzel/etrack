package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Meal represents one logged meal satiety measurement.
type Meal struct {
	ID            int64
	UserID        int64
	RecordedAt    time.Time
	SatietyBefore int
	SatietyAfter  int
}

// SQLiteStore persists meals in a local SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at dbPath and ensures the schema exists.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, errors.New("dbPath must not be empty")
	}

	if dbPath != ":memory:" && !strings.HasPrefix(dbPath, "file:") {
		dir := filepath.Dir(dbPath)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create database directory %q: %w", dir, err)
			}
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}

	// Single writer connection keeps SQLite concurrency predictable without lock errors.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", dbPath, err)
	}

	if _, err := db.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	// Best-effort WAL mode; falls back silently on filesystems (e.g. WSL 9P) where WAL is unsupported.
	_, _ = db.Exec(`PRAGMA journal_mode = WAL;`)

	const schema = `
CREATE TABLE IF NOT EXISTS meals (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	recorded_at INTEGER NOT NULL,
	satiety_before INTEGER NOT NULL CHECK(satiety_before BETWEEN 1 AND 10),
	satiety_after INTEGER NOT NULL CHECK(satiety_after BETWEEN 1 AND 10)
);
CREATE INDEX IF NOT EXISTS idx_meals_user_time ON meals(user_id, recorded_at);`

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// SaveMeal validates and inserts a meal record, returning its generated row ID.
func (s *SQLiteStore) SaveMeal(ctx context.Context, meal Meal) (int64, error) {
	if meal.UserID == 0 {
		return 0, errors.New("user_id must be non-zero")
	}
	if meal.SatietyBefore < 1 || meal.SatietyBefore > 10 {
		return 0, fmt.Errorf("satiety_before %d out of range [1, 10]", meal.SatietyBefore)
	}
	if meal.SatietyAfter < 1 || meal.SatietyAfter > 10 {
		return 0, fmt.Errorf("satiety_after %d out of range [1, 10]", meal.SatietyAfter)
	}
	if meal.RecordedAt.IsZero() {
		meal.RecordedAt = time.Now().UTC()
	}

	res, err := s.db.ExecContext(
		ctx,
		`INSERT INTO meals (user_id, recorded_at, satiety_before, satiety_after) VALUES (?, ?, ?, ?)`,
		meal.UserID,
		meal.RecordedAt.UTC().Unix(),
		meal.SatietyBefore,
		meal.SatietyAfter,
	)
	if err != nil {
		return 0, fmt.Errorf("insert meal: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read last insert id: %w", err)
	}
	return id, nil
}

// ListMeals returns all meals for userID within the half-open interval [from, to), ordered chronologically.
// Passing nil for from or to leaves that bound open.
func (s *SQLiteStore) ListMeals(ctx context.Context, userID int64, from, to *time.Time) ([]Meal, error) {
	if userID == 0 {
		return nil, errors.New("user_id must be non-zero")
	}

	var b strings.Builder
	b.WriteString(`SELECT id, user_id, recorded_at, satiety_before, satiety_after FROM meals WHERE user_id = ?`)
	args := []any{userID}

	if from != nil {
		b.WriteString(` AND recorded_at >= ?`)
		args = append(args, from.UTC().Unix())
	}
	if to != nil {
		b.WriteString(` AND recorded_at < ?`)
		args = append(args, to.UTC().Unix())
	}
	b.WriteString(` ORDER BY recorded_at ASC, id ASC`)

	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query meals: %w", err)
	}
	defer rows.Close()

	var meals []Meal
	for rows.Next() {
		var m Meal
		var ts int64
		if err := rows.Scan(&m.ID, &m.UserID, &ts, &m.SatietyBefore, &m.SatietyAfter); err != nil {
			return nil, fmt.Errorf("scan meal row: %w", err)
		}
		m.RecordedAt = time.Unix(ts, 0).UTC()
		meals = append(meals, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate meal rows: %w", err)
	}
	return meals, nil
}
