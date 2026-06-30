package middleware

import (
	"fmt"
	"os"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"radman.local/backend/internal/database"
	"radman.local/backend/internal/i18n"
	"radman.local/backend/internal/models"
)

func Protected() fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(401).JSON(fiber.Map{"error": i18n.T("middleware.token_missing")})
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("invalid token signing algorithm: %v", token.Header["alg"])
			}
			return []byte(os.Getenv("JWT_SECRET_KEY")), nil
		})

		if err != nil || !token.Valid {
			return c.Status(401).JSON(fiber.Map{"error": i18n.T("middleware.token_invalid")})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": i18n.T("middleware.token_claims")})
		}

		if claims["type"] != "access" {
			return c.Status(401).JSON(fiber.Map{"error": i18n.T("middleware.access_token_required")})
		}

		sessionID := claims["session_id"].(string)
		userID := claims["user_id"].(string)

		var session models.Session
		if err := database.DB.Where("id = ?", sessionID).First(&session).Error; err != nil {
			return c.Status(401).JSON(fiber.Map{"error": i18n.T("middleware.session_not_found")})
		}
		if session.IsRevoked {
			return c.Status(401).JSON(fiber.Map{"error": i18n.T("middleware.session_revoked")})
		}

		c.Locals("user_id", userID)
		c.Locals("session_id", sessionID)

		return c.Next()
	}
}
