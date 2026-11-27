package bot

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"meal-bot/internal/config"
	"meal-bot/internal/storage"
)

type sentDoc struct {
	filename string
	caption  string
	content  string
}

type mockSender struct {
	sentMessages []tgbot.SendMessageParams
	editedMsgs   []tgbot.EditMessageTextParams
	callbacks    []tgbot.AnswerCallbackQueryParams
	documents    []sentDoc
	nextMsgID    int
}

func (m *mockSender) SendMessage(_ context.Context, params *tgbot.SendMessageParams) (*models.Message, error) {
	m.nextMsgID++
	m.sentMessages = append(m.sentMessages, *params)
	return &models.Message{
		ID:   m.nextMsgID,
		Chat: models.Chat{ID: params.ChatID.(int64)},
		Text: params.Text,
	}, nil
}

func (m *mockSender) EditMessageText(_ context.Context, params *tgbot.EditMessageTextParams) (*models.Message, error) {
	m.editedMsgs = append(m.editedMsgs, *params)
	return &models.Message{
		ID:   params.MessageID,
		Chat: models.Chat{ID: params.ChatID.(int64)},
		Text: params.Text,
	}, nil
}

func (m *mockSender) AnswerCallbackQuery(_ context.Context, params *tgbot.AnswerCallbackQueryParams) (bool, error) {
	m.callbacks = append(m.callbacks, *params)
	return true, nil
}

func (m *mockSender) SendDocument(_ context.Context, params *tgbot.SendDocumentParams) (*models.Message, error) {
	m.nextMsgID++
	upload := params.Document.(*models.InputFileUpload)
	raw, err := io.ReadAll(upload.Data)
	if err != nil {
		return nil, err
	}
	m.documents = append(m.documents, sentDoc{
		filename: upload.Filename,
		caption:  params.Caption,
		content:  string(raw),
	})
	return &models.Message{ID: m.nextMsgID}, nil
}

func newTestHandler(t *testing.T, now time.Time) (*Handler, *storage.SQLiteStore, *mockSender) {
	t.Helper()
	return newTestHandlerWithInterval(t, now, config.DefaultWeighInInterval)
}

func newTestHandlerWithInterval(t *testing.T, now time.Time, interval int) (*Handler, *storage.SQLiteStore, *mockSender) {
	t.Helper()
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "bot_test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	loc := time.FixedZone("UTC+2", 2*3600)
	h := NewHandler(store, loc, interval)
	h.now = func() time.Time { return now }
	return h, store, &mockSender{nextMsgID: 10}
}

func msgUpdate(userID, chatID int64, text string) *models.Update {
	return msgUpdateLang(userID, chatID, "", text)
}

func msgUpdateLang(userID, chatID int64, lang, text string) *models.Update {
	return &models.Update{
		Message: &models.Message{
			ID:   1,
			From: &models.User{ID: userID, LanguageCode: lang},
			Chat: models.Chat{ID: chatID},
			Text: text,
		},
	}
}

func cbUpdate(cbID string, userID, chatID int64, messageID int, data string) *models.Update {
	return cbUpdateLang(cbID, userID, chatID, "", messageID, data)
}

func cbUpdateLang(cbID string, userID, chatID int64, lang string, messageID int, data string) *models.Update {
	return &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   cbID,
			From: models.User{ID: userID, LanguageCode: lang},
			Data: data,
			Message: models.MaybeInaccessibleMessage{
				Type: models.MaybeInaccessibleMessageTypeMessage,
				Message: &models.Message{
					ID:   messageID,
					Chat: models.Chat{ID: chatID},
				},
			},
		},
	}
}

