package model

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskStatusToVideoStatus(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   string
	}{
		{name: "not started is queued", status: TaskStatusNotStart, want: dto.VideoStatusQueued},
		{name: "submitted is queued", status: TaskStatusSubmitted, want: dto.VideoStatusQueued},
		{name: "queued is queued", status: TaskStatusQueued, want: dto.VideoStatusQueued},
		{name: "in progress", status: TaskStatusInProgress, want: dto.VideoStatusInProgress},
		{name: "success", status: TaskStatusSuccess, want: dto.VideoStatusCompleted},
		{name: "failure", status: TaskStatusFailure, want: dto.VideoStatusFailed},
		{name: "unknown enum value", status: TaskStatusUnknown, want: dto.VideoStatusUnknown},
		{name: "unrecognized value", status: TaskStatus("unexpected"), want: dto.VideoStatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotEmpty(t, tt.want)
			assert.Equal(t, tt.want, tt.status.ToVideoStatus())
		})
	}
}
