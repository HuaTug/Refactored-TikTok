// P02 · 核心 HTTP 接口端到端延迟（k6 v2 兼容版本）
//
// 运行：
//   k6 run scripts/perf/p02_core_api.js                      # 完整阶梯
//   k6 run --vus 50 --duration 30s scripts/perf/p02_core_api.js   # 冒烟
//
// 通过标准：每个接口 TP99 < 100ms（login < 200ms）、5xx 错误率 < 1%
//
// 备注：项目侧有令牌桶限流，单 VU 加了 0.5s sleep 控制单机总 RPS 不超过 2 × VUs。

import http from 'k6/http';
import { Trend, Rate } from 'k6/metrics';
import { SharedArray } from 'k6/data';
import { sleep } from 'k6';
import papaparse from 'https://jslib.k6.io/papaparse/5.1.1/index.js';

const BASE = __ENV.API_BASE || 'http://127.0.0.1:8888';
const TOKENS_FILE = __ENV.TOKENS_FILE || 'scripts/perf/tokens.csv';

const tokens = new SharedArray('tokens', () => {
  const raw = open(`../../${TOKENS_FILE}`);
  return papaparse.parse(raw, { header: true, skipEmptyLines: true })
    .data.filter(r => r.token).map(r => ({ user_id: Number(r.user_id), token: r.token }));
});

// 真实 video_id：项目演示数据 1 ~ 129
const HOT = Array.from({ length: 50 }, (_, i) => i + 1);
const pick = arr => arr[Math.floor(Math.random() * arr.length)];

// 每接口独立 Trend，便于在最终汇总里读出 p(99)
const tFeed  = new Trend('lat_feed',         true);
const tLike  = new Trend('lat_like',         true);
const tComm  = new Trend('lat_comment_list', true);
const tNotif = new Trend('lat_notif_unread', true);
const tLogin = new Trend('lat_login',        true);
const errR   = new Rate('biz_errors');

export const options = {
  scenarios: {
    ramping: {
      executor: 'ramping-vus',
      startVUs: 10,
      stages: [
        { duration: '1m', target: 50 },
        { duration: '1m', target: 100 },
        { duration: '1m', target: 200 },
        { duration: '1m', target: 500 },
        { duration: '1m', target: 1000 },
      ],
      gracefulRampDown: '15s',
    },
  },
  thresholds: {
    'lat_feed':         ['p(99)<100'],
    'lat_like':         ['p(99)<100'],
    'lat_comment_list': ['p(99)<100'],
    'lat_notif_unread': ['p(99)<100'],
    'lat_login':        ['p(99)<200'],
    'http_req_failed':  ['rate<0.01'],
    'biz_errors':       ['rate<0.05'],
  },
};

function bizOk(res) {
  if (res.status !== 200) return false;
  try { return JSON.parse(res.body).code === 10000; } catch { return false; }
}

export default function () {
  const t = tokens[(__VU - 1) % tokens.length];
  const headers = { 'Access-Token': t.token, 'Content-Type': 'application/json' };

  // 1. /v1/video/feed
  let r = http.get(`${BASE}/v1/video/feed?page_num=1&page_size=10`, { headers });
  tFeed.add(r.timings.duration);
  errR.add(!bizOk(r));

  // 2. /v1/action/like
  r = http.post(`${BASE}/v1/action/like`,
    JSON.stringify({ video_id: pick(HOT), comment_id: 0, action_type: 'like' }), { headers });
  tLike.add(r.timings.duration);
  errR.add(!bizOk(r));

  // 3. /v1/comment/list
  r = http.get(`${BASE}/v1/comment/list?video_id=${pick(HOT)}&page_num=1&page_size=10`, { headers });
  tComm.add(r.timings.duration);
  errR.add(!bizOk(r));

  // 4. /v2/notification/unread
  r = http.get(`${BASE}/v2/notification/unread`, { headers });
  tNotif.add(r.timings.duration);
  errR.add(!bizOk(r));

  // 5. /v1/user/login (1% 流量；bcrypt 较重，避免拖累其他指标)
  if (Math.random() < 0.01) {
    const u = `perf_login_${String(t.user_id - 1100000).padStart(6, '0')}`;
    r = http.post(`${BASE}/v1/user/login`,
      JSON.stringify({ username: u, password: '123456' }),
      { headers: { 'Content-Type': 'application/json' } });
    tLogin.add(r.timings.duration);
  }

  sleep(0.5);
}

export function handleSummary(data) {
  const ts = new Date().toISOString().slice(0, 16).replace(/[:T-]/g, '');
  const out = `scripts/perf/reports/p02-${ts}.json`;
  return {
    [out]: JSON.stringify(data, null, 2),
    stdout: textReport(data),
  };
}

function textReport(data) {
  const p99 = m => {
    const v = data.metrics?.[m]?.values;
    if (!v) return 'N/A';
    return (v['p(99)'] ?? v['p99'] ?? v.max ?? 0).toFixed(2) + ' ms';
  };
  const rate = m => ((data.metrics?.[m]?.values?.rate ?? 0) * 100).toFixed(3) + '%';
  return `
================ P02 · 核心 HTTP 接口 ================
  feed          TP99 = ${p99('lat_feed')}
  like          TP99 = ${p99('lat_like')}
  comment_list  TP99 = ${p99('lat_comment_list')}
  notif_unread  TP99 = ${p99('lat_notif_unread')}
  login         TP99 = ${p99('lat_login')}
  ----
  http_req_failed = ${rate('http_req_failed')}
  biz_errors      = ${rate('biz_errors')}
=======================================================
`;
}
