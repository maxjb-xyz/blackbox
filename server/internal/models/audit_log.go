package models

import "time"

type AuditLog struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	ActorUserID string    `gorm:"not null;index" json:"actor_user_id"`
	ActorEmail  string    `gorm:"not null;default:''" json:"actor_email"`
	Action      string    `gorm:"not null;index" json:"action"`
	TargetType  string    `gorm:"not null;default:''" json:"target_type"`
	TargetID    string    `gorm:"not null;default:''" json:"target_id"`
	Metadata    string    `gorm:"not null;default:'{}'" json:"metadata"`
	IPAddress   string    `gorm:"not null;default:''" json:"ip_address"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}
