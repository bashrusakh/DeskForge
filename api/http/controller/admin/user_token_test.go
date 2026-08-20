package admin

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

func TestUserTokenListRedactsRawTokenSecret(t *testing.T) {
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
	if err := db.AutoMigrate(&model.UserToken{}); err != nil {
		t.Fatalf("migrate user token: %v", err)
	}
	previousDB, previousServices := service.DB, service.AllService
	service.DB = db
	service.AllService = &service.Service{UserService: &service.UserService{}}
	t.Cleanup(func() {
		service.DB = previousDB
		service.AllService = previousServices
	})

	secret := "0123456789abcdef-secret-token"
	if err := db.Create(&model.UserToken{UserId: 8, Token: secret, ExpiredAt: 99}).Error; err != nil {
		t.Fatalf("create user token: %v", err)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/user_token/list?page=1&page_size=10", nil)
	(&UserToken{}).List(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("user token list status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if strings.Contains(body, secret) {
		t.Fatalf("user token list exposed raw secret: %s", body)
	}
	for _, expected := range []string{`"token":"0123****oken"`, `"user_id":8`, `"expired_at":99`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("user token list omitted %q: %s", expected, body)
		}
	}
}
