# -*- coding: utf-8 -*-
"""
为推荐系统召回 / 粗排灌入必要的索引数据
======================================

依赖前置：scripts/seed_user_actions.py 已执行（user_video_interactions / video_likes 已有）。

写入 MySQL：
  - video_features  : 让 popularity 召回看到新视频，并按 hot_score / ctr 排
  - video_hot_scores : video hot score time series（如果 Redis 失败可降级）

写入 Redis：
  - hot:video:hour:YYYYMMDDHH        : 1 小时热榜
  - hot:video:day:YYYYMMDD           : 24 小时热榜
  - hot:video:week                   : 7 天热榜
  - hot:video:realtime               : 实时榜
  - user:interests:{uid}             : 用户兴趣 tag (sorted set，score = 偏好强度)
  - user:category_prefer:{uid}       : 用户类目偏好 (sorted set)
  - tag:videos:{tag}                 : tag -> 视频列表
  - category:videos:{cat}            : 类目 -> 视频列表
  - user:likes:{uid}                 : 用户点赞列表（sorted set）
  - user:watch_history:{uid}         : 观看历史（sorted set）
"""
from __future__ import annotations

import collections
import json
import math
from datetime import datetime, timedelta
from typing import Dict, List, Tuple

import pymysql
import redis

MYSQL = dict(host="127.0.0.1", port=3307, user="root",
             password="TikTok@MySQL#2025!Secure", database="TikTok",
             charset="utf8mb4")
REDIS = dict(host="127.0.0.1", port=6379,
             password="Redis@TikTok2025_SecurePass", decode_responses=True)


def cnx(): return pymysql.connect(**MYSQL, autocommit=False)
def rcli(): return redis.Redis(**REDIS)


def step_1_video_features(cur):
    """从 user_video_interactions / recommendation_exposures 聚合每个视频的 features。
    清空旧的 features 重新写入（确保新视频也被覆盖）。"""
    cur.execute("""
        SELECT v.video_id,
               COALESCE(SUM(uvi.impression_count), 0)                 AS exposure_count,
               COALESCE(SUM(uvi.click_count), 0)                      AS click_count,
               COALESCE(AVG(uvi.max_watch_progress), 0)               AS finish_rate,
               COALESCE(SUM(uvi.is_liked), 0)                         AS like_count,
               COALESCE(SUM(uvi.is_favorited), 0)                     AS fav_count,
               COALESCE(AVG(uvi.engagement_score), 0)                 AS engage,
               UNIX_TIMESTAMP(NOW()) - UNIX_TIMESTAMP(v.created_at)   AS age_sec
          FROM videos v
     LEFT JOIN user_video_interactions uvi ON uvi.video_id = v.video_id
         WHERE v.open = 1 AND v.audit_status = 1 AND v.deleted_at IS NULL
         GROUP BY v.video_id, v.created_at
    """)
    rows = cur.fetchall()
    print(f"  computing features for {len(rows)} videos")

    cur.execute("DELETE FROM video_features")
    inserts = 0
    fps: List[Tuple[int, float, float, float, int]] = []  # (vid, popularity, ctr, like_rate, fav_count)

    for vid, exp, clk, finish, lk, fv, eng, age_sec in rows:
        exp = int(exp or 0)
        clk = int(clk or 0)
        ctr = (clk / exp) if exp > 0 else 0.0
        like_rate = (int(lk or 0) / exp) if exp > 0 else 0.0
        fav_rate = (int(fv or 0) / exp) if exp > 0 else 0.0
        finish = float(finish or 0)
        # 热度：曝光 + 点击 + 互动；带时间衰减
        age_days = max(1, (int(age_sec or 0) / 86400.0))
        decay = 1.0 / (1.0 + math.log(1 + age_days))
        popularity = (exp * 0.1 + clk * 1.0 + int(lk or 0) * 3.0 + int(fv or 0) * 5.0) * decay
        # 给新视频一个底线热度，避免完全沉没
        if popularity < 1:
            popularity = 1.0 + (1.0 - decay) * 5
        quality = min(99.99, 50 + finish * 30 + ctr * 20)
        freshness = max(0, 100 - age_days)
        is_hq = 1 if (quality > 70 and ctr > 0.4) else 0

        cur.execute("""
            INSERT INTO video_features
              (video_id, quality_score, popularity_score, freshness_score,
               ctr, finish_rate, like_rate, comment_rate, share_rate, favorite_rate,
               interact_score, avg_watch_duration, exposure_count, click_count,
               author_score, is_high_quality)
            VALUES (%s,%s,%s,%s,%s,%s,%s,0,0,%s,%s,0,%s,%s,60,%s)
        """, (vid, round(quality, 2), round(popularity, 2), round(freshness, 2),
              round(ctr, 6), round(finish, 4), round(like_rate, 4), round(fav_rate, 4),
              round(float(eng or 0), 2), exp, clk, is_hq))
        inserts += 1
        fps.append((int(vid), popularity, ctr, like_rate, int(fv or 0)))
    print(f"  video_features rows: {inserts}")
    return fps


