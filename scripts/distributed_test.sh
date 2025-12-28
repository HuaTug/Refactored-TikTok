#!/bin/bash

# 分布式事务测试脚本 - 多服务本地部署
# 用于测试事件驱动架构和分布式事务的稳定性

set -e

echo "======================================"
echo "分布式事务测试环境部署脚本"
echo "======================================"

# 颜色定义
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

# 1. 检查依赖
check_dependencies() {
    log_info "检查依赖..."

    if ! command -v docker &> /dev/null; then
        log_error "Docker 未安装，请先安装 Docker"
        exit 1
    fi

    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose 未安装，请先安装 Docker Compose"
        exit 1
    fi

    if ! command -v go &> /dev/null; then
        log_error "Go 未安装，请先安装 Go"
        exit 1
    fi

    log_info "依赖检查完成 ✓"
}

# 2. 启动基础设施
start_infrastructure() {
    log_info "启动基础设施服务 (MySQL, Redis, etcd, RabbitMQ, MinIO)..."

    docker-compose up -d mysql redis etcd rabbitmq minio

    log_info "等待服务启动..."
    sleep 10

    # 检查服务状态
    if docker ps | grep -q "kitex_mysql"; then
        log_info "MySQL 启动成功 ✓"
    else
        log_error "MySQL 启动失败"
        exit 1
    fi

    if docker ps | grep -q "kitex_redis"; then
        log_info "Redis 启动成功 ✓"
    else
        log_error "Redis 启动失败"
        exit 1
    fi

    if docker ps | grep -q "etcd"; then
        log_info "etcd 启动成功 ✓"
    else
        log_error "etcd 启动失败"
        exit 1
    fi

    if docker ps | grep -q "kitex_rabbitmq"; then
        log_info "RabbitMQ 启动成功 ✓"
    else
        log_error "RabbitMQ 启动失败"
        exit 1
    fi

    log_info "基础设施启动完成 ✓"
}

# 3. 编译所有微服务
build_services() {
    log_info "编译所有微服务..."

    # User Service
    log_info "编译 User Service..."
    cd cmd/user && go build -o user && cd ../..

    # Video Service
    log_info "编译 Video Service..."
    cd cmd/video && go build -o video && cd ../..

    # Interaction Service
    log_info "编译 Interaction Service..."
    cd cmd/interaction && go build -o interaction && cd ../..

    # Relation Service
    log_info "编译 Relation Service..."
    cd cmd/relation && go build -o relation && cd ../..

    # API Gateway
    log_info "编译 API Gateway..."
    cd cmd/api && go build -o api && cd ../..

    log_info "所有服务编译完成 ✓"
}

# 4. 创建日志目录
create_log_dirs() {
    log_info "创建日志目录..."

    mkdir -p logs/user
    mkdir -p logs/video
    mkdir -p logs/interaction
    mkdir -p logs/relation
    mkdir -p logs/api

    log_info "日志目录创建完成 ✓"
}

