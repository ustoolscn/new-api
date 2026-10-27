package sd99

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requestContext(t *testing.T, body string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("test_recorder", w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: Model},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName: Model,
	}
	return c, info
}

func TestSD99RequestPreservesProtocolAndPositiveBillingSeconds(t *testing.T) {
	for _, base := range []string{"https://api.sd99.vip", "https://api.sd99.vip/v1/"} {
		t.Run(base, func(t *testing.T) {
			c, info := requestContext(t, `{"model":"local-alias","prompt":"a calm sea","duration":5,"metadata":{"ratio":"16:9","resolution":"720p","generate_audio":false,"content":[{"type":"image_url","role":"reference_image","image_url":{"url":"https://media.example.com/a.jpg?sig=a%2Bb"}}]}}`)
			a := &TaskAdaptor{baseURL: base}
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			endpoint, err := a.BuildRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://api.sd99.vip/v1/videos", endpoint)
			reader, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			assert.JSONEq(t, `{"model":"video-sd-2.0(TC)","prompt":"a calm sea","duration":5,"metadata":{"ratio":"16:9","resolution":"720p","generate_audio":false,"content":[{"type":"image_url","role":"reference_image","image_url":{"url":"https://media.example.com/a.jpg?sig=a%2Bb"}}]}}`, string(body))
			req, err := relaycommon.GetTaskRequest(c)
			require.NoError(t, err)
			assert.Equal(t, 5.0, req.OutputSeconds())
			assert.Equal(t, "720p", req.Size)
			assert.True(t, req.HasAnyInputContent())
		})
	}
}

