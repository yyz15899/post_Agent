package dao

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestVoteScriptRepeat(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("请设置 TEST_REDIS_ADDR，指向本地测试 Redis")
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("TEST_REDIS_PASSWORD"),
	})
	t.Cleanup(func() { // 类似于defer(等待当前测试及子测试结束后执行)
		client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("连接测试 Redis 失败：%v", err)
	}

	prefix := "test:vote:" + rand.Text()
	keys := []string{
		prefix + ":users",
		prefix + ":scores",
		prefix + ":version",
	}
	lockKey := prefix + ":restore_lock"
	restoreKeys := append(keys, lockKey)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()

		if err := client.Del(cleanupCtx, restoreKeys...).Err(); err != nil {
			t.Errorf("清理测试数据失败：%v", err)
		}

	})

	if err := client.ZAdd(ctx, keys[1], redis.Z{
		Member: "1", // 测试帖子 ID
		Score:  0,
	}).Err(); err != nil {
		t.Fatalf("初始化测试帖子总分失败：%v", err)
	}

	// 重复点赞
	//		两个"1"分别是uid,pid  后面是direction
	delta, err := voteScript.Run(ctx, client, keys, "1", "1", 1, 1).Int64()
	if err != nil {
		t.Fatalf("首次点赞失败：%v", err)
	}
	if delta != 1 {
		t.Fatalf("首次点赞应增加 1 分，实际增加 %d", delta)
	}

	delta, err = voteScript.Run(ctx, client, keys, "1", "1", 1, 1).Int64()
	if err != nil {
		t.Fatalf("重复点赞失败：%v", err)
	}
	if delta != 0 {
		t.Fatalf("重复点赞应增加 0 分，实际增加 %d", delta)
	}

	//
	// 点赞改点踩

	delta, err = voteScript.Run(ctx, client, keys, "1", "1", -1, 2).Int64()
	if err != nil {
		t.Fatalf("点赞后点踩失败:%v", err)
	}
	if delta != -2 {
		t.Fatalf("赞改踩的差量应为 -2,实际变化 %d", delta)
	}

	// 每个用户自己的投票数值
	direction, err := client.ZScore(ctx, keys[0], "1").Result()
	if err != nil {
		t.Fatalf("读取用户方向失败：%v", err)
	}
	if direction != -1 {
		t.Fatalf("用户方向应为 -1，实际为 %v", direction)
	}

	// 帖子收到的总分数(所有用户的)
	score, err := client.ZScore(ctx, keys[1], "1").Result()
	if err != nil {
		t.Fatalf("读取帖子得分失败：%v", err)
	}
	// if score != 2 {
	// 	t.Fatalf("重复点赞后得分应为 2，实际为 %v", score)
	// }
	if score != -1 {
		t.Fatalf("点赞后点踩得分应为 -1，实际为 %v", score)
	}

	// 重复点踩
	delta, err = voteScript.Run(ctx, client, keys, "1", "1", -1, 2).Int64()
	if err != nil {
		t.Fatalf("重复点踩失败：%v", err)
	}
	if delta != 0 {
		t.Fatalf("重复点踩的差量应为 0，实际为 %d", delta)
	}

	// 后续又又两个写入版本3 4分别到达
	// 新版本4先到
	delta, err = voteScript.Run(ctx, client, keys, "1", "1", -1, 4).Int64()
	if err != nil {
		t.Fatalf("delta:%d,同步失败：%v", delta, err)
	}
	storedVersion, err := client.HGet(ctx, keys[2], "1").Result()
	if err != nil || storedVersion != "4" { // 且
		t.Fatalf("版本应推进到 4：version=%q err=%v", storedVersion, err)
	}

	// 旧版本3到达,操作应被无视
	delta, err = voteScript.Run(ctx, client, keys, "1", "1", 1, 3).Int64()
	if err != nil {
		t.Fatalf("旧版本应被跳过：delta=%d err=%v", delta, err)
	}

	direction, err = client.ZScore(ctx, keys[0], "1").Result()
	if err != nil || direction != -1 {
		t.Fatalf("旧版本到达后方向应为 -1：direction=%v err=%v", direction, err)
	}

	score, err = client.ZScore(ctx, keys[1], "1").Result()
	if err != nil || score != -1 {
		t.Fatalf("旧版本到达后总分应为 -1：score=%v err=%v", score, err)
	}

	storedVersion, err = client.HGet(ctx, keys[2], "1").Result()
	if err != nil || storedVersion != "4" {
		t.Fatalf("旧版本到达后版本应为 4：version=%q err=%v", storedVersion, err)
	}

	// 模拟总分丢失，保留旧方向和旧版本。
	removed, err := client.ZRem(ctx, keys[1], "1").Result()
	if err != nil || removed != 1 {
		t.Fatalf("删除测试总分失败：removed=%d err=%v", removed, err)
	}

	const token = "test-restore-token"
	if err := client.Set(ctx, lockKey, token, 30*time.Second).Err(); err != nil {
		t.Fatalf("设置测试恢复锁失败：%v", err)
	}

	// 参数依次是 token、帖子 ID、用户 ID、方向、版本。
	result, err := restorePostVoteSnapshotScript.Run(
		ctx, client, restoreKeys, token, "1", "1", 1, 5,
	).Int64()
	if err != nil || result != 1 {
		t.Fatalf("恢复快照失败：result=%d err=%v", result, err)
	}

	direction, err = client.ZScore(ctx, keys[0], "1").Result()
	if err != nil || direction != 1 {
		t.Fatalf("恢复后方向应为 1：direction=%v err=%v", direction, err)
	}

	score, err = client.ZScore(ctx, keys[1], "1").Result()
	if err != nil || score != 1 {
		t.Fatalf("恢复后总分应为 1：score=%v err=%v", score, err)
	}

	storedVersion, err = client.HGet(ctx, keys[2], "1").Result()
	if err != nil || storedVersion != "5" {
		t.Fatalf("恢复后版本应为 5：version=%q err=%v", storedVersion, err)
	}

}
