package router

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"rustdesk-server/api/config"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

func TestAdminLogoutRevokesSuppliedToken(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	defer func(db *sql.DB) { _ = db.Close() }(sqlDB)
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}); err != nil {
		t.Fatalf("migrate auth models: %v", err)
	}

	previousDB, previousServices, previousConfig := service.DB, service.AllService, service.Config
	service.DB = db
	service.Config = &config.Config{App: config.App{TokenExpire: time.Hour}}
	service.AllService = &service.Service{UserService: &service.UserService{}}
	t.Cleanup(func() {
		service.DB = previousDB
		service.AllService = previousServices
		service.Config = previousConfig
	})

	user := &model.User{Username: "logout-user", Status: model.COMMON_STATUS_ENABLE}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	const token = "admin-session-token"
	if err := db.Create(&model.UserToken{
		UserId:    user.Id,
		Token:     token,
		ExpiredAt: time.Now().Add(time.Hour).Unix(),
	}).Error; err != nil {
		t.Fatalf("create user token: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	LoginBind(router.Group("/api/admin"))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
	request.Header.Set("api-token", token)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}

	var remaining int64
	if err := db.Model(&model.UserToken{}).Where("token = ?", token).Count(&remaining).Error; err != nil {
		t.Fatalf("count revoked token: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("logout left supplied token stored: %d row(s)", remaining)
	}
}
