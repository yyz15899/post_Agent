package dao

import (
	"context"
	"errors"
	"micro_user/pb"
	"micro_user/usersvr/model"

	"gorm.io/gorm"
)

var ErrUsernameDuplicated = errors.New("MYSQL 约束错误")
var ErrRecordNotFound = errors.New("用户未找到")
var ErrUserNameExist = errors.New("用户名已存在,请更换用户名")

// 注册
// 查重
func CheckUser(ctx context.Context, u *pb.SignupRequest) error {
	var user model.User
	// 查重
	err := DB.WithContext(ctx).Model(&model.User{}).Where("user_name = ?", u.Username).First(&user).Error
	// 这里First(Find)入参不用u  防止改变原先用户输入数据
	if err == gorm.ErrRecordNotFound { // 不存在
		return nil
	}
	if err != nil {
		return err
	}
	return ErrUserNameExist
}

func CheckUserExist(ctx context.Context, u *pb.SigninRequest) error {
	var user model.User
	err := DB.WithContext(ctx).Where("user_name = ?", u.Username).First(&user).Error
	if err != nil {
		return err
	}
	return nil
}

func CreatUser(ctx context.Context, u *pb.SignupRequest) (int64, error) {
	// 填入数据
	// 将proto中的类型封装为model相同的
	U := &model.User{
		UserName: u.Username,
		Password: u.Password,
	}
	err := DB.WithContext(ctx).Model(&model.User{}).Create(U).Error // 此时执行insert将U的数据写入User(db),同时db的主键(id)已经被反写回了U
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return 0, ErrUserNameExist // 将违反数据库主键约束的err包成用户存在(适用于重复注册)
	}
	if err != nil {
		return 0, err
	}
	// 返回id作为uid
	return U.Id, nil

}

func GetHashPwd(ctx context.Context, u string) (string, int64, error) {
	var user model.User
	err := DB.WithContext(ctx).Model(&model.User{}).Where("user_name = ?", u).First(&user).Error
	if err == gorm.ErrRecordNotFound { // 不存在
		return "", 0, ErrRecordNotFound
	}
	if err != nil {
		return "", 0, err
	}
	return user.Password, user.Id, nil
}

func GetUserInfoByUid(ctx context.Context, uid int64) (string, error) {
	var user model.User
	err := DB.WithContext(ctx).Model(&model.User{}).Where("id = ?", uid).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		return "", ErrRecordNotFound
	}
	if err != nil {
		return "", err
	}
	return user.UserName, nil
}
