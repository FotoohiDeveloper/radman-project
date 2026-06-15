package main

import (
	"log"
	"time"

	"github.com/joho/godotenv"
	"radman.local/backend/internal/database"
	"radman.local/backend/internal/routes"
	"radman.local/backend/internal/services"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("ignore this message if you're on docker")
	}

	database.ConnectDB()

	go services.StartSMSWorker()

	app := fiber.New(fiber.Config{
		AppName: "Radman API v1.0",
		TrustProxy:  true,
		ProxyHeader: "ar-real-ip", 
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://auth.hapagate.ir"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "ar-real-ip", "X-Forwarded-For"},
		AllowMethods:     []string{"GET", "POST", "HEAD", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowCredentials: true,
	}))

	app.Use(logger.New())
	app.Use(recover.New())

	app.Use(limiter.New(limiter.Config{
		Max:        100,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c fiber.Ctx) string {
			if ip := c.Get("ar-real-ip"); ip != "" {
				return ip
			}
			if ip := c.Get("X-Forwarded-For"); ip != "" {
				return ip
			}
			return c.IP()
		},
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{
				"error": "تعداد درخواست‌های شما بیش از حد مجاز است. لطفاً یک دقیقه دیگر تلاش کنید.",
			})
		},
	}))

	// ثبت روت‌ها
	routes.SetupAuthRoutes(app)
	routes.SetupSSORoutes(app)
	routes.SetupParentRoutes(app)

	log.Println("Starting Radman API on port 3000...")
	if err := app.Listen(":3000"); err != nil {
		log.Fatal("Server failed to start: ", err)
	}
}