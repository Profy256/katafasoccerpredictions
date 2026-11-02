/**
 * Go↔TS contract diff harness.
 *
 * Both servers must run against the SAME database:
 *   Go: DATABASE_URL=<url> PORT=8088 go run ./cmd/api        (from backend/)
 *   TS: npm run dev -p 3010                                  (from frontend/)
 *
 * Usage:  node tools/parity/diff.mjs [--go http://localhost:8088] [--ts http://localhost:3010]
 *
 * Compares status, body bytes, ETag, Cache-Control and Content-Type for every
 * path. Headers that legitimately differ per server (X-Request-Id, Date,
 * platform headers) are ignored. Exit code 1 if any compared path differs.
 */

import { randomUUID } from 'node:crypto';

const args = process.argv.slice(2);
function arg(name, fallback) {
  const i = args.indexOf(`--${name}`);
  return i >= 0 ? args[i + 1] : fallback;
}

const GO = arg('go', 'http://localhost:8088');
const TS = arg('ts', 'http://localhost:3010');

const uuid = '00000000-0000-0000-0000-000000000000';
// Deterministic ids from the parity seed (see the seed script).
const league = '11111111-1111-1111-1111-111111111111';
const scheduledMatch = '33333333-3333-3333-3333-333333333333';
const finishedMatch = '44444444-4444-4444-4444-444444444444';
const analyst = '77777777-7777-7777-7777-777777777777';
const settledSlip = '66666666-6666-6666-6666-666666666666';
const openSlip = '66666666-6666-6666-6666-666666666667';
const draftSlip = '66666666-6666-6666-6666-666666666668';

const PATHS = [
  'healthz',
  'readyz',
  'v1/leagues',
  'v1/markets',
  'v1/feed',
  'v1/feed?leagues=' + league,
  'v1/feed?markets=ONE_X_TWO',
  'v1/feed?markets=BTTS',
  'v1/feed?markets=ONE_X_TWO,BTTS',
  'v1/feed?markets=NOPE',
  'v1/feed?min_confidence=50',
  'v1/feed?min_confidence=60',
  'v1/feed?min_confidence=abc',
  'v1/feed?min_confidence=150',
  'v1/feed?min_confidence=-1',
  'v1/feed?region=east-africa',
  'v1/feed?region=europe',
  'v1/feed?region=asia',
  'v1/feed?region=nowhere',
  'v1/feed?leagues=' + uuid,
  'v1/feed?leagues=not-a-uuid',
  'v1/feed?date=2026-01-01',
  `v1/matches/${scheduledMatch}`,
  `v1/matches/${finishedMatch}`,
  `v1/matches/${uuid}`,
  'v1/matches/not-a-uuid',
  'v1/accuracy',
  'v1/predictions/settled',
  'v1/predictions/settled?limit=0',
  'v1/predictions/settled?limit=1',
  'v1/predictions/settled?cursor=zzz',
  'v1/predictions/settled?outcome=hit',
  'v1/predictions/settled?outcome=miss',
  'v1/predictions/settled?outcome=bogus',
  'v1/predictions/settled?markets=BTTS',
  'v1/tips/free',
  'v1/tips/free?date=2026-10-28',
  'v1/tips/free?date=2026-10-21',
  'v1/tips/free?date=2026-01-01',
  'v1/tips/free?date=not-a-date',
  'v1/tips/free/history',
  'v1/tips/free/history?from=2026-10-01&to=2026-10-28',
  'v1/tips/free/history?from=2026-10-01&to=2026-09-01',
  'v1/tips/free/history?from=nope',
  'v1/packages',
  'v1/analysts',
  'v1/analysts/leaderboard',
  'v1/analysts/grace-namutebi',
  'v1/analysts/no-such-slug',
  'v1/stats/coverage',
  'v1/slips',
  'v1/slips?status=open',
  'v1/slips?status=settled',
  'v1/slips?status=weird',
  'v1/slips?package=vip',
  'v1/slips?package=NOPE',
  'v1/slips?analyst=' + analyst,
  'v1/slips?analyst=not-a-uuid',
  'v1/slips?limit=1',
  'v1/slips?limit=abc',
  'v1/slips?cursor=zzz',
  `v1/slips/${settledSlip}`,
  `v1/slips/${openSlip}`,
  `v1/slips/${draftSlip}`,
  `v1/slips/${uuid}`,
  'v1/slips/not-a-uuid',
  'v1/auth/me',
  'v1/predictions/also-not-a-route',
  'v1/nope',
  'v1/deeply/nested/unknown',
];

const COMPARED_HEADERS = ['etag', 'cache-control', 'content-type'];

async function fetchRaw(base, path, method = 'GET', headers = {}) {
  const res = await fetch(`${base}/${path}`, { method, headers, redirect: 'manual' });
  const body = Buffer.from(await res.arrayBuffer());
  return { status: res.status, headers: res.headers, body };
}

function headerSet(headers, names) {
  const out = {};
  for (const name of names) {
    const value = headers.get(name);
    if (value !== null) out[name] = value;
  }
  return out;
}

let mismatches = 0;
let compared = 0;

