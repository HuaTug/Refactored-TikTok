// P03 · 20× 洪峰对比（k6 版本，同时也提供 JMeter p03_burst.jmx）
//
// 流量曲线：
//   0 ─── 15s ───► 500 QPS  warm
//   15s ── 105s ─► 10000 QPS peak（瞬时拉到 20× 容量）
//
// A 组：先 `bash scripts/perf/p03_toggle_sync.sh A` 重启 cmd/interaction
// B 组：先 `bash scripts/perf/p03_toggle_sync.sh B` 重启 cmd/interaction
//
// 跑 A 组：
//   GROUP=A k6 run --summary-export scripts/perf/reports/p03-A.json scripts/perf/p03_burst.js
// 跑 B 组：
//   GROUP=B k6 run --summary-export scripts/perf/reports/p03-B.json scripts/perf/p03_burst.js
import http from 'k6/http';
import { Trend, Rate, Counter } from 'k6/metrics';
import { SharedArray } from 'k6/data';
import { randomItem } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import papaparse from 'https://jslib.k6.io/papaparse/5.1.1/index.js';

const BASE  = __ENV.API_BASE || 'http://127.0.0.1:8888';
const GROUP = __ENV.GROUP || 'B';

const tokens = new SharedArray('tokens', () => {
  const raw = open('../../scripts/perf/tokens.csv');
  return papaparse.parse(raw, { header: true, skipEmptyLines: true })
    .data.filter(r => r.token);
});

const tBurst = new Trend('burst_lat', true);
const errs   = new Counter('burst_errors');
const errR   = new Rate('burst_err_rate');

export const options = {
  scenarios: {
    warm: {
      executor: 'constant-arrival-rate',
      rate: 500,                  // 500 QPS
      timeUnit: '1s',
      duration: '15s',
      preAllocatedVUs: 200,
      maxVUs: 1000,
      exec: 'doLike',
    },
    peak: {
      executor: 'constant-arrival-rate',
      rate: 10000,                // 20×
      timeUnit: '1s',
      duration: '105s',
      preAllocatedVUs: 4000,
      maxVUs: 20000,
      startTime: '15s',
      exec: 'doLike',
    },
  },
  thresholds: {
    'burst_lat':   ['p(99)<5000'],   // 软阈值，A 组会大幅突破
    'burst_err_rate': ['rate<0.5'],  // A 组允许大量错误，B 组应 < 0.001
  },
};

const HOT = Array.from({ length: 50 }, (_, i) => i + 1);

export function doLike() {
  const t = tokens.length ? randomItem(tokens) : null;
  if (!t) return;
  const payload = JSON.stringify({
    video_id: randomItem(HOT),
    comment_id: 0,
    action_type: 'like',
  });
  const r = http.post(`${BASE}/v1/action/like`, payload, {
    headers: { 'Access-Token': t.token, 'Content-Type': 'application/json' },
    timeout: '5s',
  });
  tBurst.add(r.timings.duration);
  const ok = r.status === 200;
  errR.add(!ok);
  if (!ok) errs.add(1);
}

export function handleSummary(data) {
  const file = `scripts/perf/reports/p03-${GROUP}.json`;
  return {
    [file]: JSON.stringify(data, null, 2),
    stdout: `
================= P03 · ${GROUP} 组 =================
  peak QPS target = 10000
  burst TP99      = ${(data.metrics.burst_lat?.values?.['p(99)'] ?? 0).toFixed(2)} ms
  err rate        = ${((data.metrics.burst_err_rate?.values?.rate ?? 0) * 100).toFixed(2)} %
  total errors    = ${data.metrics.burst_errors?.values?.count ?? 0}
=====================================================
`,
  };
}
