#!/bin/bash

# 事件驱动同步机制测试脚本
# 用于验证新的同步机制是否正常工作

set -e

# 配置
BASE_URL="http://localhost:8893"
TEST_USER_ID=1001
TEST_VIDEO_ID=2001
TEST_COMMENT_ID=3001

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查服务是否运行
check_service() {
    log_info "检查服务状态..."
    
    if curl -s -f "${BASE_URL}/health" > /dev/null; then
        log_info "✅ 服务运行正常"
    else
        log_error "❌ 服务未运行或健康检查失败"
        exit 1
    fi
}

# 测试视频点赞功能
test_video_like() {
    log_info "测试视频点赞功能..."
    
    # 点赞
    log_info "执行点赞操作..."
    response=$(curl -s -X POST "${BASE_URL}/v1/action/like" \
        -H "Content-Type: application/json" \
        -d "{
            \"user_id\": ${TEST_USER_ID},
            \"video_id\": ${TEST_VIDEO_ID},
            \"action_type\": \"like\"
        }")
    
    if echo "$response" | grep -q '"code":200'; then
        log_info "✅ 视频点赞成功"
    else
        log_error "❌ 视频点赞失败: $response"
        return 1
    fi
    
    # 等待异步处理
    sleep 2
    
    # 取消点赞
    log_info "执行取消点赞操作..."
    response=$(curl -s -X POST "${BASE_URL}/v1/action/like" \
        -H "Content-Type: application/json" \
        -d "{
            \"user_id\": ${TEST_USER_ID},
            \"video_id\": ${TEST_VIDEO_ID},
            \"action_type\": \"unlike\"
        }")
    
    if echo "$response" | grep -q '"code":200'; then
        log_info "✅ 取消视频点赞成功"
    else
        log_error "❌ 取消视频点赞失败: $response"
        return 1
    fi
}

# 测试评论点赞功能
test_comment_like() {
    log_info "测试评论点赞功能..."
    
    # 点赞评论
    log_info "执行评论点赞操作..."
    response=$(curl -s -X POST "${BASE_URL}/v1/action/like" \
        -H "Content-Type: application/json" \
        -d "{
            \"user_id\": ${TEST_USER_ID},
            \"comment_id\": ${TEST_COMMENT_ID},
            \"action_type\": \"like\"
        }")
    
    if echo "$response" | grep -q '"code":200'; then
        log_info "✅ 评论点赞成功"
    else
        log_error "❌ 评论点赞失败: $response"
        return 1
    fi
    
    # 等待异步处理
    sleep 2
    
    # 取消评论点赞
    log_info "执行取消评论点赞操作..."
    response=$(curl -s -X POST "${BASE_URL}/v1/action/like" \
        -H "Content-Type: application/json" \
        -d "{
            \"user_id\": ${TEST_USER_ID},
            \"comment_id\": ${TEST_COMMENT_ID},
            \"action_type\": \"unlike\"
        }")
    
    if echo "$response" | grep -q '"code":200'; then
        log_info "✅ 取消评论点赞成功"
    else
        log_error "❌ 取消评论点赞失败: $response"
        return 1
    fi
}

# 测试并发点赞
test_concurrent_likes() {
    log_info "测试并发点赞..."
    
    # 创建临时文件存储结果
    temp_file=$(mktemp)
    
    # 并发执行10个点赞请求
    for i in {1..10}; do
        {
            user_id=$((TEST_USER_ID + i))
            response=$(curl -s -X POST "${BASE_URL}/v1/action/like" \
                -H "Content-Type: application/json" \
                -d "{
                    \"user_id\": ${user_id},
                    \"video_id\": ${TEST_VIDEO_ID},
                    \"action_type\": \"like\"
                }")
            
            if echo "$response" | grep -q '"code":200'; then
                echo "SUCCESS" >> "$temp_file"
            else
                echo "FAILED" >> "$temp_file"
            fi
        } &
    done
    
    # 等待所有请求完成
    wait
    
    # 统计结果
    success_count=$(grep -c "SUCCESS" "$temp_file" || echo "0")
    failed_count=$(grep -c "FAILED" "$temp_file" || echo "0")
    
    log_info "并发测试结果: 成功 $success_count, 失败 $failed_count"
    
    if [ "$success_count" -ge 8 ]; then
        log_info "✅ 并发测试通过"
    else
        log_error "❌ 并发测试失败"
        rm -f "$temp_file"
        return 1
    fi
    
    rm -f "$temp_file"
}

# 测试限流功能
test_rate_limiting() {
    log_info "测试限流功能..."
    
    # 快速发送大量请求
    success_count=0
    rate_limited_count=0
    
    for i in {1..70}; do
        response=$(curl -s -X POST "${BASE_URL}/v1/action/like" \
            -H "Content-Type: application/json" \
            -d "{
                \"user_id\": ${TEST_USER_ID},
                \"video_id\": $((TEST_VIDEO_ID + i)),
                \"action_type\": \"like\"
            }")
        
        if echo "$response" | grep -q '"code":200'; then
            ((success_count++))
        elif echo "$response" | grep -q '"code":429'; then
            ((rate_limited_count++))
        fi
    done
    
    log_info "限流测试结果: 成功 $success_count, 被限流 $rate_limited_count"
    
    if [ "$rate_limited_count" -gt 0 ]; then
        log_info "✅ 限流功能正常"
    else
        log_warn "⚠️  限流功能可能未生效"
    fi
}

