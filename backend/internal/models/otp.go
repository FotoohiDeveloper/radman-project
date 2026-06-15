package models

import (
	"time"
	"github.com/google/uuid"
)

type OtpSession struct {
	UID        uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	Phone      string    `gorm:"type:varchar(20);not null"`
	CodeHash   string    `gorm:"type:varchar(255);not null"`
	Attempts   int       `gorm:"default:0"`
	KycFails   int       `gorm:"default:0"` 
	IsVerified bool      `gorm:"default:false"` 
	SsoReqID   string    `gorm:"type:varchar(50);not null"` 
	ExpiresAt  time.Time `gorm:"not null"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
}