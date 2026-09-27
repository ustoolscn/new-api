package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferralModelConsumeHandlerScopeAndValidation(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.QuotaData{}))
	require.NoError(t, db.Create(&[]model.User{
		{Id: 101, Username: "my-invitee", AffCode: "my-code", InviterId: 10},
		{Id: 201, Username: "other-invitee", AffCode: "other-code", InviterId: 20},
	}).Error)
	require.NoError(t, db.Create(&[]model.QuotaData{
		{UserID: 101, ModelName: "mine", CreatedAt: 1000, Quota: 50},
		{UserID: 201, ModelName: "other", CreatedAt: 1000, Quota: 90},
	}).Error)
	for _, tt := range []struct {
		name    string
		adminID string
		query   string
		success bool
		quota   int64
	}{
		{"self aggregate", "", "start_timestamp=1000&end_timestamp=2000", true, 50},
		{"self invitee", "", "start_timestamp=1000&end_timestamp=2000&invitee_id=101", true, 50},
		{"cannot override self", "", "start_timestamp=1000&end_timestamp=2000&inviter_id=20&id=20", true, 50},
		{"foreign invitee", "", "start_timestamp=1000&end_timestamp=2000&invitee_id=201", false, 0},
		{"admin target", "20", "start_timestamp=1000&end_timestamp=2000&invitee_id=201", true, 90},
		{"admin wrong relation", "20", "start_timestamp=1000&end_timestamp=2000&invitee_id=101", false, 0},
		{"invalid admin id", "bad", "start_timestamp=1000&end_timestamp=2000", false, 0},
		{"invalid invitee id", "", "start_timestamp=1000&end_timestamp=2000&invitee_id=bad", false, 0},
		{"zero invitee id", "", "start_timestamp=1000&end_timestamp=2000&invitee_id=0", false, 0},
		{"invalid start", "", "start_timestamp=bad&end_timestamp=2000", false, 0},
		{"missing end", "", "start_timestamp=1000", false, 0},
		{"overflow end", "", "start_timestamp=1000&end_timestamp=9223372036854775808", false, 0},
		{"negative start", "", "start_timestamp=-1000&end_timestamp=2000", false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("id", 10)
			if tt.adminID != "" {
				ctx.Params = gin.Params{{Key: "id", Value: tt.adminID}}
			}
			ctx.Request = httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil)
			GetReferralModelConsumeReport(ctx)
			var payload struct {
				Success bool                              `json:"success"`
				Data    *model.ReferralModelConsumeReport `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			require.Equal(t, tt.success, payload.Success, recorder.Body.String())
			if tt.success {
				require.NotNil(t, payload.Data)
				assert.Equal(t, tt.quota, payload.Data.ConsumeQuota)
			} else {
				assert.Nil(t, payload.Data)
			}
		})
	}
}
