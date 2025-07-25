package redis

import (
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// 记录一个临时的验证码
func RecordCode(email, code string) error {
	expiration := 5 * time.Minute
	if err := redisDB.Set(email, code, expiration).Err(); err != nil {
		hlog.Info("Redis set key failed : ", err)
		return err
	}
	return nil
}

// 验证一个临时的验证码
func GetCode(email string) (string, error) {
	code, err := redisDB.Get(email).Result()
	if err != nil {
		hlog.Info("Redis get key failed : ", err)
		return "", err
	}
	return code, nil
}

func DeleteKey(key string) error {
	if err := redisDB.Del(key).Err(); err != nil {
		hlog.Info("Redis delete key failed : ", err)
		return err
	}
	return nil
}

// DelCode 删除验证码
func DelCode(email string) error {
	return DeleteKey(email)
}

// SetResetToken 设置重置密码令牌
func SetResetToken(email, token string, expiration time.Duration) error {
	key := "reset_token:" + email
	if err := redisDB.Set(key, token, expiration).Err(); err != nil {
		hlog.Info("Redis set reset token failed : ", err)
		return err
	}
	return nil
}

// GetResetToken 获取重置密码令牌
func GetResetToken(email string) (string, error) {
	key := "reset_token:" + email
	token, err := redisDB.Get(key).Result()
	if err != nil {
		hlog.Info("Redis get reset token failed : ", err)
		return "", err
	}
	return token, nil
}

// DelResetToken 删除重置密码令牌
func DelResetToken(email string) error {
	key := "reset_token:" + email
	return DeleteKey(key)
}
