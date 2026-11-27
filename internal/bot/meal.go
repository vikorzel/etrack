package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"

	"meal-bot/internal/storage"
)

func (h *Handler) handleMealCallback(cc callbackContext, data string) {
	switch {
	case strings.HasPrefix(data, "meal:before:"):
		before, err := parseSatiety(strings.TrimPrefix(data, "meal:before:"))
		if err != nil {
			cc.answer(cc.msgs.cbInvalidValue, true)
			return
		}
		cc.answer("", false)
		cc.updateMessage(cc.msgs.stepAfterText(before), satietyAfterKeyboard(cc.locale, before))

	case data == "meal:back":
		cc.answer("", false)
		cc.updateMessage(cc.msgs.stepBeforeText, satietyBeforeKeyboard(cc.locale))

	case strings.HasPrefix(data, "meal:after:"):
		h.saveMealCallback(cc, strings.TrimPrefix(data, "meal:after:"))

	case data == "meal:cancel":
		cc.answer(cc.msgs.cbCancelled, false)
		cc.updateMessage(cc.msgs.mealCancelledText, nil)

	default:
		cc.answer(cc.msgs.cbUnknownAction, false)
	}
}

func (h *Handler) saveMealCallback(cc callbackContext, payload string) {
	parts := strings.Split(payload, ":")
	if len(parts) != 2 {
		cc.answer(cc.msgs.cbInvalidData, true)
		return
	}
	before, errBefore := parseSatiety(parts[0])
	after, errAfter := parseSatiety(parts[1])
	if errBefore != nil || errAfter != nil {
		cc.answer(cc.msgs.cbInvalidValue, true)
		return
	}

	if cc.hasMsg && !h.claimSaveKey(cc.chatID, cc.messageID) {
		cc.answer(cc.msgs.cbAlreadySaved, false)
		return
	}

	now := h.now().UTC()
	_, err := h.store.SaveMeal(cc.ctx, storage.Meal{
		UserID:        cc.userID,
		RecordedAt:    now,
		SatietyBefore: before,
		SatietyAfter:  after,
	})
	if err != nil {
		if cc.hasMsg {
			h.releaseSaveKey(cc.chatID, cc.messageID)
		}
		log.Printf("save meal for user %d: %v", cc.userID, err)
		cc.answer(cc.msgs.cbSaveError, true)
		return
	}

	var markup models.ReplyMarkup
	if h.weighInInterval > 0 {
		count, countErr := h.store.CountMeals(cc.ctx, cc.userID)
		if countErr != nil {
			log.Printf("count meals for user %d: %v", cc.userID, countErr)
		} else if count > 0 && count%h.weighInInterval == 0 {
			markup = weighInPromptKeyboard(cc.locale)
		}
	}

	cc.answer(cc.msgs.cbSaved, false)
	localTime := now.In(h.loc).Format("02.01.2006 15:04")
	delta := formatSignedDelta(after - before)
	cc.updateMessage(cc.msgs.mealSavedConfirmation(localTime, before, after, delta), markup)
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