func TestStartShowsMainKeyboard(t *testing.T) {
	h, _, sender := newTestHandler(t, time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	ctx := context.Background()

	h.HandleUpdate(ctx, sender, msgUpdate(101, 101, "/start"))

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sentMessages = %d, want 1", len(sender.sentMessages))
	}
	if sender.sentMessages[0].Text != enMessages.welcomeText {
		t.Fatalf("welcome text = %q, want %q", sender.sentMessages[0].Text, enMessages.welcomeText)
	}
	kb, ok := sender.sentMessages[0].ReplyMarkup.(*models.ReplyKeyboardMarkup)
	if !ok {
		t.Fatalf("ReplyMarkup type = %T, want *models.ReplyKeyboardMarkup", sender.sentMessages[0].ReplyMarkup)
	}
	if len(kb.Keyboard) != 1 || len(kb.Keyboard[0]) != 2 {
		t.Fatalf("unexpected keyboard shape: %+v", kb.Keyboard)
	}
	if kb.Keyboard[0][0].Text != enMessages.btnRecordMeal || kb.Keyboard[0][1].Text != enMessages.btnExportCSV {
		t.Fatalf("unexpected keyboard labels: %+v", kb.Keyboard[0])
	}
}

func TestRecordMealFlowBackCancelAndDedup(t *testing.T) {
	fixedNow := time.Date(2026, 11, 24, 10, 30, 0, 0, time.UTC)
	h, store, sender := newTestHandler(t, fixedNow)
	ctx := context.Background()

	// Step 1: tap record button
	h.HandleUpdate(ctx, sender, msgUpdate(77, 77, enMessages.btnRecordMeal))
	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected step 1 prompt, got %d messages", len(sender.sentMessages))
	}

	// Select before=3 -> shows step 2
	h.HandleUpdate(ctx, sender, cbUpdate("cb1", 77, 77, 11, "meal:before:3"))
	if len(sender.editedMsgs) != 1 || !strings.Contains(sender.editedMsgs[0].Text, "Satiety before: 3/10") {
		t.Fatalf("unexpected step 2 edit: %+v", sender.editedMsgs)
	}

	// Go back -> returns to step 1
	h.HandleUpdate(ctx, sender, cbUpdate("cb2", 77, 77, 11, "meal:back"))
	if len(sender.editedMsgs) != 2 || !strings.Contains(sender.editedMsgs[1].Text, "Step 1/2") {
		t.Fatalf("unexpected back edit: %+v", sender.editedMsgs)
	}

	// Select before=3 then after=8
	h.HandleUpdate(ctx, sender, cbUpdate("cb3", 77, 77, 11, "meal:before:3"))
	h.HandleUpdate(ctx, sender, cbUpdate("cb4", 77, 77, 11, "meal:after:3:8"))

	// Duplicate tap on same message ID must not create a second row.
	h.HandleUpdate(ctx, sender, cbUpdate("cb5", 77, 77, 11, "meal:after:3:8"))

	meals, err := store.ListMeals(ctx, 77, nil, nil)
	if err != nil {
		t.Fatalf("ListMeals: %v", err)
	}
	if len(meals) != 1 {
		t.Fatalf("len(meals) = %d, want 1", len(meals))
	}
	if meals[0].SatietyBefore != 3 || meals[0].SatietyAfter != 8 {
		t.Fatalf("saved meal = %+v, want before=3 after=8", meals[0])
	}

	lastEdit := sender.editedMsgs[len(sender.editedMsgs)-1]
	if !strings.Contains(lastEdit.Text, "24.11.2026 12:30") || !strings.Contains(lastEdit.Text, "+5") || !strings.Contains(lastEdit.Text, "Meal recorded") {
		t.Fatalf("unexpected confirmation text: %q", lastEdit.Text)
	}

	// Start another meal and cancel it -> still 1 meal in DB.
	h.HandleUpdate(ctx, sender, cbUpdate("cb6", 77, 77, 12, "meal:cancel"))
	meals, err = store.ListMeals(ctx, 77, nil, nil)
	if err != nil {
		t.Fatalf("ListMeals after cancel: %v", err)
	}
	if len(meals) != 1 {
		t.Fatalf("len(meals) after cancel = %d, want 1", len(meals))
	}
}