func TestSD99UnifiedMediaAndDefaults(t *testing.T) {
	c, info := requestContext(t, `{"model":"video-sd-2.0(TC)","prompt":"move","seconds":"6","size":"1280x720","generate_audio":false,"images":["https://media.example.com/a.jpg"],"input_videos":["https://media.example.com/a.mp4"],"input_audios":["https://media.example.com/a.mp3"]}`)
	a := &TaskAdaptor{}
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"video-sd-2.0(TC)","prompt":"move","duration":6,"metadata":{"ratio":"16:9","resolution":"720p","generate_audio":false,"content":[{"type":"image_url","role":"reference_image","image_url":{"url":"https://media.example.com/a.jpg"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://media.example.com/a.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://media.example.com/a.mp3"}}]}}`, string(body))
	req, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://media.example.com/a.mp4"}, req.InputVideoURLs())
}

func TestSD99RejectsInvalidParametersBeforeSubmission(t *testing.T) {
	cases := []string{
		`"duration":0`, `"duration":4`, `"duration":16`, `"duration":5.5`,
		`"duration":18446744073686646784`, `"duration":-2`,
		`"duration":-1,"metadata":{"ratio":"adaptive"}`,
		`"duration":5,"metadata":{"resolution":"4k"}`,
		`"duration":5,"metadata":{"ratio":"adaptive"}`,
		`"duration":5,"metadata":{"generate_audio":"false"}`,
		`"duration":5,"metadata":{"content":[{"type":"image_url","role":"first_frame","image_url":{"url":"https://media.example.com/a.jpg"}}]}`,
		`"duration":5,"metadata":{"content":[{"type":"image_url","role":"reference_image","image_url":{"url":"file:///a.jpg"}}]}`,
		`"duration":5,"metadata":{"content":[{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://media.example.com/a.mp3"}}]}`,
	}
	for _, fields := range cases {
		t.Run(fields, func(t *testing.T) {
			c, info := requestContext(t, `{"model":"video-sd-2.0(TC)","prompt":"move",`+fields+`}`)
			a := &TaskAdaptor{}
			err := a.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, err)
			assert.Equal(t, http.StatusBadRequest, err.StatusCode)
			assert.False(t, c.GetBool(SubmissionAttempted))
		})
	}
}

func TestSD99ReferenceLimits(t *testing.T) {
	for _, tc := range []struct {
		kind, role string
		count      int
	}{
		{"image_url", "reference_image", 10}, {"video_url", "reference_video", 4}, {"audio_url", "reference_audio", 4},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			items := []string{}
			for i := 0; i < tc.count; i++ {
				items = append(items, fmt.Sprintf(`{"type":%q,"role":%q,%q:{"url":"https://media.example.com/%d"}}`, tc.kind, tc.role, tc.kind, i))
			}
			c, info := requestContext(t, `{"model":"video-sd-2.0(TC)","prompt":"move","duration":5,"metadata":{"content":[`+strings.Join(items, ",")+`]}}`)
			require.NotNil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
		})
	}
}

func TestSD99EditUsesDetectedDurationForBillingOnly(t *testing.T) {
	for _, duration := range []float64{4, 8.2, 15, 3.9, 15.1, math.NaN(), math.Inf(1)} {
		t.Run(fmt.Sprint(duration), func(t *testing.T) {
			c, info := requestContext(t, `{"model":"video-sd-2.0(TC)","prompt":"edit","duration":-1,"input_video_seconds":999,"metadata":{"ratio":"adaptive","resolution":"720p","duration":999,"fps":120,"content":[{"type":"video_url","role":"reference_video","video_url":{"url":"https://media.example.com/source.mp4"}}]}}`)
			a := &TaskAdaptor{probeDuration: func(_ context.Context, source string) (float64, error) {
				assert.Equal(t, "https://media.example.com/source.mp4", source)
				return duration, nil
			}}
			err := a.ValidateRequestAndSetAction(c, info)
			if duration < 4 || duration > 15 || math.IsNaN(duration) || math.IsInf(duration, 0) {
				require.NotNil(t, err)
				return
			}
			require.Nil(t, err)
			req, getErr := relaycommon.GetTaskRequest(c)
			require.NoError(t, getErr)
			assert.Equal(t, math.Ceil(duration), req.OutputSeconds())
			assert.Nil(t, req.InputVideoSeconds)
			assert.Nil(t, req.FPS)
			assert.NotContains(t, req.Metadata, "duration")
			assert.NotContains(t, req.Metadata, "fps")
			reader, buildErr := a.BuildRequestBody(c, info)
			require.NoError(t, buildErr)
			body, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			assert.Contains(t, string(body), `"duration":-1`)
		})
	}
}

func TestSD99SubmissionTaskIDsAndBusinessErrors(t *testing.T) {
	for _, tc := range []struct{ body, id string }{
		{`{"id":"first","task_id":"second"}`, "first"},
		{`{"task_id":"second"}`, "second"},
		{`{"job_id":"third"}`, "third"},
		{`{"id":"../bad"}`, ""},
		{`{"id":"valid","error":{"message":"rejected"}}`, ""},
		{`{"id":"valid","code":"fail_to_fetch_task"}`, ""},
		{`<html>error</html>`, ""},
	} {
		t.Run(tc.body, func(t *testing.T) {
			c, info := requestContext(t, `{}`)
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(tc.body))}
			id, _, err := (&TaskAdaptor{}).DoResponse(c, resp, info)
			assert.Equal(t, tc.id, id)
			if tc.id == "" {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
				writer, _ := c.Get("test_recorder")
				assert.Contains(t, writer.(*httptest.ResponseRecorder).Body.String(), "task_public")
			}
		})
	}
}

func TestSD99PollingStatusAliasesAndQueryFailures(t *testing.T) {
	for _, tc := range []struct{ body, status string }{
		{`{"status":" PENDING_REVIEW "}`, model.TaskStatusQueued},
		{`{"status":"running"}`, model.TaskStatusInProgress},
		{`{"status":"completed"}`, model.TaskStatusSuccess},
		{`{"status":"succeeded","video_url":"https://media.example.com/out.mp4"}`, model.TaskStatusSuccess},
		{`{"status":"success"}`, model.TaskStatusSuccess},
		{`{"status":"expired"}`, model.TaskStatusFailure},
		{`{"status":"refunded"}`, model.TaskStatusFailure},
		{`{"status":"failed","error":{"message":"moderation"}}`, model.TaskStatusFailure},
		{`{"status":"failed","error":{"code":"fail_to_fetch_task"}}`, ""},
		{`{"status":"failed","message":"fail_to_fetch_task"}`, ""},
		{`{"status":"new_status"}`, ""},
		{`{"error":{"message":"temporary failure"}}`, ""},
	} {
		t.Run(tc.body, func(t *testing.T) {
			result, err := (&TaskAdaptor{baseURL: "https://api.sd99.vip"}).ParseTaskResult([]byte(tc.body))
			if tc.status == "" {
				require.Error(t, err)
				assert.Nil(t, result)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.status, result.Status)
		})
	}
}

