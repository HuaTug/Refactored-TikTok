package config

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var ConfigInfo config

// 使用Viper的好处在于支持配置文件的热更新 同时viper对于大小写并不敏感 都是统一进行处理
func Init() {
	// 获取当前工作目录用于调试
	wd, _ := os.Getwd()
	logrus.Infof("Current working directory: %s", wd)

	viper.SetConfigType("yaml")
	viper.SetConfigName("config.yml")

	// 添加多个可能的配置文件路径
	configPaths := []string{
		"../../config",
		"./config",
		"../config",
		".",
	}

	for _, path := range configPaths {
		viper.AddConfigPath(path)
		absPath, _ := filepath.Abs(path)
		logrus.Infof("Added config path: %s (absolute: %s)", path, absPath)
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			logrus.Errorf("config file not found: %v", err)
		} else {
			logrus.Errorf("config error: %v", err)
		}
		return
	}

	logrus.Infof("Successfully read config file: %s", viper.ConfigFileUsed())

	// 手动从viper获取配置值，避免Unmarshal问题
	ConfigInfo.Mysql.Addr = viper.GetString("mysql.addr")
	ConfigInfo.Mysql.Database = viper.GetString("mysql.database")
	ConfigInfo.Mysql.Username = viper.GetString("mysql.username")
	ConfigInfo.Mysql.Password = viper.GetString("mysql.password")
	ConfigInfo.Mysql.Charset = viper.GetString("mysql.charset")

	ConfigInfo.CommentSharding.DatabaseCount = viper.GetInt("comment_sharding.database_count")
	ConfigInfo.CommentSharding.TableCount = viper.GetInt("comment_sharding.table_count")
	ConfigInfo.CommentSharding.MaxOpenConns = viper.GetInt("comment_sharding.max_open_conns")
	ConfigInfo.CommentSharding.MaxIdleConns = viper.GetInt("comment_sharding.max_idle_conns")
	ConfigInfo.CommentSharding.ConnMaxLifetime = viper.GetString("comment_sharding.conn_max_lifetime")
	ConfigInfo.CommentSharding.MasterDSNs = viper.GetStringSlice("comment_sharding.master_dsns")

	// 获取slave_dsns (二维数组)
	if slaveDSNsInterface := viper.Get("comment_sharding.slave_dsns"); slaveDSNsInterface != nil {
		if slaveDSNsSlice, ok := slaveDSNsInterface.([]interface{}); ok {
			ConfigInfo.CommentSharding.SlaveDSNs = make([][]string, len(slaveDSNsSlice))
			for i, item := range slaveDSNsSlice {
				if itemSlice, ok := item.([]interface{}); ok {
					ConfigInfo.CommentSharding.SlaveDSNs[i] = make([]string, len(itemSlice))
					for j, dsn := range itemSlice {
						if dsnStr, ok := dsn.(string); ok {
							ConfigInfo.CommentSharding.SlaveDSNs[i][j] = dsnStr
						}
					}
				}
			}
		}
	}

	ConfigInfo.Redis.Addr = viper.GetString("redis.addr")
	ConfigInfo.Redis.Password = viper.GetString("redis.password")

	ConfigInfo.Etcd.Addr = viper.GetString("etcd.addr")

	ConfigInfo.RabbitMq.Addr = viper.GetString("rabbitmq.addr")
	ConfigInfo.RabbitMq.Username = viper.GetString("rabbitmq.username")
	ConfigInfo.RabbitMq.Password = viper.GetString("rabbitmq.password")

	// 打印配置信息用于调试
	logrus.Infof("Config loaded - MySQL: %s:%s@%s/%s",
		ConfigInfo.Mysql.Username, "***", ConfigInfo.Mysql.Addr, ConfigInfo.Mysql.Database)
	logrus.Infof("Comment sharding - DatabaseCount: %d, TableCount: %d, MasterDSNs count: %d",
		ConfigInfo.CommentSharding.DatabaseCount,
		ConfigInfo.CommentSharding.TableCount,
		len(ConfigInfo.CommentSharding.MasterDSNs))

	if len(ConfigInfo.CommentSharding.MasterDSNs) == 0 {
		logrus.Warn("No comment sharding DSNs configured!")
	}
}
