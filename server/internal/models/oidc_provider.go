package models

import "time"

type OIDCProviderConfig struct {
	ID           string    `gorm:"primaryKey"`
	Name         string
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Enabled      bool      `gorm:"default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
