package services

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
	"bytes"
)

var (
	cachedToken string
	tokenExpiry time.Time
	tokenMutex  sync.Mutex
)

const FinnotechBaseURL = "https://api.finnotech.ir"

func getFinnotechToken() (string, error) {
	tokenMutex.Lock()
	defer tokenMutex.Unlock()

	if cachedToken != "" && time.Now().Before(tokenExpiry) {
		return cachedToken, nil
	}

	clientID := os.Getenv("FINNOTECH_CLIENT_ID")
	clientSecret := os.Getenv("FINNOTECH_CLIENT_SECRET")
	nid := os.Getenv("FINNOTECH_NID")

	auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	
	body := map[string]string{
		"grant_type": "client_credentials",
		"nid":        nid,
		"scopes":     "kyc:identification-inquiry:get,kyc:national-card-image:get",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "https://apibeta.finnotech.ir/dev/v2/oauth2/token", bytes.NewBuffer(jsonBody))
	req.Header.Add("Authorization", "Basic "+auth)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if res, ok := result["result"].(map[string]interface{}); ok {
		cachedToken = res["value"].(string)
		lifeTime := res["lifeTime"].(float64)
		tokenExpiry = time.Now().Add(time.Duration(lifeTime) * time.Millisecond).Add(-10 * time.Minute)
		return cachedToken, nil
	}

	return "", fmt.Errorf("failed to get finnotech token")
}

func CheckFinnotechIdentity(nationalCode, birthDate string) (map[string]interface{}, error) {
	token, err := getFinnotechToken()
	if err != nil {
		return nil, err
	}

	clientID := os.Getenv("FINNOTECH_CLIENT_ID")
	url := fmt.Sprintf("%s/kyc/v2/clients/%s/identificationInquiry?nationalCode=%s&birthDate=%s", 
		FinnotechBaseURL, clientID, nationalCode, birthDate)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if status, ok := result["status"].(string); ok && status == "DONE" {
		return result["result"].(map[string]interface{}), nil
	}

	return nil, fmt.Errorf("استعلام فینوتک ناموفق بود")
}