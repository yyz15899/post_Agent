package service

import (
	"context"
	"crypto/rand"
	"log"
	"micro_user/gateway/dao"
	"sync"
	"time"

	milvusIndexer "github.com/cloudwego/eino-ext/components/indexer/milvus2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	const workercount = 4
	var wg sync.WaitGroup

	for id := 1; id <= workercount; id++ {
		wg.Add(1)

		go func(workerID int) {
			defer wg.Done()
			runPostVoteSyncWorker(ctx, workerID)
		}(id)
	}
	wg.Wait()
}

// 执行同步任务
func runPostVoteSyncWorker(ctx context.Context, workerid int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 生成专属token
			claimtoken := rand.Text()
			// 每次领取5条任务
			tracer := otel.Tracer("name_gateway")
			findCtx, span := tracer.Start(ctx, "检索未完成任务")
			tasks, err := dao.ListenPostVoteTask(findCtx, 5, 30*time.Second, claimtoken)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "查找失败")
			}
			span.End()
			if err != nil || len(tasks) == 0 {
				if err != nil {
					log.Printf("worker=%d 领取任务失败: %v", workerid, err) // 继续进行
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second): // 1s到达会结束这个select(等待1s,在尝试领取(后续的continue))
				}
				continue
			}

			// 执行同步
			taskids := make([]int, len(tasks))
			for i, task := range tasks {
				taskids[i] = task.TaskID

				// 处理前检查err和租约
				if ctx.Err() != nil {
					return
				}
				if !time.Now().Before(task.ReTime) { // 当前时间已经达到或超过租约截止时间
					log.Printf("超过了租约时间")
					break
				}
				// 同步开始
				// 用span计时
				tracer := otel.Tracer("name_gateway")
				syncctx, span := tracer.Start(ctx, "同步任务") // 创建父span
				span.SetAttributes(                        // 方便统计
					attribute.Int("workerid", workerid),
					attribute.Int("task.id", task.TaskID),
					attribute.Int64("post.id", task.PostID),
				)
				syncerr := ProcessPostVoteSyncTask(syncctx, task)
				if syncerr != nil {
					span.RecordError(syncerr)
					span.SetStatus(codes.Error, "同步任务失败")
				}
				span.End()

				if syncerr != nil {
					log.Printf("worker=%d task=%d 同步失败: %v",
						workerid, task.TaskID, syncerr)
					continue // 继续本批下一条任务
				}
			}
			log.Printf("worker=%d claim_token=%s task_ids=%v",
				workerid, claimtoken, taskids)

		}
	}
}
