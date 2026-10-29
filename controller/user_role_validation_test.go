package controller

import (
	"fmt"
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

func TestCreateUserRejectsNonStandardRoles(t *testing.T) {
	db := setupOAuthAPIKeysTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	for _, role := range []int{-1, 2, 9, 11, 99, 101} {
		t.Run(fmt.Sprint(role), func(t *testing.T) {
			body, err := common.Marshal(map[string]any{
				"username": "valid-user", "password": "valid-password", "role": role,
			})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/user/", strings.NewReader(string(body)))
			c.Set("role", common.RoleRootUser)
			CreateUser(c)
			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, common.TranslateMessage(c, i18n.MsgInvalidParams), response.Message)
			var count int64
			require.NoError(t, db.Model(&model.User{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}
