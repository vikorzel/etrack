package bot

import (
	"fmt"
	"strconv"

	"github.com/go-telegram/bot/models"
)

func mainKeyboard(locale Locale) *models.ReplyKeyboardMarkup {
	msgs := messagesFor(locale)
	return &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{
				{Text: msgs.btnRecordMeal},
				{Text: msgs.btnExportCSV},
			},
		},
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
}

func satietyBeforeKeyboard(locale Locale) *models.InlineKeyboardMarkup {
	msgs := messagesFor(locale)
	rows := scaleRows(func(n int) string {
		return fmt.Sprintf("meal:before:%d", n)
	})
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: msgs.btnCancel, CallbackData: "meal:cancel"},
	})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func satietyAfterKeyboard(locale Locale, before int) *models.InlineKeyboardMarkup {
	msgs := messagesFor(locale)
	rows := scaleRows(func(n int) string {
		return fmt.Sprintf("meal:after:%d:%d", before, n)
	})
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: msgs.btnBack, CallbackData: "meal:back"},
		{Text: msgs.btnCancel, CallbackData: "meal:cancel"},
	})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func scaleRows(callbackFor func(int) string) [][]models.InlineKeyboardButton {
	row1 := make([]models.InlineKeyboardButton, 0, 5)
	row2 := make([]models.InlineKeyboardButton, 0, 5)
	for i := 1; i <= 5; i++ {
		row1 = append(row1, models.InlineKeyboardButton{
			Text:         strconv.Itoa(i),
			CallbackData: callbackFor(i),
		})
	}
	for i := 6; i <= 10; i++ {
		row2 = append(row2, models.InlineKeyboardButton{
			Text:         strconv.Itoa(i),
			CallbackData: callbackFor(i),
		})
	}
	return [][]models.InlineKeyboardButton{row1, row2}
}

func weighInPromptKeyboard(locale Locale) *models.InlineKeyboardMarkup {
	msgs := messagesFor(locale)
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: msgs.btnWeighIn, CallbackData: "weight:start"},
			},
		},
	}
}

func weightCancelKeyboard(locale Locale) *models.InlineKeyboardMarkup {
	msgs := messagesFor(locale)
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: msgs.btnCancel, CallbackData: "weight:cancel"},
			},
		},
	}
}

func exportPeriodKeyboard(locale Locale) *models.InlineKeyboardMarkup {
	msgs := messagesFor(locale)
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: msgs.btnExport7d, CallbackData: "export:7d"},
				{Text: msgs.btnExport30d, CallbackData: "export:30d"},
			},
			{
				{Text: msgs.btnExportAll, CallbackData: "export:all"},
				{Text: msgs.btnExportCustom, CallbackData: "export:custom"},
			},
			{
				{Text: msgs.btnCancel, CallbackData: "export:cancel"},
			},
		},
	}
}
