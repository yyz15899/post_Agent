package model

import "time"

type Post struct {
	Id        int64 `gorm:"primaryKey"`
	Author_id int64
	Title     string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreatePostRequest struct {
	Title   string `json:"title" binding:"required,max=200"`
	Content string `json:"content" binding:"required"`
}

// 监视milvus同步是否有问题
type PostSyncTask struct {
	TaskID int       `gorm:"primaryKey;autoIncrement"`
	PostID int64     `json:"post_id"`
	Status bool      `gorm:"default:false" json:"status"`
	ReNum  int       `json:"renum"`   // 重试次数
	ReTime time.Time `json:"re_time"` // 下次重试时间
}

// 监视redis同步是否有问题
type PostVoteSyncTask struct {
	TaskID     int       `gorm:"primaryKey;autoIncrement"`
	PostID     int64     `json:"post_id"`
	UserID     int64     `json:"user_id"`
	Status     bool      `gorm:"default:false;index:idx_vote_sync_status_time,priority:1" json:"status"`
	ReNum      int       `json:"renum"`                                                     // 重试次数
	ReTime     time.Time `gorm:"index:idx_vote_sync_status_time,priority:2" json:"re_time"` // 下次重试时间
	ClaimToken string    `gorm:"size:64;not null;default:''" json:"-"`                      // 标识租约者是否还有权力进行dao操作
}

type Direction struct {
	Direction int `gorm:"direction"`
}
