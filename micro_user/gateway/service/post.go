package service

import (
	"context"
	"errors"
	"log"
	"micro_user/gateway/dao"
	"micro_user/gateway/model"
	"strings"
	"time"

	milvusIndexer "github.com/cloudwego/eino-ext/components/indexer/milvus2"
)

var KeywordEmpty = errors.New("关键字不能为空")
var MessageEmpty = errors.New("信息不能为空")

func CreatePost(ctx context.Context, authorid int64, title, content string) (int64, error) {
	// 将userid写到创建的post authorid上
	postid, err := dao.CreatePost(ctx, authorid, title, content)
	if err != nil {
		return 0, err
	}
	return postid, nil
}

func GetPostByID(ctx context.Context, postid int64) (*model.Post, error) {
	post, err := dao.GetPostByID(ctx, postid)
	if err != nil {
		return nil, err
	}
	return post, nil
}

func SearchPosts(ctx context.Context, keyword string) ([]model.Post, error) {
	// 处理关键词
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, KeywordEmpty
	}
	posts, err := dao.SearchPosts(ctx, keyword)
	if err != nil {
		return nil, err
	}
	return posts, nil
}

// 对没有入库成功的进行再次入库
func ProcessPostSyncTask(ctx context.Context, indexer *milvusIndexer.Indexer, task model.PostSyncTask) error {
	// 先进行一次重新入库
	syncerr := ReindexPost(ctx, indexer, task.PostID)
	if syncerr != nil {
		nextTime := time.Now().Add(30 * time.Second) // 间隔30s重试
		retryerr := dao.SchedulePostSyncTaskRetry(ctx, task.TaskID, nextTime)
		if retryerr != nil {
			log.Printf("更新数据失败,err:%v", retryerr)
			return errors.Join(syncerr, retryerr) // 两个err都返回
		} else {
			return syncerr
		}
	}
	return dao.MarkPostSyncTaskDone(ctx, task.TaskID)
}
