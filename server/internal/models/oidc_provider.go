package models

import "time"

func BoolPtr(v bool) *bool {
	return &v
}

type OIDCProviderConfig struct {
	ID                   string `gorm:"primaryKey"`
	Name                 string
	Issuer               string
	ClientID             string
	ClientSecret         string
	RedirectURL          string
	RequireVerifiedEmail *bool `gorm:"default:true;not null"`
	Enabled              *bool `gorm:"default:true;not null"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
