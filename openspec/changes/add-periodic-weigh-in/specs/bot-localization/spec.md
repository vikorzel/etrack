# Spec Delta

## MODIFIED Requirements

### Requirement: Localized bot messages, keyboards, and callback responses
The system SHALL render all user-facing text—including welcome/help messages, meal-logging prompts and confirmations, periodic weigh-in buttons/prompts/confirmations/validation errors, reply keyboard buttons, inline keyboard buttons, callback query answers, and CSV export status/caption/error messages—in the user's resolved locale (`en` or `ru`).

#### Scenario: English user receives English UI
- **WHEN** a user with resolved locale `en` sends `/start`, logs a meal, or exports CSV
- **THEN** all bot messages, reply keyboard buttons, inline keyboard buttons, callback query answers, and CSV export captions are rendered in English

#### Scenario: Russian user receives Russian UI
- **WHEN** a user with resolved locale `ru` sends `/start`, logs a meal, or exports CSV
- **THEN** all bot messages, reply keyboard buttons, inline keyboard buttons, callback query answers, and CSV export captions are rendered in Russian

#### Scenario: Localized date-range validation errors
- **WHEN** a user enters an invalid custom date range while awaiting range input
- **THEN** the validation error message and format examples are rendered in the user's resolved locale (`en` or `ru`)

#### Scenario: Localized weigh-in flow in English and Russian
- **WHEN** a user with resolved locale `en` or `ru` triggers a weigh-in button, enters an invalid or valid weight, or cancels the weigh-in
- **THEN** the weigh-in button label, weight prompt, validation error message, confirmation message, and cancellation text are rendered in the user's resolved locale (`en` or `ru`)
