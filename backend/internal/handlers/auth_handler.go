package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"radman.local/backend/internal/database"
	"radman.local/backend/internal/models"
	"radman.local/backend/internal/services"
	"radman.local/backend/internal/utils"
)

type AuthHandler struct{}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

func generateSecureOTP() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

func createDirectSession(tx *gorm.DB, c fiber.Ctx, userID uuid.UUID) (string, string, error) {
    sessionID := uuid.New()
    
    accessToken, refreshToken, err := utils.GenerateTokens(userID, sessionID)
    if err != nil {
        return "", "", err
    }

    hashRT := sha256.Sum256([]byte(refreshToken))
    refreshHashHex := hex.EncodeToString(hashRT[:])

    newSession := models.Session{
        ID:               sessionID,
        UserID:           userID,
        RefreshTokenHash: refreshHashHex,
        IPAddress:        c.IP(),
        DeviceInfo:       c.Get("User-Agent"),
        IsRevoked:        false,
        ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
    }

    if err := tx.Create(&newSession).Error; err != nil {
        return "", "", err
    }

    return accessToken, refreshToken, nil
}

func (h *AuthHandler) LoginSend(c fiber.Ctx) error {
	var req struct {
		Phone string `json:"phone_number"`
	}
	// دیگه نیازی به req_id نداریم
	if err := c.Bind().Body(&req); err != nil || req.Phone == "" {
		return c.Status(400).JSON(fiber.Map{"error": "شماره موبایل الزامی است"})
	}

	// 🚨 تله امنیتی: محدودیت ارسال پیامک برای هر شماره
	var phoneOTPCount int64
	database.DB.Model(&models.OtpSession{}).
		Where("phone = ? AND created_at > ?", req.Phone, time.Now().Add(-15*time.Minute)).
		Count(&phoneOTPCount)
	if phoneOTPCount >= 3 {
		return c.Status(429).JSON(fiber.Map{"error": "به دلیل ۳ بار تلاش ناموفق، شماره شما ۱۵ دقیقه محدود شد."})
	}

	// تله زمانی ۲ دقیقه‌ای
	var lastOtp models.OtpSession
	if err := database.DB.Where("phone = ?", req.Phone).Order("created_at desc").First(&lastOtp).Error; err == nil {
		if time.Now().Before(lastOtp.ExpiresAt) {
			return c.Status(429).JSON(fiber.Map{"error": "کد قبلی هنوز منقضی نشده است. لطفا تا پایان ۲ دقیقه صبر کنید."})
		}
	}

	rawCode := generateSecureOTP()
	hashedCode, err := bcrypt.GenerateFromPassword([]byte(rawCode), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "خطا در سیستم امنیتی"})
	}

	otp := models.OtpSession{
		SsoReqID:  "direct_login", // مقدار ثابت دادیم تا دیتابیس ارور نده و نیازی به مایگریشن نباشه
		Phone:     req.Phone,
		CodeHash:  string(hashedCode),
		ExpiresAt: time.Now().Add(2 * time.Minute),
	}
	database.DB.Create(&otp)

	services.SendOTPAsync(req.Phone, rawCode)

	return c.JSON(fiber.Map{"success": true, "uid": otp.UID})
}

func (h *AuthHandler) VerifyOTP(c fiber.Ctx) error {
	var req struct {
		UID  string `json:"uid"`
		Code string `json:"code"`
	}
	
	if err := c.Bind().Body(&req); err != nil {
    	return c.Status(400).JSON(fiber.Map{"error": "فرمت درخواست نامعتبر است"})
	}
	if req.UID == "" || req.Code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "پارامترهای ورودی ناقص است"})
	}

	var otp models.OtpSession
	if err := database.DB.Where("uid = ?", req.UID).First(&otp).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "نشست یافت نشد"})
	}

	if otp.Attempts >= 2 || time.Now().After(otp.ExpiresAt) {
		database.DB.Delete(&otp)
		return c.Status(400).JSON(fiber.Map{"error": "کد منقضی شده یا دفعات مجاز تمام شده است"})
	}

	err := bcrypt.CompareHashAndPassword([]byte(otp.CodeHash), []byte(req.Code))
	if err != nil {
		database.DB.Model(&otp).UpdateColumn("attempts", gorm.Expr("attempts + ?", 1))
		return c.Status(400).JSON(fiber.Map{"error": "کد اشتباه است"})
	}

	otp.IsVerified = true
	database.DB.Save(&otp)

	var user models.User
	if err := database.DB.Where("phone_number = ?", otp.Phone).First(&user).Error; err == nil && user.Status == "active" {
		
		accToken, refToken, err := createDirectSession(database.DB, c, user.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "خطا در ورود به سیستم"})
		}

		database.DB.Delete(&otp)
		return c.JSON(fiber.Map{
			"success":       true,
			"registered":    true,
			"access_token":  accToken,
			"refresh_token": refToken,
			"user_id":       user.ID,
		})
	}

	return c.JSON(fiber.Map{"success": true, "registered": false})
}

