package database

import (
	"log"
	"time"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type TrackRecord struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	TargetType string    `gorm:"type:varchar(50);not null"`
	Status     string    `gorm:"type:varchar(20);default:'active'"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

type TrackPoint struct {
	ID         uint      `gorm:"primaryKey"`
	TrackID    uuid.UUID `gorm:"type:uuid;index;not null"`
	Lat        float64   `gorm:"not null"`
	Lon        float64   `gorm:"not null"`
	Alt        float64   `gorm:"not null"`
	Confidence string    `gorm:"type:varchar(20)"`
	ReportTime time.Time `gorm:"index;not null"`
}

var DB *gorm.DB

func ConnectTrackingDB() {
	dsn := "host=postgres_db user=radman_user password=sdfs@ dbname=radman_tracking_db port=5432 sslmode=disable TimeZone=Asia/Tehran"
	
	var err error
	for i := 1; i <= 5; i++ {
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			log.Println("✅ Tracking Database connected!")
			break
		}
		log.Printf("⚠️ Retrying DB connection... (%d)", i)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatal("❌ Failed to connect to Tracking DB:", err)
	}

	DB.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`)
	DB.AutoMigrate(&TrackRecord{}, &TrackPoint{})
}