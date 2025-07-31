package redis

import (
	"HuaTug.com/config"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/go-redis/redis"
)

var (
	redisDBVideoUpload *redis.Client
	redisDBVideoInfo   *redis.Client
)

func Load() {

	redisDBVideoUpload = redis.NewClient(&redis.Options{
		Addr:     VideoUpload.Addr,
		Password: config.ConfigInfo.Redis.Password,
		DB:       VideoUpload.DB,
	})

	redisDBVideoInfo = redis.NewClient(&redis.Options{
		Addr:     VideoInfo.Addr,
		Password: config.ConfigInfo.Redis.Password,
		DB:       VideoInfo.DB,
	})

	if _, err := redisDBVideoUpload.Ping().Result(); err != nil {
		hlog.Info("redisDBVideoUpload", err)
	}
	if _, err := redisDBVideoInfo.Ping().Result(); err != nil {
		hlog.Info("redisDBVideoInfo", err)
	}
}
