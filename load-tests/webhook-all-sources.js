import http from 'k6/http';
import { check, sleep } from 'k6';

// Multi-source throughput: slack/github/gitlab/teams in one run.
// Run: k6 run -e TARGET_URL=http://localhost:8080 load-tests/webhook-all-sources.js
export const options = {
    stages: [
        { duration: '1m', target: 50 },
        { duration: '3m', target: 50 },
        { duration: '1m', target: 200 },
        { duration: '2m', target: 200 },
        { duration: '1m', target: 0 },
    ],
    thresholds: {
        http_req_duration: ['p(95)<250'],
        http_req_failed: ['rate<0.01'],
    },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8080';

function githubPayload() {
    return {
        payload: JSON.stringify({
            action: 'opened',
            pull_request: { id: 123456, number: 42, title: 'Amazing new feature', user: { login: 'octocat' } },
        }),
        params: {
            headers: {
                'Content-Type': 'application/json',
                'X-GitHub-Event': 'pull_request',
                'X-GitHub-Delivery': `k6-gh-${Math.random()}`,
                'X-Hub-Signature-256': 'sha256=skip-for-load-test',
            },
        },
        path: '/webhooks/github',
    };
}

function slackPayload() {
    return {
        payload: JSON.stringify({ type: 'event_callback', event_id: `Ev${Math.random()}`, event: { type: 'message' } }),
        params: { headers: { 'Content-Type': 'application/json' } },
        path: '/webhooks/slack',
    };
}

function gitlabPayload() {
    return {
        payload: JSON.stringify({ object_kind: 'push', checkout_sha: 'abc123' }),
        params: { headers: { 'Content-Type': 'application/json', 'X-Gitlab-Token': 'skip' } },
        path: '/webhooks/gitlab',
    };
}

function teamsPayload() {
    return {
        payload: JSON.stringify({ id: `teams-${Math.random()}`, type: 'message' }),
        params: { headers: { 'Content-Type': 'application/json' } },
        path: '/webhooks/teams',
    };
}

const builders = [githubPayload, slackPayload, gitlabPayload, teamsPayload];

export default function () {
    const b = builders[Math.floor(Math.random() * builders.length)]();
    const res = http.post(`${BASE_URL}${b.path}`, b.payload, b.params);
    check(res, { 'status is 202 or 401 (auth skipped)': (r) => r.status === 202 || r.status === 401 });
    sleep(0.05);
}
