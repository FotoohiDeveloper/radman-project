package config

import (
	"log"

	"github.com/spf13/viper"
)

type AppConfig struct {
	Redis struct {
		Addr string
		User string
		Pass string
	}
	DB struct {
		DSN string
	}
	Radar struct {
		H3Resolution      int     `mapstructure:"h3_resolution"`
		AssociationRadius float64 `mapstructure:"association_radius_meters"`
		CoastTimeout      int     `mapstructure:"coast_timeout_seconds"`
	}
	TargetsAGL map[string]float64 `mapstructure:"targets_agl"`
}

var App AppConfig

func Load() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/app")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("❌ Error reading config file: %v", err)
	}

	// متغیرهای حساس از env خوانده می‌شن و بر config.yaml اولویت دارن
	viper.BindEnv("redis.addr", "REDIS_ADDR")
	viper.BindEnv("redis.user", "REDIS_USER")
	viper.BindEnv("redis.pass", "REDIS_PASS")
	viper.BindEnv("db.dsn", "PROCESSOR_DATABASE_URL")

	if err := viper.Unmarshal(&App); err != nil {
		log.Fatalf("❌ Unable to decode into struct: %v", err)
	}

	log.Println("⚙️  Configuration loaded successfully from config.yaml!")
}