func TestExportPresetsAndCustomRange(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) // 14:00 UTC+2
	h, store, sender := newTestHandler(t, now)
	ctx := context.Background()

	// Seed 2 meals: one 10 days ago, one today.
	oldTime := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	todayTime := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	if _, err := store.SaveMeal(ctx, storage.Meal{UserID: 55, RecordedAt: oldTime, SatietyBefore: 2, SatietyAfter: 7}); err != nil {
		t.Fatalf("SaveMeal old: %v", err)
	}
	if _, err := store.SaveMeal(ctx, storage.Meal{UserID: 55, RecordedAt: todayTime, SatietyBefore: 4, SatietyAfter: 8}); err != nil {
		t.Fatalf("SaveMeal today: %v", err)
	}

	// Open export picker via button.
	h.HandleUpdate(ctx, sender, msgUpdate(55, 55, enMessages.btnExportCSV))
	if len(sender.sentMessages) != 1 {
		t.Fatalf("expected export picker message")
	}

	// Preset 7 days -> should include only todayTime (1 row + header = 2 CSV lines).
	h.HandleUpdate(ctx, sender, cbUpdate("e1", 55, 55, 11, "export:7d"))
	if len(sender.documents) != 1 {
		t.Fatalf("documents = %d, want 1", len(sender.documents))
	}
	if sender.documents[0].filename != "meals_last_7d.csv" {
		t.Fatalf("filename = %q, want meals_last_7d.csv", sender.documents[0].filename)
	}
	rows7d, err := csv.NewReader(strings.NewReader(sender.documents[0].content)).ReadAll()
	if err != nil || len(rows7d) != 2 {
		t.Fatalf("rows7d = %v (err=%v), want 2 rows", rows7d, err)
	}

	// Preset all -> 2 data rows + header = 3 lines.
	h.HandleUpdate(ctx, sender, cbUpdate("e2", 55, 55, 11, "export:all"))
	if len(sender.documents) != 2 || sender.documents[1].filename != "meals_all.csv" {
		t.Fatalf("unexpected documents after export:all: %+v", sender.documents)
	}
	rowsAll, err := csv.NewReader(strings.NewReader(sender.documents[1].content)).ReadAll()
	if err != nil || len(rowsAll) != 3 {
		t.Fatalf("rowsAll = %v (err=%v), want 3 rows", rowsAll, err)
	}

	// Custom range flow: click custom -> invalid text keeps awaiting -> valid empty range sends no file -> valid range sends file and clears state.
	h.HandleUpdate(ctx, sender, cbUpdate("e3", 55, 55, 11, "export:custom"))
	h.HandleUpdate(ctx, sender, msgUpdate(55, 55, "bad input"))
	if !h.isAwaitingRange(55) {
		t.Fatal("expected user 55 to remain in awaitingRange after invalid input")
	}
	errMsg := sender.sentMessages[len(sender.sentMessages)-1]
	if !strings.Contains(errMsg.Text, "Could not parse period") {
		t.Fatalf("expected English date-range error, got %q", errMsg.Text)
	}

	// Valid range with no meals -> clears awaitingRange, sends info message, no document.
	prevDocs := len(sender.documents)
	h.HandleUpdate(ctx, sender, msgUpdate(55, 55, "01.10.2026 - 05.10.2026"))
	if h.isAwaitingRange(55) {
		t.Fatal("expected awaitingRange to clear after valid range")
	}
	if len(sender.documents) != prevDocs {
		t.Fatalf("expected no document for empty date range, got %d docs", len(sender.documents)-prevDocs)
	}
	lastMsg := sender.sentMessages[len(sender.sentMessages)-1]
	if !strings.Contains(lastMsg.Text, "No records found") {
		t.Fatalf("expected empty-range message, got %q", lastMsg.Text)
	}

	// Custom range matching oldTime only.
	h.HandleUpdate(ctx, sender, cbUpdate("e4", 55, 55, 11, "export:custom"))
	h.HandleUpdate(ctx, sender, msgUpdate(55, 55, "14.11.2026"))
	if len(sender.documents) != prevDocs+1 {
		t.Fatalf("expected new document for 14.11.2026")
	}
	doc := sender.documents[len(sender.documents)-1]
	if doc.filename != "meals_2026-11-14.csv" {
		t.Fatalf("filename = %q, want meals_2026-11-14.csv", doc.filename)
	}

	// /cancel clears awaitingRange.
	h.HandleUpdate(ctx, sender, cbUpdate("e5", 55, 55, 11, "export:custom"))
	h.HandleUpdate(ctx, sender, msgUpdate(55, 55, "/cancel"))
	if h.isAwaitingRange(55) {
		t.Fatal("expected /cancel to clear awaitingRange")
	}
}

