# Spec Delta

## Purpose

Prompts users to weigh in after every configurable N logged meal records (default 15, sourced from PocketBase with local env fallback) and provides an interactive flow to validate and persist body-weight entries per user.

## ADDED Requirements

### Requirement: Weigh-in interval configuration
The system SHALL load the weigh-in interval `N` from the PocketBase config record field `weigh_in_interval` when PocketBase configuration mode is active, or from `WEIGH_IN_INTERVAL` in local environment / `.env` mode, defaulting to `15` when omitted or zero, and rejecting negative or malformed values.

#### Scenario: Default interval when omitted in PocketBase
- **WHEN** the bot loads configuration from PocketBase and the `weigh_in_interval` field is absent or `0`
- **THEN** the system sets the weigh-in interval to `15`

#### Scenario: Custom interval from PocketBase
- **WHEN** the bot loads configuration from PocketBase and `weigh_in_interval` is `10`
- **THEN** the system sets the weigh-in interval to `10`

#### Scenario: Negative interval in PocketBase is rejected
- **WHEN** the bot loads configuration from PocketBase and `weigh_in_interval` is `-5`
- **THEN** configuration loading fails with a validation error

#### Scenario: Local environment fallback and validation
- **WHEN** the bot loads configuration in local mode with `WEIGH_IN_INTERVAL=20`
- **THEN** the system sets the weigh-in interval to `20`

#### Scenario: Malformed local environment interval is rejected
- **WHEN** the bot loads configuration in local mode with `WEIGH_IN_INTERVAL=abc` or `WEIGH_IN_INTERVAL=-1`
- **THEN** configuration loading fails with a validation error

### Requirement: Periodic weigh-in button after every N meal records
The system SHALL attach an inline weigh-in button to the meal-saved confirmation message whenever the user's total persisted meal count is a positive multiple of the configured weigh-in interval `N`, counting meals independently per user.

#### Scenario: Nth meal triggers weigh-in button
- **WHEN** a user saves their Nth meal (for example, meal 15 or meal 30 when `N=15`)
- **THEN** the meal-saved confirmation message includes an inline button to start a weigh-in

#### Scenario: Non-multiple meal does not trigger weigh-in button
- **WHEN** a user saves a meal whose total per-user count is not a multiple of `N` (for example, meal 14 or meal 16 when `N=15`)
- **THEN** the meal-saved confirmation message is sent without a weigh-in inline button

#### Scenario: Meal counts are isolated per user
- **WHEN** user A has 14 meals and user B saves a meal
- **THEN** user B's meal does not increment user A's count or trigger a weigh-in button for user A

### Requirement: Interactive weigh-in prompt and cancellation
The system SHALL enter weight-input mode when a user taps the weigh-in inline button, prompt for their current weight with an inline Cancel button, and exit weight-input mode without saving if the user cancels or invokes another command or menu action.

#### Scenario: Tapping weigh-in button prompts for weight
- **WHEN** a user taps the weigh-in inline button on a meal confirmation message
- **THEN** the system answers the callback query, enters weight-input mode for that user, and prompts the user to enter their weight in kilograms with an inline Cancel button

#### Scenario: Cancelling via inline Cancel button
- **WHEN** a user in weight-input mode taps the inline Cancel button
- **THEN** the system exits weight-input mode, does not save a weight record, and confirms cancellation

#### Scenario: Cancelling via command or menu button
- **WHEN** a user in weight-input mode sends `/cancel`, `/start`, `/help`, or taps a main menu button
- **THEN** the system exits weight-input mode and processes that command or menu action normally

### Requirement: Weight input parsing, validation, and persistence
The system SHALL parse text entered in weight-input mode as a positive weight in kilograms within `(0, 500]` (accepting `.` or `,` as decimal separator and an optional `kg` or `кг` suffix), reject invalid input while remaining in weight-input mode, and persist valid weights per user in SQLite with a timestamp.

#### Scenario: Valid decimal weight with dot or comma is saved
- **WHEN** a user in weight-input mode sends `"72.5"`, `"72,5"`, or `"72.5 kg"`
- **THEN** the system persists a weight record of `72.5` kg for that user with the current timestamp, exits weight-input mode, and sends a confirmation message showing the recorded weight and local timestamp

#### Scenario: Invalid or out-of-range weight keeps user in weight-input mode
- **WHEN** a user in weight-input mode sends `"abc"`, `"0"`, `"-10"`, or `"501"`
- **THEN** the system does not save a weight record, replies with a validation error and format example, and keeps the user in weight-input mode
