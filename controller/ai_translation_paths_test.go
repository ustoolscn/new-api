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

func TestStatusTranslationPathsTranslateChatNamesAndPreserveStructuredFields(t *testing.T) {
	payload := gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"chats": []gin.H{
				{
					"name": "新客户端",
					"url":  "https://example.com/named",
					"icon": "OpenAI.Color",
				},
			},
		},
	}
	translations := map[string]string{
		"新客户端": "Named client",
		"name": "Nom",
		"url":  "URL",
		"icon": "Icône",
	}

	translated := service.ApplyAITranslations(payload, statusTranslationPaths, translations)
	root, ok := translated.(map[string]any)
	if !ok {
		t.Fatalf("translated payload type = %T, want map[string]any", translated)
	}
	data, ok := root["data"].(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map[string]any", root["data"])
	}
	chats, ok := data["chats"].([]any)
	if !ok || len(chats) != 1 {
		t.Fatalf("chats = %#v, want one chat entry", data["chats"])
	}

	named, ok := chats[0].(map[string]any)
	if !ok {
		t.Fatalf("named chat type = %T, want map[string]any", chats[0])
	}
	if _, ok := named["Nom"]; ok {
		t.Fatalf("structured field key should not be translated: %#v", named)
	}
	if named["name"] != "Named client" {
		t.Fatalf("named chat name = %#v, want Named client", named["name"])
	}
	if named["url"] != "https://example.com/named" {
		t.Fatalf("named chat url = %#v, want original URL", named["url"])
	}
	if named["icon"] != "OpenAI.Color" {
		t.Fatalf("named chat icon = %#v, want original icon", named["icon"])
	}
}
