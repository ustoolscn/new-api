package controller

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

var (
	hiCodexUpdateURL    = "https://files.cooper-api.com/hi-codex/update.json"
	hiCodexUpdateClient = &http.Client{Timeout: 10 * time.Second}
)

func GetHiCodexUpdate(c *gin.Context) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, hiCodexUpdateURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create update request"})
		return
	}

	resp, err := hiCodexUpdateClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch Hi Codex update metadata"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("upstream returned status %d", resp.StatusCode)})
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read Hi Codex update metadata"})
		return
	}

	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "invalid Hi Codex update metadata"})
		return
	}

	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}
