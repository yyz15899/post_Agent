package dao

import (
	"context"
	"crypto/rand"
	"errors"
	"log"
	"micro_user/gateway/model"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const keyPostVotedZSetPre = "micro_user:post:voted" // 可以根据pid查到对应value 即每个用户投票"数值"
const keyVoteNumZSet = "micro_user:post:vote_num"
const keyVoteVersion = "micro_user:post:vote_version"

// 投票
func Vote(ctx context.Context, userid int64, postid int64, direction int, version int64) error {

	uid := strconv.FormatInt(userid, 10)
	pid := strconv.FormatInt(postid, 10)
	keys := []string{
		keyPostVotedZSetPre + pid, // KEYS[1]
		keyVoteNumZSet,            //KEYS[2]
		keyVoteVersion + pid,      // kEYS[3]	检查当前操作的执行版本
	}
	// 脚本内部计算执行ZADD等操作	同步了目前实际得分
	args := []any{uid, pid, direction, version}
	tricer := otel.Tracer("name_gateway")
	runCtx, runspan := tricer.Start(ctx, "执行lua写入redis操作")
	delta, err := voteScript.Run(runCtx, RDB, keys, args...).Int64()
	if err != nil {
		runspan.RecordError(err)
		runspan.SetStatus(codes.Error, "执行lua脚本失败")
	}
	runspan.End()
	if err != nil && redis.HasErrorPrefix(err, voteScoreCacheMiss) { // 第一次投票发现redis中没有数据,开始尝试原子写入redis(恢复快照)
		restoreCtx, restorespan := tricer.Start(ctx, "执行恢复快照操作")
		restoreerr := restorePostVoteSnapshot(restoreCtx, postid, pid, keys)
		if restoreerr != nil {
			restorespan.RecordError(restoreerr)
			restorespan.SetStatus(codes.Error, "恢复快照失败")
		}
		restorespan.End()
		if restoreerr != nil {
			return restoreerr
		}
		delta, err = voteScript.Run(ctx, RDB, keys, args...).Int64()
	}
	if err != nil {
		log.Printf("执行投票脚本失败:%v", err)
		return err
	}
	log.Printf("投票完成:user_id=%d,post_id=%d,direction=%d,delta=%d",
		userid, postid, direction, delta)
	return nil
}

// 检测该用户投票方向 1赞 -1踩 0没投过
func CheckLike(ctx context.Context, postid int64, userid int64) (int64, error) {
	like, err := RDB.ZScore(ctx, keyPostVotedZSetPre+strconv.FormatInt(postid, 10), strconv.FormatInt(userid, 10)).Result()
	if errors.Is(err, redis.Nil) { // 没投过(like=0)
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return int64(like), nil
}

// 获取用户最新的投票数值
func GetUserVote(ctx context.Context, userid int64, postid int64) (*model.PostVote, error) {
	var UserPost *model.PostVote
	err := DB.WithContext(ctx).
		Where("user_id = ? AND post_id = ?", userid, postid).
		Take(&UserPost).Error
	if err != nil {
		return nil, err
	}
	return UserPost, nil
}

// 接收用户的direction,计入DB
func UpSertPostVote(ctx context.Context, direction int64, postid int64, userid int64) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		vote := model.PostVote{
			PostID:    postid,
			UserID:    userid,
			Direction: int(direction),
		}
		// 将用户对该post投票数值计入mysql(用Clauses可以确保没有数据时创建,有数据时直接更新)
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "post_id"},
				{Name: "user_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"direction": vote.Direction,
				"version":   gorm.Expr("version + ?", 1),
			}),
			// 表示以 post_id + user_id 作为唯一投票组合。例如用户第一次为帖子点赞时插入；
			// 再次修改投票方向时(已经存在)，只更新 direction和version，不会改动其他字段。
		}).Create(&vote).Error
		if err != nil {
			log.Printf("记录用户投票方向失败:%v", err)
			return err
		}
		task := model.PostVoteSyncTask{
			PostID: postid,
			UserID: userid,
			Status: false,
			ReTime: time.Now(),
		}
		return tx.Create(&task).Error
	})

}

// 查询post总票数
func GetScoreByID(ctx context.Context, postid int64) (float64, bool, error) {
	like, err := RDB.ZScore(ctx, keyVoteNumZSet, strconv.FormatInt(postid, 10)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		log.Printf("获取票数失败,:%v", err)
		return 0, false, err
	}
	return like, true, nil // 返回true表示当前需要查询分数的post_score存在
}

