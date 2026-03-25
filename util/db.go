package util

import (
	"fmt"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db   *gorm.DB
	once sync.Once
)

func InitDB() error {
	var initErr error
	once.Do(func() {
		host := viper.GetString("db.host")
		port := viper.GetInt("db.port")
		user := viper.GetString("db.user")
		password := viper.GetString("db.password")
		database := viper.GetString("db.database")
		sslmode := viper.GetString("db.sslmode")
		timezone := viper.GetString("db.timezone")

		if sslmode == "" {
			sslmode = "disable"
		}
		if timezone == "" {
			timezone = "Asia/Shanghai"
		}

		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
			host, user, password, database, port, sslmode, timezone,
		)

		var err error
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn),
		})
		if err != nil {
			initErr = fmt.Errorf("open db failed: %w", err)
			return
		}

		sqlDB, err := db.DB()
		if err != nil {
			initErr = fmt.Errorf("get sql db failed: %w", err)
			return
		}

		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(time.Hour)
	})

	return initErr
}

func GetDB() *gorm.DB {
	return db
}
