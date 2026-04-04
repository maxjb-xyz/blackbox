package models

import "time"

type OIDCState struct {
	ID        string `gorm:"primaryKey"`
	State     string `gorm:"uniqueIndex"`
	Nonce     string
	ProviderID string
	InviteCode string
	ExpiresAt time.Time `gorm:"index"`
	CreatedAt time.Time
}
