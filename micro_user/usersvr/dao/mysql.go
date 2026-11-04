package dao

import (
	"errors"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() error {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		return errors.New("缺少环境变量 mysql_dsn")
	}
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		return err
	}
	return nil
}