func TestResolveLocale(t *testing.T) {
	cases := []struct {
		in   string
		want Locale
	}{
		{"", LocaleEN},
		{"   ", LocaleEN},
		{"en", LocaleEN},
		{"EN", LocaleEN},
		{"en-US", LocaleEN},
		{"en_GB", LocaleEN},
		{"ru", LocaleRU},
		{"RU", LocaleRU},
		{"ru-RU", LocaleRU},
		{"ru_RU", LocaleRU},
		{"de", LocaleEN},
		{"es-ES", LocaleEN},
		{"uk", LocaleEN},
	}
	for _, tc := range cases {
		if got := resolveLocale(tc.in); got != tc.want {
			t.Errorf("resolveLocale(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLocalizationRussianFlow(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 30, 0, 0, time.UTC)
	h, _, sender := newTestHandler(t, now)
	ctx := context.Background()

	// /start with ru-RU
	h.HandleUpdate(ctx, sender, msgUpdateLang(88, 88, "ru-RU", "/start"))
	if sender.sentMessages[0].Text != ruMessages.welcomeText {
		t.Fatalf("ru welcome = %q, want %q", sender.sentMessages[0].Text, ruMessages.welcomeText)
	}
	kb := sender.sentMessages[0].ReplyMarkup.(*models.ReplyKeyboardMarkup)
	if kb.Keyboard[0][0].Text != ruMessages.btnRecordMeal || kb.Keyboard[0][1].Text != ruMessages.btnExportCSV {
		t.Fatalf("unexpected ru keyboard: %+v", kb.Keyboard[0])
	}

	// Meal recording in ru
	h.HandleUpdate(ctx, sender, msgUpdateLang(88, 88, "ru", ruMessages.btnRecordMeal))
	if sender.sentMessages[1].Text != ruMessages.stepBeforeText {
		t.Fatalf("ru stepBefore = %q, want %q", sender.sentMessages[1].Text, ruMessages.stepBeforeText)
	}
	h.HandleUpdate(ctx, sender, cbUpdateLang("r1", 88, 88, "ru", 11, "meal:before:4"))
	if !strings.Contains(sender.editedMsgs[0].Text, "Сытость до: 4/10") {
		t.Fatalf("ru stepAfter = %q", sender.editedMsgs[0].Text)
	}
	h.HandleUpdate(ctx, sender, cbUpdateLang("r2", 88, 88, "ru-RU", 11, "meal:after:4:8"))
	if sender.callbacks[len(sender.callbacks)-1].Text != ruMessages.cbSaved {
		t.Fatalf("ru cbSaved = %q", sender.callbacks[len(sender.callbacks)-1].Text)
	}
	if !strings.Contains(sender.editedMsgs[1].Text, "Приём пищи записан") {
		t.Fatalf("ru confirmation = %q", sender.editedMsgs[1].Text)
	}

	// Export in ru + invalid custom date range error in ru
	h.HandleUpdate(ctx, sender, msgUpdateLang(88, 88, "ru", ruMessages.btnExportCSV))
	if sender.sentMessages[2].Text != ruMessages.exportChoosePeriod {
		t.Fatalf("ru export prompt = %q", sender.sentMessages[2].Text)
	}
	h.HandleUpdate(ctx, sender, cbUpdateLang("r3", 88, 88, "ru", 12, "export:7d"))
	if len(sender.documents) != 1 || !strings.Contains(sender.documents[0].caption, "CSV за период:") {
		t.Fatalf("ru export doc caption = %+v", sender.documents)
	}

	h.HandleUpdate(ctx, sender, cbUpdateLang("r4", 88, 88, "ru", 12, "export:custom"))
	h.HandleUpdate(ctx, sender, msgUpdateLang(88, 88, "ru", "31.02.2026"))
	lastMsg := sender.sentMessages[len(sender.sentMessages)-1]
	if !strings.Contains(lastMsg.Text, "некорректная дата начала") {
		t.Fatalf("expected Russian invalid start date error, got %q", lastMsg.Text)
	}
}

func TestLocalizationEnglishRegionalAndFallback(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 30, 0, 0, time.UTC)
	h, _, sender := newTestHandler(t, now)
	ctx := context.Background()

	// en-US -> English
	h.HandleUpdate(ctx, sender, msgUpdateLang(91, 91, "en-US", "/start"))
	if sender.sentMessages[0].Text != enMessages.welcomeText {
		t.Fatalf("en-US welcome = %q", sender.sentMessages[0].Text)
	}

	// Unsupported "de" -> English fallback
	h.HandleUpdate(ctx, sender, msgUpdateLang(92, 92, "de", "/start"))
	if sender.sentMessages[1].Text != enMessages.welcomeText {
		t.Fatalf("de fallback welcome = %q", sender.sentMessages[1].Text)
	}
	h.HandleUpdate(ctx, sender, cbUpdateLang("d1", 92, 92, "de", 11, "export:custom"))
	h.HandleUpdate(ctx, sender, msgUpdateLang(92, 92, "de", "24.11.2026 - 01.11.2026"))
	lastMsg := sender.sentMessages[len(sender.sentMessages)-1]
	if !strings.Contains(lastMsg.Text, "end date is before start date") {
		t.Fatalf("expected English fallback error, got %q", lastMsg.Text)
	}
}

func TestCrossLocaleReplyKeyboardMatching(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 30, 0, 0, time.UTC)
	h, _, sender := newTestHandler(t, now)
	ctx := context.Background()

	// User has LanguageCode="en" but taps cached Russian record button -> replies in English.
	h.HandleUpdate(ctx, sender, msgUpdateLang(95, 95, "en", ruMessages.btnRecordMeal))
	if len(sender.sentMessages) != 1 || sender.sentMessages[0].Text != enMessages.stepBeforeText {
		t.Fatalf("expected English stepBeforeText, got %+v", sender.sentMessages)
	}

	// User has LanguageCode="ru" but taps cached English export button -> replies in Russian.
	h.HandleUpdate(ctx, sender, msgUpdateLang(96, 96, "ru", enMessages.btnExportCSV))
	if len(sender.sentMessages) != 2 || sender.sentMessages[1].Text != ruMessages.exportChoosePeriod {
		t.Fatalf("expected Russian exportChoosePeriod, got %+v", sender.sentMessages)
	}
}

type countErrStore struct {
	MealStore
}

func (s countErrStore) CountMeals(context.Context, int64) (int, error) {
	return 0, errors.New("count failed")
}

func TestWeighInLocalizationMessagesPopulated(t *testing.T) {
	for _, loc := range []Locale{LocaleEN, LocaleRU} {
		msgs := messagesFor(loc)
		if msgs.btnWeighIn == "" || msgs.weightPrompt == "" || msgs.weightInvalid == "" ||
			msgs.weightSaveError == "" || msgs.weightCancelledText == "" ||
			msgs.weightSavedConfirmation("24.11.2026 12:30", "72.5") == "" {
			t.Fatalf("locale %q has empty weigh-in message field", loc)
		}
	}
}

func TestPeriodicWeighInTriggerAndIsolation(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 30, 0, 0, time.UTC)
	h, _, sender := newTestHandlerWithInterval(t, now, 3)
	ctx := context.Background()

	saveMeal := func(userID int64, msgID int) tgbot.EditMessageTextParams {
		h.HandleUpdate(ctx, sender, cbUpdate(fmt.Sprintf("cb-%d-%d", userID, msgID), userID, userID, msgID, "meal:after:3:7"))
		return sender.editedMsgs[len(sender.editedMsgs)-1]
	}

	// Meals 1 and 2 for user 10: no weigh-in button.
	if edit := saveMeal(10, 101); edit.ReplyMarkup != nil {
		t.Fatalf("meal 1 should not attach ReplyMarkup, got %#v", edit.ReplyMarkup)
	}
	if edit := saveMeal(10, 102); edit.ReplyMarkup != nil {
		t.Fatalf("meal 2 should not attach ReplyMarkup, got %#v", edit.ReplyMarkup)
	}

	// Different user saving a meal does not trigger weigh-in for user 10 or user 20.
	if edit := saveMeal(20, 201); edit.ReplyMarkup != nil {
		t.Fatalf("user 20 meal 1 should not attach ReplyMarkup, got %#v", edit.ReplyMarkup)
	}

	// Meal 3 for user 10: triggers weigh-in button.
	edit3 := saveMeal(10, 103)
	kb, ok := edit3.ReplyMarkup.(*models.InlineKeyboardMarkup)
	if !ok || len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 1 {
		t.Fatalf("meal 3 expected single inline weigh-in button, got %#v", edit3.ReplyMarkup)
	}
	btn := kb.InlineKeyboard[0][0]
	if btn.Text != enMessages.btnWeighIn || btn.CallbackData != "weight:start" {
		t.Fatalf("unexpected weigh-in button: %+v", btn)
	}

	// Meals 4 and 5: no button; meal 6: triggers again.
	if edit := saveMeal(10, 104); edit.ReplyMarkup != nil {
		t.Fatalf("meal 4 should not attach ReplyMarkup, got %#v", edit.ReplyMarkup)
	}
	if edit := saveMeal(10, 105); edit.ReplyMarkup != nil {
		t.Fatalf("meal 5 should not attach ReplyMarkup, got %#v", edit.ReplyMarkup)
	}
	edit6 := saveMeal(10, 106)
	if _, ok := edit6.ReplyMarkup.(*models.InlineKeyboardMarkup); !ok {
		t.Fatalf("meal 6 expected weigh-in inline button, got %#v", edit6.ReplyMarkup)
	}
}

