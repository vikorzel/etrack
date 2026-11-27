package bot

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"meal-bot/internal/config"
	"meal-bot/internal/storage"
)

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
	CountMeals(ctx context.Context, userID int64) (int, error)
	SaveWeight(ctx context.Context, weight storage.Weight) (int64, error)
}

// Handler processes Telegram updates for logging meals, periodic weigh-ins, and exporting CSV reports.
type Handler struct {
	store           MealStore
	loc             *time.Location
	weighInInterval int
	now             func() time.Time

	mu             sync.Mutex
	awaitingRange  map[int64]bool
	awaitingWeight map[int64]bool
	savedMsgKeys   map[string]time.Time
}

// NewHandler creates a Handler backed by store, displaying times in loc, and prompting for weigh-in every weighInInterval meals.
func NewHandler(store MealStore, loc *time.Location, weighInInterval int) *Handler {
	if loc == nil {
		loc = time.Local
	}
	if weighInInterval <= 0 {
		weighInInterval = config.DefaultWeighInInterval
	}
	return &Handler{
		store:           store,
		loc:             loc,
		weighInInterval: weighInInterval,
		now:             time.Now,
		awaitingRange:   make(map[int64]bool),
		awaitingWeight:  make(map[int64]bool),
		savedMsgKeys:    make(map[string]time.Time),
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
		h.clearPendingInput(userID)
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.welcomeText)
	case cmd == "/cancel":
		h.clearPendingInput(userID)
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.actionCancelled)
	case isRecordMealButton(text) || cmd == "/meal":
		h.clearPendingInput(userID)
		if _, err := s.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      chatID,
			Text:        msgs.stepBeforeText,
			ReplyMarkup: satietyBeforeKeyboard(locale),
		}); err != nil {
			log.Printf("send step 1 message: %v", err)
		}
	case isExportCSVButton(text) || cmd == "/export":
		h.clearPendingInput(userID)
		if _, err := s.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      chatID,
			Text:        msgs.exportChoosePeriod,
			ReplyMarkup: exportPeriodKeyboard(locale),
		}); err != nil {
			log.Printf("send export menu: %v", err)
		}
	case h.isAwaitingWeight(userID):
		h.handleWeightInput(ctx, s, chatID, userID, locale, text)
	case h.isAwaitingRange(userID):
		h.handleCustomRangeInput(ctx, s, chatID, userID, locale, text)
	default:
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.unknownInputHint)
	}
}

type callbackContext struct {
	ctx       context.Context
	sender    Sender
	cb        *models.CallbackQuery
	chatID    int64
	messageID int
	hasMsg    bool
	userID    int64
	locale    Locale
	msgs      localeMessages
}

func newCallbackContext(ctx context.Context, s Sender, cb *models.CallbackQuery) callbackContext {
	chatID, messageID, hasMsg := extractCallbackTarget(cb)
	locale := resolveLocale(cb.From.LanguageCode)
	return callbackContext{
		ctx:       ctx,
		sender:    s,
		cb:        cb,
		chatID:    chatID,
		messageID: messageID,
		hasMsg:    hasMsg,
		userID:    cb.From.ID,
		locale:    locale,
		msgs:      messagesFor(locale),
	}
}

func (c callbackContext) answer(text string, alert bool) {
	if _, err := c.sender.AnswerCallbackQuery(c.ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: c.cb.ID,
		Text:            text,
		ShowAlert:       alert,
	}); err != nil {
		log.Printf("answer callback query: %v", err)
	}
}

func (c callbackContext) updateMessage(text string, markup models.ReplyMarkup) {
	if c.hasMsg {
		if _, err := c.sender.EditMessageText(c.ctx, &tgbot.EditMessageTextParams{
			ChatID:      c.chatID,
			MessageID:   c.messageID,
			Text:        text,
			ReplyMarkup: markup,
		}); err != nil {
			log.Printf("edit message %d: %v", c.messageID, err)
		}
		return
	}
	if _, err := c.sender.SendMessage(c.ctx, &tgbot.SendMessageParams{
		ChatID:      c.chatID,
		Text:        text,
		ReplyMarkup: markup,
	}); err != nil {
		log.Printf("send fallback message: %v", err)
	}
}

func (h *Handler) handleCallback(ctx context.Context, s Sender, cb *models.CallbackQuery) {
	cc := newCallbackContext(ctx, s, cb)
	switch {
	case strings.HasPrefix(cb.Data, "meal:"):
		h.handleMealCallback(cc, cb.Data)
	case strings.HasPrefix(cb.Data, "weight:"):
		h.handleWeightCallback(cc, cb.Data)
	case strings.HasPrefix(cb.Data, "export:"):
		h.handleExportCallback(cc, cb.Data)
	default:
		cc.answer(cc.msgs.cbUnknownAction, false)
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
