package models

import "time"

type SetupState struct {
	Key       string `gorm:"primaryKey"`
	CreatedAt time.Time
}
