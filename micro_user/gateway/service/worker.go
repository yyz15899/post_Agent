package service

import (
	"context"
	"log"
	"micro_user/gateway/dao"
	"time"

	milvusIndexer "github.com/cloudwego/eino-ext/components/indexer/milvus2"
)

// 进行循环入库操作
func RunPostSyncWorker(ctx context.Context, indexer *milvusIndexer.Indexer) {
	tricker := time.NewTicker(5 * time.Second) // 5s尝试同步一次
	defer tricker.Stop()
	for {
		select {
		case <-ctx.Done(): // 上下文取消(接收到前文 stopWorker()信号)
			return
		case <-tricker.C: // 到达任务执行时刻
			// 查询未入库task,逐条处理
			Tasks, err := dao.ListenDuePostSyncTack(ctx, 10)
			if err != nil {
				log.Printf("查询同步任务失败：%v", err)
				continue
			}
			for _, task := range Tasks {
				// startedAT := time.Now()
				log.Printf("同步开始:task_id:%v,post_id:%v", task.TaskID, task.PostID)
				err := ProcessPostSyncTask(ctx, indexer, task)
				// 要等到同步失败且重试时间成功更新后，任务要等到 re_time 到期(30s)才会执行,不然不满足listendue...的db的where条件
				// log.Printf("同步结束：task_id=%d post_id=%d 耗时=%s err=%v",
				// task.TaskID, task.PostID, time.Since(startedAT), err)
				if err != nil {
					log.Printf("任务 %d 同步失败：%v", task.TaskID, err)
					continue
				}
			}
		}
	}
}

// 同步点赞任务
func RunPostVoteSyncWorker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 查询需要同步的post(每轮50条数据)
			tasks, err := dao.ListenPostVoteTask(ctx, 200)
			if err != nil {
				log.Printf("获取同步任务信息失败:%v", err)
				continue
			}
			for _, task := range tasks {
				// 同步redis
				err = ProcessPostVoteSyncTask(ctx, task)
				if err != nil {
					log.Printf("任务 %d 同步失败：%v", task.TaskID, err)
					continue
				}
			}
		}
	}
}
