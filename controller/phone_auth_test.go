package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type authAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func setupPhoneAuthControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.PasswordLoginEnabled = true
	common.EmailVerificationEnabled = false
	common.PhoneRegisterEnabled = true
	originalSMSVerificationEnabled := common.SMSVerificationEnabled

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.Token{}))

	t.Cleanup(func() {
		common.RegisterEnabled = true
		common.PasswordRegisterEnabled = true
		common.PasswordLoginEnabled = true
		common.EmailVerificationEnabled = false
		common.PhoneRegisterEnabled = false
		common.SMSVerificationEnabled = originalSMSVerificationEnabled
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func performPhoneAuthRequest(handler gin.HandlerFunc, body string) authAPIResponse {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler(c)

	var payload authAPIResponse
	_ = common.Unmarshal(w.Body.Bytes(), &payload)
	return payload
}

func performPhoneAuthRequestAsUser(handler gin.HandlerFunc, userId int, body string) authAPIResponse {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", userId)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler(c)

	var payload authAPIResponse
	_ = common.Unmarshal(w.Body.Bytes(), &payload)
	return payload
}

func performPhoneAuthGetRequest(handler gin.HandlerFunc, target string) authAPIResponse {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)

	handler(c)

	var payload authAPIResponse
	_ = common.Unmarshal(w.Body.Bytes(), &payload)
	return payload
}

func TestPhoneRegisterCreatesUserWithProvidedUsernameAndPhone(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	common.RegisterVerificationCodeWithKey("13800138000", "123456", common.PhoneVerificationPurpose)

	res := performPhoneAuthRequest(Register, `{"username":"phone_user","password":"password123","phone":"13800138000","sms_code":"123456"}`)

	assert.True(t, res.Success, res.Message)
	var user model.User
	require.NoError(t, db.First(&user, "username = ?", "phone_user").Error)
	assert.Equal(t, "13800138000", user.Phone)
}

func TestPasswordOnlyRegisterCreatesUserWhenVerificationMethodsDisabled(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	common.PhoneRegisterEnabled = false

	res := performPhoneAuthRequest(Register, `{"username":"password_user","password":"password123"}`)

	assert.True(t, res.Success, res.Message)
	var user model.User
	require.NoError(t, db.First(&user, "username = ?", "password_user").Error)
	assert.Empty(t, user.Email)
	assert.Empty(t, user.Phone)
}

func TestPasswordOnlyRegisterStillRequiresEmailWhenEmailVerificationEnabled(t *testing.T) {
	setupPhoneAuthControllerTestDB(t)
	common.PhoneRegisterEnabled = false
	common.EmailVerificationEnabled = true

	res := performPhoneAuthRequest(Register, `{"username":"password_user","password":"password123"}`)

	assert.False(t, res.Success)
}

func TestPhoneRegisterRejectsDuplicatePhone(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	hashedPassword, err := common.Password2Hash("password123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Username: "existing",
		Password: hashedPassword,
		Phone:    "13800138000",
		Status:   common.UserStatusEnabled,
	}).Error)
	common.RegisterVerificationCodeWithKey("13800138000", "123456", common.PhoneVerificationPurpose)

	res := performPhoneAuthRequest(Register, `{"username":"phone_user","password":"password123","phone":"13800138000","sms_code":"123456"}`)

	assert.False(t, res.Success)
}

func TestPhoneRegisterRejectsPhoneNumberUsername(t *testing.T) {
	setupPhoneAuthControllerTestDB(t)
	common.RegisterVerificationCodeWithKey("13900139000", "123456", common.PhoneVerificationPurpose)

	res := performPhoneAuthRequest(Register, `{"username":"13800138000","password":"password123","phone":"13900139000","sms_code":"123456"}`)

	assert.False(t, res.Success)
}

func TestPhoneRegisterRejectsPhoneMatchingExistingUsername(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	hashedPassword, err := common.Password2Hash("password123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Username: "13800138000",
		Password: hashedPassword,
		Status:   common.UserStatusEnabled,
	}).Error)
	common.RegisterVerificationCodeWithKey("13800138000", "123456", common.PhoneVerificationPurpose)

	res := performPhoneAuthRequest(Register, `{"username":"phone_user","password":"password123","phone":"13800138000","sms_code":"123456"}`)

	assert.False(t, res.Success)
}

func TestPhoneRegisterRequiresEnabledSwitch(t *testing.T) {
	setupPhoneAuthControllerTestDB(t)
	common.PhoneRegisterEnabled = false
	common.RegisterVerificationCodeWithKey("13800138000", "123456", common.PhoneVerificationPurpose)

	res := performPhoneAuthRequest(Register, `{"username":"phone_user","password":"password123","phone":"13800138000","sms_code":"123456"}`)

	assert.False(t, res.Success)
}

