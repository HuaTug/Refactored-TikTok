# -*- coding: utf-8 -*-
"""
批量创建测试用户 + 把本地 B 站视频通过 API 上传
=============================================

用法：
    python3 scripts/seed_users_upload_videos.py

功能：
  1. 通过 /v1/user/create/ 创建 5 个测试用户，密码统一 123456
     用户名：test_user_01 ~ test_user_05
     邮箱  ：test_user_01@example.com ... （带邮箱 -> 角色 user，不是 guest）
  2. 登录每个用户拿到 Access-Token
  3. 扫描本地 Bilibili 下载目录：
        /Users/zhihuaxu/Desktop/go/bilibili/output/videos_hot/<分类>/*.mp4
  4. 按分类均匀地分给 5 个用户，调用
        POST /v2/publish/start        -> 拿 upload_session_uuid
        POST /v2/publish/uploading    -> 分片上传（form-data "data"）
        POST /v2/publish/complete     -> 完成
     直到全部视频都上传完毕

注意：
  - 脚本是幂等的：如果用户已存在会返回 error，直接跳过；
    上传失败的视频会在末尾列出，不影响其他视频继续。
  - chunk_size 默认 4MB，每条视频分若干片顺序上传。
  - 视频分类直接映射为 tags 和 category，便于推荐系统使用。
"""
from __future__ import annotations

import os
import sys
import json
import time
import math
import mimetypes
import requests
from pathlib import Path

# ===================== 配置 =====================

API_BASE = "http://localhost:8888"
VIDEO_DIR = "/Users/zhihuaxu/Desktop/go/bilibili/output/videos_hot"

USERS = [
    {"username": "test_user_01", "email": "test_user_01@example.com", "sex": 1},
    {"username": "test_user_02", "email": "test_user_02@example.com", "sex": 2},
    {"username": "test_user_03", "email": "test_user_03@example.com", "sex": 1},
    {"username": "test_user_04", "email": "test_user_04@example.com", "sex": 2},
    {"username": "test_user_05", "email": "test_user_05@example.com", "sex": 1},
]
PASSWORD = "123456"

CHUNK_SIZE = 4 * 1024 * 1024       # 4 MB / chunk
REQ_TIMEOUT = 60                   # 单次 http 超时（秒）
MAX_UPLOAD_RETRY = 2               # 单分片失败重试次数


# ===================== 工具 =====================

def ok(resp_json) -> bool:
    """接口顶层 code = 10000 即算成功。"""
    return isinstance(resp_json, dict) and resp_json.get("code") == 10000


def _sleep():
    time.sleep(0.2)


def register(u: dict) -> None:
    r = requests.post(f"{API_BASE}/v1/user/create/",
                      json={
                          "username": u["username"],
                          "password": PASSWORD,
                          "email": u["email"],
                          "sex": u["sex"],
                      },
                      timeout=REQ_TIMEOUT)
    try:
        data = r.json()
    except Exception:
        data = {"raw": r.text}
    if ok(data):
        print(f"  [+] registered {u['username']}")
    else:
        # 多数情况是用户已存在；直接忽略
        msg = (data.get("message") or data.get("msg") or "").strip()
        print(f"  [-] skip register {u['username']}: {msg or data}")


def login(u: dict) -> str | None:
    r = requests.post(f"{API_BASE}/v1/user/login",
                      json={
                          "username": u["username"],
                          "password": PASSWORD,
                      },
                      timeout=REQ_TIMEOUT)
    try:
        data = r.json()
    except Exception:
        print(f"  [ERR] login {u['username']} invalid response: {r.text[:120]}")
        return None

    token = (data or {}).get("data", {}).get("token")
    uid = (data or {}).get("data", {}).get("user", {}).get("user_id")
    if token:
        print(f"  [✓] login {u['username']} user_id={uid}")
        return token
    print(f"  [ERR] login {u['username']} no token: {json.dumps(data)[:200]}")
    return None


def scan_videos(root: str) -> list[tuple[str, Path]]:
    """返回 [(category, path), ...]，category=目录名"""
    out: list[tuple[str, Path]] = []
    root_p = Path(root)
    if not root_p.exists():
        print(f"[ERR] video dir not found: {root}")
        return out
    for cat_dir in sorted(root_p.iterdir()):
        if not cat_dir.is_dir():
            continue
        for f in sorted(cat_dir.glob("*.mp4")):
            out.append((cat_dir.name, f))
    return out


# ===================== 上传流程 =====================

def publish_start(token: str, title: str, description: str,
                  category: str, tags: list[str], chunks: int) -> str | None:
    payload = {
        "title": title,
        "description": description,
        "category": category,
        "lab_name": ",".join(tags),   # 逗号分隔字符串
        "open": 1,                    # 1=public
        "chunk_total_number": chunks,
    }
    r = requests.post(f"{API_BASE}/v2/publish/start",
                      json=payload,
                      headers={"Access-Token": token,
                               "Content-Type": "application/json"},
                      timeout=REQ_TIMEOUT)
    try:
        data = r.json()
    except Exception:
        print(f"      [ERR] publish/start non-json: {r.text[:200]}")
        return None
    if not ok(data):
        print(f"      [ERR] publish/start: {data}")
        return None
    uuid = (data.get("data") or {}).get("upload_session_uuid")
    if not uuid:
        print(f"      [ERR] publish/start no uuid: {data}")
        return None
    return uuid


