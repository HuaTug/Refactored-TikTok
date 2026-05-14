# -*- coding: utf-8 -*-
"""
为 5 个测试用户灌入模拟的观看 / 点赞 / 曝光行为
================================================

写入：
  1. videos                          : 把新上传的 24 条视频 open=1, audit_status=1
  2. recommendation_exposures        : 主要训练样本来源（含 is_clicked / completion_rate / is_liked）
  3. user_video_watch_histories      : 观看历史
  4. user_video_interactions         : 用户-视频聚合统计
  5. user_behaviors                  : 行为流水（view/like/favorite）
  6. video_likes                     : 点赞明细
  7. user_profiles                   : 长期画像（avg_completion_rate / total_actions）
  8. Redis: user:recent_actions:{uid}: 实时行为流（让 Agent 不再走 fallback）

规则：
  - 每个用户预设 3 个偏好分类 + 1 个厌恶分类
  - 偏好类视频：completion_rate=0.7~1.0，70% 点赞，40% 收藏
  - 厌恶类视频：completion_rate=0.05~0.2，0% 点赞
  - 其他：completion_rate=0.3~0.6
  - 每个用户每个视频：1~3 次曝光（recommendation_exposures 多行）
"""
from __future__ import annotations

import json
import random
import time
from datetime import datetime, timedelta
from typing import Dict, List, Tuple

import pymysql
import redis

# ===================== 配置 =====================
MYSQL = dict(host="127.0.0.1", port=3307, user="root",
             password="TikTok@MySQL#2025!Secure", database="TikTok",
             charset="utf8mb4")
REDIS = dict(host="127.0.0.1", port=6379,
             password="Redis@TikTok2025_SecurePass", decode_responses=True)

# 用户偏好画像
USER_PREFS: Dict[int, Dict[str, List[str]]] = {
    8:  {"likes": ["周杰伦", "纯音乐高码率"],                       "dislikes": ["影视剪辑混剪"]},
    9:  {"likes": ["Coldplay与欧美经典", "励志演讲"],               "dislikes": ["历史向解说"]},
    10: {"likes": ["历史向解说", "毛主席", "西北大学"],              "dislikes": ["周杰伦"]},
    11: {"likes": ["影视剪辑混剪", "励志演讲"],                    "dislikes": ["毛主席"]},
    12: {"likes": ["周杰伦", "Coldplay与欧美经典", "影视剪辑混剪"], "dislikes": ["西北大学"]},
}

EXPOSURES_PER_VIDEO = 8     # 每条视频每个用户曝光 5~10 次（之前是 1~2）
EXPOSURES_MIN = 5
NEW_VIDEO_USER_RANGE = (8, 12)
NEW_VIDEO_ID_MIN = 100      # 我们上传的视频 video_id 都 > 100

random.seed(42)


# ===================== 工具 =====================

def conn_mysql():
    return pymysql.connect(**MYSQL, autocommit=False)


def conn_redis():
    return redis.Redis(**REDIS)


def random_ts(days_back: int) -> datetime:
    """过去 N 天内的随机时间。"""
    delta = random.uniform(0, days_back * 86400)
    return datetime.now() - timedelta(seconds=delta)


def category_rating(uid: int, cat: str) -> str:
    """返回 like / dislike / neutral"""
    pref = USER_PREFS.get(uid, {})
    if cat in pref.get("likes", []):
        return "like"
    if cat in pref.get("dislikes", []):
        return "dislike"
    return "neutral"


def gen_completion(rating: str) -> float:
    if rating == "like":
        return round(random.uniform(0.80, 1.0), 4)     # 高完播
    if rating == "dislike":
        return round(random.uniform(0.02, 0.10), 4)    # 极低完播
    return round(random.uniform(0.25, 0.55), 4)


def maybe_clicked(rating: str) -> int:
    """曝光 -> 是否点击。喜欢类点击率拉到 ~98%，厌恶类 0%（让标签干净）"""
    if rating == "like":     return 1 if random.random() < 0.98 else 0
    if rating == "dislike":  return 0                                  # 厌恶类 100% 不点
    return 1 if random.random() < 0.40 else 0


def maybe_liked(rating: str, clicked: int) -> int:
    if not clicked: return 0
    if rating == "like":    return 1 if random.random() < 0.85 else 0
    if rating == "dislike": return 0
    return 1 if random.random() < 0.08 else 0


def maybe_favorited(rating: str, clicked: int) -> int:
    if not clicked: return 0
    if rating == "like":    return 1 if random.random() < 0.55 else 0
    return 1 if random.random() < 0.04 else 0


# ===================== 主流程 =====================

