## Purpose

Provides Telegram `language_code`-based localization for `meal-bot`, supporting English (`en`, default) and Russian (`ru`) across all user-facing messages, keyboards, callback responses, and validation errors.

## Requirements

### Requirement: Locale resolution from Telegram language_code
The system SHALL resolve the user's active locale from Telegram's `language_code` field on incoming messages and callback queries, supporting only `en` and `ru`, with `en` as the default fallback.

#### Scenario: Empty or missing language_code defaults to English
- **WHEN** a user sends a message or callback query with an empty or missing `language_code`
- **THEN** the system resolves the locale to `en`

#### Scenario: Unsupported language_code defaults to English
- **WHEN** a user sends a message or callback query with an unsupported `language_code` (such as `"de"`, `"es"`, or `"uk"`)
- **THEN** the system resolves the locale to `en`

#### Scenario: Exact supported language_code selects matching locale
- **WHEN** a user sends a message or callback query with `language_code` `"ru"` or `"en"` (case-insensitive)
- **THEN** the system resolves the locale to `"ru"` or `"en"` respectively

#### Scenario: Regional tag normalizes to base language
- **WHEN** a user sends a message or callback query with a regional tag such as `"ru-RU"` or `"en-US"`
- **THEN** the system resolves the locale to `"ru"` or `"en"` respectively

### Requirement: Localized bot messages, keyboards, and callback responses
The system SHALL render all user-facing text—including welcome/help messages, meal-logging prompts and confirmations, reply keyboard buttons, inline keyboard buttons, callback query answers, and CSV export status/caption/error messages—in the user's resolved locale (`en` or `ru`).

#### Scenario: English user receives English UI
- **WHEN** a user with resolved locale `en` sends `/start`, logs a meal, or exports CSV
- **THEN** all bot messages, reply keyboard buttons, inline keyboard buttons, callback query answers, and CSV export captions are rendered in English

#### Scenario: Russian user receives Russian UI
- **WHEN** a user with resolved locale `ru` sends `/start`, logs a meal, or exports CSV
- **THEN** all bot messages, reply keyboard buttons, inline keyboard buttons, callback query answers, and CSV export captions are rendered in Russian

#### Scenario: Localized date-range validation errors
- **WHEN** a user enters an invalid custom date range while awaiting range input
- **THEN** the validation error message and format examples are rendered in the user's resolved locale (`en` or `ru`)

### Requirement: Cross-locale reply keyboard button matching
The system SHALL recognize reply keyboard button presses for recording a meal and exporting CSV regardless of which supported locale's button label was sent.

#### Scenario: Tapping localized or previously cached menu button
- **WHEN** a user taps either `"🍽 Record meal"` / `"🍽 Записать приём пищи"` or `"📊 Export CSV"` / `"📊 Выгрузить CSV"`
- **THEN** the system triggers the corresponding meal-logging or CSV-export flow and replies in the user's current resolved locale

### Requirement: Language-neutral CSV file format
The system SHALL keep exported CSV column headers (`id,date,time,recorded_at,satiety_before,satiety_after,delta`) and data formatting unchanged regardless of user locale.

#### Scenario: CSV export headers remain consistent across locales
- **WHEN** a user with locale `en` or `ru` exports meals to CSV
- **THEN** the generated CSV file uses the standard `id,date,time,recorded_at,satiety_before,satiety_after,delta` header row
