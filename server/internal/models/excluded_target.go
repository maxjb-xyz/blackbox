package models

import "time"

type ExcludedTarget struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Type      string    `json:"type" gorm:"index"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