func TestPeriodicWeighInCountMealsErrorFallback(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 30, 0, 0, time.UTC)
	_, baseStore, sender := newTestHandlerWithInterval(t, now, 1)
	loc := time.FixedZone("UTC+2", 2*3600)
	h := NewHandler(countErrStore{MealStore: baseStore}, loc, 1)
	h.now = func() time.Time { return now }

	ctx := context.Background()
	h.HandleUpdate(ctx, sender, cbUpdate("cb-err", 33, 33, 301, "meal:after:4:8"))

	if len(sender.editedMsgs) != 1 {
		t.Fatalf("expected 1 confirmation edit, got %d", len(sender.editedMsgs))
	}
	edit := sender.editedMsgs[0]
	if !strings.Contains(edit.Text, "Meal recorded") {
		t.Fatalf("expected meal confirmation text, got %q", edit.Text)
	}
	if edit.ReplyMarkup != nil {
		t.Fatalf("expected nil ReplyMarkup when CountMeals fails, got %#v", edit.ReplyMarkup)
	}
}

func TestWeighInFlowValidationAndPersistence(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 30, 0, 0, time.UTC) // 12:30 UTC+2
	h, store, sender := newTestHandlerWithInterval(t, now, 1)
	ctx := context.Background()

	// Tap weigh-in button -> enters awaitingWeight and shows prompt with cancel button.
	h.HandleUpdate(ctx, sender, cbUpdate("w1", 40, 40, 401, "weight:start"))
	if !h.isAwaitingWeight(40) {
		t.Fatal("expected user 40 to be awaitingWeight after weight:start")
	}
	promptEdit := sender.editedMsgs[len(sender.editedMsgs)-1]
	if promptEdit.Text != enMessages.weightPrompt {
		t.Fatalf("prompt text = %q, want %q", promptEdit.Text, enMessages.weightPrompt)
	}
	cancelKB, ok := promptEdit.ReplyMarkup.(*models.InlineKeyboardMarkup)
	if !ok || len(cancelKB.InlineKeyboard) != 1 || cancelKB.InlineKeyboard[0][0].CallbackData != "weight:cancel" {
		t.Fatalf("unexpected weight prompt keyboard: %#v", promptEdit.ReplyMarkup)
	}

	// Invalid inputs keep awaitingWeight and save nothing.
	for _, bad := range []string{"abc", "0", "-10", "501", "72.5.1", "kg"} {
		h.HandleUpdate(ctx, sender, msgUpdate(40, 40, bad))
		if !h.isAwaitingWeight(40) {
			t.Fatalf("expected awaitingWeight to remain true for invalid input %q", bad)
		}
		lastMsg := sender.sentMessages[len(sender.sentMessages)-1]
		if lastMsg.Text != enMessages.weightInvalid {
			t.Fatalf("invalid input %q message = %q, want %q", bad, lastMsg.Text, enMessages.weightInvalid)
		}
	}

	// Valid inputs with dot, comma, and kg/кг suffixes.
	validCases := []struct {
		input string
		want  float64
		text  string
	}{
		{"72.5", 72.5, "72.5 kg"},
		{"71,25", 71.25, "71.25 kg"},
		{"70.5 kg", 70.5, "70.5 kg"},
		{"69,8 кг", 69.8, "69.8 kg"},
	}
	for i, tc := range validCases {
		if i > 0 {
			h.HandleUpdate(ctx, sender, cbUpdate(fmt.Sprintf("w-start-%d", i), 40, 40, 401+i, "weight:start"))
		}
		h.HandleUpdate(ctx, sender, msgUpdate(40, 40, tc.input))
		if h.isAwaitingWeight(40) {
			t.Fatalf("expected awaitingWeight to clear after valid input %q", tc.input)
		}
		lastMsg := sender.sentMessages[len(sender.sentMessages)-1]
		if !strings.Contains(lastMsg.Text, "24.11.2026 12:30") || !strings.Contains(lastMsg.Text, tc.text) {
			t.Fatalf("unexpected confirmation for %q: %q", tc.input, lastMsg.Text)
		}
	}

	weights, err := store.ListWeights(ctx, 40, nil, nil)
	if err != nil {
		t.Fatalf("ListWeights: %v", err)
	}
	if len(weights) != len(validCases) {
		t.Fatalf("len(weights) = %d, want %d", len(weights), len(validCases))
	}
	for i, tc := range validCases {
		if weights[i].WeightKg != tc.want {
			t.Errorf("weights[%d].WeightKg = %v, want %v", i, weights[i].WeightKg, tc.want)
		}
	}
}

