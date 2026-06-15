package models

import (
	"time"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Admin struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	Username     string         `gorm:"type:varchar(50);uniqueIndex;not null"`
	PasswordHash string         `gorm:"type:varchar(255);not null"`
	FirstName    string         `gorm:"type:varchar(50);not null"`
	LastName     string         `gorm:"type:varchar(50);not null"`
	Role         string         `gorm:"type:varchar(20);default:'kyc_operator'"`
	IsActive     bool           `gorm:"default:true"`
	LastLoginAt  *time.Time
	CreatedAt    time.Time      `gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}