def step_2_redis_hot(rc, fps):
    """把 video 按 popularity 写到 4 个热榜 ZSET"""
    now = datetime.now()
    keys = [
        f"hot:video:hour:{now.strftime('%Y%m%d%H')}",
        f"hot:video:day:{now.strftime('%Y%m%d')}",
        "hot:video:week",
        "hot:video:realtime",
    ]
    pipe = rc.pipeline()
    for k in keys:
        pipe.delete(k)
        for vid, pop, ctr, lr, fv in fps:
            # 不同 key 用略不同分数，让它们有差异
            score = pop
            pipe.zadd(k, {str(vid): float(score)})
        pipe.expire(k, 7 * 86400)
    pipe.execute()
    print(f"  redis hot keys written: {len(keys)} (each with {len(fps)} videos)")


def step_3_redis_tag_category(rc, cur):
    """tag:videos:{tag} 和 category:videos:{cat}"""
    cur.execute("""
        SELECT v.video_id, v.category, v.label_names, vf.popularity_score
          FROM videos v
          JOIN video_features vf ON vf.video_id = v.video_id
         WHERE v.open=1 AND v.audit_status=1
    """)
    pipe = rc.pipeline()
    cat_set = collections.defaultdict(list)
    tag_set = collections.defaultdict(list)
    for vid, cat, labels, pop in cur.fetchall():
        if cat:
            cat_set[cat].append((vid, float(pop or 0)))
        if labels:
            for tag in str(labels).split(","):
                tag = tag.strip()
                if tag:
                    tag_set[tag].append((vid, float(pop or 0)))
    for cat, vids in cat_set.items():
        key = f"category:videos:{cat}"
        pipe.delete(key)
        for vid, score in vids:
            pipe.zadd(key, {str(vid): score})
        pipe.expire(key, 7 * 86400)
    for tag, vids in tag_set.items():
        key = f"tag:videos:{tag}"
        pipe.delete(key)
        for vid, score in vids:
            pipe.zadd(key, {str(vid): score})
        pipe.expire(key, 7 * 86400)
    pipe.execute()
    print(f"  redis category:videos:* keys: {len(cat_set)}; tag:videos:* keys: {len(tag_set)}")


def step_4_redis_user_prefs(rc, cur):
    """user:interests / user:category_prefer / user:likes / user:watch_history"""
    cur.execute("""
        SELECT uvi.user_id, v.category, v.label_names,
               uvi.is_liked, uvi.max_watch_progress, uvi.video_id,
               uvi.last_interact_at
          FROM user_video_interactions uvi
          JOIN videos v ON v.video_id = uvi.video_id
         WHERE uvi.user_id BETWEEN 8 AND 12
    """)
    interests: Dict[int, collections.Counter] = collections.defaultdict(collections.Counter)
    cat_prefer: Dict[int, collections.Counter] = collections.defaultdict(collections.Counter)
    likes: Dict[int, List[Tuple[int, float]]] = collections.defaultdict(list)
    watches: Dict[int, List[Tuple[int, float]]] = collections.defaultdict(list)

    for uid, cat, labels, lk, prog, vid, last_t in cur.fetchall():
        weight = max(0.1, float(prog or 0))   # progress 高 -> 偏好高
        if cat:
            cat_prefer[uid][cat] += weight
        if labels:
            for tag in str(labels).split(","):
                tag = tag.strip()
                if tag:
                    interests[uid][tag] += weight
        ts = last_t.timestamp() if last_t else 0
        if int(lk or 0):
            likes[uid].append((int(vid), ts))
        watches[uid].append((int(vid), ts))

    pipe = rc.pipeline()
    for uid in range(8, 13):
        # user:interests
        ikey = f"user:interests:{uid}"
        pipe.delete(ikey)
        for tag, w in interests[uid].most_common(50):
            pipe.zadd(ikey, {tag: float(w)})
        pipe.expire(ikey, 30 * 86400)
        # user:category_prefer
        ckey = f"user:category_prefer:{uid}"
        pipe.delete(ckey)
        for cat, w in cat_prefer[uid].most_common(20):
            pipe.zadd(ckey, {cat: float(w)})
        pipe.expire(ckey, 30 * 86400)
        # user:likes
        lkey = f"user:likes:{uid}"
        pipe.delete(lkey)
        for vid, ts in likes[uid]:
            pipe.zadd(lkey, {str(vid): float(ts)})
        pipe.expire(lkey, 30 * 86400)
        # user:watch_history
        wkey = f"user:watch_history:{uid}"
        pipe.delete(wkey)
        for vid, ts in watches[uid]:
            pipe.zadd(wkey, {str(vid): float(ts)})
        pipe.expire(wkey, 30 * 86400)
    pipe.execute()
    print(f"  redis user:* prefs/likes/watch_history written for uid 8-12")


def main():
    cn = cnx()
    rc = rcli()
    cur = cn.cursor()

    print("=" * 64)
    print("Step 1  写 video_features 表")
    print("=" * 64)
    fps = step_1_video_features(cur)
    cn.commit()

    print("\nStep 2  写 Redis 热榜（4 个 ZSET）")
    step_2_redis_hot(rc, fps)

    print("\nStep 3  写 Redis tag/category 倒排")
    step_3_redis_tag_category(rc, cur)

    print("\nStep 4  写 Redis 用户偏好 / 点赞 / 历史")
    step_4_redis_user_prefs(rc, cur)

    cn.close()
    print("\n[done]")


if __name__ == "__main__":
    main()
