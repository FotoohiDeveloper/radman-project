package models

type InitAuthRequest struct {
	PhoneNumber string `json:"phone_number,omitempty"`
	Provider    string `json:"provider" validate:"required"`
}

type DeviceFingerprint struct {
	AndroidID    string `json:"android_id"`
	KeystoreUUID string `json:"keystore_uuid"`
	Brand        string `json:"brand"`
	Model        string `json:"model"`
	OSVersion    string `json:"os_version"`
}

type TokenRequest struct {
	AuthCode           string            `json:"auth_code" validate:"required"`
	CodeVerifier       string            `json:"code_verifier" validate:"required"`
	DeviceFingerprint  DeviceFingerprint `json:"device_fingerprint" validate:"required"`
	PlayIntegrityToken string            `json:"play_integrity_token"`
}

type KycRequest struct {
	NationalCode string `json:"national_code"`
	BirthDate    string `json:"birth_date"`
}

type ZohalShahkarReq struct {
	Mobile       string `json:"mobile"`
	NationalCode string `json:"national_code"`
}

type ZohalIdentityReq struct {
	BirthDate    string `json:"birth_date"`
	NationalCode string `json:"national_code"`
}

type ZohalResponse struct {
	ResponseBody struct {
		Data      map[string]interface{} `json:"data"`
		ErrorCode interface{}            `json:"error_code"`
		Message   string                 `json:"message"`
	} `json:"response_body"`
	Result int `json:"result"`
}