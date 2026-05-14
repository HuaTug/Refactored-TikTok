#!/usr/bin/env python3
"""
批量注册用户并上传 bili_videos 目录下的视频到 TikTok 后端。
支持断点续传：通过 progress.json 跟踪已上传文件，重跑时自动跳过。
用法: python3 scripts/batch_upload.py
"""

import os
import sys
import math
import time
import json
import requests

API_BASE = "http://localhost:8888"
VIDEOS_DIR = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "..", "bili_videos")
CHUNK_SIZE = 5 * 1024 * 1024  # 5MB per chunk
NUM_USERS = 5
MAX_RETRIES = 3          # 每个视频最多重试次数
UPLOAD_INTERVAL = 1.0    # 每次上传间隔（秒），防止服务过载
PROGRESS_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "upload_progress.json")

# 目录名 → (category, tags) 映射
CATEGORY_MAP = {
    "NBA精彩集锦": ("运动", "NBA,篮球,集锦"),
    "NBA名场面": ("运动", "NBA,篮球,名场面"),
    "黑神话悟空": ("游戏", "黑神话悟空,国产游戏,动作"),
    "鬼畜名场面": ("搞笑", "鬼畜,搞笑,名场面"),
    "高效学习方法": ("知识", "学习方法,效率,自律"),
    "跑步运动vlog": ("运动", "跑步,运动,vlog"),
    "说唱freestyle": ("音乐", "说唱,freestyle,嘻哈"),
    "詹姆斯扣篮合集": ("运动", "NBA,詹姆斯,扣篮"),
    "英雄联盟精彩操作": ("游戏", "英雄联盟,LOL,精彩操作"),
    "英语四六级备考": ("知识", "英语,四六级,备考"),
    "考研经验分享": ("知识", "考研,经验分享,学习"),
    "网易云热歌榜": ("音乐", "网易云,热歌,音乐"),
    "科比高光时刻": ("运动", "NBA,科比,高光时刻"),
    "深夜食堂": ("美食", "美食,深夜食堂,治愈"),
    "校园风景航拍": ("生活", "校园,航拍,风景"),
    "校园篮球赛": ("运动", "校园,篮球赛,大学生"),
    "校园恋爱故事": ("生活", "校园,恋爱,青春"),
    "搞笑短剧合集": ("搞笑", "搞笑,短剧,喜剧"),
    "库里三分球集锦": ("运动", "NBA,库里,三分球"),
    "大学食堂美食": ("美食", "大学,食堂,美食"),
    "大学社团招新": ("生活", "大学,社团,招新"),
    "大学生街头采访": ("生活", "大学生,街头采访,随机"),
    "大学生考试周崩溃瞬间": ("搞笑", "大学生,考试周,崩溃"),
    "大学生考公上岸": ("知识", "考公,上岸,大学生"),
    "大学生翻唱": ("音乐", "翻唱,大学生,校园"),
    "大学生穷游攻略": ("旅行", "穷游,攻略,大学生"),
    "大学生游戏日常": ("游戏", "大学生,游戏,日常"),
    "大学生期末周": ("搞笑", "大学生,期末,考试"),
    "大学生时间管理": ("知识", "时间管理,大学生,自律"),
    "大学生日常vlog": ("生活", "大学生,日常,vlog"),
    "大学生整活": ("搞笑", "大学生,整活,搞笑"),
    "大学生才艺展示": ("生活", "大学生,才艺,表演"),
    "大学生开学": ("生活", "大学生,开学,新学期"),
    "大学生宿舍神器": ("生活", "大学生,宿舍,好物"),
    "大学生健身": ("运动", "大学生,健身,运动"),
    "大学毕业季": ("生活", "大学,毕业季,青春"),
    "大学期末复习": ("知识", "大学,期末复习,学习"),
    "大学宿舍搞笑日常": ("搞笑", "大学,宿舍,搞笑日常"),
    "大学军训": ("生活", "大学,军训,新生"),
    "城市夜景vlog": ("生活", "城市,夜景,vlog"),
    "周杰伦经典歌曲": ("音乐", "周杰伦,经典,华语"),
    "吉他弹唱校园": ("音乐", "吉他,弹唱,校园"),
    "古风音乐纯享版": ("音乐", "古风,音乐,纯享"),
    "古风歌曲合集": ("音乐", "古风,歌曲,合集"),
    "原神": ("游戏", "原神,开放世界,二次元"),
    "伤感音乐合集": ("音乐", "伤感,音乐,情感"),
    "二次元神曲": ("音乐", "二次元,动漫,神曲"),
    "乔丹经典比赛": ("运动", "NBA,乔丹,经典"),
    "i love u so": ("音乐", "英文歌,情歌,日推"),
}


