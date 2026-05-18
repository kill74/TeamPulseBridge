import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '1m', target: 50 },    // Ramp up
        { duration: '3m', target: 50 },    // Sustained
        { duration: '1m', target: 200 },   // Spike
        { duration: '3m', target: 200 },   // Sustained spike
        { duration: '1m', target: 0 },     // Ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<200'],  // 95% of requests < 200ms
        http_req_failed: ['rate<0.01'],    // < 1% error rate
    },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8080';

export default function () {
    const payload = JSON.stringify({
        action: 'opened',
        pull_request: {
            id: 123456,
            number: 42,
            title: 'Amazing new feature',
            user: { login: 'octocat' },
        },
    });

    const params = {
        headers: {
            'Content-Type': 'application/json',
            'X-GitHub-Event': 'pull_request',
            'X-GitHub-Delivery': `k6-delivery-${Math.random()}`,
            'X-Hub-Signature-256': 'sha256=skip-for-load-test',
        },
    };

    const res = http.post(`${BASE_URL}/webhooks/github`, payload, params);

    check(res, {
        'status is 202': (r) => r.status === 202,
    });

    sleep(0.1);
}
