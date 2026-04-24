package handlers

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"blackbox/server/internal/auth"
	"blackbox/server/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

const auditLogRetention = 10_000

var auditPruneInFlight atomic.Bool

func extractClientIP(r *http.Request) string {
	remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	if isTrustedProxy(remoteIP) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if idx := strings.Index(xff, ","); idx >= 0 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}
	}
	return remoteIP
}

func isTrustedProxy(ip string) bool {
	// Allow an explicit override for reverse proxies on different hosts
	if trusted := os.Getenv("TRUSTED_PROXY_IP"); trusted != "" && ip == trusted {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback()
}

func pruneAuditLogs(db *gorm.DB) {
	if !auditPruneInFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer auditPruneInFlight.Store(false)
		if err := db.Exec(
			"DELETE FROM audit_logs WHERE id NOT IN (SELECT id FROM audit_logs ORDER BY created_at DESC LIMIT ?)",
			auditLogRetention,
		).Error; err != nil {
			log.Printf("WriteAuditLog prune failed: %v", err)
		}
	}()
}

func WriteAuditLog(
	db *gorm.DB,
	r *http.Request,
	claims *auth.Claims,
	action, targetType, targetID string,
	meta map[string]interface{},
) {
	if claims == nil {
		return
	}

	actorEmail := claims.Email
	if actorEmail == "" {
		var user models.User
		if err := db.Select("email").First(&user, "id = ?", claims.UserID).Error; err == nil {
			actorEmail = user.Email
		}
	}

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		metaBytes = []byte("{}")
	}

	entry := models.AuditLog{
		ID:          ulid.Make().String(),
		ActorUserID: claims.UserID,
		ActorEmail:  actorEmail,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		Metadata:    string(metaBytes),
		IPAddress:   extractClientIP(r),
		CreatedAt:   time.Now().UTC(),
	}

	if err := db.Create(&entry).Error; err != nil {
		log.Printf("WriteAuditLog insert failed: %v", err)
		return
	}

	pruneAuditLogs(db)
}

func ListAuditLogs(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminRequest(w, r) {
			return
		}

		page, perPage, ok := parsePaginationParams(w, r)
		if !ok {
			return
		}

		q := db.Model(&models.AuditLog{})
		if action := r.URL.Query().Get("action"); action != "" {
			q = q.Where("action = ?", action)
		}
		if actorID := r.URL.Query().Get("actor_user_id"); actorID != "" {
			q = q.Where("actor_user_id = ?", actorID)
		}

		var total int64
		if err := q.Count(&total).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to count audit logs")
			return
		}

		var rows []models.AuditLog
		offset := (page - 1) * perPage
		if err := q.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&rows).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list audit logs")
			return
		}

		items := make([]auditLogItem, len(rows))
		for i, row := range rows {
			var meta map[string]interface{}
			if err := json.Unmarshal([]byte(row.Metadata), &meta); err != nil {
				meta = map[string]interface{}{}
			}
			items[i] = auditLogItem{
				ID:          row.ID,
				ActorUserID: row.ActorUserID,
				ActorEmail:  row.ActorEmail,
				Action:      row.Action,
				TargetType:  row.TargetType,
				TargetID:    row.TargetID,
				Metadata:    meta,
				IPAddress:   row.IPAddress,
				CreatedAt:   row.CreatedAt,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"total":    total,
			"page":     page,
			"per_page": perPage,
			"items":    items,
		}); err != nil {
			log.Printf("ListAuditLogs encode: %v", err)
		}
	}
}

type auditLogItem struct {
	ID          string                 `json:"id"`
	ActorUserID string                 `json:"actor_user_id"`
	ActorEmail  string                 `json:"actor_email"`
	Action      string                 `json:"action"`
	TargetType  string                 `json:"target_type"`
	TargetID    string                 `json:"target_id"`
	Metadata    map[string]interface{} `json:"metadata"`
	IPAddress   string                 `json:"ip_address"`
	CreatedAt   time.Time              `json:"created_at"`
}

func parsePaginationParams(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	page := 1
	perPage := 50

	if v := r.URL.Query().Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid page")
			return 0, 0, false
		}
		page = n
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "per_page must be between 1 and 200")
			return 0, 0, false
		}
		perPage = n
	}
	return page, perPage, true
}
