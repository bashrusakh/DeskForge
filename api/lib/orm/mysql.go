package orm

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"time"
)

type MysqlConfig struct {
	Dsn          string
	MaxIdleConns int
	MaxOpenConns int
}

func NewMysql(mysqlConf *MysqlConfig, logwriter logger.Writer) *gorm.DB {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:               mysqlConf.Dsn, // DSN data source name
		DefaultStringSize: 256,           // string 
		//DisableDatetimePrecision:  true,                    //  datetime ，MySQL 5.6 
		//DontSupportRenameIndex:    true,                    // ，MySQL 5.7  MariaDB 
		//DontSupportRenameColumn:   true,                    //  `change` ，MySQL 8  MariaDB 
		//SkipInitializeWithVersion: false,                   //  MySQL 
	}), &gorm.Config{
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
	sqlDB.SetMaxIdleConns(mysqlConf.MaxIdleConns)

	// SetMaxOpenConns 。
	sqlDB.SetMaxOpenConns(mysqlConf.MaxOpenConns)

	return db
}
