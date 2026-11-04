package handler

import (
	"context"
	"errors"
	"log"
	"micro_user/pb"
	"micro_user/usersvr/dao"
	"micro_user/usersvr/middleware"
	"micro_user/usersvr/service"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Serve struct {
	pb.UnimplementedUserServiceServer
}
type uidKey struct{}

func (s *Serve) Signup(ctx context.Context, in *pb.SignupRequest) (*pb.SignupResponse, error) {
	// 检查输入
	if len(in.Username) == 0 {
		return nil, status.Error(codes.InvalidArgument, "用户名不能为空")
	}
	if len(in.Password) == 0 {
		return nil, status.Error(codes.InvalidArgument, "密码不能为空")
	}
	// 注册逻辑
	uid, err := service.Signup(ctx, in)
	if err != nil {
		switch {
		case errors.Is(err, dao.ErrUserNameExist):
			return nil, status.Error(codes.AlreadyExists, "用户名已存在")

		case errors.Is(err, service.PasswordTooLong):
			return nil, status.Error(codes.InvalidArgument, "密码过长")

		default:
			log.Printf("signup failed: %v", err)
			return nil, status.Error(codes.Internal, "注册失败")
		}

	}
	return &pb.SignupResponse{UserId: uid}, nil
}

func (s *Serve) Signin(ctx context.Context, in *pb.SigninRequest) (*pb.SigninResponse, error) {
	// 检查输入
	if len(in.Username) == 0 {
		return nil, status.Error(codes.InvalidArgument, "用户名不能为空")
	}
	if len(in.Password) == 0 {
		return nil, status.Error(codes.InvalidArgument, "密码不能为空")
	}
	// 拿到uid和token
	token, uid, err := service.SignIn(ctx, in)
	if err != nil {
		switch {
		case errors.Is(err, dao.ErrRecordNotFound),
			errors.Is(err, service.ErrMismatchedHashAndPassword):
			return nil, status.Error(codes.InvalidArgument, "用户名或密码错误")

		default:
			log.Printf("Signin failed,err:%v", err) // 补充日志(给service说明err)
			return nil, status.Error(codes.Internal, "服务繁忙")
		}

	}

	return &pb.SigninResponse{UserId: uid, Token: token}, nil
}

func (s *Serve) GetProfile(ctx context.Context, _ *emptypb.Empty) (*pb.UserInfo, error) {
	// 判断jwt通过后此时ctx中包含uid
	uid, ok := middleware.UserIdFromContext(ctx)
	if !ok {
		log.Printf("UserIdFromContext failed")
		return nil, status.Error(codes.InvalidArgument, "参数错误")
	}
	// 将uid带入获取username
	username, err := service.GetUserInfoByUid(ctx, uid)
	if err != nil {
		switch {
		case errors.Is(err, dao.ErrRecordNotFound):
			return nil, status.Error(codes.InvalidArgument, "用户不存在")
		case errors.Is(err, context.Canceled):
			return nil, status.Error(codes.Canceled, "请求已取消")
		case errors.Is(err, context.DeadlineExceeded):
			return nil, status.Error(codes.DeadlineExceeded, "请求超时")
		default:
			log.Printf("GetUserInfoByUid failed,uid=%d err:%v", uid, err)
			return nil, status.Error(codes.Internal, "内部错误")
		}
	}
	// 封装
	resp := &pb.UserInfo{
		UserId:   strconv.FormatInt(uid, 10),
		Username: username,
	}
	return resp, nil
}
