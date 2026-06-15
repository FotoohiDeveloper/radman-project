package routes

import (
	"github.com/gofiber/fiber/v3"
	"radman.local/backend/internal/handlers"
	"radman.local/backend/internal/middleware"
)

func SetupSSORoutes(api fiber.Router) {
	ssoHandler := handlers.NewSSOHandler()

	sso := api.Group("/auth")
	
	// sso.Post("/init", ssoHandler.InitFlow)
	// sso.Post("/token", ssoHandler.Token)
	// sso.Get("/portal", func(c fiber.Ctx) error { return c.SendString("SSO Portal") })
	
	sso.Post("/refresh", ssoHandler.RefreshToken)

	protectedSSO := api.Group("/user", middleware.Protected())
	
	protectedSSO.Get("/me", ssoHandler.GetMe)
	
	protectedSSO.Post("/logout", ssoHandler.Logout)
}