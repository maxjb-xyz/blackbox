package models

import "time"

type EntryNote struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	EntryID   string    `gorm:"index" json:"entry_id"`
	UserID    string    `gorm:"index" json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
