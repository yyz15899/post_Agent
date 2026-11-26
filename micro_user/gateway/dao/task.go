package dao

import (
	"context"
	"errors"
	"log"
	"micro_user/gateway/model"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 监视(返回)所有没有入库成功的task列表
func ListenDuePostSyncTack(ctx context.Context, limit int) ([]model.PostSyncTask, error) {
	var tasks []model.PostSyncTask
	err := DB.WithContext(ctx).
		Order("re_time ASC").
		Where("status = ? AND re_time <= ?", false, time.Now()).
		Limit(limit).
		Find(&tasks).Error // 存入PostSyncTask
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// 重新尝试入库(成功之后更新)
func SchedulePostSyncTaskRetry(ctx context.Context, taskid int, nexttime time.Time) error {
	// 拿到对应task
	err := DB.WithContext(ctx).
		Model(&model.PostSyncTask{}).
		Where("task_id = ?", taskid).
		Updates(map[string]any{
			"re_num":  gorm.Expr("re_num + ?", 1),
			"re_time": nexttime,
		}).Error
	if err != nil {
		return err
	}
	return nil

}

// 可以思考一下为什么这两个函数不建议合并

// 将完成入库的记录(更新同步)
func MarkPostSyncTaskDone(ctx context.Context, taskid int) error {
	err := DB.WithContext(ctx).
		Model(&model.PostSyncTask{}).
		Where("task_id = ?", taskid).
		Update("status", true).Error
	if err != nil {
		return err
	}
	return nil
}

// ------------------------------------

// 同步redis
func ListenPostVoteTask(ctx context.Context, limit int, lease time.Duration, claimtoken string) ([]model.PostVoteSyncTask, error) {

	var tasks []model.PostVoteSyncTask
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 查找未完成同步的task
		err := tx.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}). // 其他worker轮询时跳过该行(Find操作)
			Order("re_time ASC").
			Where("status = ? AND re_time < ? ", 0, time.Now()).
			Limit(limit).
			Find(&tasks).Error
		if err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}
		tids := make([]int, len(tasks))

		// 计算租约时间
		nexttime := time.Now().Add(lease)

		// 将上面查到的task回填进结构体中 因为这些数据在auth建表时没有或采用默认值
		for i, task := range tasks {
			tids[i] = task.TaskID
			tasks[i].ClaimToken = claimtoken
			tasks[i].ReTime = nexttime
		}

		// 更新re_time
		result := tx.WithContext(ctx).
			Model(&model.PostVoteSyncTask{}).
			Where("task_id IN ?", tids).                                            // 匹配所有符合的task
			Updates(map[string]any{"re_time": nexttime, "claim_token": claimtoken}) // 将token更新为当前租约者的标识
		if result.Error != nil {
			log.Printf("更新任务 re_time 失败, tids=%v, err=%v", tids, result.Error)
			return result.Error
		}
		if result.RowsAffected != int64(len(tids)) {
			log.Printf("检索数据可能不完整")
			return errors.New("服务繁忙")
		}

		return nil
	})
	if err != nil {
		log.Printf("事务执行失败:%v", err)
		return nil, err
	}

	return tasks, nil
}

// 更新状态
func MarkPostVoteSyncTaskDone(ctx context.Context, task model.PostVoteSyncTask) error {
	result := DB.WithContext(ctx).
		Model(&model.PostVoteSyncTask{}).
		Where("task_id = ? AND claim_token = ? AND status = ? AND re_time > ?", task.TaskID, task.ClaimToken, false, time.Now()).
		Update("status", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		log.Printf("检索数据可能不完整")
		return errors.New("服务繁忙")
	}
	return nil
}

// 安排重试
func SchedulePostVoteSyncTaskRetry(ctx context.Context, task model.PostVoteSyncTask, nexttime time.Time) error {
	// 拿到对应task
	result := DB.WithContext(ctx).
		Model(&model.PostVoteSyncTask{}).
		Where("task_id = ? AND claim_token = ? AND status = ? AND re_time > ?", task.TaskID, task.ClaimToken, false, time.Now()).
		Updates(map[string]any{
			"re_num":      gorm.Expr("re_num + ?", 1),
			"re_time":     nexttime,
			"claim_token": "",
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		log.Printf("检索数据可能不完整")
		return errors.New("服务繁忙")
	}

	return nil

}

// 查询pending积压数据(自动查看积压数据)
func CountPendingPostVoteSyncTasks(ctx context.Context) (int64, error) {
	var pending int64

	err := DB.WithContext(ctx).
		Model(&model.PostVoteSyncTask{}).
		Where("status = ?", false).
		Count(&pending).Error
	if err != nil {
		return 0, err
	}

	return pending, nil
}