# 5. 启动多实例服务 (模拟分布式环境)
start_services() {
    log_info "启动微服务集群..."

    # 启动 User Service (2个实例)
    log_info "启动 User Service 实例 1 (端口 8881)..."
    cd cmd/user
    USER_SERVICE_PORT=8881 nohup ./user > ../../logs/user/user-1.log 2>&1 &
    echo $! > ../../logs/user/user-1.pid
    cd ../..
    sleep 2

    log_info "启动 User Service 实例 2 (端口 8882)..."
    cd cmd/user
    USER_SERVICE_PORT=8882 nohup ./user > ../../logs/user/user-2.log 2>&1 &
    echo $! > ../../logs/user/user-2.pid
    cd ../..
    sleep 2

    # 启动 Video Service (2个实例)
    log_info "启动 Video Service 实例 1 (端口 8883)..."
    cd cmd/video
    VIDEO_SERVICE_PORT=8883 nohup ./video > ../../logs/video/video-1.log 2>&1 &
    echo $! > ../../logs/video/video-1.pid
    cd ../..
    sleep 2

    log_info "启动 Video Service 实例 2 (端口 8884)..."
    cd cmd/video
    VIDEO_SERVICE_PORT=8884 nohup ./video > ../../logs/video/video-2.log 2>&1 &
    echo $! > ../../logs/video/video-2.pid
    cd ../..
    sleep 2

    # 启动 Interaction Service (2个实例)
    log_info "启动 Interaction Service 实例 1 (端口 8885)..."
    cd cmd/interaction
    INTERACTION_SERVICE_PORT=8885 nohup ./interaction > ../../logs/interaction/interaction-1.log 2>&1 &
    echo $! > ../../logs/interaction/interaction-1.pid
    cd ../..
    sleep 2

    log_info "启动 Interaction Service 实例 2 (端口 8886)..."
    cd cmd/interaction
    INTERACTION_SERVICE_PORT=8886 nohup ./interaction > ../../logs/interaction/interaction-2.log 2>&1 &
    echo $! > ../../logs/interaction/interaction-2.pid
    cd ../..
    sleep 2

    # 启动 Relation Service (2个实例)
    log_info "启动 Relation Service 实例 1 (端口 8887)..."
    cd cmd/relation
    RELATION_SERVICE_PORT=8887 nohup ./relation > ../../logs/relation/relation-1.log 2>&1 &
    echo $! > ../../logs/relation/relation-1.pid
    cd ../..
    sleep 2

    log_info "启动 Relation Service 实例 2 (端口 8888)..."
    cd cmd/relation
    RELATION_SERVICE_PORT=8888 nohup ./relation > ../../logs/relation/relation-2.log 2>&1 &
    echo $! > ../../logs/relation/relation-2.pid
    cd ../..
    sleep 2

    # 启动 API Gateway (1个实例)
    log_info "启动 API Gateway (端口 8080)..."
    cd cmd/api
    nohup ./api > ../../logs/api/api.log 2>&1 &
    echo $! > ../../logs/api/api.pid
    cd ../..
    sleep 3

    log_info "所有服务启动完成 ✓"
}

# 6. 显示服务状态
show_status() {
    log_info "======================================"
    log_info "服务状态"
    log_info "======================================"

    echo ""
    echo "基础设施:"
    docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep -E "kitex|etcd|minio"

    echo ""
    echo "微服务实例:"
    echo "User Service:"
    if [ -f logs/user/user-1.pid ]; then
        PID=$(cat logs/user/user-1.pid)
        if ps -p $PID > /dev/null; then
            echo "  ✓ 实例 1 运行中 (PID: $PID, Port: 8881)"
        else
            echo "  ✗ 实例 1 已停止"
        fi
    fi

    if [ -f logs/user/user-2.pid ]; then
        PID=$(cat logs/user/user-2.pid)
        if ps -p $PID > /dev/null; then
            echo "  ✓ 实例 2 运行中 (PID: $PID, Port: 8882)"
        else
            echo "  ✗ 实例 2 已停止"
        fi
    fi

    echo ""
    echo "Video Service:"
    if [ -f logs/video/video-1.pid ]; then
        PID=$(cat logs/video/video-1.pid)
        if ps -p $PID > /dev/null; then
            echo "  ✓ 实例 1 运行中 (PID: $PID, Port: 8883)"
        else
            echo "  ✗ 实例 1 已停止"
        fi
    fi

    if [ -f logs/video/video-2.pid ]; then
        PID=$(cat logs/video/video-2.pid)
        if ps -p $PID > /dev/null; then
            echo "  ✓ 实例 2 运行中 (PID: $PID, Port: 8884)"
        else
            echo "  ✗ 实例 2 已停止"
        fi
    fi

    echo ""
    echo "Interaction Service:"
    if [ -f logs/interaction/interaction-1.pid ]; then
        PID=$(cat logs/interaction/interaction-1.pid)
        if ps -p $PID > /dev/null; then
            echo "  ✓ 实例 1 运行中 (PID: $PID, Port: 8885)"
        else
            echo "  ✗ 实例 1 已停止"
        fi
    fi

    if [ -f logs/interaction/interaction-2.pid ]; then
        PID=$(cat logs/interaction/interaction-2.pid)
        if ps -p $PID > /dev/null; then
            echo "  ✓ 实例 2 运行中 (PID: $PID, Port: 8886)"
        else
            echo "  ✗ 实例 2 已停止"
        fi
    fi

    echo ""
    echo "Relation Service:"
    if [ -f logs/relation/relation-1.pid ]; then
        PID=$(cat logs/relation/relation-1.pid)
        if ps -p $PID > /dev/null; then
            echo "  ✓ 实例 1 运行中 (PID: $PID, Port: 8887)"
        else
            echo "  ✗ 实例 1 已停止"
        fi
    fi

    if [ -f logs/relation/relation-2.pid ]; then
        PID=$(cat logs/relation/relation-2.pid)
        if ps -p $PID > /dev/null; then
            echo "  ✓ 实例 2 运行中 (PID: $PID, Port: 8888)"
        else
            echo "  ✗ 实例 2 已停止"
        fi
    fi

    echo ""
    echo "API Gateway:"
    if [ -f logs/api/api.pid ]; then
        PID=$(cat logs/api/api.pid)
        if ps -p $PID > /dev/null; then
            echo "  ✓ 运行中 (PID: $PID, Port: 8080)"
        else
            echo "  ✗ 已停止"
        fi
    fi

    echo ""
    log_info "======================================"
    log_info "访问地址:"
    log_info "======================================"
    echo "API Gateway:       http://localhost:8080"
    echo "RabbitMQ 管理界面: http://localhost:15672 (guest/guest)"
    echo "MinIO 控制台:      http://localhost:9003 (tiktok_minio_admin/MainMinIO@TikTok#2025!SecurePass)"
    echo ""
}

