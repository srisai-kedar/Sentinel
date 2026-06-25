import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';

http.setResponseCallback(http.expectedStatuses(200, 429));

const allowed = new Counter('allowed_200');
const blocked = new Counter('blocked_429');

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  scenarios: {
    distributed_clients: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '15s', target: 20 },
        { duration: '30s', target: 50 },
        { duration: '30s', target: 100 },
        { duration: '15s', target: 0 },
      ],
    },
    hot_client: {
      executor: 'constant-vus',
      vus: 5,
      duration: '60s',
      startTime: '15s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    'http_req_duration{endpoint:api_test}': ['p(99)<2000'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost';

export default function () {
  const vu = __VU;
  const clientIP = `10.200.${Math.floor(vu / 256)}.${vu % 256}`;
  const res = http.get(`${BASE_URL}/api/test`, {
    headers: { 'X-Forwarded-For': clientIP },
    tags: { endpoint: 'api_test' },
  });

  const ok = res.status === 200;
  const limited = res.status === 429;
  if (ok) allowed.add(1);
  if (limited) blocked.add(1);

  check(res, {
    'status 200 or 429': (r) => r.status === 200 || r.status === 429,
    '429 has retry-after': (r) => r.status !== 429 || r.headers['Retry-After'] !== undefined,
    '200 has remaining header': (r) => r.status !== 200 || r.headers['X-Ratelimit-Remaining'] !== undefined,
  });

  if (limited) {
    sleep(0.2);
  } else {
    sleep(0.02);
  }

  return { ok, limited };
}

export function handleSummary(data) {
  const out = JSON.stringify(data, null, 2);
  return {
    'load-tests/results/rate-limit-summary.json': out,
    stdout: summarizeRateLimit(data),
  };
}

function summarizeRateLimit(data) {
  const m = data.metrics;
  const reqs = m.http_reqs?.values?.count ?? 0;
  const rate = m.http_reqs?.values?.rate ?? 0;
  const failed = m.http_req_failed?.values?.rate ?? 0;
  const dur = m.http_req_duration?.values ?? {};
  const allowedN = m.allowed_200?.values?.count ?? 0;
  const blockedN = m.blocked_429?.values?.count ?? 0;
  return [
    '=== Sentinel Rate Limit (/api/test) ===',
    `total requests: ${reqs}`,
    `request rate: ${rate.toFixed(2)} req/s`,
    `allowed (200): ${allowedN}`,
    `blocked (429): ${blockedN}`,
    `server error rate: ${(failed * 100).toFixed(2)}%`,
    `latency p50: ${dur.med?.toFixed(2) ?? 'n/a'} ms`,
    `latency p95: ${dur['p(95)']?.toFixed(2) ?? 'n/a'} ms`,
    `latency p99: ${dur['p(99)']?.toFixed(2) ?? 'n/a'} ms`,
  ].join('\n');
}
