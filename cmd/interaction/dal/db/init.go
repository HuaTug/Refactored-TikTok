package db

import (
	"HuaTug.com/cmd/model"
	"HuaTug.com/config"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormopentracing "gorm.io/plugin/opentracing"
)

var DB *gorm.DB

// Init init DB
func Init() {
	var err error
	//dsn := utils.GetMysqlDsn()
	dsn := config.ConfigInfo.Mysql.Username + ":" + config.ConfigInfo.Mysql.Password + "@tcp(" + config.ConfigInfo.Mysql.Addr + ")/" + config.ConfigInfo.Mysql.Database + "?charset=utf8mb4&parseTime=True&loc=Local"
	DB, err = gorm.Open(mysql.Open(dsn),
		&gorm.Config{
			PrepareStmt:            true,
			SkipDefaultTransaction: true,
		},
	)
	if err != nil {
		panic(err)
	}
	if err = DB.Use(gormopentracing.New()); err != nil {
		panic(err)
	}

	// 自动迁移事件驱动相关表
	if err = migrateEventDrivenTables(); err != nil {
		panic(err)
	}

	// if err = DB.Use(sharding.NewSharding("user_id", 4, "users")); err != nil {
	// 	panic(err)
	// }
}

// migrateEventDrivenTables 迁移事件驱动相关表
func migrateEventDrivenTables() error {
	hlog.Info("Starting event-driven tables migration...")

	// 迁移sync_events表
	if err := DB.AutoMigrate(&model.SyncEvent{}); err != nil {
		hlog.Errorf("Failed to migrate sync_events table: %v", err)
		return err
	}

	hlog.Info("Event-driven tables migration completed successfully")
	return nil
}
