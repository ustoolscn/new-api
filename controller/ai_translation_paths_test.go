package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func TestUserGroupsTranslationPathsTranslateDisplayKeysAndPreserveRawKeyField(t *testing.T) {
	payload := gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"default": gin.H{
				"key":   "default",
				"ratio": 1,
				"desc":  "Default group source",
			},
		},
	}
	translations := map[string]string{
		"default":              "Translated default key",
		"Default group source": "Default group",
	}

	translated := service.ApplyAITranslations(payload, userGroupsTranslationPaths, translations)
	root, ok := translated.(map[string]any)
	if !ok {
		t.Fatalf("translated payload type = %T, want map[string]any", translated)
	}
	data, ok := root["data"].(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map[string]any", root["data"])
	}
	if _, ok := data["default"]; ok {
		t.Fatalf("raw display key should be translated: %#v", data)
	}
	group, ok := data["Translated default key"].(map[string]any)
	if !ok {
		t.Fatalf("translated display key missing: %#v", data)
	}
	if group["key"] != "default" {
		t.Fatalf("key = %#v, want default", group["key"])
	}
	if group["desc"] != "Default group" {
		t.Fatalf("desc = %#v, want Default group", group["desc"])
	}
}
