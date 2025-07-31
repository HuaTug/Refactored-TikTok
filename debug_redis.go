package main

import (
	"context"
	"fmt"
	"log"

	"HuaTug.com/config"
	redis "HuaTug.com/cmd/video/infras/redis"
	"github.com/spf13/viper"
)

func main() {
	fmt.Println("开始Redis BitMap测试...")
	
	// 手动设置配置文件路径
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	viper.AddConfigPath("./config")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("配置文件读取失败: %v", err)
	}
	if err := viper.Unmarshal(&config.ConfigInfo); err != nil {
		log.Fatalf("配置解析失败: %v", err)
	}
	
	redis.Load()
	
	ctx := context.Background()
	
	// 测试参数
	uid := "1"
	uuid := "test_bitmap_debug"
	chunkNumber := int64(1)
	
	fmt.Printf("测试键名: b:%s:%s\n", uid, uuid)
	fmt.Printf("分片编号: %d\n", chunkNumber)
	
	// 1. 首先清除测试键（如果存在）
	fmt.Println("\n1. 清除测试数据...")
	if err := redis.DeleteVideoEvent(ctx, uuid, uid); err != nil {
		fmt.Printf("清除测试数据失败（可能不存在）: %v\n", err)
	}
	
	// 2. 创建测试会话
	fmt.Println("\n2. 创建测试会话...")
	if err := redis.CreateVideoEventV2(ctx, "测试标题", "测试描述", uid, uuid, "1", "测试标签", "测试分类"); err != nil {
		log.Fatalf("创建测试会话失败: %v", err)
	}
	
	// 3. 更新分片状态
	fmt.Println("\n3. 更新分片状态...")
	if err := redis.UpdateChunkUploadStatus(ctx, uuid, uid, chunkNumber); err != nil {
		log.Fatalf("更新分片状态失败: %v", err)
	}
	
	// 4. 检查分片状态
	fmt.Println("\n4. 检查分片状态...")
	uploadedChunks, err := redis.GetUploadedChunksStatus(ctx, uuid, uid)
	if err != nil {
		log.Fatalf("获取分片状态失败: %v", err)
	}
	
	fmt.Printf("分片状态数组: %v\n", uploadedChunks)
	fmt.Printf("分片1状态: %v\n", uploadedChunks[0])
	
	// 5. 检查所有分片是否完成
	fmt.Println("\n5. 检查所有分片是否完成...")
	allUploaded, err := redis.IsAllChunksUploadedV2(ctx, uuid, uid)
	if err != nil {
		log.Fatalf("检查所有分片状态失败: %v", err)
	}
	
	fmt.Printf("所有分片是否完成: %v\n", allUploaded)
	
	// 6. 清理测试数据
	fmt.Println("\n6. 清理测试数据...")
	if err := redis.DeleteVideoEvent(ctx, uuid, uid); err != nil {
		fmt.Printf("清理测试数据失败: %v\n", err)
	}
	
	fmt.Println("\n测试完成！")
}
