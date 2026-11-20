package dao

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client

func InitRdb() error {
	ctx := context.Background()
	if err := godotenv.Load(".env"); err != nil {
		log.Fatalf("加载配置失败：%v", err)
	}
	RDB = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("redis.Addr"),
		Password: os.Getenv("redis.pwd"),
	})
	_, err := RDB.Ping(ctx).Result() //检测连接
	if err != nil {
		return fmt.Errorf("connect redis failed: %w", err)
	}
	return nil
}
