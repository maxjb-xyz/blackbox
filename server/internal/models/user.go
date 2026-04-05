package models

import "time"

type User struct {
	ID           string `gorm:"primaryKey"`
	Username     string `gorm:"uniqueIndex"`
	Email        string `gorm:"uniqueIndex:idx_users_email,where:email <> '';not null;default:''"`
	PasswordHash string
	IsAdmin      bool
	OIDCIssuer   string `gorm:"not null;default:'';uniqueIndex:idx_users_oidc_identity,where:oidc_issuer <> '' AND oidc_subject <> ''"`
	OIDCSubject  string `gorm:"not null;default:'';uniqueIndex:idx_users_oidc_identity,where:oidc_issuer <> '' AND oidc_subject <> ''"`
	TokenVersion int    `gorm:"not null;default:0"`
	CreatedAt    time.Time
}
