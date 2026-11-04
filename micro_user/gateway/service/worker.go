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
	tricker := time.NewTicker(5 * time.Second)
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
				err := ProcessPostSyncTask(ctx, indexer, task)
				if err != nil {
					log.Printf("任务 %d 同步失败：%v", task.TaskID, err)
					continue
				}
			}
		}
	}
}
