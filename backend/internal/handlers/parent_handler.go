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
	"radman.local/backend/internal/i18n"
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

func isAdult(birthDate *time.Time) bool {
	if birthDate == nil {
		return false
	}
	eighteenYearsAgo := time.Now().AddDate(-18, 0, 0)
	return birthDate.Before(eighteenYearsAgo)
}

func (h *ParentHandler) SendChildOTP(c fiber.Ctx) error {
	parentIDStr := c.Locals("user_id").(string)

	var parent models.User
	if err := database.DB.Where("id = ?", parentIDStr).First(&parent).Error; err != nil {
		return c.Status(444).JSON(fiber.Map{"error": i18n.T("parent.not_found")})
	}

	if !isAdult(parent.BirthDate) {
		return c.Status(403).JSON(fiber.Map{"error": i18n.T("parent.underage")})
	}

	var req struct {
		ChildPhone string `json:"child_phone_number"`
	}
	if err := c.Bind().Body(&req); err != nil || req.ChildPhone == "" {
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("parent.child_phone_required")})
	}

	var existingUser models.User
	if err := database.DB.Where("phone_number = ?", req.ChildPhone).First(&existingUser).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("parent.phone_exists")})
	}

	var lastOtp models.OtpSession
	if err := database.DB.Where("sso_req_id = ?", parentIDStr).Order("created_at desc").First(&lastOtp).Error; err == nil {
		if time.Now().Before(lastOtp.ExpiresAt) {
			return c.Status(429).JSON(fiber.Map{"error": i18n.T("auth.otp_cooldown")})
		}
	}

	rawCode := generateParentSecureOTP()
	hashedCode, err := bcrypt.GenerateFromPassword([]byte(rawCode), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": i18n.T("auth.security_error")})
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

func (h *ParentHandler) VerifyChildOTP(c fiber.Ctx) error {
	parentIDStr := c.Locals("user_id").(string)

	var req struct {
		UID  string `json:"uid"`
		Code string `json:"code"`
	}
	c.Bind().Body(&req)

	var otp models.OtpSession
	if err := database.DB.Where("uid = ?", req.UID).First(&otp).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": i18n.T("auth.session_not_found")})
	}

	if otp.Attempts >= 2 || time.Now().After(otp.ExpiresAt) {
		database.DB.Delete(&otp)
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("auth.otp_exhausted")})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(otp.CodeHash), []byte(req.Code)); err != nil {
		otp.Attempts++
		database.DB.Save(&otp)
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("auth.wrong_code")})
	}

	var parent models.User
	if err := database.DB.Where("id = ?", parentIDStr).First(&parent).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": i18n.T("parent.not_found")})
	}

	shahkar, err := services.CheckShahkar(otp.Phone, *parent.NationalID)
	if err != nil || !shahkar {
		otp.KycFails++
		database.DB.Save(&otp)
		return c.Status(403).JSON(fiber.Map{"error": i18n.T("parent.shahkar_mismatch")})
	}

	otp.IsVerified = true
	database.DB.Save(&otp)

	return c.JSON(fiber.Map{"success": true, "message": i18n.T("parent.child_verified")})
}

func (h *ParentHandler) RegisterChildKYC(c fiber.Ctx) error {
	parentIDStr := c.Locals("user_id").(string)
	parentUUID, _ := uuid.Parse(parentIDStr)

	var parent models.User
	if err := database.DB.Where("id = ?", parentIDStr).First(&parent).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": i18n.T("parent.not_found")})
	}

	if !isAdult(parent.BirthDate) {
		return c.Status(403).JSON(fiber.Map{"error": i18n.T("parent.underage")})
	}

	var req struct {
		UID          string `json:"uid"`
		NationalCode string `json:"national_code"`
		BirthDate    string `json:"birth_date"`
	}
	if err := c.Bind().Body(&req); err != nil || req.UID == "" || req.NationalCode == "" || req.BirthDate == "" {
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("parent.fill_fields")})
	}

	var otp models.OtpSession
	if err := database.DB.Where("uid = ?", req.UID).First(&otp).Error; err != nil || !otp.IsVerified {
		return c.Status(403).JSON(fiber.Map{"error": i18n.T("parent.verify_first")})
	}

	var existingChild models.User
	if err := database.DB.Where("national_id = ?", req.NationalCode).First(&existingChild).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("parent.child_nid_exists")})
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
		return c.Status(500).JSON(fiber.Map{"error": i18n.T("parent.child_create_error")})
	}

	relation := models.ChildRelation{
		ParentID: parentUUID,
		ChildID:  childUser.ID,
	}
	if err := tx.Create(&relation).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": i18n.T("parent.relation_create_error")})
	}
	tx.Commit()

	database.DB.Delete(&otp)

	return c.JSON(fiber.Map{
		"success":  true,
		"child_id": childUser.ID,
		"message":  i18n.T("parent.child_kyc_done"),
	})
}