func TestWeighInCancellationAndStateMutualExclusion(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 30, 0, 0, time.UTC)
	h, store, sender := newTestHandlerWithInterval(t, now, 1)
	ctx := context.Background()

	// Inline cancel button clears awaitingWeight.
	h.HandleUpdate(ctx, sender, cbUpdate("c1", 50, 50, 501, "weight:start"))
	h.HandleUpdate(ctx, sender, cbUpdate("c2", 50, 50, 501, "weight:cancel"))
	if h.isAwaitingWeight(50) {
		t.Fatal("expected weight:cancel to clear awaitingWeight")
	}
	if sender.editedMsgs[len(sender.editedMsgs)-1].Text != enMessages.weightCancelledText {
		t.Fatalf("unexpected cancel text: %q", sender.editedMsgs[len(sender.editedMsgs)-1].Text)
	}

	// /cancel clears awaitingWeight.
	h.HandleUpdate(ctx, sender, cbUpdate("c3", 50, 50, 502, "weight:start"))
	h.HandleUpdate(ctx, sender, msgUpdate(50, 50, "/cancel"))
	if h.isAwaitingWeight(50) {
		t.Fatal("expected /cancel to clear awaitingWeight")
	}

	// Main menu button clears awaitingWeight.
	h.HandleUpdate(ctx, sender, cbUpdate("c4", 50, 50, 503, "weight:start"))
	h.HandleUpdate(ctx, sender, msgUpdate(50, 50, enMessages.btnRecordMeal))
	if h.isAwaitingWeight(50) {
		t.Fatal("expected Record meal button to clear awaitingWeight")
	}

	// Mutual exclusion with awaitingRange.
	h.HandleUpdate(ctx, sender, cbUpdate("c5", 50, 50, 504, "export:custom"))
	if !h.isAwaitingRange(50) {
		t.Fatal("expected awaitingRange to be true")
	}
	h.HandleUpdate(ctx, sender, cbUpdate("c6", 50, 50, 505, "weight:start"))
	if h.isAwaitingRange(50) || !h.isAwaitingWeight(50) {
		t.Fatalf("expected awaitingWeight=true and awaitingRange=false, got weight=%v range=%v", h.isAwaitingWeight(50), h.isAwaitingRange(50))
	}
	h.HandleUpdate(ctx, sender, cbUpdate("c7", 50, 50, 506, "export:custom"))
	if !h.isAwaitingRange(50) || h.isAwaitingWeight(50) {
		t.Fatalf("expected awaitingRange=true and awaitingWeight=false, got range=%v weight=%v", h.isAwaitingRange(50), h.isAwaitingWeight(50))
	}

	weights, err := store.ListWeights(ctx, 50, nil, nil)
	if err != nil {
		t.Fatalf("ListWeights: %v", err)
	}
	if len(weights) != 0 {
		t.Fatalf("expected 0 saved weights after cancellations, got %d", len(weights))
	}
}