func (h *AuthHandler) RegisterKYC(c fiber.Ctx) error {
    var req struct {
        UID          string `json:"uid"`
        NationalCode string `json:"national_code"`
        BirthDate    string `json:"birth_date"`
    }

    if err := c.Bind().Body(&req); err != nil {
        return c.Status(400).JSON(fiber.Map{"error": "فرمت درخواست اشتباه است یا Content-Type تنظیم نشده"})
    }

    if req.UID == "" || req.NationalCode == "" || req.BirthDate == "" {
        return c.Status(400).JSON(fiber.Map{"error": "لطفاً تمام فیلدها را پر کنید"})
    }

    var otp models.OtpSession
    if err := database.DB.Where("uid = ?", req.UID).First(&otp).Error; err != nil || !otp.IsVerified {
        return c.Status(403).JSON(fiber.Map{"error": "نشست یافت نشد یا اول باید شماره را تایید کنید"})
    }

    if otp.KycFails >= 3 {
        database.DB.Delete(&otp)
        return c.Status(403).JSON(fiber.Map{"error": "تعداد دفعات مجاز خطا به پایان رسید. از ابتدا شروع کنید."})
    }

    shahkar, err := services.CheckShahkar(otp.Phone, req.NationalCode)
    if err != nil || !shahkar {
        database.DB.Model(&otp).UpdateColumn("kyc_fails", gorm.Expr("kyc_fails + ?", 1))
        return c.Status(403).JSON(fiber.Map{"error": "عدم تطابق شماره با کد ملی"})
    }

    identity, err := services.CheckFinnotechIdentity(req.NationalCode, req.BirthDate)
    if err != nil {
        return c.Status(400).JSON(fiber.Map{"error": "خطا در استعلام فینوتک"})
    }

    deathStatus, _ := identity["deathStatus"].(string)
    if deathStatus != "زنده" {
        database.DB.Create(&models.Blocklist{
            Phone:  otp.Phone,
            Reason: "استعلام فوت شده در سیستم ثبت احوال",
        })
        database.DB.Delete(&otp)
        return c.Status(403).JSON(fiber.Map{"error": "دسترسی مسدود گردید"})
    }

    firstName, _ := identity["firstName"].(string)
    lastName, _ := identity["lastName"].(string)
    fatherName, _ := identity["fatherName"].(string)
    genderStr, _ := identity["gender"].(string)

    identityNoRaw := identity["identityNo"]
    identityNo := fmt.Sprintf("%v", identityNoRaw)
    identitySeri, _ := identity["identitySeri"].(string)
    identitySerial, _ := identity["identitySerial"].(string)
    officeName, _ := identity["officeName"].(string)
    officeCode, _ := identity["officeCode"].(string)

    gender := 1
    if genderStr == "زن" {
        gender = 2
    }

    miladiDate, _ := services.ShamsiToGregorian(req.BirthDate)

    newUser := models.User{
        PhoneNumber:    &otp.Phone,
        NationalID:     &req.NationalCode,
        FirstName:      &firstName,
        LastName:       &lastName,
        FatherName:     &fatherName,
        BirthDate:      &miladiDate,
        Gender:         &gender,
        IdentityNo:     &identityNo,
        IdentitySeri:   &identitySeri,
        IdentitySerial: &identitySerial,
        OfficeName:     &officeName,
        OfficeCode:     &officeCode,
        Status:         "active",
    }

    tx := database.DB.Begin()

    if err := tx.Create(&newUser).Error; err != nil {
        tx.Rollback()
        return c.Status(500).JSON(fiber.Map{"error": "خطا در ثبت اطلاعات کاربر"})
    }

    accToken, refToken, err := createDirectSession(tx, c, newUser.ID)
    if err != nil {
        tx.Rollback()
        return c.Status(500).JSON(fiber.Map{"error": "خطا در ایجاد نشست"})
    }

    if err := tx.Delete(&otp).Error; err != nil {
        tx.Rollback()
        return c.Status(500).JSON(fiber.Map{"error": "خطا در تکمیل فرآیند"})
    }

    tx.Commit()

    return c.JSON(fiber.Map{
        "success": true,
        "user_data": fiber.Map{
            "firstName":  firstName,
            "lastName":   lastName,
            "fatherName": fatherName,
        },
        "access_token":  accToken,
        "refresh_token": refToken,
        "user_id":       newUser.ID,
    })
}