package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeFrontendPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "topup", input: "/console/topup", want: "/wallet"},
		{name: "topup query", input: "/console/topup?pay=success", want: "/wallet?pay=success"},
		{name: "topup fragment", input: "/console/topup#history", want: "/wallet#history"},
		{name: "topup nested path", input: "/console/topup/history", want: "/wallet/history"},
		{name: "log", input: "/console/log", want: "/usage-logs"},
		{name: "personal", input: "/console/personal", want: "/profile"},
		{name: "prefix collision", input: "/console/topupper", want: "/console/topupper"},
		{name: "log prefix collision", input: "/console/logs", want: "/console/logs"},
		{name: "personal prefix collision", input: "/console/personality", want: "/console/personality"},
		{name: "unmapped", input: "/console/other", want: "/console/other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeFrontendPath(tt.input))
		})
	}
}
