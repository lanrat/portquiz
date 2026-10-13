// Tests for core.js: node --test server/web/core_test.mjs
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  ETA, Results, STANDARD_BLOCKED, TimeoutEstimator, blockedCheck, buildReport, formatDuration, formatPorts,
  largestClosed, mapColumns, parsePorts, roundETA, rttStats, segmentHeights, testPort,
} from './core.js';

test('parsePorts', () => {
  assert.equal(parsePorts('').length, 65535);
  assert.deepEqual(parsePorts('80, 22,1000-1002,22'), [22, 80, 1000, 1001, 1002]);
  assert.deepEqual(parsePorts(' 5 - 6 ,'), [5, 6]);
  for (const bad of ['0', '65536', '10-5', 'x', '1-', ',']) {
    assert.throws(() => parsePorts(bad), Error, bad);
  }
});

test('formatPorts', () => {
  assert.equal(formatPorts([1, 2, 3, 22, 80, 81]), '1-3,22,80-81');
  assert.equal(formatPorts(parsePorts('')), '1-65535');
  assert.equal(formatPorts([]), '');
});

test('Results ranges and counts', () => {
  const r = new Results([1, 2, 3, 4, 5, 7, 8]);
  r.set(1, 'dropped');
  r.set(2, 'dropped');
  r.set(3, 'open', 12.5);
  r.set(4, 'browser');
  // 5 has no result yet
  r.set(7, 'rejected');
  r.set(8, 'rejected');
  r.set(8, 'open'); // a second result for a port is ignored
  r.set(9, 'nonsense');
  assert.equal(r.done, 6);
  assert.equal(r.count('dropped'), 2);
  assert.equal(r.count('open'), 1);
  assert.deepEqual(r.rtts, [12.5]);
  assert.deepEqual(r.ranges(), [
    { first: 1, last: 2, status: 'dropped' },
    { first: 3, last: 3, status: 'open' },
    { first: 4, last: 4, status: 'browser' },
    { first: 7, last: 8, status: 'rejected' },
  ]);
});

test('TimeoutEstimator adapts and rechecks like the command line client', () => {
  const e = new TimeoutEstimator(2000, true);
  assert.equal(e.timeout(), 2000);
  e.sample(100);
  assert.equal(e.timeout(), 500); // the floor
  for (let i = 0; i < 50; i++) e.sample(400);
  assert.ok(e.timeout() > 790 && e.timeout() <= 2000, String(e.timeout())); // about twice the response time
  const rechecks = Array.from({ length: 40 }, () => e.shouldRecheck()).filter(Boolean).length;
  assert.equal(rechecks, 2);
  e.missed(900);
  assert.ok(e.timeout() >= 1800);
  assert.equal(e.misses, 1);
  assert.equal(new TimeoutEstimator(2000, false).timeout(), 2000);
});

// fakeNet answers requests from a table of what happens on each port.
function fakeNet(behavior) {
  const calls = [];
  return {
    calls,
    async request(port, timeout) {
      calls.push(['request', port, timeout]);
      return behavior[port].shift();
    },
    async browserBlocks(port) {
      calls.push(['browserBlocks', port]);
      return behavior.blocked?.includes(port) ?? false;
    },
    async answers(port) {
      calls.push(['answers', port]);
      return behavior.answers?.includes(port) ?? false;
    },
  };
}

const opts = () => ({ token: 'tok', estimator: new TimeoutEstimator(2000, true), retries: 1 });

test('testPort classifies each outcome', async () => {
  const net = fakeNet({
    1: [{ type: 'response', body: 'tok', ms: 20 }],
    2: [{ type: 'response', body: '<html>proxy</html>', ms: 20 }],
    3: [{ type: 'failed', ms: 2 }],
    4: [{ type: 'failed', ms: 30 }],
    5: [{ type: 'failed', ms: 30 }],
    6: [{ type: 'aborted' }, { type: 'aborted' }],
    7: [{ type: 'aborted' }, { type: 'response', body: 'tok', ms: 1500 }],
    blocked: [3],
    answers: [5],
  });
  const o = opts();
  assert.deepEqual(await testPort(1, net, o), { status: 'open', rtt: 20 });
  assert.deepEqual(await testPort(2, net, o), { status: 'interfered' });
  assert.deepEqual(await testPort(3, net, o), { status: 'browser' });
  assert.deepEqual(await testPort(4, net, o), { status: 'rejected' });
  assert.deepEqual(await testPort(5, net, o), { status: 'interfered' });
  // the first timeout is rechecked with the full timeout
  const p6 = net.calls.length;
  net.calls.length = p6;
  assert.deepEqual(await testPort(6, net, { ...o, estimator: new TimeoutEstimator(2000, false) }), { status: 'dropped' });
  assert.deepEqual(await testPort(7, net, o), { status: 'open', rtt: 1500 });
  // a refused port is not retried
  assert.equal(net.calls.filter((c) => c[0] === 'request' && c[1] === 4).length, 1);
});

