package orm

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"time"
)

type PostgresqlConfig struct {
	Dsn          string
	MaxIdleConns int
	MaxOpenConns int
}

func NewPostgresql(conf *PostgresqlConfig, logwriter logger.Writer) *gorm.DB {
	db, err := gorm.Open(postgres.Open(conf.Dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: logger.New(
			logwriter, // io writer
			logger.Config{
				SlowThreshold: time.Second, // Slow SQL threshold
				LogLevel:      logger.Warn, // Log level
				//IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
				ParameterizedQueries: true, // Don't include params in the SQL log
				Colorful:             true,
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
	sqlDB.SetMaxIdleConns(conf.MaxIdleConns)

	// SetMaxOpenConns 。
	sqlDB.SetMaxOpenConns(conf.MaxOpenConns)

	return db
}
