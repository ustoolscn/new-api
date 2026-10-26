package controller

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/sd99"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSD99SubmissionErrorsNeverTriggerAutomaticRetry(t *testing.T) {
	for _, status := range []int{307, 429, 500, 502} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(sd99.SubmissionAttempted, true)
		assert.False(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: status}, 3))
	}
}

func TestSD99VideoDownloadUsesSavedKeyAndStripsItOnRedirect(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	oldMemory := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	fetch := system_setting.GetFetchSetting()
	oldFetch := *fetch
	fetch.EnableSSRFProtection = false
	service.InitHttpClient()
	t.Cleanup(func() { common.MemoryCacheEnabled = oldMemory; *fetch = oldFetch })

	// Minimal MP4 ftyp signature, enough to verify actual media bytes rather
	// than trusting an upstream Content-Type header on an HTML/JSON error.
	video := make([]byte, 24)
	binary.BigEndian.PutUint32(video[:4], 24)
	copy(video[4:8], "ftyp")
	copy(video[8:12], "isom")
	copy(video[16:], "isomiso2")
	var cdnAuth, providerAuth string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/error" {
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte(`{"error":"not a video"}`))
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(video)
	}))
	t.Cleanup(cdn.Close)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerAuth = r.Header.Get("Authorization")
		assert.Equal(t, "/v1/videos/upstream/content", r.URL.Path)
		http.Redirect(w, r, cdn.URL+"/output", http.StatusFound)
	}))
	t.Cleanup(provider.Close)
	baseURL := provider.URL + "/v1"
	ch := model.Channel{Id: 1001, Type: constant.ChannelTypeSD99, Key: "rotated-channel-key", BaseURL: &baseURL, Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&ch).Error)
	task := model.Task{
		TaskID: "task_public", UserId: 7, ChannelId: ch.Id, Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "upstream", Key: "original-task-key"},
		Data:        []byte(`{"status":"completed"}`),
	}
	require.NoError(t, db.Create(&task).Error)

	for _, tc := range []struct {
		name, body   string
		user, status int
	}{
		{"content fallback", `{"status":"completed"}`, 7, 200},
		{"direct media", `{"status":"completed","video_url":"` + cdn.URL + `/output"}`, 7, 200},
		{"JSON disguised as video", `{"status":"completed","video_url":"` + cdn.URL + `/error"}`, 7, 502},
		{"different owner", `{"status":"completed"}`, 8, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			providerAuth, cdnAuth = "", ""
			require.NoError(t, db.Model(&model.Task{}).Where("id = ?", task.ID).Update("data", []byte(tc.body)).Error)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_public/content", nil)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			c.Set("id", tc.user)
			VideoProxy(c)
			assert.Equal(t, tc.status, w.Code)
			assert.Empty(t, cdnAuth)
			if tc.name == "content fallback" {
				assert.Equal(t, "Bearer original-task-key", providerAuth)
			} else {
				assert.Empty(t, providerAuth)
			}
			if tc.status == http.StatusOK {
				assert.Equal(t, video, w.Body.Bytes())
			}
		})
	}
}

func TestSD99TimeoutDoesNotTurnPendingTaskIntoFailure(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	for _, task := range []model.Task{
		{TaskID: "sd99_waiting", Platform: "59", Status: model.TaskStatusInProgress, Progress: "30%", SubmitTime: 10},
		{TaskID: "other_waiting", Platform: "54", Status: model.TaskStatusInProgress, Progress: "30%", SubmitTime: 10},
	} {
		require.NoError(t, db.Create(&task).Error)
	}
	tasks := model.GetTimedOutUnfinishedTasks(20, 100)
	require.Len(t, tasks, 1)
	assert.Equal(t, "other_waiting", tasks[0].TaskID)
	var sd99Task model.Task
	require.NoError(t, db.Where("task_id = ?", "sd99_waiting").First(&sd99Task).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), sd99Task.Status)
}
