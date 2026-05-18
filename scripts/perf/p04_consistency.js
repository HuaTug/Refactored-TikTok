// P04 · 千并发同时点赞 → 数据一致性
//
// 目标：1000 个不同用户在 60 秒内各对 video_id=9000777 点赞 1 次。
// 跑完后 5 分钟用 p04_verify.sh 校验：
//   COUNT(video_likes) == SCARD(video:likes:9000777) == GET(video:like_count:9000777) == 1000
//
// 运行：
//   k6 run scripts/perf/p04_consistency.js
//   TARGET_VID=9000777 k6 run scripts/perf/p04_consistency.js
//
// 关键设计：
//   - executor: per-vu-iterations  → 每个 VU 只发 1 次请求
//   - VU 数 = 1000，与 token 数一一对应
//   - action_type = "1" 固定为点赞，避免误删

import http from 'k6/http';
import { check } from 'k6';
import { SharedArray } from 'k6/data';
import papaparse from 'https://jslib.k6.io/papaparse/5.1.1/index.js';

const BASE = __ENV.API_BASE || 'http://127.0.0.1:8888';
const VID  = Number(__ENV.TARGET_VID || 9000777);

const tokens = new SharedArray('tokens', () => {
  const raw = open('../../scripts/perf/tokens.csv');
  const data = papaparse.parse(raw, { header: true, skipEmptyLines: true })
    .data.filter(r => r.token);
  if (data.length < 1000) {
    console.error(`需要至少 1000 个 token，当前只有 ${data.length}。请扩大 gen_tokens.go -n 参数`);
  }
  return data;
});

export const options = {
  scenarios: {
    one_per_vu: {
      executor: 'per-vu-iterations',
      vus: 1000,
      iterations: 1,
      maxDuration: '90s',
    },
  },
  thresholds: {
    'http_req_failed': ['rate<0.01'],
  },
};

export default function () {
  const idx = (__VU - 1) % tokens.length;
  const t = tokens[idx];
  const r = http.post(`${BASE}/v1/action/like`,
    JSON.stringify({ video_id: VID, comment_id: 0, action_type: 'like' }),
    { headers: { 'Access-Token': t.token, 'Content-Type': 'application/json' } });
  check(r, { 'like 200': res => res.status === 200 });
}
