package events

import (
	"log"
	"time"

	"blackbox/shared/types"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

func LogSystem(db *gorm.DB, service, event, content string) {
	if db == nil {
		log.Printf("events.LogSystem: db is nil, dropping event service=%s event=%s", service, event)
		return
	}

	entry := types.Entry{
		ID:        ulid.Make().String(),
		Timestamp: time.Now().UTC(),
		NodeName:  "server",
		Source:    "system",
		Service:   service,
		Event:     event,
		Content:   content,
	}
	if err := db.Create(&entry).Error; err != nil {
		log.Printf("events.LogSystem: failed to write event service=%s event=%s: %v", service, event, err)
	}
}
