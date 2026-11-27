package bot

import (
	"errors"
	"fmt"
	"strings"

	"meal-bot/internal/export"
)

// Locale represents a supported bot UI language.
type Locale string

const (
	LocaleEN Locale = "en"
	LocaleRU Locale = "ru"
)

// resolveLocale normalizes Telegram's From.LanguageCode to a supported Locale,
// defaulting empty or unsupported codes to LocaleEN.
func resolveLocale(languageCode string) Locale {
	normalized := strings.ToLower(strings.TrimSpace(languageCode))
	if normalized == "" {
		return LocaleEN
	}
	if idx := strings.IndexAny(normalized, "-_"); idx != -1 {
		normalized = normalized[:idx]
	}
	if normalized == string(LocaleRU) {
		return LocaleRU
	}
	return LocaleEN
}

type localeMessages struct {
	btnRecordMeal string
	btnExportCSV  string

	btnCancel       string
	btnBack         string
	btnExport7d     string
	btnExport30d    string
	btnExportAll    string
	btnExportCustom string

	welcomeText        string
	actionCancelled    string
	exportChoosePeriod string
	unknownInputHint   string

	stepBeforeText        string
	stepAfterText         func(before int) string
	mealSavedConfirmation func(localTime string, before, after int, delta string) string
	mealCancelledText     string

	cbInvalidValue  string
	cbInvalidData   string
	cbAlreadySaved  string
	cbSaveError     string
	cbSaved         string
	cbCancelled     string
	cbUnknownAction string

	export7dLabel       func(from, to string) string
	export30dLabel      func(from, to string) string
	exportAllLabel      string
	exportAllStatus     string
	exportPeriodStatus  func(label string) string
	exportCustomPrompt  string
	exportRangeInvalid  func(reason string) string
	exportCancelledText string
	exportDBError       string
	exportEmptyRange    func(period string) string
	exportBuildError    string
	exportCaption       func(period string, count int) string
	exportSendError     string

	errEmptyRange         string
	errInvalidRangeFormat string
	errInvalidStartDate   func(token string) string
	errInvalidEndDate     func(token string) string
	errEndBeforeStart     string
}

var enMessages = localeMessages{
	btnRecordMeal: "🍽 Record meal",
	btnExportCSV:  "📊 Export CSV",

	btnCancel:       "❌ Cancel",
	btnBack:         "⬅️ Back",
	btnExport7d:     "Last 7 days",
	btnExport30d:    "Last 30 days",
	btnExportAll:    "All time",
	btnExportCustom: "📅 Custom range",

	welcomeText: "Hi! I can help you track your satiety before and after meals.\n\n" +
		"• «🍽 Record meal» — answer 2 questions (1–10 scale)\n" +
		"• «📊 Export CSV» — export data to CSV for a selected period",
	actionCancelled:    "Action cancelled.",
	exportChoosePeriod: "Choose a period for CSV export:",
	unknownInputHint: "Use the buttons below:\n" +
		"• «🍽 Record meal» — log satiety\n" +
		"• «📊 Export CSV» — download CSV",

	stepBeforeText: "Step 1/2. Rate your satiety BEFORE the meal (1 — very hungry, 10 — overstuffed):",
	stepAfterText: func(before int) string {
		return fmt.Sprintf("Satiety before: %d/10\n\nStep 2/2. Rate your satiety AFTER the meal (1 — very hungry, 10 — overstuffed):", before)
	},
	mealSavedConfirmation: func(localTime string, before, after int, delta string) string {
		return fmt.Sprintf(
			"✅ Meal recorded (%s)\nSatiety before: %d/10\nSatiety after: %d/10\nChange: %s",
			localTime, before, after, delta,
		)
	},
	mealCancelledText: "Recording cancelled.",

	cbInvalidValue:  "Invalid value",
	cbInvalidData:   "Invalid data",
	cbAlreadySaved:  "Already saved",
	cbSaveError:     "Failed to save",
	cbSaved:         "Saved!",
	cbCancelled:     "Cancelled",
	cbUnknownAction: "Unknown action",

	export7dLabel: func(from, to string) string {
		return fmt.Sprintf("%s — %s (7 days)", from, to)
	},
	export30dLabel: func(from, to string) string {
		return fmt.Sprintf("%s — %s (30 days)", from, to)
	},
	exportAllLabel:  "all time",
	exportAllStatus: "Exporting all time",
	exportPeriodStatus: func(label string) string {
		return "Exporting period: " + label
	},
	exportCustomPrompt: "Enter a date or date range.\n\n" +
		"Examples:\n" +
		"• 01.11.2026 - 24.11.2026\n" +
		"• 2026-11-01 - 2026-11-24\n" +
		"• 24.11.2026\n\n" +
		"Send /cancel to cancel",
	exportRangeInvalid: func(reason string) string {
		return fmt.Sprintf(
			"Could not parse period: %s.\n\n"+
				"Format examples:\n"+
				"• 01.11.2026 - 24.11.2026\n"+
				"• 2026-11-01 - 2026-11-24\n"+
				"• 24.11.2026\n\n"+
				"Send /cancel to cancel",
			reason,
		)
	},
	exportCancelledText: "Export cancelled.",
	exportDBError:       "Failed to fetch records from the database.",
	exportEmptyRange: func(period string) string {
		return fmt.Sprintf("No records found for period «%s».", period)
	},
	exportBuildError: "Failed to generate CSV file.",
	exportCaption: func(period string, count int) string {
		return fmt.Sprintf("CSV for period: %s\nRecords: %d", period, count)
	},
	exportSendError: "Failed to send CSV file.",

	errEmptyRange:         "empty period",
	errInvalidRangeFormat: "invalid date or range format",
	errInvalidStartDate: func(token string) string {
		return fmt.Sprintf("invalid start date %q", token)
	},
	errInvalidEndDate: func(token string) string {
		return fmt.Sprintf("invalid end date %q", token)
	},
	errEndBeforeStart: "end date is before start date",
}