def step_1_fix_video_status(cur) -> None:
    """新视频改成 open=1, audit_status=1，否则训练负采样查不到。"""
    cur.execute("""
        UPDATE videos
           SET open = 1, audit_status = 1
         WHERE user_id BETWEEN %s AND %s
           AND video_id >= %s
    """, (*NEW_VIDEO_USER_RANGE, NEW_VIDEO_ID_MIN))
    print(f"  videos updated to open=1: {cur.rowcount}")


def fetch_videos(cur) -> List[Tuple[int, int, str]]:
    """返回所有 public 视频 [(video_id, user_id_author, category)]"""
    cur.execute("""
        SELECT video_id, user_id, COALESCE(category, '其他')
          FROM videos
         WHERE open = 1 AND audit_status = 1
           AND deleted_at IS NULL
         ORDER BY video_id
    """)
    return list(cur.fetchall())


def step_2_clear_old(cur) -> None:
    """清掉旧的测试数据（仅针对 8~12 这 5 个用户），避免重复跑爆表。"""
    uids = tuple(USER_PREFS.keys())
    placeholders = ",".join(["%s"] * len(uids))
    for tbl in ("recommendation_exposures",
                "user_video_watch_histories",
                "user_video_interactions",
                "user_behaviors",
                "video_likes"):
        cur.execute(f"DELETE FROM {tbl} WHERE user_id IN ({placeholders})", uids)
        print(f"  cleared {tbl}: {cur.rowcount} rows")


def step_3_seed_actions(cur, videos: List[Tuple[int, int, str]]) -> Dict[int, dict]:
    """主灌入。返回每个用户的统计字典。"""
    stats: Dict[int, dict] = {
        uid: {"exp": 0, "click": 0, "like": 0, "fav": 0, "watch": 0}
        for uid in USER_PREFS
    }

    # 按用户、视频、曝光次数三层循环
    for uid in USER_PREFS:
        # 每个用户对每个视频做若干次曝光（rating 不同次数不同）
        for video_id, author_id, category in videos:
            if author_id == uid:
                continue   # 不给自己看
            rating0 = category_rating(uid, category)
            # 喜欢类多曝光（10 次），中性 5~7 次，讨厌少曝光（3~4 次）
            if rating0 == "like":
                n_exp = random.randint(EXPOSURES_PER_VIDEO, EXPOSURES_PER_VIDEO + 4)
            elif rating0 == "dislike":
                n_exp = random.randint(2, 4)
            else:
                n_exp = random.randint(EXPOSURES_MIN, EXPOSURES_PER_VIDEO)
            for pos_i in range(n_exp):
                rating = rating0
                clicked = maybe_clicked(rating)
                liked = maybe_liked(rating, clicked)
                favorited = maybe_favorited(rating, clicked)
                completion = gen_completion(rating) if clicked else 0.0
                watch_dur = int(completion * random.randint(60, 600))   # 假设视频 1~10 分钟
                ts = random_ts(7)
                ts_str = ts.strftime("%Y-%m-%d %H:%M:%S")

                # ----- recommendation_exposures (训练主表) -----
                cur.execute("""
                    INSERT INTO recommendation_exposures
                      (user_id, video_id, recall_source, position, score,
                       is_clicked, is_liked, is_commented, is_shared, is_favorited,
                       watch_duration, completion_rate, exposure_time, request_id)
                    VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
                """, (
                    uid, video_id, "popular", pos_i,
                    round(random.uniform(0.3, 0.95), 4),
                    clicked, liked, 0, 0, favorited,
                    watch_dur, completion, ts_str,
                    f"seed_{uid}_{video_id}_{pos_i}",
                ))
                stats[uid]["exp"] += 1
                stats[uid]["click"] += clicked
                stats[uid]["like"] += liked
                stats[uid]["fav"] += favorited

                # ----- 衍生表（仅当点击了才写）-----
                if clicked:
                    # user_video_watch_histories: 唯一约束 (user_id, video_id)
                    # 用 ON DUPLICATE KEY UPDATE 保留最大 progress 的那条
                    cur.execute("""
                        INSERT INTO user_video_watch_histories
                          (user_id, video_id, watch_duration, completion_rate, watch_time)
                        VALUES (%s,%s,%s,%s,%s)
                        ON DUPLICATE KEY UPDATE
                          watch_duration = GREATEST(watch_duration, VALUES(watch_duration)),
                          completion_rate = GREATEST(completion_rate, VALUES(completion_rate)),
                          watch_time = GREATEST(watch_time, VALUES(watch_time))
                    """, (uid, video_id, watch_dur, completion, ts_str))
                    stats[uid]["watch"] += 1

                    # user_behaviors: view 行（流水可以重复）
                    cur.execute("""
                        INSERT INTO user_behaviors
                          (user_id, video_id, behavior_type, behavior_time)
                        VALUES (%s,%s,'view',%s)
                    """, (uid, video_id, ts_str))

                    if liked:
                        cur.execute("""
                            INSERT INTO user_behaviors
                              (user_id, video_id, behavior_type, behavior_time)
                            VALUES (%s,%s,'like',%s)
                        """, (uid, video_id, ts_str))
                        # video_likes 也可能有 (user_id, video_id) 唯一约束
                        cur.execute("""
                            INSERT IGNORE INTO video_likes (user_id, video_id, created_at)
                            VALUES (%s,%s,%s)
                        """, (uid, video_id, ts_str))

                    if favorited:
                        cur.execute("""
                            INSERT INTO user_behaviors
                              (user_id, video_id, behavior_type, behavior_time)
                            VALUES (%s,%s,'favorite',%s)
                        """, (uid, video_id, ts_str))

        # ----- user_video_interactions: 用户-视频聚合（每 user-video 一行）-----
        cur.execute("""
            SELECT video_id,
                   COUNT(*) AS imp,
                   SUM(is_clicked) AS clk,
                   SUM(watch_duration) AS wt,
                   MAX(completion_rate) AS mp,
                   MAX(is_liked) AS lk,
                   MAX(is_favorited) AS fv,
                   MIN(exposure_time) AS first_t,
                   MAX(exposure_time) AS last_t
              FROM recommendation_exposures
             WHERE user_id = %s
             GROUP BY video_id
        """, (uid,))
        rows = cur.fetchall()
        for vid, imp, clk, wt, mp, lk, fv, first_t, last_t in rows:
            cur.execute("""
                INSERT INTO user_video_interactions
                  (user_id, video_id, impression_count, click_count, total_watch_time,
                   max_watch_progress, replay_count,
                   is_liked, is_favorited, is_shared, comment_count,
                   engagement_score, first_interact_at, last_interact_at)
                VALUES (%s,%s,%s,%s,%s,%s,0,%s,%s,0,0,%s,%s,%s)
            """, (uid, vid, int(imp), int(clk or 0), int(wt or 0),
                  float(mp or 0), int(lk or 0), int(fv or 0),
                  round(float(mp or 0) * 0.5 + (lk or 0) * 0.3 + (fv or 0) * 0.2, 4),
                  first_t, last_t))

    return stats


