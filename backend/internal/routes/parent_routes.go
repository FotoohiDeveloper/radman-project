package routes

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"radman.local/backend/internal/handlers"
	"radman.local/backend/internal/middleware"
)

func SetupParentRoutes(api fiber.Router) {
	parentHandler := handlers.NewParentHandler()

	parent := api.Group("/user/parent", middleware.Protected())

	strictParentLimiter := limiter.New(limiter.Config{
		Max:        10,
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
				"error": "تعداد درخواست‌های شما برای این بخش بیش از حد مجاز است. لطفا ۱۵ دقیقه دیگر تلاش کنید.",
			})
		},
	})

	parent.Post("/child/send", strictParentLimiter, parentHandler.SendChildOTP)
	parent.Post("/child/verify", strictParentLimiter, parentHandler.VerifyChildOTP)
	parent.Post("/child/kyc", parentHandler.RegisterChildKYC)
	parent.Post("/child/upload-documents", parentHandler.UploadChildDocuments)
	parent.Get("/children", parentHandler.GetChildrenList)
}