## Context

`meal-bot` is a stateless-per-request Telegram bot (`internal/bot/handler.go`) backed by SQLite (`internal/storage/sqlite.go`) and a CSV/date-range helper (`internal/export/csv.go`). Currently, all bot prompts, reply/inline keyboard labels, callback answers, and date-range validation errors are hardcoded in Russian. See [proposal.md](./proposal.md) for motivation.

## Goals / Non-Goals

**Goals:**
- Resolve user locale (`en` or `ru`, defaulting to `en`) directly from Telegram's `From.LanguageCode` on every incoming message and callback query.
- Centralize all user-facing strings in `internal/bot` with compile-time completeness for `en` and `ru`.
- Keep reply keyboard button matching working across both supported locales.
- Localize `ParseDateRange` validation errors presented by the bot.

**Non-Goals:**
- Persisting user language preferences in SQLite or adding a `/language` command (Telegram's `language_code` is the sole source of truth).
- Localizing CSV column headers (`id,date,time,recorded_at,satiety_before,satiety_after,delta`) or filenames.
- Adding third-party i18n dependencies (`go-i18n`, etc.).

## Decisions

### 1. Stateless per-update locale resolution
`resolveLocale(languageCode string) Locale` normalizes `From.LanguageCode` (trim, lowercase, take primary subtag before `-` or `_`). `"ru"` maps to `LocaleRU`; everything else (`"en"`, `"en-US"`, `""`, unsupported codes) maps to `LocaleEN`.

*Alternative considered:* Storing `language_code` in SQLite per `user_id`. Rejected because Telegram sends `From.LanguageCode` on every `Message` and `CallbackQuery`, and stateless resolution automatically reflects client language changes without DB migrations.

### 2. Typed Go struct catalog (`internal/bot/i18n.go`)
All UI strings and formatting functions live in a `localeMessages` struct with `enMessages` and `ruMessages` instances selected by `messagesFor(loc Locale)`.

*Alternative considered:* Embedded JSON/YAML files with `go-i18n`. Rejected because the bot has ~25 strings across 2 languages; a Go struct guarantees compile-time checks for missing keys and avoids extra dependencies.

### 3. Cross-locale reply keyboard matching
`handleMessage` matches `BtnRecordMeal` and `BtnExportCSV` against both `en` and `ru` labels (in addition to `/meal` and `/export`).

*Rationale:* Telegram persistent reply keyboards remain visible on the client even if the user changes their Telegram app language, so accepting both locales prevents dead buttons.

### 4. Sentinel / typed errors in `internal/export`
`export.ParseDateRange` returns sentinel/typed errors (e.g., `ErrEmptyRange`, `ErrInvalidRangeFormat`, `ErrInvalidStartDate`, `ErrInvalidEndDate`, `ErrEndBeforeStart`) rather than hardcoded Russian strings. `internal/bot` translates them via `errors.Is` / `errors.As`.

*Alternative considered:* Passing `locale` into `ParseDateRange`. Rejected to keep `internal/export` independent of UI localization.

## Risks / Trade-offs

- **Telegram `language_code` omitted by some clients/bots** → Falls back cleanly to `en` as specified.
- **Date display format in messages** → Keep `DD.MM.YYYY` (`02.01.2006`) for consistency with `ParseDateRange` examples and existing tests, while localizing surrounding prose (`7 days` / `7 дней`, `all time` / `всё время`).
