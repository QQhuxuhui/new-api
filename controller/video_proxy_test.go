package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type videoProxyRoundTripperFunc func(*http.Request) (*http.Response, error)

func (function videoProxyRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func setupVideoProxyTest(t *testing.T, channel *model.Channel, task *model.Task, trustedTransport http.RoundTripper) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:video_proxy_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := database.AutoMigrate(&model.Channel{}, &model.Task{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	previousDB := model.DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousFetchSetting := *system_setting.GetFetchSetting()
	model.DB = database
	common.MemoryCacheEnabled = false
	*system_setting.GetFetchSetting() = system_setting.FetchSetting{
		EnableSSRFProtection: true,
		AllowPrivateIp:       false,
		DomainFilterMode:     false,
		IpFilterMode:         false,
	}
	trustedClient := service.GetHttpClient()
	previousTrustedClient := *trustedClient
	trustedClient.Transport = trustedTransport
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		*system_setting.GetFetchSetting() = previousFetchSetting
		*trustedClient = previousTrustedClient
	})

	if err := database.Create(channel).Error; err != nil {
		t.Fatalf("insert test channel: %v", err)
	}
	task.ChannelId = channel.Id
	if err := database.Create(task).Error; err != nil {
		t.Fatalf("insert test task: %v", err)
	}

	router := gin.New()
	router.GET("/v1/videos/:task_id/content", VideoProxy)
	return router
}

func TestVideoProxyTrustsPrivateAdministratorChannelURLAndRedirects(t *testing.T) {
	trustedRequests := 0
	trustedTransport := videoProxyRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		trustedRequests++
		switch request.URL.Path {
		case "/v1/videos/upstream-task/content":
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"http://127.0.0.1/video"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    request,
			}, nil
		case "/video":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("video-data")),
				Request:    request,
			}, nil
		default:
			return nil, errors.New("unexpected trusted request URL: " + request.URL.String())
		}
	})

	baseURL := "http://127.0.0.1"
	router := setupVideoProxyTest(t, &model.Channel{
		Type:    constant.ChannelTypeSora,
		Name:    "private trusted channel",
		Key:     "secret",
		BaseURL: &baseURL,
	}, &model.Task{
		TaskID: "upstream-task",
		Status: model.TaskStatusSuccess,
	}, trustedTransport)

	request := httptest.NewRequest(http.MethodGet, "/v1/videos/upstream-task/content", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected trusted channel video fetch to succeed, got status %d: %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "video-data" {
		t.Fatalf("unexpected video body %q", response.Body.String())
	}
	if trustedRequests != 2 {
		t.Fatalf("expected trusted client to follow redirect, got %d requests", trustedRequests)
	}
}

func TestVideoProxyProtectsPrivateUpstreamReturnedURLs(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		configure   func(*model.Task)
	}{
		{
			name:        "Ali",
			channelType: constant.ChannelTypeAli,
			configure: func(task *model.Task) {
				task.FailReason = "http://127.0.0.1/video"
			},
		},
		{
			name:        "Gemini",
			channelType: constant.ChannelTypeGemini,
			configure: func(task *model.Task) {
				task.PrivateData.Key = "gemini-key"
				task.SetData(map[string]any{"uri": "http://127.0.0.1/video"})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trustedTransport := videoProxyRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				t.Fatalf("untrusted upstream-returned URL used trusted client: %s", request.URL)
				return nil, errors.New("untrusted URL used trusted client")
			})
			task := &model.Task{
				TaskID: "upstream-task-" + test.name,
				Status: model.TaskStatusSuccess,
			}
			test.configure(task)
			router := setupVideoProxyTest(t, &model.Channel{
				Type: test.channelType,
				Name: test.name + " channel",
				Key:  "secret",
			}, task, trustedTransport)

			request := httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID+"/content", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("expected private upstream-returned URL to be blocked, got status %d: %s", response.Code, response.Body.String())
			}
		})
	}
}