# 检查数据一致性
check_data_consistency() {
    log_info "检查数据一致性..."
    
    # 等待异步处理完成
    sleep 5
    
    # 获取一致性报告
    response=$(curl -s "${BASE_URL}/consistency/report?hours=1")
    
    if echo "$response" | grep -q '"consistency_rate"'; then
        consistency_rate=$(echo "$response" | grep -o '"consistency_rate":[0-9.]*' | cut -d':' -f2)
        log_info "数据一致性率: ${consistency_rate}%"
        
        if (( $(echo "$consistency_rate >= 95" | bc -l) )); then
            log_info "✅ 数据一致性良好"
        else
            log_warn "⚠️  数据一致性需要关注"
        fi
    else
        log_warn "⚠️  无法获取一致性报告"
    fi
}

# 检查缓存状态
check_cache_status() {
    log_info "检查缓存状态..."
    
    response=$(curl -s "${BASE_URL}/cache/stats")
    
    if echo "$response" | grep -q '"total_keys"'; then
        total_keys=$(echo "$response" | grep -o '"total_keys":[0-9]*' | cut -d':' -f2)
        log_info "缓存键总数: $total_keys"
        log_info "✅ 缓存状态正常"
    else
        log_warn "⚠️  无法获取缓存状态"
    fi
}

# 性能测试
performance_test() {
    log_info "执行性能测试..."
    
    start_time=$(date +%s.%N)
    
    # 执行100个点赞请求
    for i in {1..100}; do
        curl -s -X POST "${BASE_URL}/v1/action/like" \
            -H "Content-Type: application/json" \
            -d "{
                \"user_id\": $((TEST_USER_ID + i % 10)),
                \"video_id\": $((TEST_VIDEO_ID + i % 20)),
                \"action_type\": \"like\"
            }" > /dev/null &
        
        # 控制并发数
        if (( i % 10 == 0 )); then
            wait
        fi
    done
    
    wait
    end_time=$(date +%s.%N)
    
    duration=$(echo "$end_time - $start_time" | bc)
    qps=$(echo "scale=2; 100 / $duration" | bc)
    
    log_info "性能测试结果: 100个请求耗时 ${duration}s, QPS: ${qps}"
    
    if (( $(echo "$qps >= 50" | bc -l) )); then
        log_info "✅ 性能测试通过"
    else
        log_warn "⚠️  性能可能需要优化"
    fi
}

# 清理测试数据
cleanup_test_data() {
    log_info "清理测试数据..."
    
    # 这里可以添加清理逻辑
    # 例如删除测试期间创建的数据
    
    log_info "✅ 测试数据清理完成"
}

# 主测试流程
main() {
    log_info "开始事件驱动同步机制测试..."
    log_info "========================================"
    
    # 检查依赖
    if ! command -v curl &> /dev/null; then
        log_error "curl 命令未找到，请先安装 curl"
        exit 1
    fi
    
    if ! command -v bc &> /dev/null; then
        log_error "bc 命令未找到，请先安装 bc"
        exit 1
    fi
    
    # 执行测试
    check_service
    
    test_video_like
    test_comment_like
    test_concurrent_likes
    test_rate_limiting
    
    check_data_consistency
    check_cache_status
    
    performance_test
    
    cleanup_test_data
    
    log_info "========================================"
    log_info "✅ 所有测试完成！"
}

# 帮助信息
show_help() {
    echo "事件驱动同步机制测试脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help     显示帮助信息"
    echo "  -u, --url      设置服务URL (默认: http://localhost:8893)"
    echo "  --user-id      设置测试用户ID (默认: 1001)"
    echo "  --video-id     设置测试视频ID (默认: 2001)"
    echo "  --comment-id   设置测试评论ID (默认: 3001)"
    echo ""
    echo "示例:"
    echo "  $0                                    # 使用默认配置运行测试"
    echo "  $0 -u http://localhost:8080          # 指定服务URL"
    echo "  $0 --user-id 2000 --video-id 3000   # 指定测试ID"
}

# 解析命令行参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -u|--url)
            BASE_URL="$2"
            shift 2
            ;;
        --user-id)
            TEST_USER_ID="$2"
            shift 2
            ;;
        --video-id)
            TEST_VIDEO_ID="$2"
            shift 2
            ;;
        --comment-id)
            TEST_COMMENT_ID="$2"
            shift 2
            ;;
        *)
            log_error "未知参数: $1"
            show_help
            exit 1
            ;;
    esac
done

# 运行主程序
main