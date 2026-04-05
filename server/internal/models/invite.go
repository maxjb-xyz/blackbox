package models

import "time"

type InviteCode struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Code      string    `json:"code" gorm:"uniqueIndex"`
	CreatedBy string    `json:"created_by"`
	UsedBy    string    `json:"used_by"`
	ExpiresAt time.Time `json:"expires_at" gorm:"index"`
	CreatedAt time.Time `json:"created_at"`
}
