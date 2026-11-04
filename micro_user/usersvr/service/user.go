package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"micro_user/pb"
	"micro_user/usersvr/config"
	"micro_user/usersvr/dao"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var PasswordTooLong = errors.New("密码过长")
var ErrMismatchedHashAndPassword = errors.New("密码或用户名错误")

func Signup(ctx context.Context, req *pb.SignupRequest) (int64, error) {
	// 查重
	err := dao.CheckUser(ctx, req)
	if err != nil {
		return 0, err // 错误交给handler翻译
	}

	// 加密密码
	enpwd, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return 0, PasswordTooLong
		}
		return 0, err
	}
	// 存db
	pwd := string(enpwd)
	// 封装
	r := &pb.SignupRequest{
		Username: req.Username,
		Password: pwd,
	}
	uid, err := dao.CreatUser(ctx, r)
	if err != nil {
		log.Printf("CreatUser failed")
		return 0, err
	}
	return uid, nil

}

// 登录
func SignIn(ctx context.Context, req *pb.SigninRequest) (string, int64, error) {
	// 查存(GetHashPwd中已有)
	// 查出密码
	hashpwd, uid, err := dao.GetHashPwd(ctx, req.Username)
	if err != nil {
		return "", 0, err
	}
	// 验证密码
	err = bcrypt.CompareHashAndPassword([]byte(hashpwd), []byte(req.Password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return "", 0, ErrMismatchedHashAndPassword
		}
		return "", 0, fmt.Errorf("校验密码失败,err: %w", err)
	}
	// 生成并返回token(header,payload,signature)
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(uid, 10),                        // 标识符,同时存入用作表示的数据(uid)
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)), // jwt超时时限
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// NewWithClaims作用是接收签名算法(HS256)并生成header,同时组装传入的claims组成jwt的前两部分(还差第三部分(签名))
	strtoken, err := token.SignedString(config.LocalSecretKey) // 对前两部分进行加密,生成第三部分(防止篡改)
	if err != nil {
		return "", 0, err
	}
	return strtoken, uid, nil
}

func GetUserInfoByUid(ctx context.Context, uid int64) (string, error) {
	// 检测ctx中的超时信号
	// select {
	// case <-time.After(10 * time.Second):
	// 	// 模拟客户端3s超时时间到后,服务端任务还有10s执行完
	// case <-ctx.Done():
	// 	log.Println("service 收到取消信号", ctx.Err())
	// 	return "", ctx.Err()
	// }
	username, err := dao.GetUserInfoByUid(ctx, uid)
	if err != nil {
		return "", err
	}
	return username, nil
}
