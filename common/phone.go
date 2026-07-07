package common

import (
	"errors"
	"regexp"
	"strings"
)

var mainlandPhonePattern = regexp.MustCompile(`^1[3-9]\d{9}$`)

func NormalizeMainlandPhone(phone string) (string, error) {
	normalized := strings.TrimSpace(phone)
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.TrimPrefix(normalized, "+86")
	normalized = strings.TrimPrefix(normalized, "86")
	if !mainlandPhonePattern.MatchString(normalized) {
		return "", errors.New("invalid mainland phone number")
	}
	return normalized, nil
}
