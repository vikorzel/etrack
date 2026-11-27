package bot

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"meal-bot/internal/export"
	"meal-bot/internal/storage"
)

const savedKeyTTL = time.Hour

// Sender abstracts the Telegram Bot API calls used by Handler so tests can mock transport.
type Sender interface {
	SendMessage(ctx context.Context, params *tgbot.SendMessageParams) (*models.Message, error)
	EditMessageText(ctx context.Context, params *tgbot.EditMessageTextParams) (*models.Message, error)
	AnswerCallbackQuery(ctx context.Context, params *tgbot.AnswerCallbackQueryParams) (bool, error)
	SendDocument(ctx context.Context, params *tgbot.SendDocumentParams) (*models.Message, error)
}

// MealStore defines the persistence operations required by Handler.
type MealStore interface {
	SaveMeal(ctx context.Context, meal storage.Meal) (int64, error)
	ListMeals(ctx context.Context, userID int64, from, to *time.Time) ([]storage.Meal, error)
}

// Handler processes Telegram updates for logging meals and exporting CSV reports.
type Handler struct {
	store MealStore
	loc   *time.Location
	now   func() time.Time

	mu            sync.Mutex
	awaitingRange map[int64]bool
	savedMsgKeys  map[string]time.Time
}

// NewHandler creates a Handler backed by store and displaying times in loc.
func NewHandler(store MealStore, loc *time.Location) *Handler {
	if loc == nil {
		loc = time.Local
	}
	return &Handler{
		store:         store,
		loc:           loc,
		now:           time.Now,
		awaitingRange: make(map[int64]bool),
		savedMsgKeys:  make(map[string]time.Time),
	}
}

// DefaultHandler returns a go-telegram/bot HandlerFunc adapter.
func (h *Handler) DefaultHandler() tgbot.HandlerFunc {
	return func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
		h.HandleUpdate(ctx, b, update)
	}
}

// HandleUpdate routes incoming Telegram messages and inline callback queries.
func (h *Handler) HandleUpdate(ctx context.Context, s Sender, update *models.Update) {
	if update == nil || s == nil {
		return
	}
	if update.CallbackQuery != nil {
		h.handleCallback(ctx, s, update.CallbackQuery)
		return
	}
	if update.Message != nil && update.Message.From != nil {
		h.handleMessage(ctx, s, update.Message)
	}
}

func (h *Handler) handleMessage(ctx context.Context, s Sender, msg *models.Message) {
	userID := msg.From.ID
	chatID := msg.Chat.ID
	locale := resolveLocale(msg.From.LanguageCode)
	msgs := messagesFor(locale)
	text := strings.TrimSpace(msg.Text)
	cmd := parseCommand(text)

	switch {
	case cmd == "/start" || cmd == "/help":
		h.setAwaitingRange(userID, false)
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.welcomeText)
	case cmd == "/cancel":
		h.setAwaitingRange(userID, false)
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.actionCancelled)
	case isRecordMealButton(text) || cmd == "/meal":
		h.setAwaitingRange(userID, false)
		if _, err := s.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      chatID,
			Text:        msgs.stepBeforeText,
			ReplyMarkup: satietyBeforeKeyboard(locale),
		}); err != nil {
			log.Printf("send step 1 message: %v", err)
		}
	case isExportCSVButton(text) || cmd == "/export":
		h.setAwaitingRange(userID, false)
		if _, err := s.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      chatID,
			Text:        msgs.exportChoosePeriod,
			ReplyMarkup: exportPeriodKeyboard(locale),
		}); err != nil {
			log.Printf("send export menu: %v", err)
		}
	case h.isAwaitingRange(userID):
		from, to, fileLabel, displayLabel, err := export.ParseDateRange(text, h.loc)
		if err != nil {
			h.sendText(ctx, s, chatID, msgs.exportRangeInvalid(formatDateRangeError(locale, err)))
			return
		}
		h.setAwaitingRange(userID, false)
		h.sendCSVExport(ctx, s, chatID, userID, locale, &from, &to, "meals_"+fileLabel+".csv", displayLabel)
	default:
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.unknownInputHint)
	}
}

