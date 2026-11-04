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
	ReTime time.Time `json:"te_time"` // 下次重试时间
}
