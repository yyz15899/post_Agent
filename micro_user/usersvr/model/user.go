package model

import "time"

type User struct {
	Id        int64 `gorm:"primaryKey"`
	Password  string
	UserName  string `gorm:"uniqueIndex"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