func (h *Handler) handleCallback(ctx context.Context, s Sender, cb *models.CallbackQuery) {
	chatID, messageID, hasMsg := extractCallbackTarget(cb)
	userID := cb.From.ID
	locale := resolveLocale(cb.From.LanguageCode)
	msgs := messagesFor(locale)
	data := cb.Data

	answer := func(text string, alert bool) {
		if _, err := s.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            text,
			ShowAlert:       alert,
		}); err != nil {
			log.Printf("answer callback query: %v", err)
		}
	}

	updateMessage := func(text string, markup models.ReplyMarkup) {
		if hasMsg {
			if _, err := s.EditMessageText(ctx, &tgbot.EditMessageTextParams{
				ChatID:      chatID,
				MessageID:   messageID,
				Text:        text,
				ReplyMarkup: markup,
			}); err != nil {
				log.Printf("edit message %d: %v", messageID, err)
			}
			return
		}
		if _, err := s.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      chatID,
			Text:        text,
			ReplyMarkup: markup,
		}); err != nil {
			log.Printf("send fallback message: %v", err)
		}
	}

	switch {
	case strings.HasPrefix(data, "meal:before:"):
		before, err := parseSatiety(strings.TrimPrefix(data, "meal:before:"))
		if err != nil {
			answer(msgs.cbInvalidValue, true)
			return
		}
		answer("", false)
		updateMessage(msgs.stepAfterText(before), satietyAfterKeyboard(locale, before))

	case data == "meal:back":
		answer("", false)
		updateMessage(msgs.stepBeforeText, satietyBeforeKeyboard(locale))

	case strings.HasPrefix(data, "meal:after:"):
		parts := strings.Split(strings.TrimPrefix(data, "meal:after:"), ":")
		if len(parts) != 2 {
			answer(msgs.cbInvalidData, true)
			return
		}
		before, errBefore := parseSatiety(parts[0])
		after, errAfter := parseSatiety(parts[1])
		if errBefore != nil || errAfter != nil {
			answer(msgs.cbInvalidValue, true)
			return
		}

		if hasMsg && !h.claimSaveKey(chatID, messageID) {
			answer(msgs.cbAlreadySaved, false)
			return
		}

		now := h.now().UTC()
		_, err := h.store.SaveMeal(ctx, storage.Meal{
			UserID:        userID,
			RecordedAt:    now,
			SatietyBefore: before,
			SatietyAfter:  after,
		})
		if err != nil {
			if hasMsg {
				h.releaseSaveKey(chatID, messageID)
			}
			log.Printf("save meal for user %d: %v", userID, err)
			answer(msgs.cbSaveError, true)
			return
		}

		answer(msgs.cbSaved, false)
		localTime := now.In(h.loc).Format("02.01.2006 15:04")
		delta := formatSignedDelta(after - before)
		updateMessage(msgs.mealSavedConfirmation(localTime, before, after, delta), nil)

	case data == "meal:cancel":
		answer(msgs.cbCancelled, false)
		updateMessage(msgs.mealCancelledText, nil)

	case data == "export:7d":
		answer("", false)
		nowLocal := h.now().In(h.loc)
		startToday := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, h.loc)
		from := startToday.AddDate(0, 0, -6)
		label := msgs.export7dLabel(from.Format("02.01.2006"), startToday.Format("02.01.2006"))
		updateMessage(msgs.exportPeriodStatus(label), nil)
		h.sendCSVExport(ctx, s, chatID, userID, locale, &from, nil, "meals_last_7d.csv", label)

	case data == "export:30d":
		answer("", false)
		nowLocal := h.now().In(h.loc)
		startToday := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, h.loc)
		from := startToday.AddDate(0, 0, -29)
		label := msgs.export30dLabel(from.Format("02.01.2006"), startToday.Format("02.01.2006"))
		updateMessage(msgs.exportPeriodStatus(label), nil)
		h.sendCSVExport(ctx, s, chatID, userID, locale, &from, nil, "meals_last_30d.csv", label)

	case data == "export:all":
		answer("", false)
		updateMessage(msgs.exportAllStatus, nil)
		h.sendCSVExport(ctx, s, chatID, userID, locale, nil, nil, "meals_all.csv", msgs.exportAllLabel)

	case data == "export:custom":
		h.setAwaitingRange(userID, true)
		answer("", false)
		updateMessage(msgs.exportCustomPrompt, nil)

	case data == "export:cancel":
		h.setAwaitingRange(userID, false)
		answer(msgs.cbCancelled, false)
		updateMessage(msgs.exportCancelledText, nil)

	default:
		answer(msgs.cbUnknownAction, false)
	}
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

func (h *Handler) sendText(ctx context.Context, s Sender, chatID int64, text string) {
	if _, err := s.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}); err != nil {
		log.Printf("send message: %v", err)
	}
}

func (h *Handler) sendTextWithMainMenu(ctx context.Context, s Sender, chatID int64, locale Locale, text string) {
	if _, err := s.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: mainKeyboard(locale),
	}); err != nil {
		log.Printf("send message with menu: %v", err)
	}
}

func (h *Handler) setAwaitingRange(userID int64, waiting bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if waiting {
		h.awaitingRange[userID] = true
	} else {
		delete(h.awaitingRange, userID)
	}
}

func (h *Handler) isAwaitingRange(userID int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.awaitingRange[userID]
}

// claimSaveKey records chatID:messageID once to avoid duplicate saves on rapid double-taps.
func (h *Handler) claimSaveKey(chatID int64, messageID int) bool {
	now := h.now()
	key := fmt.Sprintf("%d:%d", chatID, messageID)

	h.mu.Lock()
	defer h.mu.Unlock()

	for k, ts := range h.savedMsgKeys {
		if now.Sub(ts) > savedKeyTTL {
			delete(h.savedMsgKeys, k)
		}
	}
	if _, exists := h.savedMsgKeys[key]; exists {
		return false
	}
	h.savedMsgKeys[key] = now
	return true
}

func (h *Handler) releaseSaveKey(chatID int64, messageID int) {
	key := fmt.Sprintf("%d:%d", chatID, messageID)
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.savedMsgKeys, key)
}

func extractCallbackTarget(cb *models.CallbackQuery) (chatID int64, messageID int, hasMsg bool) {
	if cb.Message.Message != nil {
		return cb.Message.Message.Chat.ID, cb.Message.Message.ID, true
	}
	return cb.From.ID, 0, false
}

func parseCommand(text string) string {
	if !strings.HasPrefix(text, "/") {
		return ""
	}
	firstField := strings.Fields(text)[0]
	if idx := strings.Index(firstField, "@"); idx != -1 {
		firstField = firstField[:idx]
	}
	return strings.ToLower(firstField)
}

func parseSatiety(raw string) (int, error) {
	val, err := strconv.Atoi(raw)
	if err != nil || val < 1 || val > 10 {
		return 0, fmt.Errorf("satiety %q must be between 1 and 10", raw)
	}
	return val, nil
}

func formatSignedDelta(delta int) string {
	if delta > 0 {
		return fmt.Sprintf("+%d", delta)
	}
	return strconv.Itoa(delta)
}

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
