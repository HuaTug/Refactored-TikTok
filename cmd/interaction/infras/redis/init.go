package redis

import (
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/go-redis/redis"
)

var (
	RedisDBInteraction *redis.Client
)

func Load() {

	RedisDBInteraction = redis.NewClient(&redis.Options{
		Addr:     Interaction.Addr,
		Password: Interaction.PassWord,
		DB:       Interaction.DB,
	})

	if _, err := RedisDBInteraction.Ping().Result(); err != nil {
		hlog.Info("redisDBCommentInfo", err)
	}
}
