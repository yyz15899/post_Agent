package model

import "time"

// 记录 user给post投票数值
type PostVote struct {
	ID        int64 `gorm:"primaryKey;autoIncrement"`
	PostID    int64 `gorm:"not null;index:uk_post_user,unique,priority:1"`
	UserID    int64 `gorm:"not null;index:uk_post_user,unique,priority:2"`
	Direction int   `gorm:"type:tinyint;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int64 `gorm:"not null;default:1"`
}
