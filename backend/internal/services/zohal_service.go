package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
	ptime "github.com/yaa110/go-persian-calendar"
	"radman.local/backend/internal/models"
)

const ZohalBaseURL = "https://service.zohal.io/api/v0/services/inquiry"

func CheckShahkar(mobile, nationalCode string) (bool, error) {
	reqBody := models.ZohalShahkarReq{
		Mobile:       mobile,
		NationalCode: nationalCode,
	}
	
	respData, err := sendZohalRequest("/shahkar", reqBody)
	if err != nil {
		return false, err
	}

	matched, ok := respData.ResponseBody.Data["matched"].(bool)
	if !ok {
		return false, fmt.Errorf("invalid response format from shahkar")
	}
	return matched, nil
}

func CheckNationalIdentity(nationalCode, birthDate string) (map[string]interface{}, error) {
	reqBody := models.ZohalIdentityReq{
		NationalCode: nationalCode,
		BirthDate:    birthDate,
	}

	respData, err := sendZohalRequest("/national_identity_inquiry", reqBody)
	if err != nil {
		return nil, err
	}

	matched, _ := respData.ResponseBody.Data["matched"].(bool)
	if !matched {
		return nil, fmt.Errorf("national code and birth date do not match")
	}

	return respData.ResponseBody.Data, nil
}

func sendZohalRequest(endpoint string, payload interface{}) (*models.ZohalResponse, error) {
	token := os.Getenv("ZOHAL_API_TOKEN")
	
	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", ZohalBaseURL+endpoint, bytes.NewBuffer(jsonPayload))
	
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var zohalResp models.ZohalResponse
	if err := json.NewDecoder(res.Body).Decode(&zohalResp); err != nil {
		return nil, err
	}

	if zohalResp.Result != 1 {
		return nil, fmt.Errorf("zohal error: %s", zohalResp.ResponseBody.Message)
	}

	return &zohalResp, nil
}

func ShamsiToGregorian(shamsiStr string) (time.Time, error) {
	var year, month, day int
	_, err := fmt.Sscanf(shamsiStr, "%d/%d/%d", &year, &month, &day)
	if err != nil {
		return time.Time{}, fmt.Errorf("فرمت تاریخ نامعتبر است")
	}

	// ساعت رو روی ۱۲ ظهر تنظیم می‌کنیم که مشکل جابجایی روز به خاطر تایم‌زون پیش نیاد
	pt := ptime.Date(year, ptime.Month(month), day, 12, 0, 0, 0, ptime.Iran())
	return pt.Time(), nil
}