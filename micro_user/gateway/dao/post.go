package dao

import (
	"context"
	"errors"
	"log"
	"micro_user/gateway/model"
	"time"

	"gorm.io/gorm"
)

var ErrPostExist = errors.New("帖子已存在")
var ErrPostNotExist = errors.New("帖子不存在")

// v2:post同步加入milvus库中
func CreatePost(c context.Context, id int64, Title, Content string) (int64, error) {
	P := &model.Post{
		Author_id: id,
		Title:     Title,
		Content:   Content,
	}
	// 执行事务
	err := DB.WithContext(c).Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&model.Post{}).Create(P).Error // 将P内容写到Post
		if err != nil {
			return err
		}
		// 利用P.id创建postsynctask
		task := &model.PostSyncTask{
			PostID: P.Id,
			Status: false,
			ReNum:  0,
			ReTime: time.Now(), // 立即尝试同步
		}
		// 保存任务
		err = tx.Model(model.PostSyncTask{}).Create(task).Error
		if err != nil {
			return err
		}
		return nil
	})
	// if errors.Is(err, gorm.ErrDuplicatedKey) {
	// 	return 0, ErrPostExist
	// }	// post使用自增主键,这里先不管重复
	if err != nil {
		log.Printf("事务执行失败:%v", err)
		return 0, err
	}
	return P.Id, nil
}

func GetPostByID(c context.Context, postid int64) (*model.Post, error) {
	var post model.Post
	err := DB.WithContext(c).Where("id = ?", postid).First(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPostNotExist
	}
	if err != nil {
		return nil, err
	}
	return &post, nil
}

func SearchPosts(ctx context.Context, keyword string) ([]model.Post, error) {
	var posts []model.Post
	pattern := "%" + keyword + "%"
	err := DB.WithContext(ctx).
		Where("title Like ? OR content LIKE ?", pattern, pattern).
		Order("id DESC").
		Limit(5).Find(&posts).Error
	if err != nil {
		return nil, err
	}
	return posts, nil
}

// 准备入库(milvus)数据
func ListPost(ctx context.Context, limit int) ([]model.Post, error) {
	var post []model.Post
	err := DB.WithContext(ctx).Order("id ASC").Limit(limit).Find(&post).Error
	if err != nil {
		return nil, err
	}
	return post, nil
}