// 重新从DB中查询用户看到的post的总分值
func SumPostVoteScore(ctx context.Context, postid int64) (float64, error) {
	var score float64
	err := DB.WithContext(ctx).
		Model(&model.PostVote{}).
		Select("COALESCE(SUM(direction),0)").
		Where("post_id = ?", postid).
		Scan(&score).Error // 扫描写入score
	if err != nil {
		return 0, err
	}
	return score, nil

}

// 恢复redis中投票数据 记录快照(根据pid从DB中查出所有在redis中没有数据的postvote)
func LoadPostVoteSnapshot(ctx context.Context, postid int64) ([]model.PostVote, float64, error) {
	var Votes []model.PostVote
	err := DB.WithContext(ctx).
		Select("user_id", "direction", "version").
		Where("post_id = ?", postid).
		Find(&Votes).Error
	if err != nil {
		return nil, 0, err
	}
	// 计算post总分
	var Scores float64
	for _, vote := range Votes {
		Scores += float64(vote.Direction)
	}

	return Votes, Scores, nil
}

// 尝试进行锁操作(防止并发抢恢复redis的任务)
const keyPostVoteRestoreLockPre = "micro_user:post:vote_restore_lock:"

// 恢复快照
func restorePostVoteSnapshot(ctx context.Context, postid int64, pid string, keys []string) error {
	tracer := otel.Tracer("name_gateway")
	lockctx, lockspan := tracer.Start(ctx, "执行获取锁操作")
	token, locked, err := TryLockPostVoteRestore(lockctx, postid)
	if err == nil && !locked {
		err = errors.New("上锁失败")
	}
	if err != nil {
		lockspan.RecordError(err)
		lockspan.SetStatus(codes.Error, "获取锁失败")
	}
	lockspan.End()
	if err != nil {
		return err
	}

	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		unlockCtx, unlockspan := tracer.Start(unlockCtx, "执行释放锁操作")
		if _, err := UnLockPostVoteRestore(unlockCtx, postid, token); err != nil {
			unlockspan.RecordError(err)
			unlockspan.SetStatus(codes.Error, "释放锁失败")
			log.Printf("释放投票缓存恢复锁失败: %v", err)
		}
		unlockspan.End()
	}()
	loadctx, loadspan := tracer.Start(ctx, "获取votepost的数据")
	votes, _, err := LoadPostVoteSnapshot(loadctx, postid) // 空快照是正常现象
	if err != nil {
		loadspan.RecordError(err)
		loadspan.SetStatus(codes.Error, "获取votepost数据失败")
	}
	loadspan.End()
	if err != nil {
		return err
	}
	args := make([]any, 0, 2+3*len(votes))
	args = append(args, token, pid)
	for _, vote := range votes {
		args = append(args, vote.UserID, vote.Direction, vote.Version)
	}
	restoreKeys := append(keys, keyPostVoteRestoreLockPre+pid)
	restore_runCtx, restore_runspan := tracer.Start(ctx, "执行lua脚本原子写入(恢复)操作")
	result, err := restorePostVoteSnapshotScript.Run(restore_runCtx, RDB, restoreKeys, args...).Int64()
	if err == nil && result == 0 { // 没有报错但琐失效
		err = errors.New("锁失效")
	}
	if err != nil {
		restore_runspan.RecordError(err)
		restore_runspan.SetStatus(codes.Error, "执行lua恢复redis失败")
	}
	restore_runspan.End()
	if err != nil {
		return err
	}
	return nil
}

// 尝试对redis恢复操作上锁,防止多个请求抢任务
func TryLockPostVoteRestore(ctx context.Context, postid int64) (string, bool, error) {
	lockkey := keyPostVoteRestoreLockPre + strconv.FormatInt(postid, 10)
	token := rand.Text()
	// 给每个需要操作的post写入token
	locked, err := RDB.SetNX(ctx, lockkey, token, 30*time.Second).Result()
	if err != nil {
		return "", false, err
	}
	if !locked {
		return "", false, nil
	}
	return token, locked, nil
}

// 每个redis恢复执行完成  进行解锁
func UnLockPostVoteRestore(ctx context.Context, postid int64, token string) (bool, error) {
	lockkey := keyPostVoteRestoreLockPre + strconv.FormatInt(postid, 10)
	deleted, err := unlockPostVoteRestoreScript.Run(ctx, RDB, []string{lockkey}, token).Int64()
	return deleted == 1, err
	// lua里DEL操作返回的是实际被删除的key的数量
}

// 原子恢复redis数据
func RestorePostVoteCache(ctx context.Context, postid int64) error {
	pid := strconv.FormatInt(postid, 10)
	keys := []string{
		keyPostVotedZSetPre + pid,
		keyVoteNumZSet,
		keyVoteVersion + pid,
	}
	return restorePostVoteSnapshot(ctx, postid, pid, keys)
}
