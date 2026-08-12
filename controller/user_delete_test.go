package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestDeleteUserReturnsSuccessAfterHardDelete(t *testing.T) {
	db := setupManageUserBanTest(t)
	user := &model.User{
		Username: "hard-delete-user",
		Password: "12345678",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		AffCode:  "hard-delete-aff",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("role", common.RoleAdminUser)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(user.Id)}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/user/1", nil)

	DeleteUser(c)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"success":true`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDeleteUserReturnsFailureWhenHardDeleteFails(t *testing.T) {
	db := setupManageUserBanTest(t)
	user := &model.User{
		Username: "hard-delete-failure",
		Password: "12345678",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		AffCode:  "hard-delete-failure-aff",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("test:fail_hard_delete", func(tx *gorm.DB) {
		tx.AddError(errors.New("forced delete failure"))
	}); err != nil {
		t.Fatalf("register delete failure callback: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("role", common.RoleAdminUser)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(user.Id)}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/user/1", nil)

	DeleteUser(c)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"success":false`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
