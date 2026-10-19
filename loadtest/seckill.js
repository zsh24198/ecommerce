import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import { SharedArray } from 'k6/data';

// 200-299 正常响应 + 429 限流均视为成功（限流是预期行为）
http.setResponseCallback(http.expectedStatuses({ min: 200, max: 299 }, 429));

const tokens = new SharedArray('tokens', () =>
  JSON.parse(open('./tokens.json'))
);

const BASE = __ENV.BASE || 'http://localhost:8080';
const ACTIVITY_ID = __ENV.AID || '2';
const ITEM_ID = parseInt(__ENV.ITEM_ID || '2');

const successTrend = new Trend('success_duration');
const successCount = new Counter('success_count');
const soldOutCount = new Counter('soldout_count');
const sysErrorCount = new Counter('syserror_count');
const rateLimitedCount = new Counter('ratelimited_count');
const alreadyBoughtCount = new Counter('alreadybought_count');

export const options = {
  stages: [
    { duration: '30s', target: 50 },    // 阶段1 预热
    { duration: '60s', target: 100 },   // 阶段2 轻载
    { duration: '60s', target: 200 },   // 阶段3 中载
    { duration: '60s', target: 500 },   // 阶段4 峰值（机器上限）
  ],
  thresholds: {
    success_duration: ['p(99)<200'],
    syserror_count:   ['count<100'],
    http_req_failed:  ['rate<0.01'],
  },
};

export default function () {
  const token = tokens[Math.floor(Math.random() * tokens.length)].token;
  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
  };
  const body = JSON.stringify({ item_id: ITEM_ID, quantity: 1 });
  const url = `${BASE}/api/v1/seckill/activities/${ACTIVITY_ID}/buy`;
  const res = http.post(url, body, params);

  const code = res.json('code');
  let category = 'syserror';
  if (res.status === 200 && code === 0) {
    successCount.add(1);
    successTrend.add(res.timings.duration);
    category = 'success';
  } else if (res.status === 200 && code === 5018) {
    soldOutCount.add(1);
    category = 'soldout';
  } else if (res.status === 200 && code === 5022) {
    alreadyBoughtCount.add(1);
    category = 'alreadybought';
  } else if (res.status === 429) {
    rateLimitedCount.add(1);
    category = 'ratelimited';
  } else {
    sysErrorCount.add(1);
  }
  check(res, {
    'classified': () => true,
    'is success or soldout or ratelimited or alreadybought': () => category === 'success' || category === 'soldout' || category === 'ratelimited' || category === 'alreadybought',
  });

  sleep(0.1);   // 每用户 10 QPS，控制总流量
}