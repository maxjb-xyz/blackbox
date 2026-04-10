package correlation

import (
	"time"

	"blackbox/shared/types"
	"gorm.io/gorm"
)

// FindCause returns the top-scoring cause candidate for the given service,
// or nil if none found. Kept for backwards compatibility with existing callers.
func FindCause(db *gorm.DB, service string, at time.Time) (*types.Entry, error) {
	candidates, err := ScoreCauses(db, []string{service}, at, "")
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	return candidates[0].Entry, nil
}
