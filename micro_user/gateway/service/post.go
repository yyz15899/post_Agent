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
	"go.opentelemetry.io/otel"
	"gorm.io/gorm"
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
	//	设置子ctx 用于检测每条数据的入库超时
	attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second) // 检测第一条数据卡住,其他数据能否入库
	//
	// 先进行一次重新入库
	// syncerr := ReindexPost(ctx, indexer, task.PostID)
	syncerr := ReindexPost(attemptCtx, indexer, task.PostID)
	cancel()
	if syncerr != nil {
		nextTime := time.Now().Add(30 * time.Second) // 此次同步失败,则间隔30s重试
		retryerr := dao.SchedulePostSyncTaskRetry(ctx, task.TaskID, nextTime)
		if retryerr != nil {
			log.Printf("更新数据失败,err:%v", retryerr)
			return errors.Join(syncerr, retryerr) // 两个err都返回
		}
		return syncerr
	}

	return dao.MarkPostSyncTaskDone(ctx, task.TaskID)
}

// 只用记录上次(本次)用户的投票方向,也就是当前显示的实际票数
func VotePostByID(ctx context.Context, uid int64, pid int64, vote int) error {
	// 保存票数(本次用户的投票方向)到DB
	//
	tracer := otel.Tracer("gateway_vote")
	lookupCtx, lookupSpan := tracer.Start(ctx, "dao_vote.post_vote")
	err := dao.UpSertPostVote(lookupCtx, int64(vote), pid, uid)
	lookupSpan.End()
	if err != nil {
		return err
	}
	return nil

}

// 再次尝试计入redis
func ProcessPostVoteSyncTask(ctx context.Context, task model.PostVoteSyncTask) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	//读取本次投票方向
	UserPostVote, syncerr := dao.GetUserVote(ctx, task.UserID, task.PostID)
	if syncerr == nil {
		// 查询vote数值成功
		syncerr = dao.Vote(ctx, task.UserID, task.PostID, UserPostVote.Direction)
	}
	if syncerr != nil {
		if ctx.Err() != nil { // 服务正在关闭时，保留未完成任务，重启后继续处理。
			return syncerr
		}
		// 失败则记录
		nexttime := time.Now().Add(30 * time.Second)
		retryerr := dao.SchedulePostVoteSyncTaskRetry(ctx, task, nexttime)
		if retryerr != nil {
			return errors.Join(syncerr, retryerr) // 两个err都返回
		}
		return syncerr // 本次同步失败，已安排后续重试
	}
	// redis同步成功
	return dao.MarkPostVoteSyncTaskDone(ctx, task)
}

func GetScoreByID(ctx context.Context, postid int64) (float64, error) {
	sc, err := dao.GetScoreByID(ctx, postid)
	if err != nil {
		return sc, err
	}
	return sc, nil
}

// 查询当前票数
func GetScoreNow(ctx context.Context, userid int64, postid int64) (int, error) {
	vote, err := dao.GetUserVote(ctx, userid, postid)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return vote.Direction, nil
}
