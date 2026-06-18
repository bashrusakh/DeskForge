package orm

import (
	"fmt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"time"
)

type SqliteConfig struct {
	MaxIdleConns int
	MaxOpenConns int
}

func NewSqlite(sqliteConf *SqliteConfig, logwriter logger.Writer) *gorm.DB {
	// _txlock=immediate: begin every transaction with BEGIN IMMEDIATE so writers
	// serialize at BEGIN — closes the last-admin delete race (see getAdminUserCountTx),
	// since SQLite ignores the FOR UPDATE row hint.
	// _busy_timeout=5000: wait up to 5s for the write lock instead of erroring SQLITE_BUSY.
	db, err := gorm.Open(sqlite.Open("./data/rustdeskapi.db?_txlock=immediate&_busy_timeout=5000"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: logger.New(
			logwriter, // io writer
			logger.Config{
				SlowThreshold:             time.Second, // Slow SQL threshold
				LogLevel:                  logger.Warn, // Log level
				IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
				ParameterizedQueries:      true,        // Don't include params in the SQL log
				Colorful:                  true,
			},
		),
	})
	if err != nil {
		fmt.Println(err)
	}
	sqlDB, err2 := db.DB()
	if err2 != nil {
		fmt.Println(err2)
	}
	// SetMaxIdleConns 
	sqlDB.SetMaxIdleConns(sqliteConf.MaxIdleConns)

	// SetMaxOpenConns 。
	sqlDB.SetMaxOpenConns(sqliteConf.MaxOpenConns)

	return db
}
