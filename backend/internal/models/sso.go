package models

import (
	"time"
	"github.com/google/uuid"
)

type SsoRequest struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	CodeChallenge  string     `gorm:"type:varchar(255);not null"`
	IPAddress      string     `gorm:"type:varchar(45);not null"`
	Status         string     `gorm:"type:varchar(20);default:'started'"`
	AuthCode       *string    `gorm:"type:varchar(100);uniqueIndex"`
	UserID         *uuid.UUID `gorm:"type:uuid"`
	ExpiresAt      time.Time  `gorm:"not null"`
	FinalExpiresAt time.Time  `gorm:"not null"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
}