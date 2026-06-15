package models

import (
	"time"
	"github.com/google/uuid"
)

type Session struct {
	ID               uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	UserID           uuid.UUID `gorm:"type:uuid;not null"`
	RefreshTokenHash string    `gorm:"type:varchar(255);uniqueIndex;not null"`
	IPAddress        string    `gorm:"type:varchar(45);not null"`
	DeviceInfo       string    `gorm:"type:text"`
	IsRevoked        bool      `gorm:"default:false"`
	ExpiresAt        time.Time `gorm:"not null"`
	CreatedAt        time.Time `gorm:"autoCreateTime"`
}