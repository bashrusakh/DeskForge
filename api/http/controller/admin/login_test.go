package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

func TestLoginLogout(t *testing.T) {
	tests := []struct {
		name       string
		closeDB    bool
		wantStatus int
		wantCode   int
	}{
		{name: "success", wantStatus: http.StatusOK, wantCode: 0},
		{name: "revocation failure", closeDB: true, wantStatus: http.StatusInternalServerError, wantCode: 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			const token = "logout-controller-token"
			user := &model.User{IdModel: model.IdModel{Id: 1}}
			if err := db.Create(&model.UserToken{
				UserId:    user.Id,
				Token:     token,
				ExpiredAt: time.Now().Add(time.Hour).Unix(),
			}).Error; err != nil {
				t.Fatalf("create user token: %v", err)
			}

			if tt.closeDB {
				if err := sqlDB.Close(); err != nil {
					t.Fatalf("close sqlite: %v", err)
				}
			}

			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
			context.Set("curUser", user)
			context.Set("token", token)
			(&Login{}).Logout(context)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("logout status = %d, want %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			var payload struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode logout response: %v", err)
			}
			if payload.Code != tt.wantCode {
				t.Fatalf("logout response code = %d, want %d", payload.Code, tt.wantCode)
			}
			if tt.closeDB {
				body := recorder.Body.String()
				if strings.Contains(body, token) || strings.Contains(body, "database is closed") {
					t.Fatalf("logout failure exposed secret or database error: %s", body)
				}
				if !strings.Contains(body, "logout failed") {
					t.Fatalf("logout failure omitted safe message: %s", body)
				}
			}
		})
	}
}
