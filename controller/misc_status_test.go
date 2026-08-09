package controller

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildStatusResponseReportsDefaultFrontend(t *testing.T) {
	response := buildStatusResponse()
	data, ok := response["data"].(gin.H)
	require.True(t, ok)
	assert.Equal(t, "default", data["theme"])
}