def load_progress():
    """加载已上传进度"""
    if os.path.exists(PROGRESS_FILE):
        with open(PROGRESS_FILE, "r") as f:
            return json.load(f)
    return {"uploaded": []}


def save_progress(progress):
    """保存上传进度"""
    with open(PROGRESS_FILE, "w") as f:
        json.dump(progress, f, ensure_ascii=False, indent=2)


def register_user(username, password):
    """注册用户，返回是否成功"""
    resp = requests.post(f"{API_BASE}/v1/user/create/", json={
        "username": username,
        "password": password,
        "email": f"{username}@test.com",
        "sex": 1,
    }, timeout=10)
    data = resp.json()
    if data.get("code") == 10000 or "exist" in str(data).lower() or "duplicate" in str(data).lower():
        print(f"  [OK] 用户 {username} 注册成功/已存在")
        return True
    print(f"  [WARN] 用户 {username} 注册响应: {data}")
    return True  # 继续尝试登录


def login_user(username, password):
    """登录用户，返回 access token"""
    resp = requests.post(f"{API_BASE}/v1/user/login", json={
        "username": username,
        "password": password,
    }, timeout=10)
    data = resp.json()
    token = None
    if "data" in data and data["data"]:
        token = data["data"].get("token")
    if not token:
        print(f"  [ERROR] 用户 {username} 登录失败: {data}")
        return None
    print(f"  [OK] 用户 {username} 登录成功, token={token[:20]}...")
    return token


def upload_video(token, filepath, title, description, tags, category):
    """通过分片上传流程上传一个视频，带超时保护"""
    filesize = os.path.getsize(filepath)
    total_chunks = max(1, math.ceil(filesize / CHUNK_SIZE))
    filename = os.path.basename(filepath)

    # Step 1: Start upload session
    start_resp = requests.post(f"{API_BASE}/v2/publish/start",
        headers={"Access-Token": token},
        json={
            "title": title,
            "description": description,
            "lab_name": tags,
            "category": category,
            "open": 1,
            "chunk_total_number": total_chunks,
        }, timeout=60)
    start_data = start_resp.json()

    uuid = None
    if "data" in start_data and start_data["data"]:
        uuid = start_data["data"].get("upload_session_uuid") or start_data["data"].get("uuid")
    if not uuid:
        print(f"    [ERROR] 创建上传会话失败: {start_data}")
        return False

    # Step 2: Upload chunks
    with open(filepath, "rb") as f:
        for chunk_num in range(1, total_chunks + 1):
            chunk_data = f.read(CHUNK_SIZE)
            if not chunk_data:
                break

            upload_resp = requests.post(f"{API_BASE}/v2/publish/uploading",
                headers={"Access-Token": token},
                data={
                    "uuid": uuid,
                    "chunk_number": chunk_num,
                    "filename": filename,
                },
                files={"data": (filename, chunk_data, "application/octet-stream")},
                timeout=180)
            up_data = upload_resp.json()
            if up_data.get("code") != 10000:
                print(f"    [ERROR] 分片 {chunk_num}/{total_chunks} 上传失败: {up_data}")
                return False

    # Step 3: Complete upload
    complete_resp = requests.post(f"{API_BASE}/v2/publish/complete",
        headers={"Access-Token": token},
        json={"uuid": uuid},
        timeout=180)
    comp_data = complete_resp.json()
    if comp_data.get("code") != 10000:
        print(f"    [ERROR] 完成上传失败: {comp_data}")
        return False

    video_id = comp_data.get("data", {}).get("video_id", "?")
    print(f"    [OK] video_id={video_id}")
    return True


def collect_videos():
    """收集所有视频文件及其元信息"""
    videos = []
    videos_dir = os.path.normpath(VIDEOS_DIR)

    for dirname in sorted(os.listdir(videos_dir)):
        dirpath = os.path.join(videos_dir, dirname)
        if not os.path.isdir(dirpath):
            continue

        cat_info = CATEGORY_MAP.get(dirname, ("生活", dirname))
        category, tags = cat_info

        for fname in sorted(os.listdir(dirpath)):
            if not fname.lower().endswith(".mp4"):
                continue
            filepath = os.path.join(dirpath, fname)

            # 从文件名提取标题（去掉编号前缀和扩展名）
            name = os.path.splitext(fname)[0]
            # 去掉开头的数字编号（如 "001_xxx" → "xxx"）
            parts = name.split("_", 1)
            title = parts[1] if len(parts) > 1 else name
            # 截断过长的标题
            if len(title) > 200:
                title = title[:200]

            description = f"{dirname} - {title}"

            videos.append({
                "filepath": filepath,
                "title": title,
                "description": description,
                "tags": tags,
                "category": category,
                "dirname": dirname,
            })

    return videos


