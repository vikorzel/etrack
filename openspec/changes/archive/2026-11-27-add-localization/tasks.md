## 1. Localization Catalog & Locale Resolution

- [x] 1.1 Create `internal/bot/i18n.go` with `Locale` (`en`, `ru`), `resolveLocale(languageCode string) Locale` (normalizing regional tags like `ru-RU`/`en-US` and defaulting empty/unsupported codes to `en`), and `messagesFor(loc Locale)`
- [x] 1.2 Populate complete `en` and `ru` message bundles in `internal/bot/i18n.go` covering welcome/help text, cancel messages, meal-logging prompts and confirmation, reply keyboard buttons, inline keyboard buttons, callback answers, and CSV export prompts/captions/errors

## 2. Export Date-Range Error Localization

- [x] 2.1 Introduce sentinel/typed validation errors in `internal/export/csv.go` for `ParseDateRange` (empty range, invalid format, invalid start/end date, end before start) while preserving `internal/export/csv_test.go` behavior
- [x] 2.2 Add a helper in `internal/bot/i18n.go` to format `export.ParseDateRange` errors in the resolved locale (`en` or `ru`)

## 3. Handler & Keyboard Integration

- [x] 3.1 Update `internal/bot/handler.go` (`handleMessage`, `handleCallback`, `sendCSVExport`, `sendTextWithMainMenu`, and keyboard builders) to resolve `From.LanguageCode` and pass the active `Locale` to all text and markup builders
- [x] 3.2 Update reply keyboard button matching in `handleMessage` to accept both `en` and `ru` labels for recording a meal and exporting CSV

## 4. Tests & Verification

- [x] 4.1 Update existing tests in `internal/bot/handler_test.go` to account for `en` default when `LanguageCode` is empty and add tests for `ru`, `ru-RU`, `en-US`, and unsupported `language_code` fallback across messages, callbacks, keyboards, and CSV export
- [x] 4.2 Add/update tests in `internal/export/csv_test.go` verifying sentinel/typed errors from `ParseDateRange`
- [x] 4.3 Run `go vet ./...` and `go test -v ./...` and verify all tests pass