test('testPort rechecks a sample of timeouts with the full timeout', async () => {
  const e = new TimeoutEstimator(2000, true);
  e.sample(50);
  const net = fakeNet({ 9: [{ type: 'aborted' }, { type: 'aborted' }, { type: 'response', body: 'tok', ms: 900 }] });
  const r = await testPort(9, net, { token: 'tok', estimator: e, retries: 1 });
  assert.deepEqual(r, { status: 'open', rtt: 900, late: true });
  assert.deepEqual(net.calls.filter((c) => c[0] === 'request').map((c) => c[2]), [500, 500, 2000]);
  assert.ok(e.timeout() >= 1800);
});

test('blockedCheck', () => {
  assert.equal(blockedCheck(5, 1000, [3]).threshold, null, 'the test address failed at once');
  assert.equal(blockedCheck(1000, 1000, []).threshold, null, 'nothing blocked');
  assert.equal(blockedCheck(1000, 1000, [3, 8]).threshold, 50, 'at least 50ms');
  assert.equal(blockedCheck(1000, 1000, [30]).threshold, 120);
  assert.equal(blockedCheck(1000, 1000, [400]).threshold, 500, 'at most half the hold');
});

test('the standard list has the well-known ports', () => {
  for (const p of [21, 22, 25, 53, 110, 143, 993, 995, 5060, 6000, 10080]) assert.ok(STANDARD_BLOCKED.has(p), p);
  for (const p of [80, 443, 8080, 3389]) assert.ok(!STANDARD_BLOCKED.has(p), p);
});

test('ETA counts down and settles', () => {
  const e = new ETA(1000, 0);
  assert.equal(e.update(10, 1000), null);
  assert.equal(e.update(0, 5000), null);
  assert.equal(e.update(200, 2000), 8000);
  const later = e.update(400, 4000);
  assert.ok(later > 5000 && later < 6100, later);
  assert.equal(e.update(1000, 9000), 0);
});

test('roundETA and formatDuration', () => {
  assert.equal(roundETA(-1), 0);
  assert.equal(roundETA(1), 5000);
  assert.equal(roundETA(59000), 60000);
  assert.equal(roundETA(84000), 80000);
  assert.equal(roundETA(760000), 780000);
  assert.equal(formatDuration(45000), '45s');
  assert.equal(formatDuration(200000), '3m 20s');
  assert.equal(formatDuration(720000), '12m');
  assert.equal(formatDuration(3900000), '1h 5m');
});

test('mapColumns and segmentHeights', () => {
  const ranges = [
    { first: 1, last: 50, status: 'dropped' },
    { first: 51, last: 51, status: 'open' },
    { first: 52, last: 100, status: 'dropped' },
  ];
  const { cols, start } = mapColumns(ranges, 1, 100, 4);
  assert.equal(cols.length, 4);
  assert.equal(start(1), 26);
  assert.deepEqual(cols.map((c) => c.reduce((a, b) => a + b, 0)), [25, 25, 25, 25]);
  assert.equal(cols[2][0], 1); // the open port, in the third column
  const h = segmentHeights(cols[2], 25, 64, 3);
  assert.equal(h[0], 3);
  assert.equal(h[2], 61);
  assert.equal(mapColumns(ranges, 1, 3, 100).cols.length, 3);
});

test('largestClosed merges closed ranges, leaving out browser-blocked ports', () => {
  const runs = largestClosed([
    { first: 1, last: 10, status: 'dropped' },
    { first: 11, last: 11, status: 'rejected' },
    { first: 12, last: 12, status: 'browser' },
    { first: 13, last: 20, status: 'dropped' },
    { first: 21, last: 21, status: 'open' },
  ], 5);
  assert.deepEqual(runs, [
    { first: 1, last: 11, status: null },
    { first: 13, last: 20, status: 'dropped' },
  ]);
});

test('rttStats', () => {
  assert.equal(rttStats([]), undefined);
  assert.deepEqual(rttStats([3, 1, 2, 10]), { min: 1, median: 2, p95: 10, max: 10 });
});

test('buildReport matches the command line client format', () => {
  const r = new Results([1, 2, 3]);
  r.set(1, 'open', 10);
  r.set(2, 'browser');
  const e = new TimeoutEstimator(2000, true);
  const rep = buildReport({
    info: { version: 'v1.8.0', server: '192.0.2.10' },
    host: 'example.com',
    results: r,
    started: 0,
    finished: 2000,
    canceled: true,
    options: { timeout: 2000, adaptive: true, parallel: 128, rate: 500 },
    estimator: e,
    retries: 1,
    detection: 'runtime',
  });
  assert.equal(rep.client, 'browser');
  assert.equal(rep.tested_per_second, 1);
  assert.equal(rep.options.ports, '1-3');
  const k = rep.kinds[0];
  assert.equal(k.tested, 2);
  assert.equal(k.untested, 1);
  assert.equal(k.open, 1);
  assert.equal(k.browser_blocked, 1);
  assert.deepEqual(k.open_rtt_ms, { min: 10, median: 10, p95: 10, max: 10 });
  assert.deepEqual(k.ranges.map((x) => x.status), ['open', 'browser']);
  JSON.parse(JSON.stringify(rep));
});
