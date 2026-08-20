package service

import (
	"database/sql"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"rustdesk-server/api/model"
)

func TestUserServiceLogout(t *testing.T) {
	tests := []struct {
		name      string
		closeDB   bool
		wantError bool
	}{
		{name: "revokes token", wantError: false},
		{name: "returns database error", closeDB: true, wantError: true},
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

			previousDB, previousServices := DB, AllService
			DB = db
			AllService = &Service{UserService: &UserService{}}
			t.Cleanup(func() {
				DB = previousDB
				AllService = previousServices
			})

			const token = "logout-service-token"
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

			err = AllService.UserService.Logout(user, token)
			if (err != nil) != tt.wantError {
				t.Fatalf("Logout() error = %v, want error: %t", err, tt.wantError)
			}
			if tt.wantError {
				return
			}

			var remaining int64
			if err := db.Model(&model.UserToken{}).Where("token = ?", token).Count(&remaining).Error; err != nil {
				t.Fatalf("count revoked token: %v", err)
			}
			if remaining != 0 {
				t.Fatalf("Logout() left token stored: %d row(s)", remaining)
			}
		})
	}
}
