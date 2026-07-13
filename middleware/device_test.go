package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDeviceBanCheckBlocksBannedFingerprint(t *testing.T) {
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserDevice{}))
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })

	const fingerprint = "blocked-browser-device"
	require.NoError(t, db.Create(&model.UserDevice{
		UserId:          1,
		FingerprintHash: model.HashDeviceFingerprint(fingerprint),
		Banned:          true,
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(DeviceBanCheck())
	router.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set(model.DeviceFingerprintHeader, fingerprint)
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestDeviceBanCheckAllowsUnbannedFingerprint(t *testing.T) {
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserDevice{}))
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(DeviceBanCheck())
	router.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set(model.DeviceFingerprintHeader, "allowed-browser-device")
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}