def publish_chunk(token: str, uuid: str, chunk_number: int,
                  filename: str, blob: bytes) -> bool:
    # 注意 handler 里通过 c.BindAndValidate 读 VideoPublishUploadingParam，
    # 所以 uuid / chunk_number / filename 作为 form 字段；文件作为 FormFile("data")。
    files = {
        "data": (filename, blob, "application/octet-stream"),
    }
    fields = {
        "uuid": uuid,
        "chunk_number": str(chunk_number),
        "filename": filename,
        "is_m3u8": "false",
    }
    last_err = None
    for attempt in range(MAX_UPLOAD_RETRY + 1):
        try:
            r = requests.post(f"{API_BASE}/v2/publish/uploading",
                              headers={"Access-Token": token},
                              data=fields,
                              files=files,
                              timeout=REQ_TIMEOUT * 3)
            if r.status_code != 200:
                last_err = f"status={r.status_code} body={r.text[:160]}"
                time.sleep(0.5)
                continue
            data = r.json()
            if ok(data):
                return True
            last_err = json.dumps(data)[:200]
        except Exception as e:
            last_err = str(e)
        time.sleep(0.5)
    print(f"      [ERR] chunk {chunk_number} fail: {last_err}")
    return False


def publish_complete(token: str, uuid: str) -> bool:
    r = requests.post(f"{API_BASE}/v2/publish/complete",
                      json={"uuid": uuid},
                      headers={"Access-Token": token,
                               "Content-Type": "application/json"},
                      timeout=REQ_TIMEOUT * 3)
    try:
        data = r.json()
    except Exception:
        print(f"      [ERR] complete non-json: {r.text[:160]}")
        return False
    if ok(data):
        return True
    print(f"      [ERR] complete: {data}")
    return False


def title_from_filename(path: Path) -> tuple[str, str]:
    """从 'BV1xx_标题.mp4' 里提取标题；返回 (title, desc)。"""
    stem = path.stem
    parts = stem.split("_", 1)
    if len(parts) == 2 and parts[0].startswith(("BV", "bv")):
        bvid, title = parts
        return title[:120], f"来源 B 站 {bvid}"
    return stem[:120], ""


def upload_one(token: str, category: str, path: Path) -> bool:
    size = path.stat().st_size
    chunks = max(1, math.ceil(size / CHUNK_SIZE))
    title, desc = title_from_filename(path)
    tags = [category, "爆款"]

    print(f"    ▸ [{category}] {path.name}  "
          f"({size/1024/1024:.1f}MB, {chunks} chunks)")

    uuid = publish_start(token, title, desc, category, tags, chunks)
    if not uuid:
        return False

    # 分片上传
    with path.open("rb") as f:
        for i in range(1, chunks + 1):
            blob = f.read(CHUNK_SIZE)
            if not blob:
                break
            if not publish_chunk(token, uuid, i, path.name, blob):
                return False
            print(f"      · chunk {i}/{chunks} ok ({len(blob)/1024/1024:.1f}MB)")

    if not publish_complete(token, uuid):
        return False
    print(f"      ✓ complete uuid={uuid[:8]}…")
    return True


# ===================== main =====================

def main() -> int:
    print("=" * 64)
    print("Step 1/3  注册 5 个测试用户")
    print("=" * 64)
    for u in USERS:
        register(u)
        _sleep()

    print("\n" + "=" * 64)
    print("Step 2/3  登录获取 token")
    print("=" * 64)
    tokens: dict[str, str] = {}
    for u in USERS:
        t = login(u)
        if t:
            tokens[u["username"]] = t
        _sleep()

    if not tokens:
        print("[fatal] 没有任何用户登录成功，退出")
        return 1

    print("\n" + "=" * 64)
    print("Step 3/3  扫描本地视频 + 上传")
    print("=" * 64)
    videos = scan_videos(VIDEO_DIR)
    print(f"  共 {len(videos)} 个 mp4，分 {len(set(c for c, _ in videos))} 个分类")

    # 轮询把视频分给 5 个用户
    usernames = list(tokens.keys())
    success = 0
    failures: list[tuple[str, str, str]] = []  # (user, category, filename)

    for i, (category, path) in enumerate(videos):
        uname = usernames[i % len(usernames)]
        token = tokens[uname]
        print(f"\n  [{i+1}/{len(videos)}] uploader={uname}")
        try:
            ok_ = upload_one(token, category, path)
        except Exception as e:
            ok_ = False
            print(f"      [EXC] {e}")
        if ok_:
            success += 1
        else:
            failures.append((uname, category, path.name))

    print("\n" + "=" * 64)
    print(f"完成：成功 {success}/{len(videos)}")
    print("=" * 64)
    if failures:
        print("失败列表：")
        for u, c, n in failures:
            print(f"  - [{u}] [{c}] {n}")
    return 0 if not failures else 2


if __name__ == "__main__":
    sys.exit(main())
