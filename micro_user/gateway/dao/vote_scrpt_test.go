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
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()

		if err := client.Del(cleanupCtx, keys...).Err(); err != nil {
			t.Errorf("清理测试数据失败：%v", err)
		}

	})

	// 重复点赞
	//		两个"1"分别是uid,pid  后面是direction
	delta, err := voteScript.Run(ctx, client, keys, "1", "1", 1).Int64()
	if err != nil {
		t.Fatalf("首次点赞失败：%v", err)
	}
	if delta != 1 {
		t.Fatalf("首次点赞应增加 1 分，实际增加 %d", delta)
	}

	delta, err = voteScript.Run(ctx, client, keys, "1", "1", 1).Int64()
	if err != nil {
		t.Fatalf("重复点赞失败：%v", err)
	}
	if delta != 0 {
		t.Fatalf("重复点赞应增加 0 分，实际增加 %d", delta)
	}

	//
	// 点赞改点踩

	delta, err = voteScript.Run(ctx, client, keys, "1", "1", -1).Int64()
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
	delta, err = voteScript.Run(ctx, client, keys, "1", "1", -1).Int64()
	if err != nil {
		t.Fatalf("重复点踩失败：%v", err)
	}
	if delta != 0 {
		t.Fatalf("重复点踩的差量应为 0，实际为 %d", delta)
	}

}