func TestWeighInLocalizationRussian(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 30, 0, 0, time.UTC)
	h, _, sender := newTestHandlerWithInterval(t, now, 1)
	ctx := context.Background()

	// 1st meal triggers Russian weigh-in button.
	h.HandleUpdate(ctx, sender, cbUpdateLang("ru1", 60, 60, "ru-RU", 601, "meal:after:3:8"))
	edit := sender.editedMsgs[len(sender.editedMsgs)-1]
	kb, ok := edit.ReplyMarkup.(*models.InlineKeyboardMarkup)
	if !ok || kb.InlineKeyboard[0][0].Text != ruMessages.btnWeighIn {
		t.Fatalf("expected Russian weigh-in button, got %#v", edit.ReplyMarkup)
	}

	// Start weigh-in in Russian.
	h.HandleUpdate(ctx, sender, cbUpdateLang("ru2", 60, 60, "ru", 601, "weight:start"))
	if sender.editedMsgs[len(sender.editedMsgs)-1].Text != ruMessages.weightPrompt {
		t.Fatalf("expected Russian weightPrompt, got %q", sender.editedMsgs[len(sender.editedMsgs)-1].Text)
	}

	// Invalid weight in Russian.
	h.HandleUpdate(ctx, sender, msgUpdateLang(60, 60, "ru", "abc"))
	if sender.sentMessages[len(sender.sentMessages)-1].Text != ruMessages.weightInvalid {
		t.Fatalf("expected Russian weightInvalid, got %q", sender.sentMessages[len(sender.sentMessages)-1].Text)
	}

	// Valid weight in Russian.
	h.HandleUpdate(ctx, sender, msgUpdateLang(60, 60, "ru", "68,4"))
	lastMsg := sender.sentMessages[len(sender.sentMessages)-1]
	if !strings.Contains(lastMsg.Text, "Вес записан") || !strings.Contains(lastMsg.Text, "68.4 кг") {
		t.Fatalf("expected Russian weight confirmation, got %q", lastMsg.Text)
	}

	// Cancel in Russian.
	h.HandleUpdate(ctx, sender, cbUpdateLang("ru3", 60, 60, "ru", 602, "weight:start"))
	h.HandleUpdate(ctx, sender, cbUpdateLang("ru4", 60, 60, "ru", 602, "weight:cancel"))
	if sender.editedMsgs[len(sender.editedMsgs)-1].Text != ruMessages.weightCancelledText {
		t.Fatalf("expected Russian weightCancelledText, got %q", sender.editedMsgs[len(sender.editedMsgs)-1].Text)
	}
}
