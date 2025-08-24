# 服务端分片上传API文档

## 概述

修改后的视频上传API支持服务端分片功能。客户端上传一个完整的大文件，服务端根据指定的分片数量自动进行分片，然后将分片依次上传到MinIO存储服务器。

## API流程

### 1. 开始上传 (VideoPublishStartV2)

**接口:** `POST /douyin/publish/start/v2/`

**参数:**
- `title`: 视频标题
- `description`: 视频描述  
- `lab_name`: 标签（逗号分隔）
- `category`: 分类
- `open`: 隐私设置 (0=私密, 1=公开, 2=朋友可见)
- `chunk_total_number`: 期望的分片总数（服务端会使用这个值作为参考）

**响应:**
```json
{
    "status_code": 0,
    "status_msg": "success",
    "data": {
        "upload_session_uuid": "xxx-xxx-xxx",
        "video_id": 123,
        "user_quota": {...},
        "temp_upload_path": "/tmp/xxx",
        "session_expires_at": 1629789600
    }
}
```

### 2. 上传文件并分片 (VideoPublishUploadingV2)

**接口:** `POST /douyin/publish/uploading/v2/`

**参数:**
- `uuid`: 上传会话UUID
- `chunk_number`: 要分片的数量（不是分片序号）
- `filename`: 文件名
- `is_m3u8`: 是否为M3U8格式
- `data`: 完整的视频文件（multipart/form-data）

**处理流程:**
1. 服务端接收完整文件
2. 根据`chunk_number`参数计算每个分片的大小
3. 将文件分割成指定数量的分片
4. 依次将每个分片上传到MinIO
5. 返回所有分片的上传结果

**响应:**
```json
{
    "status_code": 0,
    "status_msg": "success",
    "data": {
        "message": "All chunks uploaded successfully",
        "total_chunks": 5,
        "file_size": 104857600
    }
}
```

### 3. 完成上传 (VideoPublishCompleteV2)

**接口:** `POST /douyin/publish/complete/v2/`

**参数:**
- `uuid`: 上传会话UUID

**响应:**
```json
{
    "status_code": 0,
    "status_msg": "success",
    "data": {
        "video_id": 123,
        "video_url": "https://minio.example.com/videos/xxx.mp4"
    }
}
```

## 关键变更

### 1. 分片逻辑变更
- **之前**: 客户端负责分片，每次上传一个分片
- **现在**: 客户端上传完整文件，服务端自动分片

### 2. chunk_number参数含义变更
- **之前**: 表示当前分片的序号 (1, 2, 3...)
- **现在**: 表示要分片的总数量 (如5表示分成5个分片)

### 3. 服务端处理流程
1. 接收完整文件到内存
2. 计算分片大小: `fileSize / chunkNumber`
3. 循环处理每个分片:
   - 提取分片数据: `fileData[startOffset:endOffset]`
   - 计算分片MD5
   - 调用MinIO上传接口
   - 更新上传状态

## 优势

1. **简化客户端**: 客户端不需要处理分片逻辑
2. **灵活分片**: 服务端可以根据文件大小动态调整分片策略
3. **一致性**: 所有分片操作在服务端统一处理，保证一致性
4. **错误处理**: 服务端可以重试失败的分片上传

## 注意事项

1. **内存使用**: 服务端需要将完整文件加载到内存中
2. **文件大小限制**: 需要根据服务器内存配置合理的文件大小上限
3. **超时设置**: 大文件上传可能需要更长的超时时间
4. **错误恢复**: 如果某个分片上传失败，会继续尝试其他分片

## 示例

### 上传100MB视频文件，分成5个分片

```bash
# 1. 开始上传
curl -X POST "http://localhost:8080/douyin/publish/start/v2/" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "title=测试视频" \
  -F "description=这是一个测试视频" \
  -F "chunk_total_number=5"

# 2. 上传文件并分片
curl -X POST "http://localhost:8080/douyin/publish/uploading/v2/" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "uuid=generated-uuid-from-step1" \
  -F "chunk_number=5" \
  -F "filename=test_video.mp4" \
  -F "data=@/path/to/test_video.mp4"

# 3. 完成上传
curl -X POST "http://localhost:8080/douyin/publish/complete/v2/" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "uuid=generated-uuid-from-step1"
```

服务端会自动将100MB文件分成5个约20MB的分片，并依次上传到MinIO。