# 主函数
main() {
    case "${1:-start}" in
        start)
            check_dependencies
            create_log_dirs
            start_infrastructure
            build_services
            start_services
            show_status

            log_info "======================================"
            log_info "部署完成！"
            log_info "======================================"
            echo ""
            echo "后续操作:"
            echo "  查看状态: ./scripts/distributed_test.sh status"
            echo "  停止服务: ./scripts/distributed_test.sh stop"
            echo "  查看日志: ./scripts/distributed_test.sh logs <service>"
            echo "  运行测试: ./scripts/distributed_test.sh test"
            ;;

        status)
            show_status
            ;;

        stop)
            log_info "停止所有服务..."

            # 停止微服务
            for pidfile in logs/*/*.pid; do
                if [ -f "$pidfile" ]; then
                    PID=$(cat "$pidfile")
                    if ps -p $PID > /dev/null; then
                        kill $PID
                        log_info "已停止进程 $PID"
                    fi
                    rm "$pidfile"
                fi
            done

            # 停止基础设施
            docker-compose down

            log_info "所有服务已停止 ✓"
            ;;

        logs)
            SERVICE=${2:-api}
            if [ -f "logs/$SERVICE/${SERVICE}-1.log" ]; then
                tail -f "logs/$SERVICE/${SERVICE}-1.log"
            elif [ -f "logs/$SERVICE/${SERVICE}.log" ]; then
                tail -f "logs/$SERVICE/${SERVICE}.log"
            else
                log_error "日志文件不存在: logs/$SERVICE/"
                exit 1
            fi
            ;;

        test)
            log_info "运行分布式事务测试..."
            ./scripts/test_distributed_transaction.sh
            ;;

        restart)
            $0 stop
            sleep 3
            $0 start
            ;;

        *)
            echo "用法: $0 {start|stop|status|logs|test|restart}"
            echo ""
            echo "命令说明:"
            echo "  start   - 启动所有服务"
            echo "  stop    - 停止所有服务"
            echo "  status  - 查看服务状态"
            echo "  logs    - 查看服务日志"
            echo "  test    - 运行分布式事务测试"
            echo "  restart - 重启所有服务"
            exit 1
            ;;
    esac
}

main "$@"
