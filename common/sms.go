package common

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const aliyunDypnsEndpoint = "https://dypnsapi.aliyuncs.com/"

type aliyunSMSResponse struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
	Success bool   `json:"Success"`
}

func GenerateNumericVerificationCode(length int) string {
	if length <= 0 {
		length = 6
	}
	var builder strings.Builder
	for builder.Len() < length {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			builder.WriteByte(byte('0' + time.Now().UnixNano()%10))
			continue
		}
		builder.WriteString(n.String())
	}
	return builder.String()
}

func SendSMSVerificationCode(phone string, code string) error {
	if !SMSVerificationEnabled {
		return errors.New("sms verification is disabled")
	}
	if SMSAccessKeyId == "" || SMSAccessKeySecret == "" || SMSSignName == "" || SMSTemplateCode == "" {
		return errors.New("sms verification is not configured")
	}

	templateParam := strings.TrimSpace(SMSTemplateParam)
	if templateParam == "" {
		templateParam = `{"code":"%s","min":"%d"}`
	}
	if strings.Contains(templateParam, "##code##") || strings.Contains(templateParam, "##min##") {
		templateParam = strings.ReplaceAll(templateParam, "##code##", code)
		templateParam = strings.ReplaceAll(templateParam, "##min##", strconv.Itoa(SMSValidTime/60))
	} else if strings.Contains(templateParam, "%s") || strings.Contains(templateParam, "%d") {
		templateParam = fmt.Sprintf(templateParam, code, SMSValidTime/60)
	}

	params := map[string]string{
		"Action":           "SendSmsVerifyCode",
		"Version":          "2017-05-25",
		"Format":           "JSON",
		"AccessKeyId":      SMSAccessKeyId,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"CountryCode":      "86",
		"PhoneNumber":      phone,
		"SignName":         SMSSignName,
		"TemplateCode":     SMSTemplateCode,
		"TemplateParam":    templateParam,
		"CodeLength":       strconv.Itoa(SMSCodeLength),
		"ValidTime":        strconv.Itoa(SMSValidTime),
		"DuplicatePolicy":  "1",
		"Interval":         strconv.Itoa(SMSInterval),
		"CodeType":         "1",
	}
	if SMSSchemeName != "" {
		params["SchemeName"] = SMSSchemeName
	}

	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	form.Set("Signature", signAliyunRPCParams(params))

	req, err := http.NewRequest(http.MethodPost, aliyunDypnsEndpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var parsed aliyunSMSResponse
	if err := Unmarshal(body, &parsed); err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusBadRequest || parsed.Code != "OK" || !parsed.Success {
		if parsed.Message != "" {
			return fmt.Errorf("send sms verification failed: %s", parsed.Message)
		}
		return fmt.Errorf("send sms verification failed: %s", resp.Status)
	}
	return nil
}

func signAliyunRPCParams(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	encoded := make([]string, 0, len(keys))
	for _, key := range keys {
		encoded = append(encoded, aliyunPercentEncode(key)+"="+aliyunPercentEncode(params[key]))
	}
	canonicalizedQueryString := strings.Join(encoded, "&")
	stringToSign := "POST&%2F&" + aliyunPercentEncode(canonicalizedQueryString)
	mac := hmac.New(sha1.New, []byte(SMSAccessKeySecret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func aliyunPercentEncode(value string) string {
	encoded := url.QueryEscape(value)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}
