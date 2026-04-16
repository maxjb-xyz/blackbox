package handlers

import (
	"errors"
	"fmt"
	"log"

	"blackbox/shared/types"
	"gorm.io/gorm"
)

var errEntryIDConflict = errors.New("entry id already exists with different payload")

// createEntryIdempotent inserts an entry once and treats an exact duplicate as
// an already-saved retry. A reused ID with different content remains an error.
func createEntryIdempotent(database *gorm.DB, entry types.Entry, logContext string) (created bool, err error) {
	if err := database.Create(&entry).Error; err == nil {
		return true, nil
	} else if !errors.Is(err, gorm.ErrDuplicatedKey) {
		log.Printf("%s: create entry id=%s failed: %v", logContext, entry.ID, err)
		return false, err
	}

	var existing types.Entry
	if lookupErr := database.First(&existing, "id = ?", entry.ID).Error; lookupErr != nil {
		log.Printf("%s: duplicate entry id=%s but lookup failed: %v", logContext, entry.ID, lookupErr)
		return false, fmt.Errorf("lookup existing duplicate entry: %w", lookupErr)
	}
	if sameStoredEntry(existing, entry) {
		return false, nil
	}

	log.Printf("%s: rejected conflicting duplicate entry id=%s", logContext, entry.ID)
	return false, errEntryIDConflict
}

func sameStoredEntry(a, b types.Entry) bool {
	return a.ID == b.ID &&
		a.Timestamp.Equal(b.Timestamp) &&
		a.NodeName == b.NodeName &&
		a.Source == b.Source &&
		a.Service == b.Service &&
		a.ComposeService == b.ComposeService &&
		a.Event == b.Event &&
		a.Content == b.Content &&
		a.Metadata == b.Metadata &&
		a.CorrelatedID == b.CorrelatedID
}
