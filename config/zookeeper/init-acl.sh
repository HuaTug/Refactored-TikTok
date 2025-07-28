#!/bin/bash

# ZooKeeper ACL 初始化脚本
# 等待ZooKeeper服务启动
echo "等待ZooKeeper服务启动..."
sleep 30

# ZooKeeper连接信息
ZK_HOST="localhost:2181"
ZK_CLI="/opt/kafka/bin/zookeeper-shell.sh"

# 用户凭据
ADMIN_USER="admin"
ADMIN_PASS="ZkAdmin@TikTok#2025!Secure"
APP_USER="tiktok"
APP_PASS="ZkTikTok@2025!AppUser"

echo "开始配置ZooKeeper ACL权限..."

# 创建临时脚本文件
cat > /tmp/zk_acl_setup.txt << EOF
# 添加认证用户
addauth digest ${ADMIN_USER}:${ADMIN_PASS}
addauth digest ${APP_USER}:${APP_PASS}

# 为根路径设置ACL权限
# admin用户：完全权限 (cdrwa)
# tiktok用户：读写权限 (crw)
setAcl / digest:${ADMIN_USER}:$(echo -n ${ADMIN_USER}:${ADMIN_PASS} | openssl dgst -binary -sha1 | openssl base64):cdrwa,digest:${APP_USER}:$(echo -n ${APP_USER}:${APP_PASS} | openssl dgst -binary -sha1 | openssl base64):crw

# 验证ACL设置
getAcl /

# 创建测试节点验证权限
create /test "test_data"
getAcl /test

quit
EOF

# 执行ACL配置
echo "执行ACL配置..."
${ZK_CLI} ${ZK_HOST} < /tmp/zk_acl_setup.txt

# 清理临时文件
rm -f /tmp/zk_acl_setup.txt

echo "ZooKeeper ACL配置完成！"
echo "管理员用户: ${ADMIN_USER} (完全权限)"
echo "应用用户: ${APP_USER} (读写权限)"
echo ""
echo "测试命令："
echo "1. 未认证访问（应失败）："
echo "   ./zkCli.sh -server ${ZK_HOST}"
echo "   getAcl /"
echo ""
echo "2. 认证访问（应成功）："
echo "   ./zkCli.sh -server ${ZK_HOST}"
echo "   addauth digest ${ADMIN_USER}:${ADMIN_PASS}"
echo "   getAcl /"