var ruMessages = localeMessages{
	btnRecordMeal: "🍽 Записать приём пищи",
	btnExportCSV:  "📊 Выгрузить CSV",

	btnCancel:       "❌ Отмена",
	btnBack:         "⬅️ Назад",
	btnExport7d:     "За 7 дней",
	btnExport30d:    "За 30 дней",
	btnExportAll:    "За всё время",
	btnExportCustom: "📅 Свой период",

	welcomeText: "Привет! Я помогу отслеживать сытость до и после приёма пищи.\n\n" +
		"• «🍽 Записать приём пищи» — ответить на 2 вопроса (шкала 1–10)\n" +
		"• «📊 Выгрузить CSV» — выгрузить данные в CSV за выбранный период",
	actionCancelled:    "Действие отменено.",
	exportChoosePeriod: "Выберите период для выгрузки данных в CSV:",
	unknownInputHint: "Используйте кнопки ниже:\n" +
		"• «🍽 Записать приём пищи» — записать сытость\n" +
		"• «📊 Выгрузить CSV» — скачать CSV",

	stepBeforeText: "Шаг 1/2. Оцените сытость ДО приёма пищи (1 — очень голоден, 10 — переел):",
	stepAfterText: func(before int) string {
		return fmt.Sprintf("Сытость до: %d/10\n\nШаг 2/2. Оцените сытость ПОСЛЕ приёма пищи (1 — очень голоден, 10 — переел):", before)
	},
	mealSavedConfirmation: func(localTime string, before, after int, delta string) string {
		return fmt.Sprintf(
			"✅ Приём пищи записан (%s)\nСытость до: %d/10\nСытость после: %d/10\nИзменение: %s",
			localTime, before, after, delta,
		)
	},
	mealCancelledText: "Запись отменена.",

	cbInvalidValue:  "Некорректное значение",
	cbInvalidData:   "Некорректные данные",
	cbAlreadySaved:  "Уже сохранено",
	cbSaveError:     "Ошибка сохранения",
	cbSaved:         "Сохранено!",
	cbCancelled:     "Отменено",
	cbUnknownAction: "Неизвестное действие",

	export7dLabel: func(from, to string) string {
		return fmt.Sprintf("%s — %s (7 дней)", from, to)
	},
	export30dLabel: func(from, to string) string {
		return fmt.Sprintf("%s — %s (30 дней)", from, to)
	},
	exportAllLabel:  "всё время",
	exportAllStatus: "Выгрузка за всё время",
	exportPeriodStatus: func(label string) string {
		return "Выгрузка за период: " + label
	},
	exportCustomPrompt: "Введите дату или диапазон дат.\n\n" +
		"Примеры:\n" +
		"• 01.11.2026 - 24.11.2026\n" +
		"• 2026-11-01 - 2026-11-24\n" +
		"• 24.11.2026\n\n" +
		"Для отмены введите /cancel",
	exportRangeInvalid: func(reason string) string {
		return fmt.Sprintf(
			"Не удалось распознать период: %s.\n\n"+
				"Примеры формата:\n"+
				"• 01.11.2026 - 24.11.2026\n"+
				"• 2026-11-01 - 2026-11-24\n"+
				"• 24.11.2026\n\n"+
				"Для отмены введите /cancel",
			reason,
		)
	},
	exportCancelledText: "Выгрузка отменена.",
	exportDBError:       "Не удалось получить записи из базы данных.",
	exportEmptyRange: func(period string) string {
		return fmt.Sprintf("За период «%s» записей не найдено.", period)
	},
	exportBuildError: "Не удалось сформировать CSV-файл.",
	exportCaption: func(period string, count int) string {
		return fmt.Sprintf("CSV за период: %s\nЗаписей: %d", period, count)
	},
	exportSendError: "Не удалось отправить CSV-файл.",

	errEmptyRange:         "пустой период",
	errInvalidRangeFormat: "неверный формат даты или диапазона",
	errInvalidStartDate: func(token string) string {
		return fmt.Sprintf("некорректная дата начала %q", token)
	},
	errInvalidEndDate: func(token string) string {
		return fmt.Sprintf("некорректная дата окончания %q", token)
	},
	errEndBeforeStart: "конечная дата меньше начальной",
}

func messagesFor(loc Locale) localeMessages {
	if loc == LocaleRU {
		return ruMessages
	}
	return enMessages
}

func isRecordMealButton(text string) bool {
	return text == enMessages.btnRecordMeal || text == ruMessages.btnRecordMeal
}

func isExportCSVButton(text string) bool {
	return text == enMessages.btnExportCSV || text == ruMessages.btnExportCSV
}

func formatDateRangeError(loc Locale, err error) string {
	msgs := messagesFor(loc)
	var dateErr *export.InvalidDateError
	switch {
	case errors.Is(err, export.ErrEmptyRange):
		return msgs.errEmptyRange
	case errors.Is(err, export.ErrInvalidRangeFormat):
		return msgs.errInvalidRangeFormat
	case errors.As(err, &dateErr):
		if errors.Is(dateErr.Sentinel, export.ErrInvalidStartDate) {
			return msgs.errInvalidStartDate(dateErr.Token)
		}
		if errors.Is(dateErr.Sentinel, export.ErrInvalidEndDate) {
			return msgs.errInvalidEndDate(dateErr.Token)
		}
	case errors.Is(err, export.ErrEndBeforeStart):
		return msgs.errEndBeforeStart
	}
	return err.Error()
}
