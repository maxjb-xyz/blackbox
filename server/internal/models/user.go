package models

import "time"

type User struct {
	ID           string    `gorm:"primaryKey"`
	Username     string    `gorm:"uniqueIndex"`
	PasswordHash string
	IsAdmin      bool
	OIDCSubject  string    `gorm:"index"`
	CreatedAt    time.Time
}
