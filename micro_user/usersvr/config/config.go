package config

import (
	"errors"
	"os"
)

var LocalSecretKey []byte // 本地密钥(通过jwt中方法加密成)

func InitConfig() error {
	key := os.Getenv("JWT_SECRET")
	if key == "" {
		return errors.New("密钥错误")
	}
	LocalSecretKey = []byte(key)
	return nil

}
