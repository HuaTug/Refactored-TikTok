# 本地运行与验证指南（macOS）

## 先决条件
- Go 1.20+（仓库声明 1.24.4，建议使用最新稳定版）
- Docker & Docker Compose
- zsh 终端

## 一键启动基础设施
```bash
# 在项目根目录
docker-compose up -d mysql redis etcd rabbitmq minio jaeger
```

## 初始化与代码生成
```bash
# 生成 RPC/HTTP 代码（如需要）
chmod +x kitex_gen.sh hertz_gen.sh
./kitex_gen.sh
./hertz_gen.sh
```

## 构建与运行
```bash
# 构建服务
make build

# 直接在宿主机运行（开发模式）
make go
# 或单独运行
make api
make users
make videos
make interactions
make relations
```

## 健康检查
```bash
curl http://localhost:8080/ping
```

## Smoke 测试（示例）
- 注册/登录：`POST /douyin/user/register/`，`POST /douyin/user/login/`
- 拉取 Feed：`GET /douyin/feed/`
- 发布：`POST /douyin/publish/action/`（可参考 docs/chunk_upload_api.md）

更多接口见 `docs/api.md` 与 `idl/*.thrift`。