def step_4_user_profiles(cur, stats: Dict[int, dict]) -> None:
    """长期画像 user_profiles。
    填齐所有数值字段，让 5 个用户的画像有显著差异（DeepFM 才能学到 user-aware 特征）。
    """
    # 先用真实数据算每个用户的统计
    for uid, s in stats.items():
        cur.execute("""
            SELECT
                COALESCE(AVG(NULLIF(watch_duration,0)), 0)        AS avg_dur,
                COALESCE(AVG(completion_rate), 0)                  AS avg_comp,
                COALESCE(SUM(is_clicked), 0)                       AS total_view,
                COALESCE(SUM(is_liked), 0)                         AS total_like,
                COALESCE(SUM(is_shared), 0)                        AS total_share,
                COALESCE(SUM(is_commented), 0)                     AS total_cmt
              FROM recommendation_exposures
             WHERE user_id = %s
        """, (uid,))
        avg_dur, avg_comp, tv, tl, ts, tc = cur.fetchone()

        # like_rate / comment_rate / share_rate 都基于点击量算
        tv = int(tv or 0)
        like_rate = round(int(tl or 0) / max(tv, 1), 4)
        comment_rate = round(int(tc or 0) / max(tv, 1), 4)
        share_rate = round(int(ts or 0) / max(tv, 1), 4)
        # 用户等级：按点击数分档，让数值有梯度
        if tv >= 100:    user_level = 5
        elif tv >= 60:   user_level = 4
        elif tv >= 30:   user_level = 3
        else:            user_level = 2
        # 内容质量偏好（1~5）：完成率高 -> 偏好高质量
        if avg_comp >= 0.6:   q_pref = 5
        elif avg_comp >= 0.4: q_pref = 4
        elif avg_comp >= 0.25: q_pref = 3
        else:                  q_pref = 2
        # 时长偏好（1~5）：看长视频多的偏好高
        if avg_dur >= 200:    d_pref = 4
        elif avg_dur >= 120:  d_pref = 3
        else:                 d_pref = 2

        # 把分类偏好也写到 category_preference json
        cur.execute("""
            SELECT v.category, COUNT(*) AS n
              FROM recommendation_exposures re
              JOIN videos v ON v.video_id = re.video_id
             WHERE re.user_id = %s AND re.is_clicked = 1
             GROUP BY v.category
        """, (uid,))
        cats = cur.fetchall()
        total_clk = sum(n for _, n in cats) or 1
        cat_pref = {c: round(n / total_clk, 4) for c, n in cats}

        # upsert
        cur.execute("SELECT 1 FROM user_profiles WHERE user_id=%s", (uid,))
        if cur.fetchone():
            cur.execute("""
                UPDATE user_profiles SET
                  avg_watch_duration=%s,
                  avg_completion_rate=%s,
                  like_rate=%s,
                  comment_rate=%s,
                  share_rate=%s,
                  total_view_count=%s,
                  total_like_count=%s,
                  total_comment_count=%s,
                  total_share_count=%s,
                  user_level=%s,
                  content_quality_pref=%s,
                  video_duration_pref=%s,
                  category_preference=%s,
                  last_active_at=NOW()
                WHERE user_id=%s
            """, (round(float(avg_dur or 0), 2), round(float(avg_comp or 0), 4),
                  like_rate, comment_rate, share_rate,
                  tv, int(tl or 0), int(tc or 0), int(ts or 0),
                  user_level, q_pref, d_pref,
                  json.dumps(cat_pref, ensure_ascii=False), uid))
        else:
            cur.execute("""
                INSERT INTO user_profiles
                  (user_id, avg_watch_duration, avg_completion_rate,
                   like_rate, comment_rate, share_rate,
                   total_view_count, total_like_count, total_comment_count, total_share_count,
                   user_level, content_quality_pref, video_duration_pref,
                   category_preference, last_active_at)
                VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s, NOW())
            """, (uid, round(float(avg_dur or 0), 2), round(float(avg_comp or 0), 4),
                  like_rate, comment_rate, share_rate,
                  tv, int(tl or 0), int(tc or 0), int(ts or 0),
                  user_level, q_pref, d_pref,
                  json.dumps(cat_pref, ensure_ascii=False)))

        print(f"  uid={uid}: avg_dur={float(avg_dur or 0):.0f}s  avg_comp={float(avg_comp or 0):.3f}  "
              f"like_rate={like_rate:.3f}  level={user_level}  q_pref={q_pref}  d_pref={d_pref}")