for (const path of PATHS) {
  const [go, ts] = await Promise.all([fetchRaw(GO, path), fetchRaw(TS, path)]);
  compared += 1;

  const problems = [];
  if (go.status !== ts.status) problems.push(`status ${go.status} vs ${ts.status}`);
  if (!go.body.equals(ts.body)) {
    problems.push(
      `body\n    go: ${go.body.toString('utf8').slice(0, 400)}\n    ts: ${ts.body.toString('utf8').slice(0, 400)}`,
    );
  }
  const goHeaders = headerSet(go.headers, COMPARED_HEADERS);
  const tsHeaders = headerSet(ts.headers, COMPARED_HEADERS);
  if (JSON.stringify(goHeaders) !== JSON.stringify(tsHeaders)) {
    problems.push(`headers ${JSON.stringify(goHeaders)} vs ${JSON.stringify(tsHeaders)}`);
  }

  if (problems.length > 0) {
    mismatches += 1;
    console.log(`FAIL GET /${path}`);
    for (const p of problems) console.log(`  ${p}`);
  } else {
    console.log(`ok   GET /${path} (${go.status}, ${go.body.length}B)`);
  }
}

// OPTIONS: Go's CORS middleware answers preflight before routing, on every path.
{
  const path = 'v1/feed';
  const [go, ts] = await Promise.all([
    fetchRaw(GO, path, 'OPTIONS'),
    fetchRaw(TS, path, 'OPTIONS'),
  ]);
  compared += 1;
  const problems = [];
  if (go.status !== ts.status) problems.push(`status ${go.status} vs ${ts.status}`);
  const corsHeaders = [
    'access-control-allow-methods',
    'access-control-allow-headers',
    'access-control-max-age',
    'access-control-allow-origin',
  ];
  const goHeaders = headerSet(go.headers, corsHeaders);
  const tsHeaders = headerSet(ts.headers, corsHeaders);
  if (JSON.stringify(goHeaders) !== JSON.stringify(tsHeaders)) {
    problems.push(`cors ${JSON.stringify(goHeaders)} vs ${JSON.stringify(tsHeaders)}`);
  }
  if (problems.length > 0) {
    mismatches += 1;
    console.log(`FAIL OPTIONS /${path}`);
    for (const p of problems) console.log(`  ${p}`);
  } else {
    console.log(`ok   OPTIONS /${path} (${go.status})`);
  }
}

// A permitted Origin must echo the right CORS headers. The local dev env on
// both sides allows only the localhost frontend origins.
{
  const path = 'v1/leagues';
  const origin = 'http://localhost:3010';
  const [go, ts] = await Promise.all([
    fetchRaw(GO, path, 'GET', { Origin: origin }),
    fetchRaw(TS, path, 'GET', { Origin: origin }),
  ]);
  compared += 1;
  const corsHeaders = [
    'access-control-allow-origin',
    'access-control-allow-credentials',
    'access-control-allow-methods',
  ];
  const goHeaders = headerSet(go.headers, corsHeaders);
  const tsHeaders = headerSet(ts.headers, corsHeaders);
  if (JSON.stringify(goHeaders) !== JSON.stringify(tsHeaders)) {
    mismatches += 1;
    console.log(`FAIL GET /${path} with Origin`);
    console.log(`  ${JSON.stringify(goHeaders)} vs ${JSON.stringify(tsHeaders)}`);
  } else {
    console.log(`ok   GET /${path} with Origin (${JSON.stringify(goHeaders)})`);
  }
}

// Cursor pagination: take Go's nextCursor from page 1 and follow it on both
// servers. The cursor must be identical between them (same encode) and the
// second page must match byte for byte. Same for the slip list cursor.
for (const [label, first, second] of [
  ['settled ledger page 2', 'v1/predictions/settled?limit=1', 'cursor'],
  ['slip list page 2', 'v1/slips?limit=1', 'cursor'],
]) {
  const goPage1 = await fetchRaw(GO, first);
  const tsPage1 = await fetchRaw(TS, first);
  compared += 1;

  const problems = [];
  if (!goPage1.body.equals(tsPage1.body)) {
    problems.push(
      `page1 body\n    go: ${goPage1.body.toString('utf8').slice(0, 400)}\n    ts: ${tsPage1.body.toString('utf8').slice(0, 400)}`,
    );
  } else {
    let cursor = '';
    try {
      cursor = JSON.parse(goPage1.body.toString('utf8')).nextCursor ?? '';
    } catch {
      /* no cursor: page 1 comparison already covers it */
    }
    if (cursor !== '') {
      const path = `${first}&${second}=${encodeURIComponent(cursor)}`;
      const [go2, ts2] = await Promise.all([fetchRaw(GO, path), fetchRaw(TS, path)]);
      compared += 1;
      if (go2.status !== ts2.status || !go2.body.equals(ts2.body)) {
        problems.push(
          `page2 (${path})\n    go: ${go2.status} ${go2.body.toString('utf8').slice(0, 400)}\n    ts: ${ts2.status} ${ts2.body.toString('utf8').slice(0, 400)}`,
        );
      }
    }
  }

  if (problems.length > 0) {
    mismatches += 1;
    console.log(`FAIL ${label}`);
    for (const p of problems) console.log(`  ${p}`);
  } else {
    console.log(`ok   ${label} (page1 ${goPage1.body.length}B)`);
  }
}

console.log(`\n${compared - mismatches}/${compared} matched`);
process.exit(mismatches > 0 ? 1 : 0);
