package bot

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"meal-bot/internal/storage"
)

func (h *Handler) handleWeightCallback(cc callbackContext, data string) {
	switch data {
	case "weight:start":
		h.setAwaitingWeight(cc.userID, true)
		cc.answer("", false)
		cc.updateMessage(cc.msgs.weightPrompt, weightCancelKeyboard(cc.locale))

	case "weight:cancel":
		h.setAwaitingWeight(cc.userID, false)
		cc.answer(cc.msgs.cbCancelled, false)
		cc.updateMessage(cc.msgs.weightCancelledText, nil)

	default:
		cc.answer(cc.msgs.cbUnknownAction, false)
	}
}

func (h *Handler) handleWeightInput(ctx context.Context, s Sender, chatID, userID int64, locale Locale, text string) {
	msgs := messagesFor(locale)
	weightKg, err := parseWeightKg(text)
	if err != nil {
		h.sendText(ctx, s, chatID, msgs.weightInvalid)
		return
	}

	now := h.now().UTC()
	if _, err := h.store.SaveWeight(ctx, storage.Weight{
		UserID:     userID,
		RecordedAt: now,
		WeightKg:   weightKg,
	}); err != nil {
		log.Printf("save weight for user %d: %v", userID, err)
		h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.weightSaveError)
		return
	}

	h.setAwaitingWeight(userID, false)
	localTime := now.In(h.loc).Format("02.01.2006 15:04")
	h.sendTextWithMainMenu(ctx, s, chatID, locale, msgs.weightSavedConfirmation(localTime, formatWeightKg(weightKg)))
}

func parseWeightKg(raw string) (float64, error) {
	s := strings.TrimSpace(raw)
	lower := strings.ToLower(s)
	switch {
	case strings.HasSuffix(lower, "kg"):
		s = strings.TrimSpace(s[:len(s)-len("kg")])
	case strings.HasSuffix(lower, "кг"):
		s = strings.TrimSpace(s[:len(s)-len("кг")])
	}
	if s == "" {
		return 0, fmt.Errorf("empty weight")
	}

	s = strings.ReplaceAll(s, ",", ".")
	if strings.ContainsAny(s, "+-eE \t\r\n") || strings.Count(s, ".") > 1 {
		return 0, fmt.Errorf("invalid weight format %q", raw)
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(val) || math.IsInf(val, 0) || val <= 0 || val > 500 {
		return 0, fmt.Errorf("weight %q out of range (0, 500]", raw)
	}
	return val, nil
}

func formatWeightKg(weight float64) string {
	return strconv.FormatFloat(weight, 'f', -1, 64)
}
