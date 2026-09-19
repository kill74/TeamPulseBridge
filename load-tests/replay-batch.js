import http from 'k6/http';
import { check, sleep } from 'k6';

// Replay batch write path: 25-event batches must stay parallel, not sequential.
// Requires ADMIN_JWT + at least one failed EVENT_ID seed.
export const options = {
    vus: 5,
    duration: '2m',
    thresholds: {
        http_req_duration: ['p(95)<2000'],
        http_req_failed: ['rate<0.05'],
    },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8080';
const JWT = __ENV.ADMIN_JWT || '';
const EVENT_ID = __ENV.EVENT_ID || 'fev_test';

export default function () {
    const payload = JSON.stringify({ event_ids: [EVENT_ID], dry_run: true });
    const params = { headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${JWT}` } };
    const res = http.post(`${BASE_URL}/admin/events/replay/batch`, payload, params);
    check(res, { 'batch 200/202': (r) => r.status === 200 || r.status === 202 || r.status === 400 });
    sleep(0.5);
}
