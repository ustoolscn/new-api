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
}

func setupPhoneAuthControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.PasswordLoginEnabled = true
	common.EmailVerificationEnabled = false
	common.PhoneRegisterEnabled = true

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

func TestPhoneRegisterCreatesUserWithProvidedUsernameAndPhone(t *testing.T) {
	db := setupPhoneAuthControllerTestDB(t)
	common.RegisterVerificationCodeWithKey("13800138000", "123456", common.PhoneVerificationPurpose)

	res := performPhoneAuthRequest(Register, `{"username":"phone_user","password":"password123","phone":"13800138000","sms_code":"123456"}`)

	assert.True(t, res.Success, res.Message)
	var user model.User
	require.NoError(t, db.First(&user, "username = ?", "phone_user").Error)
	assert.Equal(t, "13800138000", user.Phone)
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
