package database

import (
	"log"
	"os"
	"time"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"radman.local/backend/internal/models"
)

var DB *gorm.DB

func ConnectDB() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=postgres_db user=radman_user password=sdfs@ dbname=radman_db port=5432 sslmode=disable TimeZone=Asia/Tehran"
	}

	var err error
	for i := 1; i <= 10; i++ {
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			log.Println("✅ Successfully connected to PostgreSQL!")
			break
		}
		log.Printf("⚠️ Attempt %d: Connection failed, retrying...", i)
		time.Sleep(4 * time.Second)
	}

	if err != nil {
		log.Fatal("❌ Failed to connect to the database permanently: ", err)
	}

	DB.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`)

	err = DB.AutoMigrate(
		&models.User{}, 
		&models.Admin{}, 
		&models.Session{}, 
		&models.OtpSession{}, 
		&models.Blocklist{}, 
		&models.SsoRequest{},
		&models.ChildRelation{},
	)
	if err != nil {
		log.Fatal("❌ Error migrating database tables: ", err)
	}
	
	log.Println("✅ Database tables synced successfully.")
}