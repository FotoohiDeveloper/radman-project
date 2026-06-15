package routes

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"radman.local/backend/internal/handlers"
)

func SetupAuthRoutes(api fiber.Router) {
	authHandler := handlers.NewAuthHandler()

	auth := api.Group("/auth")
	authLimiter := limiter.New(limiter.Config{
		Max:        5,
		Expiration: 15 * time.Minute,
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
				"error": "تعداد درخواست‌های شما بیش از حد مجاز است. لطفاً چند دقیقه دیگر تلاش کنید.",
			})
		},
	})

	auth.Post("/send", authLimiter, authHandler.LoginSend)
	auth.Post("/verify", authLimiter, authHandler.VerifyOTP)
	auth.Post("/kyc", authHandler.RegisterKYC)
}