package bot

import (
	"fmt"
	"time"
)

const savedKeyTTL = time.Hour

func (h *Handler) setAwaitingRange(userID int64, waiting bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if waiting {
		delete(h.awaitingWeight, userID)
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

func (h *Handler) setAwaitingWeight(userID int64, waiting bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if waiting {
		delete(h.awaitingRange, userID)
		h.awaitingWeight[userID] = true
	} else {
		delete(h.awaitingWeight, userID)
	}
}

func (h *Handler) isAwaitingWeight(userID int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.awaitingWeight[userID]
}

func (h *Handler) clearPendingInput(userID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.awaitingRange, userID)
	delete(h.awaitingWeight, userID)
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
