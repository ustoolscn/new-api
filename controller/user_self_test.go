package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateSelfPartialProfile(t *testing.T) {
	const originalPassword = "CurrentPassword123"
	hash, err := common.Password2Hash(originalPassword)
	require.NoError(t, err)

	for _, tt := range []struct {
		name        string
		body        string
		password    string
		displayName string
		username    string
		errorKey    string
	}{
		{
			name:     "password without username",
			body:     `{"original_password":"CurrentPassword123","password":"NewPassword123"}`,
			password: "NewPassword123",
		},
		{
			name:        "display name without username",
			body:        `{"display_name":"Updated Name"}`,
			displayName: "Updated Name",
		},
		{
			name:     "explicit username remains supported",
			body:     `{"username":" renamed-user ","display_name":"Updated Name"}`,
			username: "renamed-user", displayName: "Updated Name",
		},
		{
			name:     "wrong original password",
			body:     `{"original_password":"WrongPassword123","password":"NewPassword123"}`,
			errorKey: i18n.MsgUserOriginalPasswordError,
		},
		{
			name:     "missing original password",
			body:     `{"password":"NewPassword123"}`,
			errorKey: i18n.MsgUserOriginalPasswordError,
		},
		{
			name:     "password too short",
			body:     `{"original_password":"CurrentPassword123","password":"short"}`,
			errorKey: i18n.MsgInvalidInput,
		},
		{
			name:     "password too long",
			body:     `{"original_password":"CurrentPassword123","password":"123456789012345678901"}`,
			errorKey: i18n.MsgInvalidInput,
		},
		{
			name:     "explicit empty username rejected",
			body:     `{"username":"  ","original_password":"CurrentPassword123","password":"NewPassword123"}`,
			errorKey: i18n.MsgInvalidParams,
		},
		{
			name:     "old validation placeholder is a valid password",
			body:     `{"original_password":"CurrentPassword123","password":"$I_LOVE_U"}`,
			password: "$I_LOVE_U",
		},
		{
			name:     "client cannot choose account or privileged fields",
			body:     `{"id":999,"role":100,"quota":999999,"group":"vip","status":2,"original_password":"CurrentPassword123","password":"NewPassword123"}`,
			password: "NewPassword123",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupOAuthAPIKeysTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.User{}))
			user := model.User{
				Username: "password-user", DisplayName: "Original Name", Password: hash,
				Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
				Quota: 12345, Group: "default", Email: "user@example.com",
			}
			require.NoError(t, db.Create(&user).Error)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/user/self", strings.NewReader(tt.body))
			c.Set("id", user.Id)
			UpdateSelf(c)

			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.Equal(t, http.StatusOK, recorder.Code)
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, tt.errorKey == "", response.Success, response.Message)
			if tt.errorKey != "" {
				assert.Equal(t, common.TranslateMessage(c, tt.errorKey), response.Message)
			}

			var saved model.User
			require.NoError(t, db.First(&saved, user.Id).Error)
			expected := user
			if tt.password != "" {
				assert.True(t, common.ValidatePasswordAndHash(tt.password, saved.Password))
				assert.False(t, common.ValidatePasswordAndHash(originalPassword, saved.Password))
				expected.Password = saved.Password
			}
			if tt.displayName != "" {
				expected.DisplayName = tt.displayName
			}
			if tt.username != "" {
				expected.Username = tt.username
			}
			assert.Equal(t, expected, saved, "unrequested account fields must be preserved")
		})
	}
}
