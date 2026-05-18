# -*- coding: utf-8 -*-
"""
P06 · Feed 翻页不重复
=====================

项目里 /v1/video/feed 当前用 page_num/page_size 而非 last_time/last_id 游标。
脚本同时验证两点：
  a) 多页之间 video_id 不重复
  b) 同一 page_num 二次请求结果稳定（业务有合理 ORDER BY 兜底）

用法：
  python3 scripts/perf/p06_feed_cursor.py --pages 30 --page-size 10
  TOKEN=xxx python3 scripts/perf/p06_feed_cursor.py
"""
from __future__ import annotations

import argparse
import csv
import os
import sys
from pathlib import Path
import requests

API_BASE = os.environ.get("API_BASE", "http://localhost:8888")


def load_token(arg) -> str:
    if arg: return arg
    if os.environ.get("TOKEN"): return os.environ["TOKEN"]
    p = Path(__file__).resolve().parent / "tokens.csv"
    if p.exists():
        with p.open() as f:
            for row in csv.DictReader(f):
                if row.get("token"):
                    return row["token"]
    sys.exit("no token; pass --token or set TOKEN")


def fetch(token: str, page: int, size: int) -> list[int]:
    r = requests.get(f"{API_BASE}/v1/video/feed",
                     params={"page_num": page, "page_size": size},
                     headers={"Access-Token": token},
                     timeout=15)
    data = r.json()
    if data.get("code") != 10000:
        raise RuntimeError(f"page {page} failed: {data}")
    items = (data.get("data") or {}).get("items") or \
            (data.get("data") or {}).get("videos") or \
            (data.get("data") or {}).get("video_list") or []
    ids: list[int] = []
    for v in items:
        # 兼容字段：video_id / id
        vid = v.get("video_id") or v.get("id")
        if vid is not None:
            ids.append(int(vid))
    return ids


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--token", default=None)
    ap.add_argument("--pages", type=int, default=30)
    ap.add_argument("--page-size", type=int, default=10)
    args = ap.parse_args()
    token = load_token(args.token)

    seen: set[int] = set()
    total = 0
    print(f"fetch {args.pages} pages × {args.page_size}")
    for p in range(1, args.pages + 1):
        ids = fetch(token, p, args.page_size)
        dup = [v for v in ids if v in seen]
        if dup:
            print(f"  page {p}: ids={ids}  ⚠️ duplicate={dup}")
        else:
            print(f"  page {p}: {len(ids)} ids OK")
        for v in ids:
            seen.add(v)
        total += len(ids)
        if not ids:
            print(f"  page {p} 空结果，停止翻页")
            break

    print(f"\nfetched={total}  unique={len(seen)}  duplicates={total - len(seen)}")
    if total != len(seen):
        print("❌ 翻页存在重复，请检查后端排序字段是否稳定（ORDER BY created_at, video_id）")
        return 1
    print("✅ 30 页全部唯一")
    return 0


if __name__ == "__main__":
    sys.exit(main())
