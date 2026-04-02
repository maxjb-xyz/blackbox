package models

import "time"

type InviteCode struct {
	ID        string `gorm:"primaryKey"`
	Code      string `gorm:"uniqueIndex"`
	CreatedBy string
	UsedBy    string
	ExpiresAt time.Time `gorm:"index"`
	CreatedAt time.Time
}
