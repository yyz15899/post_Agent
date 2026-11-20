package service

import (
	"context"
	"micro_user/gateway/dao"
)

func CountPendingPostVoteSyncTasks(c context.Context) (int64, error) {
	pending_num, err := dao.CountPendingPostVoteSyncTasks(c)
	if err != nil {
		return 0, err
	}
	return pending_num, nil

}
