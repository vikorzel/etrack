## Why

All user-facing messages, buttons, and error responses in `meal-bot` are currently hardcoded in Russian. Supporting Telegram's `language_code` with English (`en`) as the default and Russian (`ru`) as a supported locale makes the bot accessible to English-speaking users while preserving the Russian experience for Russian-speaking users.

## What Changes

- Detect and normalize the user's locale from Telegram `From.LanguageCode` on incoming messages and callback queries, supporting `en` and `ru` with `en` as the default fallback for empty or unsupported language codes.
- Extract all hardcoded Russian UI strings, reply keyboard labels, inline keyboard labels, callback query responses, and CSV export status/error messages into a localization catalog supporting `en` and `ru`.
- Update `handleMessage` to match reply keyboard button presses (`Record meal` / `Записать приём пищи`, `Export CSV` / `Выгрузить CSV`) across all supported locales.
- Localize date-range parsing validation messages returned to the user while keeping CSV column headers (`id,date,time,recorded_at,satiety_before,satiety_after,delta`) language-neutral.

## Capabilities

### New Capabilities
- `bot-localization`: Locale resolution from Telegram `language_code` (`en` default, `ru` supported) and localized rendering of all bot messages, keyboards, callback answers, and user-facing validation errors.

### Modified Capabilities
<!-- None; no existing specs exist in openspec/specs/. -->

## Impact

- `internal/bot/handler.go`: Locale resolution from `Message.From.LanguageCode` / `CallbackQuery.From.LanguageCode`, localized text/keyboard builders, and locale-aware reply-button matching.
- `internal/bot/i18n.go` (new): Message catalog and locale resolution helpers for `en` and `ru`.
- `internal/export/csv.go`: Typed or localized validation errors from `ParseDateRange` so the bot can present errors in the user's language.
- `internal/bot/handler_test.go` and `internal/export/csv_test.go`: Unit tests covering `en` (default), `ru`, regional tags (e.g. `ru-RU`, `en-US`), and unsupported `language_code` fallback behavior.
