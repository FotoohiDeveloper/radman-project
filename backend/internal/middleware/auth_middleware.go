package middleware

import (
	"fmt"
	"os"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"radman.local/backend/internal/database"
	"radman.local/backend/internal/models"
)

func Protected() fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(401).JSON(fiber.Map{"error": "دسترسی غیرمجاز: توکن ارسال نشده است"})
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("الگوریتم توکن نامعتبر است")
			}
			return []byte(os.Getenv("JWT_SECRET_KEY")), nil
		})

		if err != nil || !token.Valid {
			return c.Status(401).JSON(fiber.Map{"error": "دسترسی غیرمجاز: توکن منقضی یا نامعتبر است"})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": "دسترسی غیرمجاز: اطلاعات توکن مخدوش است"})
		}

		if claims["type"] != "access" {
			return c.Status(401).JSON(fiber.Map{"error": "برای دسترسی به API باید از access_token استفاده کنید"})
		}

		sessionID := claims["session_id"].(string)
		userID := claims["user_id"].(string)

		var session models.Session
		if err := database.DB.Where("id = ?", sessionID).First(&session).Error; err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "نشست یافت نشد، لطفا مجددا وارد شوید"})
		}
		if session.IsRevoked {
			return c.Status(401).JSON(fiber.Map{"error": "این نشست باطل شده است. احتمالا از دستگاه دیگری وارد شده‌اید."})
		}

		c.Locals("user_id", userID)
		c.Locals("session_id", sessionID)

		return c.Next()
	}
}