def main():
    print("=" * 60)
    print("批量上传 bili_videos 到 TikTok 后端")
    print("=" * 60)

    # 0. 加载进度
    progress = load_progress()
    uploaded_set = set(progress["uploaded"])
    if uploaded_set:
        print(f"\n[断点续传] 已上传 {len(uploaded_set)} 个视频，将跳过这些文件")

    # 1. 收集所有视频
    videos = collect_videos()
    print(f"\n共发现 {len(videos)} 个视频文件")

    if not videos:
        print("未找到视频文件，请检查 bili_videos 目录")
        sys.exit(1)

    # 过滤掉已上传的视频
    pending_videos = [v for v in videos if v["filepath"] not in uploaded_set]
    print(f"待上传: {len(pending_videos)} 个视频")

    if not pending_videos:
        print("所有视频已上传完毕！")
        return

    # 2. 注册并登录 5 个用户
    print(f"\n--- 注册 {NUM_USERS} 个用户 ---")
    tokens = []
    for i in range(1, NUM_USERS + 1):
        username = f"test{i}"
        password = "123456"
        register_user(username, password)
        token = login_user(username, password)
        if token:
            tokens.append(token)
        else:
            print(f"  [FATAL] 用户 {username} 无法登录，跳过")

    if not tokens:
        print("无可用用户 token，退出")
        sys.exit(1)

    print(f"\n成功获取 {len(tokens)} 个用户的 token")

    # 3. 将待上传视频平分给用户
    print(f"\n--- 开始上传 {len(pending_videos)} 个视频 ---")
    success_count = 0
    fail_count = 0
    skip_count = len(uploaded_set)
    start_time = time.time()

    for idx, video in enumerate(pending_videos):
        # 根据在总列表中的位置分配用户
        total_idx = videos.index(video)
        user_idx = total_idx % len(tokens)
        token = tokens[user_idx]
        username = f"test{user_idx + 1}"

        size_mb = os.path.getsize(video["filepath"]) / (1024 * 1024)
        print(f"\n[{idx+1}/{len(pending_videos)}] {username} 上传: {video['title'][:50]}... ({size_mb:.1f}MB) [{video['category']}]")

        uploaded = False
        for attempt in range(1, MAX_RETRIES + 1):
            try:
                ok = upload_video(
                    token=token,
                    filepath=video["filepath"],
                    title=video["title"],
                    description=video["description"],
                    tags=video["tags"],
                    category=video["category"],
                )
                if ok:
                    success_count += 1
                    uploaded = True
                    # 记录进度
                    progress["uploaded"].append(video["filepath"])
                    save_progress(progress)
                    break
                else:
                    if attempt < MAX_RETRIES:
                        wait = attempt * 3
                        print(f"    [RETRY] 第 {attempt}/{MAX_RETRIES} 次失败，{wait}秒后重试...")
                        time.sleep(wait)
            except requests.exceptions.ConnectionError as e:
                print(f"    [CONNECTION ERROR] {e}")
                if attempt < MAX_RETRIES:
                    wait = attempt * 5
                    print(f"    [RETRY] 连接失败，{wait}秒后重试...")
                    time.sleep(wait)
            except Exception as e:
                print(f"    [EXCEPTION] {e}")
                if attempt < MAX_RETRIES:
                    wait = attempt * 3
                    print(f"    [RETRY] {wait}秒后重试...")
                    time.sleep(wait)

        if not uploaded:
            fail_count += 1
            print(f"    [FAILED] 视频上传最终失败（已重试 {MAX_RETRIES} 次）")

        # 上传间隔，防止服务过载
        time.sleep(UPLOAD_INTERVAL)

    elapsed = time.time() - start_time
    print(f"\n{'=' * 60}")
    print(f"上传完成!")
    print(f"  本次成功: {success_count}")
    print(f"  本次失败: {fail_count}")
    print(f"  之前已传: {skip_count}")
    print(f"  总计完成: {success_count + skip_count}/{len(videos)}")
    print(f"  耗时: {elapsed:.1f}s")
    print(f"{'=' * 60}")


if __name__ == "__main__":
    main()
