package handlers

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
	"os"
	
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/golang-jwt/jwt/v5"
	"radman.local/backend/internal/database"
	"radman.local/backend/internal/models"
	"radman.local/backend/internal/utils"
)

type SSOHandler struct{}

func NewSSOHandler() *SSOHandler {
	return &SSOHandler{}
}

func (h *SSOHandler) InitFlow(c fiber.Ctx) error {

	var req struct {
		CodeChallenge string `json:"code_challenge"`
	}
	if err := c.Bind().Body(&req); err != nil || req.CodeChallenge == "" {
		return c.Status(400).JSON(fiber.Map{"error": "code_challenge is required"})
	}

	ssoReq := models.SsoRequest{
		CodeChallenge:  req.CodeChallenge,
		IPAddress:      c.IP(),
		Status:         "started",
		ExpiresAt:      time.Now().Add(2 * time.Minute),
		FinalExpiresAt: time.Now().Add(6 * time.Minute),
	}
	database.DB.Create(&ssoReq)

	portalURL := "https://auth.hapagate.ir/portal?req_id=" + ssoReq.ID.String()
	return c.JSON(fiber.Map{
		"url":    portalURL,
		"req_id": ssoReq.ID,
	})
}

func (h *SSOHandler) Token(c fiber.Ctx) error {
	var req models.TokenRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "فرمت درخواست نامعتبر است"})
	}

	if req.AuthCode == "" || req.CodeVerifier == "" {
		return c.Status(400).JSON(fiber.Map{"error": "فیلد auth_code یا code_verifier خالی است! (شاید پستمن روی JSON تنظیم نشده)"})
	}

	var ssoReq models.SsoRequest
	if err := database.DB.Where("auth_code = ?", req.AuthCode).First(&ssoReq).Error; err != nil {
		return c.Status(403).JSON(fiber.Map{"error": "کد نامعتبر است یا قبلا استفاده شده"})
	}

	hash := sha256.New()
	hash.Write([]byte(req.CodeVerifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(hash.Sum(nil))

	if expectedChallenge != ssoReq.CodeChallenge {
		return c.Status(403).JSON(fiber.Map{"error": "خطر امنیتی: عدم تطابق کلاینت (احتمال سرقت کد)"})
	}

	database.DB.Model(&ssoReq).Updates(map[string]interface{}{
		"status":    "exchanged",
		"auth_code": nil,
	})

	database.DB.Model(&models.Session{}).
		Where("user_id = ? AND is_revoked = ?", ssoReq.UserID, false).
		Update("is_revoked", true)

	sessionID := uuid.New()

	accessToken, refreshToken, err := utils.GenerateTokens(*ssoReq.UserID, sessionID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "خطا در تولید کلیدهای دسترسی"})
	}

	hashRT := sha256.Sum256([]byte(refreshToken))
	refreshHashHex := hex.EncodeToString(hashRT[:])

	deviceInfo := req.DeviceFingerprint.Brand + " " + req.DeviceFingerprint.Model
	newSession := models.Session{
		ID:               sessionID,
		UserID:           *ssoReq.UserID,
		RefreshTokenHash: refreshHashHex,
		IPAddress:        c.IP(),
		DeviceInfo:       deviceInfo,
		IsRevoked:        false,
		ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
	}

	if err := database.DB.Create(&newSession).Error; err != nil {
		fmt.Println("🚨 Database Error creating session:", err)
		return c.Status(500).JSON(fiber.Map{"error": "خطا در ذخیره نشست در سرور"})
	}

	return c.JSON(fiber.Map{
		"success":       true,
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user_id":       ssoReq.UserID,
		"session_id":    newSession.ID,
	})
}

func (h *SSOHandler) GetMe(c fiber.Ctx) error {
	userID := c.Locals("user_id").(string)

	var user models.User
	if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "کاربر یافت نشد"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"first_name":   user.FirstName,
			"last_name":    user.LastName,
			"father_name":  user.FatherName,
			"phone_number": user.PhoneNumber,
			"birth_date":   user.BirthDate,
			"national_id":  user.NationalID,
		},
	})
}

func (h *SSOHandler) Logout(c fiber.Ctx) error {
	sessionID := c.Locals("session_id").(string)

	if err := database.DB.Model(&models.Session{}).Where("id = ?", sessionID).Update("is_revoked", true).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "خطا در خروج از حساب کاربری"})
	}

	return c.JSON(fiber.Map{
		"success": true, 
		"message": "با موفقیت از سیستم خارج شدید",
	})
}

func (h *SSOHandler) RefreshToken(c fiber.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind().Body(&req); err != nil || req.RefreshToken == "" {
		return c.Status(400).JSON(fiber.Map{"error": "رفرش توکن ارسال نشده است"})
	}

	token, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET_KEY")), nil
	})

	if err != nil || !token.Valid {
		return c.Status(401).JSON(fiber.Map{"error": "رفرش توکن نامعتبر یا منقضی شده است. لطفا دوباره لاگین کنید."})
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["type"] != "refresh" {
		return c.Status(401).JSON(fiber.Map{"error": "توکن ارسال شده از نوع رفرش نیست"})
	}

	sessionIDStr := claims["session_id"].(string)
	userIDStr := claims["user_id"].(string)

	var session models.Session
	if err := database.DB.Where("id = ?", sessionIDStr).First(&session).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "نشست یافت نشد"})
	}

	if session.IsRevoked {
		return c.Status(401).JSON(fiber.Map{"error": "این نشست باطل شده است"})
	}

	hashRT := sha256.Sum256([]byte(req.RefreshToken))
	refreshHashHex := hex.EncodeToString(hashRT[:])
	
	if session.RefreshTokenHash != refreshHashHex {
		session.IsRevoked = true
		database.DB.Save(&session)
		return c.Status(403).JSON(fiber.Map{"error": "خطر امنیتی: عدم تطابق توکن. نشست شما باطل شد."})
	}

	userID, _ := uuid.Parse(userIDStr)
	sessionID, _ := uuid.Parse(sessionIDStr)

	newAccess, newRefresh, err := utils.GenerateTokens(userID, sessionID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "خطا در تولید کلیدهای دسترسی جدید"})
	}

	newHashRT := sha256.Sum256([]byte(newRefresh))
	session.RefreshTokenHash = hex.EncodeToString(newHashRT[:])
	database.DB.Save(&session)

	return c.JSON(fiber.Map{
		"success":       true,
		"access_token":  newAccess,
		"refresh_token": newRefresh,
	})
}
