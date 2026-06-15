package handlers

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"radman.local/backend/internal/database"
	"radman.local/backend/internal/models"
	"radman.local/backend/internal/services"
)

type ParentHandler struct{}

func NewParentHandler() *ParentHandler {
	return &ParentHandler{}
}

func generateParentSecureOTP() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

// 🚨 متد کمکی برای بررسی سن قانونی (حداقل ۱۸ سال)
func isAdult(birthDate *time.Time) bool {
	if birthDate == nil {
		return false
	}
	eighteenYearsAgo := time.Now().AddDate(-18, 0, 0)
	return birthDate.Before(eighteenYearsAgo)
}

// ۱. ارسال پیامک کد تایید به شماره جدید فرزند همراه با شرط ۱۸ سال و تله زمانی ۲ دقیقه‌ای
func (h *ParentHandler) SendChildOTP(c fiber.Ctx) error {
	parentIDStr := c.Locals("user_id").(string)

	// استخراج اطلاعات والد برای چک کردن سن ۱۸ سال
	var parent models.User
	if err := database.DB.Where("id = ?", parentIDStr).First(&parent).Error; err != nil {
		return c.Status(444).JSON(fiber.Map{"error": "اطلاعات والد یافت نشد"})
	}

	// 🚨 اعمال شرط سن بالای ۱۸ سال
	if !isAdult(parent.BirthDate) {
		return c.Status(403).JSON(fiber.Map{"error": "افراد زیر ۱۸ سال (فرزندان) مجاز به ثبت فرزند جدید در سیستم نیستند."})
	}

	var req struct {
		ChildPhone string `json:"child_phone_number"`
	}
	if err := c.Bind().Body(&req); err != nil || req.ChildPhone == "" {
		return c.Status(400).JSON(fiber.Map{"error": "شماره موبایل فرزند الزامی است"})
	}

	var existingUser models.User
	if err := database.DB.Where("phone_number = ?", req.ChildPhone).First(&existingUser).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "این شماره موبایل قبلاً در سیستم ثبت شده است"})
	}

	var lastOtp models.OtpSession
	if err := database.DB.Where("sso_req_id = ?", parentIDStr).Order("created_at desc").First(&lastOtp).Error; err == nil {
		if time.Now().Before(lastOtp.ExpiresAt) {
			return c.Status(429).JSON(fiber.Map{"error": "شما هر ۲ دقیقه یک‌بار مجاز به ارسال کد جدید هستید. لطفاً کمی صبر کنید."})
		}
	}

	rawCode := generateParentSecureOTP()
	hashedCode, err := bcrypt.GenerateFromPassword([]byte(rawCode), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "خطا در سیستم امنیتی"})
	}

	otp := models.OtpSession{
		SsoReqID:  parentIDStr,
		Phone:     req.ChildPhone,
		CodeHash:  string(hashedCode),
		ExpiresAt: time.Now().Add(2 * time.Minute),
	}
	database.DB.Create(&otp)

	services.SendOTPAsync(req.ChildPhone, rawCode)

	return c.JSON(fiber.Map{"success": true, "uid": otp.UID})
}

// ۲. تایید OTP و استعلام شاهکار شماره جدید با کد ملی والدی که لاگین است
func (h *ParentHandler) VerifyChildOTP(c fiber.Ctx) error {
	parentIDStr := c.Locals("user_id").(string)

	var req struct {
		UID  string `json:"uid"`
		Code string `json:"code"`
	}
	c.Bind().Body(&req)

	var otp models.OtpSession
	if err := database.DB.Where("uid = ?", req.UID).First(&otp).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "نشست یافت نشد"})
	}

	if otp.Attempts >= 2 || time.Now().After(otp.ExpiresAt) {
		database.DB.Delete(&otp)
		return c.Status(400).JSON(fiber.Map{"error": "کد منقضی شده یا دفعات مجاز تمام شده است"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(otp.CodeHash), []byte(req.Code)); err != nil {
		otp.Attempts++
		database.DB.Save(&otp)
		return c.Status(400).JSON(fiber.Map{"error": "کد اشتباه است"})
	}

	var parent models.User
	if err := database.DB.Where("id = ?", parentIDStr).First(&parent).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "اطلاعات والد یافت نشد"})
	}

	shahkar, err := services.CheckShahkar(otp.Phone, *parent.NationalID)
	if err != nil || !shahkar {
		otp.KycFails++
		database.DB.Save(&otp)
		return c.Status(403).JSON(fiber.Map{"error": "عدم تطابق شماره جدید با کد ملی والد (سیم‌کارت حتما باید به نام خودتان باشد)"})
	}

	otp.IsVerified = true
	database.DB.Save(&otp)

	return c.JSON(fiber.Map{"success": true, "message": "شماره جدید با موفقیت تایید و احراز شد"})
}

