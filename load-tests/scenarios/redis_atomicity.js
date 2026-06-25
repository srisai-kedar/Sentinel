import http from 'k6/http';
import { check } from 'k6';
import { Counter, Rate } from 'k6/metrics';

http.setResponseCallback(http.expectedStatuses(200, 429));

const allowed = new Counter('allowed_200');
const blocked = new Counter('blocked_429');
const serverErrors = new Rate('server_errors');

// 50 simultaneous requests from ONE client → token bucket capacity is 10.
// Redis Lua must allow exactly 10 across all 3 gateway instances.

export function setup() {
  const ip = __ENV.CLIENT_IP || `198.51.100.${Math.floor(Math.random() * 200) + 1}`;
  return { clientIP: ip };
}

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  scenarios: {
    burst: {
      executor: 'shared-iterations',
      vus: 50,
      iterations: 50,
      maxDuration: '15s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    allowed_200: ['count==10'],
    blocked_429: ['count==40'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost';

export default function (data) {
  const res = http.get(`${BASE_URL}/api/test`, {
    headers: { 'X-Forwarded-For': data.clientIP },
  });

  if (res.status === 200) allowed.add(1);
  if (res.status === 429) blocked.add(1);
  serverErrors.add(res.status >= 500);

  check(res, {
    'status 200 or 429': (r) => r.status === 200 || r.status === 429,
    '429 includes Retry-After': (r) => r.status !== 429 || !!r.headers['Retry-After'],
  });
}

export function handleSummary(data) {
  const a = data.metrics.allowed_200?.values?.count ?? 0;
  const b = data.metrics.blocked_429?.values?.count ?? 0;
  const summary = [
    '=== Redis Atomicity Burst ===',
    `allowed (200): ${a}`,
    `blocked (429): ${b}`,
    `expected: 10 allowed / 40 blocked`,
    `pass: ${a === 10 && b === 40}`,
  ].join('\n');
  return {
    'load-tests/results/redis-atomicity-summary.json': JSON.stringify(data, null, 2),
    'load-tests/results/redis-atomicity-result.json': JSON.stringify({
      allowed_200: a,
      blocked_429: b,
      pass: a === 10 && b === 40,
      date: new Date().toISOString(),
    }, null, 2),
    stdout: summary + '\n',
  };
}
