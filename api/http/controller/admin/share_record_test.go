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

func TestAdminShareRecordListUsesSafeResponse(t *testing.T) {
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
	if err := db.AutoMigrate(&model.ShareRecord{}); err != nil {
		t.Fatalf("migrate share record: %v", err)
	}
	if err := db.Create(&model.ShareRecord{
		UserId:       8,
		PeerId:       "admin-peer",
		ShareToken:   "admin-share-token",
		PasswordType: "fixed",
		Password:     "admin-password",
		Expire:       1800,
	}).Error; err != nil {
		t.Fatalf("create share record: %v", err)
	}

	previousDB, previousServices := service.DB, service.AllService
	service.DB = db
	service.AllService = &service.Service{ShareRecordService: &service.ShareRecordService{}}
	t.Cleanup(func() {
		service.DB = previousDB
		service.AllService = previousServices
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/share_record/list?page=1&page_size=10", nil)
	(&ShareRecord{}).List(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("admin share record list status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{"admin-share-token", "admin-password", `"share_token"`, `"password_type"`, `"password"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("admin share record response exposed %q: %s", forbidden, body)
		}
	}
	for _, expected := range []string{`"user_id":8`, `"peer_id":"admin-peer"`, `"expire":1800`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("admin share record response omitted %q: %s", expected, body)
		}
	}
}