func TestSD99DownloadSelectsOutputsAndScopesCredentials(t *testing.T) {
	for _, tc := range []struct {
		body, endpoint string
		auth           bool
	}{
		{`{}`, "https://api.sd99.vip/v1/videos/id/content", true},
		{`{"content_url":"http://localhost:3000/v1/videos/id/content"}`, "https://api.sd99.vip/v1/videos/id/content", true},
		{`{"video_url":"http://127.0.0.1:3000/v1/videos/id/content"}`, "https://api.sd99.vip/v1/videos/id/content", true},
		{`{"video_url":"http://[::1]:3000/v1/videos/id/content"}`, "https://api.sd99.vip/v1/videos/id/content", true},
		{`{"video_url":"http://localhost:3000/v1/videos/other/content"}`, "http://localhost:3000/v1/videos/other/content", false},
		{`{"video_url":"http://localhost:3000/out.mp4"}`, "http://localhost:3000/out.mp4", false},
		{`{"video_url":"http://localhost.example.com:3000/v1/videos/id/content"}`, "http://localhost.example.com:3000/v1/videos/id/content", false},
		{`{"content_url":"/v1/videos/id/content?sig=a%2Bb"}`, "https://api.sd99.vip/v1/videos/id/content?sig=a%2Bb", true},
		{`{"video_url":"/generated/a.mp4?sig=a%2Bb"}`, "https://api.sd99.vip/generated/a.mp4?sig=a%2Bb", false},
		{`{"video_url":"https://cdn.example.com/out.mp4"}`, "https://cdn.example.com/out.mp4", false},
		{`{"video_url":"https://other.example.com/v1/videos/id/content"}`, "https://other.example.com/v1/videos/id/content", false},
		{`{"data":[{"type":"image","url":"https://cdn.example.com/cover.jpg"},{"type":"video","role":"reference_video","url":"https://cdn.example.com/in.mp4"}],"output":{"assets":[{"type":"output_video","url":"https://cdn.example.com/out.mp4"}]}}`, "https://cdn.example.com/out.mp4", false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			endpoint, auth, err := DownloadURL("https://api.sd99.vip/v1", "id", []byte(tc.body))
			require.NoError(t, err)
			assert.Equal(t, tc.endpoint, endpoint)
			assert.Equal(t, tc.auth, auth)
		})
	}
	_, _, err := DownloadURL("https://api.sd99.vip", "id", []byte(`{"video_url":"file:///etc/passwd"}`))
	require.Error(t, err)
}

func TestSD99SubmissionDoesNotFollowRedirect(t *testing.T) {
	var targetCalls int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++; w.WriteHeader(200) }))
	t.Cleanup(target.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)
	c, info := requestContext(t, `{}`)
	a := &TaskAdaptor{baseURL: server.URL, apiKey: "secret"}
	resp, err := a.DoRequest(c, info, strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	assert.Zero(t, targetCalls)
	assert.True(t, c.GetBool(SubmissionAttempted))
}

func TestSD99HTTPQueryFailureDoesNotBecomeGenerationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/videos/id", r.URL.Path)
		assert.Equal(t, "Bearer task-key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"failed","message":"temporary failure"}`))
	}))
	t.Cleanup(server.Close)
	_, err := (&TaskAdaptor{}).FetchTask(server.URL+"/v1", "task-key", map[string]any{"task_id": "id"}, "")
	require.Error(t, err)
}
