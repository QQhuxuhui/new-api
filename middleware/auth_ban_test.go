package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAuthBanTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
	})

	dsn := fmt.Sprintf("file:auth_ban_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}
	return db
}

func authBanTestRouter(user *model.User, auth gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Use(sessions.Sessions("auth-ban-test", cookie.NewStore([]byte("auth-ban-secret"))))
	router.GET("/seed", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", user.Id)
		session.Set("username", user.Username)
		session.Set("role", user.Role)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", user.Group)
		_ = session.Save()
		c.Status(http.StatusNoContent)
	})
	router.GET("/protected", auth, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	return router
}

func TestAuth_RejectsBannedUserWithStaleEnabledSession(t *testing.T) {
	db := setupAuthBanTest(t)
	tests := []struct {
		name string
		role int
		auth gin.HandlerFunc
	}{
		{name: "user", role: common.RoleCommonUser, auth: UserAuth()},
		{name: "admin", role: common.RoleAdminUser, auth: AdminAuth()},
	}

	for index, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			user := &model.User{
				Username: fmt.Sprintf("banned-session-%d", index), Password: "12345678",
				Role: testCase.role, Status: common.UserStatusBanned, AffCode: fmt.Sprintf("ban-session-%d", index),
			}
			if err := db.Create(user).Error; err != nil {
				t.Fatalf("create user: %v", err)
			}
			router := authBanTestRouter(user, testCase.auth)

			seed := httptest.NewRecorder()
			router.ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/seed", nil))
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			for _, value := range seed.Result().Header.Values("Set-Cookie") {
				request.Header.Add("Cookie", strings.SplitN(value, ";", 2)[0])
			}
			request.Header.Set("New-Api-User", strconv.Itoa(user.Id))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":false`) {
				t.Fatalf("banned stale session passed: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAuth_RejectsBannedUserAccessToken(t *testing.T) {
	db := setupAuthBanTest(t)
	accessToken := "banned-access-token"
	user := &model.User{
		Username: "banned-access", Password: "12345678", Role: common.RoleCommonUser,
		Status: common.UserStatusBanned, AccessToken: &accessToken, AffCode: "ban-access",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	router := authBanTestRouter(user, UserAuth())
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", strconv.Itoa(user.Id))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":false`) {
		t.Fatalf("banned access token passed: status=%d body=%s", response.Code, response.Body.String())
	}
}