func (h *ParentHandler) UploadChildDocuments(c fiber.Ctx) error {
	parentIDStr := c.Locals("user_id").(string)
	parentUUID, err := uuid.Parse(parentIDStr)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": i18n.T("parent.unauthorized")})
	}

	childIDStr := c.FormValue("child_id")
	if childIDStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("parent.child_id_required")})
	}
	childUUID, err := uuid.Parse(childIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("parent.child_id_invalid")})
	}

	var relation models.ChildRelation
	if err := database.DB.Where("parent_id = ? AND child_id = ?", parentUUID, childUUID).First(&relation).Error; err != nil {
		return c.Status(403).JSON(fiber.Map{"error": i18n.T("parent.upload_forbidden")})
	}

	parentCardHeader, err := c.FormFile("parent_identity_card")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("parent.parent_card_required")})
	}

	childCardHeader, err := c.FormFile("child_identity_card")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("parent.child_card_required")})
	}

	const maxFileSize = 3 * 1024 * 1024
	if parentCardHeader.Size > maxFileSize || childCardHeader.Size > maxFileSize {
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("parent.file_too_large")})
	}

	allowedExtensions := map[string]bool{".jpg": true, ".jpeg": true, ".png": true}
	parentExt := strings.ToLower(filepath.Ext(parentCardHeader.Filename))
	childExt := strings.ToLower(filepath.Ext(childCardHeader.Filename))
	if !allowedExtensions[parentExt] || !allowedExtensions[childExt] {
		return c.Status(400).JSON(fiber.Map{"error": i18n.T("parent.invalid_file_type")})
	}

	s3Client := s3.NewFromConfig(aws.Config{
		Region: os.Getenv("ARVAN_S3_REGION"),
		Credentials: credentials.NewStaticCredentialsProvider(
			os.Getenv("ARVAN_S3_ACCESS_KEY"),
			os.Getenv("ARVAN_S3_SECRET_KEY"),
			"",
		),
	}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(os.Getenv("ARVAN_S3_ENDPOINT"))
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
		return c.Status(500).JSON(fiber.Map{"error": i18n.T("parent.parent_upload_error")})
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
		return c.Status(500).JSON(fiber.Map{"error": i18n.T("parent.child_upload_error")})
	}

	s3PublicURL := os.Getenv("ARVAN_S3_ENDPOINT")
	parentURL := fmt.Sprintf("%s/%s/%s", s3PublicURL, bucketName, parentKey)
	childURL := fmt.Sprintf("%s/%s/%s", s3PublicURL, bucketName, childKey)

	err = database.DB.Model(&models.User{}).Where("id = ?", childUUID).Updates(map[string]interface{}{
		"parent_identity_card_url": parentURL,
		"child_identity_card_url":  childURL,
	}).Error
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": i18n.T("parent.doc_save_error")})
	}

	var parentUser models.User
	if err := database.DB.Where("id = ?", parentIDStr).First(&parentUser).Error; err == nil {
		services.SendOTPAsync(*parentUser.PhoneNumber, i18n.T("parent.child_registered_sms"))
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": i18n.T("parent.docs_uploaded"),
	})
}

func (h *ParentHandler) GetChildrenList(c fiber.Ctx) error {
	parentIDStr := c.Locals("user_id").(string)
	parentUUID, _ := uuid.Parse(parentIDStr)

	var childRelations []models.ChildRelation
	if err := database.DB.Where("parent_id = ?", parentUUID).Find(&childRelations).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": i18n.T("parent.children_fetch_error")})
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
		return c.Status(500).JSON(fiber.Map{"error": i18n.T("parent.children_data_error")})
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

	return c.JSON(fiber.Map{"success": true, "data": result})
}