def step_5_redis(rcli, cur) -> None:
    """把最近 50 条行为写入 Redis ZSET（让 Agent 实时层有数据）。"""
    for uid in USER_PREFS:
        key = f"user:recent_actions:{uid}"
        rcli.delete(key)

        cur.execute("""
            SELECT re.video_id, v.category,
                   re.is_clicked, re.is_liked, re.is_favorited,
                   re.watch_duration, re.completion_rate,
                   UNIX_TIMESTAMP(re.exposure_time)*1000 AS ts_ms
              FROM recommendation_exposures re
              JOIN videos v ON v.video_id = re.video_id
             WHERE re.user_id = %s
             ORDER BY re.exposure_time DESC
             LIMIT 50
        """, (uid,))
        rows = cur.fetchall()

        pipe = rcli.pipeline()
        for vid, cat, clk, lk, fv, wd, comp, ts_ms in rows:
            # 推断 ActionType：优先 favorite > like > view
            if fv: at = "favorite"
            elif lk: at = "like"
            elif clk: at = "view"
            else: at = "view"   # 哪怕没点也给个 view（带低 progress 表示 skip）
            action = {
                "video_id": int(vid),
                "action_type": at,
                "timestamp": int(ts_ms),
                "duration": int(wd or 0),
                "progress": float(comp or 0.0),
                "category": cat or "",
                "tags": "",
            }
            pipe.zadd(key, {json.dumps(action, ensure_ascii=False): float(ts_ms)})
        pipe.expire(key, 7 * 86400)
        n = pipe.execute()
        print(f"  user {uid}: {len(rows)} actions written to Redis")


def main() -> int:
    print("=" * 64)
    print("Step 0  连接 MySQL / Redis")
    print("=" * 64)
    cnx = conn_mysql()
    rcli = conn_redis()
    cur = cnx.cursor()

    print("\nStep 1  把新视频 open=1, audit_status=1")
    step_1_fix_video_status(cur)

    print("\nStep 2  清旧测试数据")
    step_2_clear_old(cur)

    cnx.commit()

    print("\nStep 3  生成 video 列表")
    videos = fetch_videos(cur)
    print(f"  共 {len(videos)} 条 public 视频参与训练")
    if not videos:
        print("[fatal] no public videos")
        return 1

    print("\nStep 4  灌入模拟行为（每用户 ~exposures）")
    stats = step_3_seed_actions(cur, videos)
    cnx.commit()
    print("\n  统计：")
    for uid, s in stats.items():
        ctr = s["click"] / max(s["exp"], 1)
        print(f"    uid={uid}  exp={s['exp']:>4d}  click={s['click']:>3d}  "
              f"ctr={ctr:.2%}  like={s['like']:>3d}  fav={s['fav']:>3d}  watch={s['watch']:>3d}")

    print("\nStep 5  更新 user_profiles")
    step_4_user_profiles(cur, stats)
    cnx.commit()

    print("\nStep 6  写入 Redis 实时行为流")
    step_5_redis(rcli, cur)

    cnx.close()
    print("\n[done] all data written.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
