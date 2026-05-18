# -*- coding: utf-8 -*-
"""
P05 · 大文件分片上传 + 模拟断网续传
====================================

流程：
  1. 调 /v2/publish/start，拿 upload_session_uuid
  2. 按 5MB / 片读取本地文件，逐片 POST /v2/publish/uploading
  3. 第 20 片**主动抛错**模拟断网，跳过该片
  4. 在循环结束后再补传"曾经失败"的片
  5. 调 /v2/publish/complete，拿到 video_id
  6. 拉取 video_id 对应的 video_url，下载回本地，比对 MD5

通过标准：
  - 本地文件 MD5 == 后端合并产物 MD5
  - 每片只成功落地 1 次（手动补传不会重复写入）

用法：
  python3 scripts/perf/p05_chunk_upload.py /path/to/200mb.mp4
  TOKEN=xxx API_BASE=http://localhost:8888 \
    python3 scripts/perf/p05_chunk_upload.py /path/to/200mb.mp4
"""
from __future__ import annotations

import csv
import hashlib
import math
import os
import sys
import time
from pathlib import Path
import argparse
import requests

CHUNK_SIZE = 5 * 1024 * 1024  # 5MB
DEFAULT_FAIL_AT = 20          # 第 20 片故意失败
API_BASE = os.environ.get("API_BASE", "http://localhost:8888")


def load_token(arg_token: str | None) -> str:
    if arg_token:
        return arg_token
    env = os.environ.get("TOKEN")
    if env:
        return env
    # 兜底：读 tokens.csv 第一行
    csv_path = Path(__file__).resolve().parent / "tokens.csv"
    if csv_path.exists():
        with csv_path.open() as f:
            r = csv.DictReader(f)
            for row in r:
                if row.get("token"):
                    return row["token"]
    sys.exit("缺少 token：请通过 -t / --token 或环境变量 TOKEN 提供")


def md5_of(path: Path) -> str:
    h = hashlib.md5()
    with path.open("rb") as f:
        for blk in iter(lambda: f.read(1 << 20), b""):
            h.update(blk)
    return h.hexdigest()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("file", help="本地 mp4 文件路径")
    ap.add_argument("-t", "--token", help="Access-Token（默认读 tokens.csv 或 env）")
    ap.add_argument("--fail-at", type=int, default=DEFAULT_FAIL_AT,
                    help=f"第几片故意失败，模拟断网（默认 {DEFAULT_FAIL_AT}，0=不失败）")
    ap.add_argument("--title", default="P05_chunk_resume_test")
    args = ap.parse_args()

    src = Path(args.file).resolve()
    if not src.exists():
        sys.exit(f"文件不存在: {src}")
    token = load_token(args.token)
    size = src.stat().st_size
    chunks = max(1, math.ceil(size / CHUNK_SIZE))
    src_md5 = md5_of(src)
    print(f"file={src.name}  size={size/1024/1024:.1f}MB  chunks={chunks}  md5={src_md5}")

    # ---- 1. start ----
    r = requests.post(f"{API_BASE}/v2/publish/start",
                      headers={"Access-Token": token, "Content-Type": "application/json"},
                      json={
                          "title": args.title,
                          "description": "P05 chunk upload + resume test",
                          "lab_name": "perf,resume",
                          "category": "知识",
                          "open": 1,
                          "chunk_total_number": chunks,
                      }, timeout=30)
    data = r.json()
    if data.get("code") != 10000:
        sys.exit(f"start 失败: {data}")
    uuid = (data["data"] or {}).get("upload_session_uuid")
    print(f"upload_session_uuid = {uuid}")

    # ---- 2. 上传分片，第 fail_at 片故意跳过 ----
    failed_chunks: list[int] = []
    with src.open("rb") as f:
        for i in range(1, chunks + 1):
            blob = f.read(CHUNK_SIZE)
            if not blob:
                break
            if i == args.fail_at:
                print(f"  · chunk {i}/{chunks} -- [模拟断网] 故意跳过")
                failed_chunks.append(i)
                continue
            ok = upload_chunk(token, uuid, i, src.name, blob)
            print(f"  · chunk {i}/{chunks} {'OK' if ok else 'FAIL'} ({len(blob)/1024/1024:.1f}MB)")
            if not ok:
                failed_chunks.append(i)

    # ---- 3. 网络恢复，补传 ----
    if failed_chunks:
        print(f"\n[recover] 补传 {len(failed_chunks)} 片: {failed_chunks}")
        with src.open("rb") as f:
            for i in failed_chunks:
                f.seek((i - 1) * CHUNK_SIZE)
                blob = f.read(CHUNK_SIZE)
                ok = upload_chunk(token, uuid, i, src.name, blob)
                print(f"  · resume chunk {i}: {'OK' if ok else 'FAIL'}")
                if not ok:
                    sys.exit(f"补传失败 chunk={i}")

    # ---- 4. complete ----
    r = requests.post(f"{API_BASE}/v2/publish/complete",
                      headers={"Access-Token": token, "Content-Type": "application/json"},
                      json={"uuid": uuid}, timeout=180)
    data = r.json()
    if data.get("code") != 10000:
        sys.exit(f"complete 失败: {data}")
    video_id = (data["data"] or {}).get("video_id")
    print(f"\ncomplete OK, video_id={video_id}")

    # ---- 5. 等转码 + 比对 MD5 ----
    print("\n等待转码 30s ...")
    time.sleep(30)

    # 优先尝试通过流代理拿原片 MD5
    play_url = f"{API_BASE}/v1/stream/video?video_id={video_id}"
    print(f"download to verify: {play_url}")
    dl_path = Path(f"/tmp/p05_dl_{video_id}.mp4")
    with requests.get(play_url, headers={"Access-Token": token},
                      stream=True, timeout=600) as g:
        g.raise_for_status()
        with dl_path.open("wb") as out:
            for blk in g.iter_content(1 << 20):
                out.write(blk)
    dst_md5 = md5_of(dl_path)
    print(f"\n本地  MD5 = {src_md5}")
    print(f"服务  MD5 = {dst_md5}")
    print("一致 ✅" if src_md5 == dst_md5 else "不一致 ❌（请检查后端是否对原始流做了转码再返回）")
    return 0 if src_md5 == dst_md5 else 1


def upload_chunk(token, uuid, chunk_no, filename, blob, retries=2):
    for _ in range(retries + 1):
        try:
            r = requests.post(f"{API_BASE}/v2/publish/uploading",
                              headers={"Access-Token": token},
                              data={"uuid": uuid, "chunk_number": str(chunk_no),
                                    "filename": filename, "is_m3u8": "false"},
                              files={"data": (filename, blob,
                                              "application/octet-stream")},
                              timeout=180)
            if r.status_code == 200 and r.json().get("code") == 10000:
                return True
        except requests.exceptions.RequestException as e:
            print(f"      [exc] {e}")
        time.sleep(0.5)
    return False


if __name__ == "__main__":
    sys.exit(main())
