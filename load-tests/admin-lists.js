import http from 'k6/http';
import { check, sleep } from 'k6';

// Admin list + replay-batch read path: catches full-scan file-store regressions.
// Requires ADMIN_JWT env. Run: k6 run -e TARGET_URL=http://localhost:8080 -e ADMIN_JWT=$JWT load-tests/admin-lists.js
export const options = {
    stages: [
        { duration: '30s', target: 10 },
        { duration: '2m', target: 10 },
        { duration: '30s', target: 0 },
    ],
    thresholds: {
        http_req_duration: ['p(95)<400'],
        http_req_failed: ['rate<0.05'],
    },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8080';
const JWT = __ENV.ADMIN_JWT || '';

export default function () {
    const params = { headers: { Authorization: `Bearer ${JWT}` } };
    let res = http.get(`${BASE_URL}/admin/events/failed?limit=20`, params);
    check(res, { 'failed list 200': (r) => r.status === 200 });
    res = http.get(`${BASE_URL}/admin/events/replay-audit?limit=20`, params);
    check(res, { 'audit list 200': (r) => r.status === 200 });
    res = http.get(`${BASE_URL}/admin/events/security-audit?limit=20`, params);
    check(res, { 'security list 200': (r) => r.status === 200 || r.status === 404 });
    sleep(0.2);
}
