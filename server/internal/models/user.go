package models

import "time"

type User struct {
	ID           string    `gorm:"primaryKey"`
	Username     string    `gorm:"uniqueIndex"`
	PasswordHash string
	IsAdmin      bool
	OIDCSubject  string    `gorm:"index"`
	TokenVersion int       `gorm:"not null;default:0"`
	CreatedAt    time.Time
}
