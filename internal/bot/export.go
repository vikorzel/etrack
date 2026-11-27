package bot

import (
	"bytes"
	"context"
	"log"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"meal-bot/internal/export"
)

func (h *Handler) handleExportCallback(cc callbackContext, data string) {
	switch data {
	case "export:7d":
		cc.answer("", false)
		nowLocal := h.now().In(h.loc)
		startToday := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, h.loc)
		from := startToday.AddDate(0, 0, -6)
		label := cc.msgs.export7dLabel(from.Format("02.01.2006"), startToday.Format("02.01.2006"))
		cc.updateMessage(cc.msgs.exportPeriodStatus(label), nil)
		h.sendCSVExport(cc.ctx, cc.sender, cc.chatID, cc.userID, cc.locale, &from, nil, "meals_last_7d.csv", label)

	case "export:30d":
		cc.answer("", false)
		nowLocal := h.now().In(h.loc)
		startToday := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, h.loc)
		from := startToday.AddDate(0, 0, -29)
		label := cc.msgs.export30dLabel(from.Format("02.01.2006"), startToday.Format("02.01.2006"))
		cc.updateMessage(cc.msgs.exportPeriodStatus(label), nil)
		h.sendCSVExport(cc.ctx, cc.sender, cc.chatID, cc.userID, cc.locale, &from, nil, "meals_last_30d.csv", label)

	case "export:all":
		cc.answer("", false)
		cc.updateMessage(cc.msgs.exportAllStatus, nil)
		h.sendCSVExport(cc.ctx, cc.sender, cc.chatID, cc.userID, cc.locale, nil, nil, "meals_all.csv", cc.msgs.exportAllLabel)

	case "export:custom":
		h.setAwaitingRange(cc.userID, true)
		cc.answer("", false)
		cc.updateMessage(cc.msgs.exportCustomPrompt, nil)

	case "export:cancel":
		h.setAwaitingRange(cc.userID, false)
		cc.answer(cc.msgs.cbCancelled, false)
		cc.updateMessage(cc.msgs.exportCancelledText, nil)

	default:
		cc.answer(cc.msgs.cbUnknownAction, false)
	}
}

func (h *Handler) handleCustomRangeInput(ctx context.Context, s Sender, chatID, userID int64, locale Locale, text string) {
	msgs := messagesFor(locale)
	from, to, fileLabel, displayLabel, err := export.ParseDateRange(text, h.loc)
	if err != nil {
		h.sendText(ctx, s, chatID, msgs.exportRangeInvalid(formatDateRangeError(locale, err)))
		return
	}
	h.setAwaitingRange(userID, false)
	h.sendCSVExport(ctx, s, chatID, userID, locale, &from, &to, "meals_"+fileLabel+".csv", displayLabel)
}

func (h *Handler) sendCSVExport(
	ctx context.Context,
	s Sender,
	chatID int64,
	userID int64,
	locale Locale,
	from *time.Time,
	to *time.Time,
	filename string,
	displayPeriod string,
) {
	msgs := messagesFor(locale)
	meals, err := h.store.ListMeals(ctx, userID, from, to)
	if err != nil {
		log.Printf("list meals for user %d: %v", userID, err)
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.exportDBError)
		return
	}
	if len(meals) == 0 {
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.exportEmptyRange(displayPeriod))
		return
	}

	csvBytes, err := export.BuildMealsCSV(meals, h.loc)
	if err != nil {
		log.Printf("build csv for user %d: %v", userID, err)
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.exportBuildError)
		return
	}

	caption := msgs.exportCaption(displayPeriod, len(meals))
	if _, err := s.SendDocument(ctx, &tgbot.SendDocumentParams{
		ChatID: chatID,
		Document: &models.InputFileUpload{
			Filename: filename,
			Data:     bytes.NewReader(csvBytes),
		},
		Caption: caption,
	}); err != nil {
		log.Printf("send csv document: %v", err)
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.exportSendError)
	}
}
