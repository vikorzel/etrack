# Proposal

## Why

Users tracking meal satiety over time also benefit from periodic body-weight check-ins to correlate satiety habits with weight trends, without being nagged on every meal. Showing a weigh-in prompt automatically after every N logged meals (configurable in PocketBase, defaulting to 15) keeps weight tracking lightweight and adjustable without redeploying the bot.

## What Changes

- Add a configurable `weigh_in_interval` setting (integer, default `15`) loaded from the PocketBase `configs` record, with a matching `WEIGH_IN_INTERVAL` local environment / `.env` fallback.
- Persist user weight entries in SQLite in a new `weights` table isolated by `user_id`, alongside a meal-count query per user.
- After saving a meal, check the user's total recorded meal count; whenever the count is a positive multiple of `weigh_in_interval` (i.e., `count % N == 0`), attach a **"Weigh in"** inline button to the meal-saved confirmation message.
- Add an interactive weigh-in flow triggered by the button: prompt the user to enter their weight in kilograms (accepting `.` or `,` decimal separators, validating positive realistic values, and supporting cancellation via inline button or `/cancel`), save the entry to SQLite, and confirm the recorded weight.
- Localize all new weigh-in UI strings (buttons, prompts, validation errors, confirmations, and cancellations) in both English (`en`) and Russian (`ru`).
- Document the new `weigh_in_interval` PocketBase field and `WEIGH_IN_INTERVAL` environment variable in `README.md` and `.env.example`.
- Note: Existing meal CSV export (`meals_*.csv`) remains unchanged; exporting weight records to CSV is out of scope for this change.

## Capabilities

### New Capabilities
- `periodic-weigh-in`: Configurable periodic weigh-in trigger after every N meal records per user, interactive weight input and validation flow, and SQLite weight persistence.

### Modified Capabilities
- `bot-localization`: Extend localized bot messages, inline buttons, callback responses, and validation errors to cover the periodic weigh-in flow in `en` and `ru`.

## Impact

- `internal/config`: Add `WeighInInterval` to `Config`, parse `weigh_in_interval` from PocketBase records and `WEIGH_IN_INTERVAL` from env/`.env` with default `15` and validation.
- `internal/storage`: Add `weights` SQLite table schema and index, `Weight` struct, `CountMeals`, `SaveWeight`, and `ListWeights` methods on `SQLiteStore`.
- `internal/bot`: Extend `MealStore` interface, pass `weighInInterval` to `Handler`, add per-user `awaitingWeight` state, attach the weigh-in button on every Nth meal save, and handle `weight:start` / `weight:cancel` callbacks and weight text input.
- `internal/bot/i18n.go`: Add `en` and `ru` strings for weigh-in UI and validation errors.
- `cmd/bot/main.go`: Pass `cfg.WeighInInterval` to `mealbot.NewHandler` and include it in startup log output.
- `README.md` and `.env.example`: Document `weigh_in_interval` / `WEIGH_IN_INTERVAL`.
