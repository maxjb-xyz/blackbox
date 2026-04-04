package models

import "time"

type AppSetting struct {
	Key       string    `gorm:"primaryKey"`
	Value     string
	UpdatedAt time.Time
}
