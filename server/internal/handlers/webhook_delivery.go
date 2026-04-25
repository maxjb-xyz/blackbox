package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const webhookDeliveryRetention = 1_000

var webhookPruneInFlight atomic.Bool

func pruneWebhookDeliveries(db *gorm.DB) {
	if !webhookPruneInFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer webhookPruneInFlight.Store(false)
		if err := db.Exec(
			"DELETE FROM webhook_deliveries WHERE id NOT IN (SELECT id FROM webhook_deliveries ORDER BY received_at DESC LIMIT ?)",
			webhookDeliveryRetention,
		).Error; err != nil {
			log.Printf("RecordWebhookDelivery prune failed: %v", err)
		}
	}()
}

func RecordWebhookDelivery(
	db *gorm.DB,
	source, payloadSnippet, matchedIncidentID, status, errorMessage string,
) {
	row := models.WebhookDelivery{
		ID:             ulid.Make().String(),
		Source:         source,
		ReceivedAt:     time.Now().UTC(),
		PayloadSnippet: payloadSnippet,
		Status:         status,
		ErrorMessage:   errorMessage,
	}
	if matchedIncidentID != "" {
		row.MatchedIncidentID = &matchedIncidentID
	}
	if err := db.Create(&row).Error; err != nil {
		log.Printf("RecordWebhookDelivery insert failed: %v", err)
		return
	}
	pruneWebhookDeliveries(db)
}

func ListWebhookDeliveries(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}

		page, perPage, ok := parsePaginationParams(w, r)
		if !ok {
			return
		}

		q := db.Model(&models.WebhookDelivery{})
		if source := r.URL.Query().Get("source"); source != "" {
			q = q.Where("source = ?", source)
		}
		if status := r.URL.Query().Get("status"); status != "" {
			q = q.Where("status = ?", status)
		}

		var total int64
		if err := q.Count(&total).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to count webhook deliveries")
			return
		}

		var rows []models.WebhookDelivery
		offset := (page - 1) * perPage
		if err := q.Order("received_at DESC").Offset(offset).Limit(perPage).Find(&rows).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list webhook deliveries")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"total":    total,
			"page":     page,
			"per_page": perPage,
			"items":    rows,
		}); err != nil {
			log.Printf("ListWebhookDeliveries encode: %v", err)
		}
	}
}

func payloadSnippet(body []byte) string {
	const max = 2000
	runes := []rune(string(body))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max])
}
