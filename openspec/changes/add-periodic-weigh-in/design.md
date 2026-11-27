# Design

## Context

`meal-bot` is a single-binary Go Telegram bot using `github.com/go-telegram/bot` and pure-Go SQLite (`modernc.org/sqlite`). Configuration is fetched at startup either from a PocketBase `configs` collection (when `PB_CONFIG_HOST` is set) or from environment variables / `.env`. The bot tracks meal satiety in a `meals` table and manages per-user multi-step text input (currently custom CSV date ranges) via an in-memory map guarded by `Handler.mu`.

See `proposal.md` for motivation and `specs/periodic-weigh-in/spec.md` and `specs/bot-localization/spec.md` for normative requirements.

## Goals / Non-Goals

**Goals:**
- Support configuring `weigh_in_interval` in PocketBase (and `WEIGH_IN_INTERVAL` locally) with a safe default of `15` and strict startup validation.
- Trigger a weigh-in inline button reliably on every Nth meal per user without altering the existing 2-step meal logging flow.
- Persist recorded weights in SQLite with full per-user isolation and clean separation from the `meals` table.
- Keep all new UI text localized in `en` and `ru` following the existing `internal/bot/i18n.go` pattern.

**Non-Goals:**
- Live reloading of `weigh_in_interval` from PocketBase without restarting the bot (config is loaded once at startup, matching `db_path` and `bot_timezone`).
- Exporting weights in `meals_*.csv` or adding a separate weight CSV export flow.
- Editing or deleting historical weight entries.

## Decisions

### 1. Configuration field & default handling
- **Decision**: Add `WeighInInterval int` to `config.Config`, `WeighInInterval *int` (or `int` with `json:"weigh_in_interval"`) to `pbConfigRecord`, and `WEIGH_IN_INTERVAL` lookup in local mode. Treat omitted / empty / `0` as the default `15`, and return an error if `< 0` (or non-integer in env mode).
- **Rationale**: PocketBase `Number` fields default to `0` when left empty in the admin UI. Treating `0` as "use default (15)" avoids breaking existing PocketBase `configs` records that haven't added or populated `weigh_in_interval` yet, while still failing fast on negative numbers or invalid strings.
- **Alternatives considered**: Rejecting `0` as invalid — rejected because PocketBase serializes unset numeric fields as `0`, which would break existing deployments on upgrade.

### 2. SQLite schema & meal count query
- **Decision**: Add a `weights` table alongside `meals` in `SQLiteStore` startup schema migration:
  ```sql
  CREATE TABLE IF NOT EXISTS weights (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id INTEGER NOT NULL,
      recorded_at INTEGER NOT NULL,
      weight_kg REAL NOT NULL CHECK(weight_kg > 0)
  );
  CREATE INDEX IF NOT EXISTS idx_weights_user_time ON weights(user_id, recorded_at);
  ```
  Add `CountMeals(ctx, userID) (int, error)`, `SaveWeight(ctx, weight) (int64, error)`, and `ListWeights(ctx, userID, from, to) ([]Weight, error)` to `SQLiteStore`, and add `CountMeals` and `SaveWeight` to `bot.MealStore`.
- **Rationale**: `SELECT COUNT(*) FROM meals WHERE user_id = ?` uses the existing `idx_meals_user_time` index (leading column `user_id`), is fast, and survives bot restarts without maintaining separate counters that could drift.
- **Alternatives considered**: Keeping an in-memory per-user counter — rejected because it would reset on every bot restart.

### 3. Weigh-in trigger & failure degradation
- **Decision**: After `SaveMeal` succeeds in `meal:after:...`, query `CountMeals(ctx, userID)`. If `err == nil && h.weighInInterval > 0 && count > 0 && count%h.weighInInterval == 0`, attach an inline keyboard with a single `"⚖️ Weigh in"` (`weight:start`) button to the meal confirmation message. If `CountMeals` returns an error, log it and still show the normal meal-saved confirmation without failing the meal save.
- **Rationale**: Meal recording is the primary action; a transient count error should not mislead the user into thinking their meal wasn't saved.

### 4. Mutual exclusion of text-input states
- **Decision**: Add `awaitingWeight map[int64]bool` to `Handler` alongside `awaitingRange map[int64]bool`. Entering `awaitingWeight` clears `awaitingRange`, and entering `awaitingRange` (or any command `/start`, `/help`, `/cancel`, `/meal`, `/export`, or main-menu button) clears `awaitingWeight`.
- **Rationale**: Prevents ambiguous routing when a user starts a custom export range and then taps an older weigh-in button (or vice versa).

### 5. Weight parsing rules
- **Decision**: Create a small pure helper `parseWeightKg(input string) (float64, error)`:
  - Trim whitespace; strip optional trailing `"kg"` or `"кг"` (case-insensitive) and re-trim.
  - Replace `,` with `.` (rejecting strings with multiple decimal separators or signs/non-numeric tokens).
  - Parse with `strconv.ParseFloat(..., 64)`, reject `NaN`/`Inf`, and require `val > 0 && val <= 500`.
  - Round or format for display cleanly (e.g., `strconv.FormatFloat(w, 'f', -1, 64)` or up to 2 decimals without trailing zeros).
- **Rationale**: Russian and European users frequently type `72,5` instead of `72.5`, and users sometimes append `kg`/`кг`. Keeping the upper bound at `500` kg catches accidental typos (like entering grams `72500` or a date) while accommodating all realistic human weights.

### 6. Constructor signature compatibility
- **Decision**: Update `NewHandler(store MealStore, loc *time.Location, weighInInterval int) *Handler` (defaulting `weighInInterval <= 0` to `config.DefaultWeighInInterval` = `15` defensively).
- **Rationale**: `Handler` is an internal package (`internal/bot`) with a single production caller (`cmd/bot/main.go`) and unit tests in `internal/bot/handler_test.go`, so adding the parameter directly keeps wiring explicit and simple.

## Risks / Trade-offs

- **[In-memory `awaitingWeight` lost on restart]** → If the bot restarts while a user is typing their weight, the next message falls through to the unknown-input hint; the user can simply tap the `"⚖️ Weigh in"` button on the previous confirmation message again.
- **[Repeated taps on an old `"⚖️ Weigh in"` button]** → Allowed intentionally (it opens the prompt and replaces the button with the weight prompt / Cancel button when edited), so a user who dismissed or restarted can still record their weight without getting stuck.
- **[`weigh_in_interval` read only at startup]** → Changing the interval in PocketBase requires restarting the bot, consistent with `db_path` and `bot_timezone`.

## Migration Plan

1. **PocketBase (optional)**: Add an optional `Number` field `weigh_in_interval` to the `configs` collection (leave empty or `0` to use default `15`, or set a custom positive integer).
2. **SQLite (automatic)**: On bot startup, `NewSQLiteStore` runs `CREATE TABLE IF NOT EXISTS weights` and `CREATE INDEX IF NOT EXISTS idx_weights_user_time`; existing `meals` data is untouched.
3. **Rollback**: Reverting to the previous binary leaves the `weights` table dormant in SQLite and ignores the extra `weigh_in_interval` field in PocketBase JSON responses.