func TestPasswordLoginAcceptsPhoneNumber(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	hashedPassword, err := common.Password2Hash("password123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Username: "phone_user",
		Password: hashedPassword,
		Phone:    "13800138000",
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)

	user := model.User{
		Username: "13800138000",
		Password: "password123",
	}

	require.NoError(t, user.ValidateAndFill())
	assert.Equal(t, "phone_user", user.Username)
}

func TestPasswordLoginPrefersExactUsernameOverPhoneMatch(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	phoneUserPassword, err := common.Password2Hash("phonepass123")
	require.NoError(t, err)
	exactUserPassword, err := common.Password2Hash("exactpass123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Username: "phone_user",
		Password: phoneUserPassword,
		Phone:    "13800138000",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "phone-user",
	}).Error)
	require.NoError(t, db.Create(&model.User{
		Username: "13800138000",
		Password: exactUserPassword,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "exact-user",
	}).Error)

	user := model.User{
		Username: "13800138000",
		Password: "exactpass123",
	}

	require.NoError(t, user.ValidateAndFill())
	assert.Equal(t, "13800138000", user.Username)
}

func TestPasswordResetAcceptsPhoneVerificationCode(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	hashedPassword, err := common.Password2Hash("oldpass123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Username: "phone_user",
		Password: hashedPassword,
		Phone:    "13800138000",
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)
	common.RegisterVerificationCodeWithKey("13800138000", "123456", common.PasswordResetPurpose)

	res := performPhoneAuthRequest(ResetPassword, `{"phone":"13800138000","token":"123456"}`)

	assert.True(t, res.Success, res.Message)
	require.NotEmpty(t, res.Data)
	user := model.User{
		Username: "phone_user",
		Password: res.Data,
	}
	require.NoError(t, user.ValidateAndFill())
	assert.False(t, common.VerifyCodeWithKey("13800138000", "123456", common.PasswordResetPurpose))
}

func TestPasswordResetRejectsInvalidPhoneVerificationCode(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	hashedPassword, err := common.Password2Hash("oldpass123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Username: "phone_user",
		Password: hashedPassword,
		Phone:    "13800138000",
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)
	common.RegisterVerificationCodeWithKey("13800138000", "123456", common.PasswordResetPurpose)

	res := performPhoneAuthRequest(ResetPassword, `{"phone":"13800138000","token":"000000"}`)

	assert.False(t, res.Success)
	user := model.User{
		Username: "phone_user",
		Password: "oldpass123",
	}
	require.NoError(t, user.ValidateAndFill())
}

func TestPasswordResetRejectsAmbiguousPhone(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	hashedPassword, err := common.Password2Hash("oldpass123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Username: "first_phone_user",
		Password: hashedPassword,
		Phone:    "13800138000",
		Status:   common.UserStatusEnabled,
		AffCode:  "first-phone-user",
	}).Error)
	require.NoError(t, db.Create(&model.User{
		Username: "second_phone_user",
		Password: hashedPassword,
		Phone:    "13800138000",
		Status:   common.UserStatusEnabled,
		AffCode:  "second-phone-user",
	}).Error)
	common.RegisterVerificationCodeWithKey("13800138000", "123456", common.PasswordResetPurpose)

	res := performPhoneAuthRequest(ResetPassword, `{"phone":"13800138000","token":"123456"}`)

	assert.False(t, res.Success)
}

func TestSendPasswordResetPhoneDoesNotRevealMissingPhone(t *testing.T) {
	setupPhoneAuthControllerTestDB(t)
	common.SMSVerificationEnabled = false

	res := performPhoneAuthGetRequest(SendPasswordResetPhone, "/?phone=13800138000")

	assert.True(t, res.Success, res.Message)
}

func TestPhoneBindStoresVerifiedPhone(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Username: "binding_user",
		Password: "password123",
		Status:   common.UserStatusEnabled,
	}).Error)
	common.RegisterVerificationCodeWithKey("13800138000", "123456", common.PhoneVerificationPurpose)

	res := performPhoneAuthRequestAsUser(PhoneBind, 1, `{"phone":"13800138000","code":"123456"}`)

	assert.True(t, res.Success, res.Message)
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, "13800138000", user.Phone)
	assert.False(t, common.VerifyCodeWithKey("13800138000", "123456", common.PhoneVerificationPurpose))
}

func TestPhoneBindRejectsInvalidVerificationCode(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Username: "binding_user",
		Password: "password123",
		Status:   common.UserStatusEnabled,
	}).Error)
	common.RegisterVerificationCodeWithKey("13800138000", "123456", common.PhoneVerificationPurpose)

	res := performPhoneAuthRequestAsUser(PhoneBind, 1, `{"phone":"13800138000","code":"000000"}`)

	assert.False(t, res.Success)
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Empty(t, user.Phone)
}
