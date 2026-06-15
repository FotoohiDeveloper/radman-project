package models

import (
	"time"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID                uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	PhoneNumber       *string        `gorm:"type:varchar(20);uniqueIndex;null"`
	NationalID        *string        `gorm:"type:varchar(10);uniqueIndex;null"`
	FirstName         *string        `gorm:"type:varchar(50);null"`
	LastName          *string        `gorm:"type:varchar(50);null"`
	FatherName        *string        `gorm:"type:varchar(50);null"`
	BirthDate         *time.Time     `gorm:"type:date;null"`
	Gender            *int           `gorm:"type:smallint;null"`
	
	AdvanceKyc        int8           `gorm:"type:smallint;default:0"` 
	AdvanceKycStatus  int8           `gorm:"type:smallint;default:0"`

	IdentityNo        *string        `gorm:"type:varchar(20);null"`
	IdentitySeri      *string        `gorm:"type:varchar(10);null"`
	IdentitySerial    *string        `gorm:"type:varchar(20);null"`
	OfficeName        *string        `gorm:"type:varchar(100);null"`
	OfficeCode        *string        `gorm:"type:varchar(20);null"`

	ParentIdentityCardURL *string    `gorm:"type:varchar(255);null"`
	ChildIdentityCardURL  *string    `gorm:"type:varchar(255);null"`

	PreferredLanguage string         `gorm:"type:varchar(10);default:'fa'"`
	Role              string         `gorm:"type:varchar(20);default:'citizen'"`
	Status            string         `gorm:"type:varchar(20);default:'pending_kyc'"`
	AcceptedTermsAt   time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP"`
	CreatedAt         time.Time      `gorm:"autoCreateTime"`
	UpdatedAt         time.Time      `gorm:"autoUpdateTime"`
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

type ChildRelation struct {
	ParentID  uuid.UUID `gorm:"type:uuid;primaryKey"`
	ChildID   uuid.UUID `gorm:"type:uuid;primaryKey;uniqueIndex"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}