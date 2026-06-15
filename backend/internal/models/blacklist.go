package models

import (
	"time"
	"github.com/google/uuid"
)

type Blocklist struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	Phone     string    `gorm:"type:varchar(20);uniqueIndex;not null"`
	Reason    string    `gorm:"type:varchar(255);not null"`
	IsActive  bool      `gorm:"default:true"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}