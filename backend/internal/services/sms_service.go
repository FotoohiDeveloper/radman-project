package services

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
)

type SMSParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SMSVerifyRequest struct {
	Mobile     string         `json:"mobile"`
	TemplateId int            `json:"templateId"`
	Parameters []SMSParameter `json:"parameters"`
}

type SMSPayload struct {
	Phone string
	Code  string
}

var smsQueue = make(chan SMSPayload, 1000)

func StartSMSWorker() {
	log.Println("🚀 SMS Worker started in background...")
	for payload := range smsQueue {
		sendVerifySMS(payload.Phone, payload.Code)
	}
}

func SendOTPAsync(phone, code string) {
	smsQueue <- SMSPayload{Phone: phone, Code: code}
}

func sendVerifySMS(phone, code string) {
	apiKey := os.Getenv("SMSIR_API_KEY")
	templateIDStr := os.Getenv("SMSIR_OTP_TEMPLATE")
	templateID, _ := strconv.Atoi(templateIDStr)

	reqBody := SMSVerifyRequest{
		Mobile:     phone,
		TemplateId: templateID,
		Parameters: []SMSParameter{
			{Name: "CODE", Value: code},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.sms.ir/v1/send/verify", bytes.NewBuffer(bodyBytes))
	req.Header.Add("X-API-KEY", apiKey)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("❌ Error sending SMS:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("✅ OTP SMS sent successfully to %s\n", phone)
	} else {
		log.Printf("⚠️ SMS API returned status: %d for %s\n", resp.StatusCode, phone)
	}
}
