import http from 'k6/http';
import { check, sleep } from 'k6';
                                                                                                                                                                                    
// Baseline throughput + latency through Nginx → 3 gateway instances.
// Target: /health (no rate limit) to measure gateway + LB overhead.

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  stages: [
    { duration: '15s', target: 25 },
    { duration: '30s', target: 50 },
    { duration: '30s', target: 100 },
    { duration: '15s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(99)<500'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost';

export default function () {
  const res = http.get(`${BASE_URL}/health`);
  check(res, {
    'status is 200': (r) => r.status === 200,
    'body has ok': (r) => r.body && r.body.includes('"status":"ok"'),
  });
  sleep(0.05);
}

export function handleSummary(data) {
  const out = JSON.stringify(data, null, 2);
  console.log(out);
  return {
    'load-tests/results/baseline-summary.json': out,
    stdout: textSummary(data),
  };
}

function textSummary(data) {
  const m = data.metrics;
  const line = (name) => {
    const metric = m[name];
    if (!metric) return `${name}: n/a`;
    if (metric.values.avg !== undefined) {
      return `${name}: avg=${metric.values.avg.toFixed(2)} p95=${metric.values['p(95)']?.toFixed(2)} p99=${metric.values['p(99)']?.toFixed(2)}`;
    }
    return `${name}: ${JSON.stringify(metric.values)}`;
  };
  return [
    '=== Sentinel Baseline (/health) ===',
    line('http_req_duration'),
    line('http_reqs'),
    line('http_req_failed'),
    line('vus'),
    line('iterations'),
  ].join('\n');
}
