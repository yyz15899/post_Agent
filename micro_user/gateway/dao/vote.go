package dao

import (
	"context"
	"errors"
	"log"
	"micro_user/gateway/model"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const keyPostVotedZSetPre = "micro_user:post:voted" // 可以根据pid查到对应value 即每个用户投票"数值"
const keyVoteNumZSet = "micro_user:post:vote_num"

// 投票
func Vote(ctx context.Context, userid int64, postid int64, direction int) error {
	uid := strconv.FormatInt(userid, 10)
	pid := strconv.FormatInt(postid, 10)
	keys := []string{
		keyPostVotedZSetPre + pid, // KEYS[1]
		keyVoteNumZSet,            //KEYS[2]
	}
	// 脚本内部计算执行ZADD等操作	同步了目前实际得分
	delta, err := voteScript.Run(
		ctx,
		RDB,
		keys,
		uid,
		pid,
		direction,
	).Int64()
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
			DoUpdates: clause.AssignmentColumns([]string{"direction"}),
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
