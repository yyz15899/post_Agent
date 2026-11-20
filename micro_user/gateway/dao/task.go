package dao

import (
	"context"
	"micro_user/gateway/model"
	"time"

	"gorm.io/gorm"
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
func ListenPostVoteTask(ctx context.Context, limit int) ([]model.PostVoteSyncTask, error) {
	var tasks []model.PostVoteSyncTask
	err := DB.WithContext(ctx).
		Order("re_time ASC").
		Where("status = ? AND re_time < ?", 0, time.Now()).
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// 更新状态
func MarkPostVoteSyncTaskDone(ctx context.Context, taskid int64) error {
	err := DB.WithContext(ctx).
		Model(&model.PostVoteSyncTask{}).
		Where("task_id = ?", taskid).
		Update("status", true).Error
	if err != nil {
		return err
	}
	return nil
}

// 安排重试
func SchedulePostVoteSyncTaskRetry(ctx context.Context, taskid int, nexttime time.Time) error {
	// 拿到对应task
	err := DB.WithContext(ctx).
		Model(&model.PostVoteSyncTask{}).
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
