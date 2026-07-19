package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTaskProxyContentURLAcceptsCanonicalAndCompatibilityRoutes(t *testing.T) {
	const taskID = "task_example"

	assert.True(t, isTaskProxyContentURL("https://example.com/v1/video/generations/"+taskID+"/content", taskID))
	assert.True(t, isTaskProxyContentURL("https://example.com/v1/videos/"+taskID+"/content", taskID))
	assert.False(t, isTaskProxyContentURL("https://example.com/video.mp4", taskID))
}
