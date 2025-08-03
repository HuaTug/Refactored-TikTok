#!/bin/bash

# 测试事件驱动同步服务的脚本

echo "=== 事件驱动同步服务测试 ==="

# 检查服务是否运行
echo "1. 检查interaction服务状态..."
if pgrep -f "interaction" > /dev/null; then
    echo "✅ Interaction服务正在运行"
else
    echo "❌ Interaction服务未运行，请先启动服务"
    exit 1
fi

# 检查消费者是否运行
echo "2. 检查消费者服务状态..."
if pgrep -f "consumer" > /dev/null; then
    echo "✅ 消费者服务正在运行"
else
    echo "⚠️  消费者服务未运行，建议启动消费者服务以获得最佳效果"
fi

# 检查数据库连接
echo "3. 检查数据库连接..."
mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT 1;" > /dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "✅ 数据库连接正常"
else
    echo "❌ 数据库连接失败"
    exit 1
fi

# 检查sync_events表是否存在
echo "4. 检查sync_events表..."
TABLE_EXISTS=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SHOW TABLES LIKE 'sync_events';" 2>/dev/null | wc -l)
if [ $TABLE_EXISTS -gt 1 ]; then
    echo "✅ sync_events表已创建"
else
    echo "❌ sync_events表不存在，请检查数据库迁移"
    exit 1
fi

# 检查video_likes表是否存在
echo "5. 检查video_likes表..."
TABLE_EXISTS=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SHOW TABLES LIKE 'video_likes';" 2>/dev/null | wc -l)
if [ $TABLE_EXISTS -gt 1 ]; then
    echo "✅ video_likes表已存在"
else
    echo "❌ video_likes表不存在，请检查数据库结构"
    exit 1
fi

# 模拟点赞操作测试
echo "6. 执行点赞操作测试..."

# 记录测试前的数据状态
echo "   记录测试前的数据状态..."
USER_BEHAVIORS_BEFORE=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT COUNT(*) FROM user_behaviors WHERE behavior_type='like';" 2>/dev/null | tail -n 1)
VIDEO_LIKES_BEFORE=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT COUNT(*) FROM video_likes;" 2>/dev/null | tail -n 1)
SYNC_EVENTS_BEFORE=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT COUNT(*) FROM sync_events;" 2>/dev/null | tail -n 1)

echo "   测试前状态: user_behaviors=$USER_BEHAVIORS_BEFORE, video_likes=$VIDEO_LIKES_BEFORE, sync_events=$SYNC_EVENTS_BEFORE"

# 使用curl模拟点赞请求（需要根据实际API调整）
echo "   发送点赞请求..."
# 这里需要根据实际的API接口调整请求格式
# curl -X POST "http://localhost:8080/api/like" \
#      -H "Content-Type: application/json" \
#      -d '{"user_id": 1, "video_id": 1, "action_type": "1"}' > /dev/null 2>&1

echo "   ⚠️  请手动执行点赞操作来测试同步功能"

# 等待一段时间让异步处理完成
echo "   等待异步处理完成..."
sleep 10

# 检查数据变化
echo "7. 检查数据同步结果..."
USER_BEHAVIORS_AFTER=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT COUNT(*) FROM user_behaviors WHERE behavior_type='like';" 2>/dev/null | tail -n 1)
VIDEO_LIKES_AFTER=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT COUNT(*) FROM video_likes;" 2>/dev/null | tail -n 1)
SYNC_EVENTS_AFTER=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT COUNT(*) FROM sync_events;" 2>/dev/null | tail -n 1)

echo "   测试后状态: user_behaviors=$USER_BEHAVIORS_AFTER, video_likes=$VIDEO_LIKES_AFTER, sync_events=$SYNC_EVENTS_AFTER"

# 检查同步事件状态
echo "8. 检查同步事件处理状态..."
PENDING_EVENTS=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT COUNT(*) FROM sync_events WHERE status='pending';" 2>/dev/null | tail -n 1)
COMPLETED_EVENTS=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT COUNT(*) FROM sync_events WHERE status='completed';" 2>/dev/null | tail -n 1)
FAILED_EVENTS=$(mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT COUNT(*) FROM sync_events WHERE status='failed';" 2>/dev/null | tail -n 1)

echo "   同步事件状态: pending=$PENDING_EVENTS, completed=$COMPLETED_EVENTS, failed=$FAILED_EVENTS"

if [ $FAILED_EVENTS -gt 0 ]; then
    echo "   ⚠️  发现失败的同步事件，请检查日志"
    mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT id, event_type, status, error_message, created_at FROM sync_events WHERE status='failed' ORDER BY created_at DESC LIMIT 5;" 2>/dev/null
fi

echo ""
echo "=== 测试完成 ==="
echo ""
echo "📋 测试总结:"
echo "   - 如果sync_events表中有新记录，说明事件发布成功"
echo "   - 如果video_likes表中有对应记录，说明同步成功"
echo "   - 如果有failed状态的事件，请检查错误日志"
echo ""
echo "🔧 手动测试建议:"
echo "   1. 执行点赞操作"
echo "   2. 检查user_behaviors表是否有新记录"
echo "   3. 等待10-15秒后检查video_likes表是否有对应记录"
echo "   4. 检查sync_events表中的事件状态"
echo ""
echo "📊 监控命令:"
echo "   - 查看同步事件: mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e \"SELECT * FROM sync_events ORDER BY created_at DESC LIMIT 10;\""
echo "   - 查看点赞记录: mysql -h localhost -P 3307 -u root -p'TikTok@MySQL#2025!Secure' -D TikTok -e \"SELECT * FROM video_likes ORDER BY created_at DESC LIMIT 10;\""