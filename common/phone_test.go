package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeMainlandPhone(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "plain mainland number",
			input: "13800138000",
			want:  "13800138000",
		},
		{
			name:  "number with country code",
			input: "+86 13800138000",
			want:  "13800138000",
		},
		{
			name:    "invalid prefix",
			input:   "12800138000",
			wantErr: true,
		},
		{
			name:    "invalid length",
			input:   "1380013800",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeMainlandPhone(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