// ۳. استعلام ثبت احوال فرزند و ایجاد کاربر موقت با ساختار قفل منحصربه‌فرد و بررسی سن قانونی والد
func (h *ParentHandler) RegisterChildKYC(c fiber.Ctx) error {
	parentIDStr := c.Locals("user_id").(string)
	parentUUID, _ := uuid.Parse(parentIDStr)

	var parent models.User
	if err := database.DB.Where("id = ?", parentIDStr).First(&parent).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "اطلاعات والد یافت نشد"})
	}

	// 🚨 اعمال شرط سن بالای ۱۸ سال در زمان استعلام و ساخت فرزند
	if !isAdult(parent.BirthDate) {
		return c.Status(403).JSON(fiber.Map{"error": "افراد زیر ۱۸ سال مجاز به ثبت فرزند جدید نیستند."})
	}

	var req struct {
		UID          string `json:"uid"`
		NationalCode string `json:"national_code"`
		BirthDate    string `json:"birth_date"`
	}
	if err := c.Bind().Body(&req); err != nil || req.UID == "" || req.NationalCode == "" || req.BirthDate == "" {
		return c.Status(400).JSON(fiber.Map{"error": "لطفاً تمام فیلدها را پر کنید"})
	}

	var otp models.OtpSession
	if err := database.DB.Where("uid = ?", req.UID).First(&otp).Error; err != nil || !otp.IsVerified {
		return c.Status(403).JSON(fiber.Map{"error": "ابتدا باید مرحله تایید شماره موبایل را انجام دهید"})
	}

	var existingChild models.User
	if err := database.DB.Where("national_id = ?", req.NationalCode).First(&existingChild).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "این کد ملی فرزند قبلاً توسط یکی از والدین در سیستم ثبت شده است"})
	}

	identity, err := services.CheckFinnotechIdentity(req.NationalCode, req.BirthDate)

	var childUser models.User
	childUser.ID = uuid.New()
	childUser.PhoneNumber = &otp.Phone
	childUser.NationalID = &req.NationalCode
	childUser.Role = "child"
	childUser.Status = "pending_approval"

	if err == nil {
		firstName, _ := identity["firstName"].(string)
		lastName, _ := identity["lastName"].(string)
		fatherName, _ := identity["fatherName"].(string)
		genderStr, _ := identity["gender"].(string)
		gender := 1
		if genderStr == "زن" {
			gender = 2
		}

		identityNoRaw := identity["identityNo"]
		identityNo := fmt.Sprintf("%v", identityNoRaw)
		identitySeri, _ := identity["identitySeri"].(string)
		identitySerial, _ := identity["identitySerial"].(string)
		officeName, _ := identity["officeName"].(string)
		officeCode, _ := identity["officeCode"].(string)
		miladiDate, _ := services.ShamsiToGregorian(req.BirthDate)

		childUser.FirstName = &firstName
		childUser.LastName = &lastName
		childUser.FatherName = &fatherName
		childUser.BirthDate = &miladiDate
		childUser.Gender = &gender
		childUser.IdentityNo = &identityNo
		childUser.IdentitySeri = &identitySeri
		childUser.IdentitySerial = &identitySerial
		childUser.OfficeName = &officeName
		childUser.OfficeCode = &officeCode
	} else {
		miladiDate, _ := services.ShamsiToGregorian(req.BirthDate)
		childUser.BirthDate = &miladiDate
	}

	tx := database.DB.Begin()
	if err := tx.Create(&childUser).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "خطا در ایجاد اکانت فرزند"})
	}

	relation := models.ChildRelation{
		ParentID: parentUUID,
		ChildID:  childUser.ID,
	}
	if err := tx.Create(&relation).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "خطا در برقراری ارتباط والد و فرزندی"})
	}
	tx.Commit()

	database.DB.Delete(&otp)

	return c.JSON(fiber.Map{
		"success":  true,
		"child_id": childUser.ID,
		"message":  "اطلاعات اولیه فرزند ثبت شد. لطفاً در مرحله بعد مدارک شناسایی را بارگذاری کنید.",
	})
}

