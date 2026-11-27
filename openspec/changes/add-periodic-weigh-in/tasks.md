# Tasks

## 1. Configuration (`internal/config`)

- [x] 1.1 Add `DefaultWeighInInterval = 15` and `WeighInInterval int` to `config.Config`, parse `weigh_in_interval` from PocketBase config records and `WEIGH_IN_INTERVAL` from local env/`.env` (defaulting omitted/`0` to `15` and rejecting negative or non-integer values), and verify with unit tests in `internal/config/config_test.go` via `go test -v ./internal/config/...`
- [x] 1.2 Document `weigh_in_interval` (PocketBase `configs` schema and example JSON) in `README.md` and `WEIGH_IN_INTERVAL` in `README.md` and `.env.example`, and verify the documented field names and defaults match `internal/config/config.go`

## 2. Storage (`internal/storage`)

- [x] 2.1 Add the `weights` table and `idx_weights_user_time` index to `NewSQLiteStore` schema initialization, define `storage.Weight`, and implement `CountMeals`, `SaveWeight`, and `ListWeights` on `SQLiteStore`, verifying per-user isolation, validation, and chronological ordering with unit tests in `internal/storage/sqlite_test.go` via `go test -v ./internal/storage/...`

## 3. Localization & Bot Weigh-In Flow (`internal/bot` & `cmd/bot`)

- [x] 3.1 Add `en` and `ru` strings to `internal/bot/i18n.go` for the weigh-in button, weight input prompt, invalid weight error, weight saved confirmation, weight save error, and weigh-in cancellation, and verify all fields are populated for both locales via `go test -v ./internal/bot/...`
- [x] 3.2 Extend `bot.MealStore` with `CountMeals` and `SaveWeight`, update `NewHandler` to accept `weighInInterval` (wire `cfg.WeighInInterval` in `cmd/bot/main.go` and startup logs), and attach the inline `"⚖️ Weigh in"` (`weight:start`) button on every Nth meal save per user; verify with unit tests in `internal/bot/handler_test.go` covering Nth meal trigger, non-multiple meals, per-user isolation, and `CountMeals` error fallback
- [x] 3.3 Implement `parseWeightKg`, per-user `awaitingWeight` state (mutually exclusive with `awaitingRange` and cleared on commands/menu buttons), `weight:start` and `weight:cancel` callback handlers, and weight text message handling; verify with unit tests in `internal/bot/handler_test.go` covering `.`/`,` decimals, `kg`/`кг` suffixes, invalid/out-of-range inputs, cancellation paths, and `en`/`ru` localization via `go test -v ./internal/bot/...`

## 4. Verification

- [x] 4.1 Run `go vet ./...` and `go test -v ./...` and verify the full test suite and static analysis pass with zero errors