// ۴. بارگذاری مدارک شناسایی فرزند و والد به فضای ابری S3 ابر آروان
func (h *ParentHandler) UploadChildDocuments(c fiber.Ctx) error {
	parentIDStr := c.Locals("user_id").(string)
	parentUUID, err := uuid.Parse(parentIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "کاربر غیرمجاز"})
	}

	childIDStr := c.FormValue("child_id")
	if childIDStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "ارسال آیدی فرزند (child_id) الزامی است"})
	}
	childUUID, err := uuid.Parse(childIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "فرمت آیدی فرزند نامعتبر است"})
	}

	var relation models.ChildRelation
	if err := database.DB.Where("parent_id = ? AND child_id = ?", parentUUID, childUUID).First(&relation).Error; err != nil {
		return c.Status(403).JSON(fiber.Map{"error": "شما مجاز به بارگذاری مدارک برای این فرزند نیستید"})
	}

	parentCardHeader, err := c.FormFile("parent_identity_card")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "بارگذاری تصویر شناسنامه/کارت‌ملی والد الزامی است"})
	}

	childCardHeader, err := c.FormFile("child_identity_card")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "بارگذاری تصویر شناسنامه فرزند الزامی است"})
	}

	const maxFileSize = 3 * 1024 * 1024 // 3MB
	if parentCardHeader.Size > maxFileSize || childCardHeader.Size > maxFileSize {
		return c.Status(400).JSON(fiber.Map{"error": "حجم هر تصویر نباید بیشتر از ۳ مگابایت باشد"})
	}

	allowedExtensions := map[string]bool{".jpg": true, ".jpeg": true, ".png": true}
	parentExt := strings.ToLower(filepath.Ext(parentCardHeader.Filename))
	childExt := strings.ToLower(filepath.Ext(childCardHeader.Filename))
	if !allowedExtensions[parentExt] || !allowedExtensions[childExt] {
		return c.Status(400).JSON(fiber.Map{"error": "فرمت فایل‌ها حتماً باید تصویر (png, jpg, jpeg) باشد"})
	}

	s3Client := s3.NewFromConfig(aws.Config{
		Region: "ir-thr-at1",
		Credentials: credentials.NewStaticCredentialsProvider(
			os.Getenv("ARVAN_S3_ACCESS_KEY"),
			os.Getenv("ARVAN_S3_SECRET_KEY"),
			"",
		),
	}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("https://alifotoohi.s3.ir-thr-at1.arvanstorage.ir")
		o.UsePathStyle = true
	})

	bucketName := os.Getenv("ARVAN_S3_BUCKET_NAME")

	parentFile, _ := parentCardHeader.Open()
	defer parentFile.Close()
	parentKey := fmt.Sprintf("identities/%s_parent%s", childUUID.String(), parentExt)

	_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(parentKey),
		Body:   parentFile,
		ACL:    types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "خطا در آپلود مدرک والد به فضای ابری"})
	}

	childFile, _ := childCardHeader.Open()
	defer childFile.Close()
	childKey := fmt.Sprintf("identities/%s_child%s", childUUID.String(), childExt)

	_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(childKey),
		Body:   childFile,
		ACL:    types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "خطا در آپلود مدرک فرزند به فضای ابری"})
	}

	parentURL := fmt.Sprintf("https://alifotoohi.s3.ir-thr-at1.arvanstorage.ir/%s/%s", bucketName, parentKey)
	childURL := fmt.Sprintf("https://alifotoohi.s3.ir-thr-at1.arvanstorage.ir/%s/%s", bucketName, childKey)

	err = database.DB.Model(&models.User{}).Where("id = ?", childUUID).Updates(map[string]interface{}{
		"parent_identity_card_url": parentURL,
		"child_identity_card_url":  childURL,
	}).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "خطا در نهایی‌سازی مدارک فرزند در دیتابیس"})
	}

	var parent models.User
	if err := database.DB.Where("id = ?", parentIDStr).First(&parent).Error; err == nil {
		services.SendOTPAsync(*parent.PhoneNumber, "ثبت نام فرزند شما انجام شد و پس از بررسی ادمین فعال می‌گردد.")
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "مدارک با موفقیت بارگذاری شد. نتیجه پس از بررسی ادمین به شما پیامک خواهد شد.",
	})
}

// ۵. مشاهده لیست فرزندان ثبت شده توسط والد به همراه وضعیت تایید آن‌ها
func (h *ParentHandler) GetChildrenList(c fiber.Ctx) error {
	parentIDStr := c.Locals("user_id").(string)
	parentUUID, _ := uuid.Parse(parentIDStr)

	var childRelations []models.ChildRelation
	if err := database.DB.Where("parent_id = ?", parentUUID).Find(&childRelations).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "خطا در واکشی ارتباطات فرزندی"})
	}

	if len(childRelations) == 0 {
		return c.JSON(fiber.Map{"success": true, "data": []interface{}{}})
	}

	var childIDs []uuid.UUID
	for _, rel := range childRelations {
		childIDs = append(childIDs, rel.ChildID)
	}

	var children []models.User
	if err := database.DB.Where("id IN ?", childIDs).Find(&children).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "خطا در واکشی اطلاعات فرزندان"})
	}

	var result []fiber.Map
	for _, child := range children {
		result = append(result, fiber.Map{
			"child_id":      child.ID,
			"first_name":    child.FirstName,
			"last_name":     child.LastName,
			"national_id":   child.NationalID,
			"phone_number":  child.PhoneNumber,
			"status":        child.Status,
			"registered_at": child.CreatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    result,
	})
}