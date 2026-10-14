// The browser test's logic, kept free of the DOM and the network so it can
// be tested with Node (core_test.mjs). app.js does the rest.

export const MAX_PORT = 65535;

// Ports browsers refuse to connect to, from the Fetch standard's list of bad
// ports. The page detects blocked ports at runtime, see blockedCheck, and
// only falls back to this list when it cannot.
export const STANDARD_BLOCKED = new Set([
  1, 7, 9, 11, 13, 15, 17, 19, 20, 21, 22, 23, 25, 37, 42, 43, 53, 69, 77, 79, 87, 95, 101, 102,
  103, 104, 109, 110, 111, 113, 115, 117, 119, 123, 135, 137, 139, 143, 161, 179, 389, 427, 465,
  512, 513, 514, 515, 526, 530, 531, 532, 540, 548, 554, 556, 563, 587, 601, 636, 989, 990, 993,
  995, 1719, 1720, 1723, 2049, 3659, 4045, 4190, 5060, 5061, 6000, 6566, 6665, 6666, 6667, 6668,
  6669, 6679, 6697, 10080,
]);

// Statuses, as in the command line client's JSON report, plus "browser"
// for ports the browser refused to connect to. A port's status is stored as
// its index here plus one, 0 meaning no result yet.
export const STATUSES = ['open', 'rejected', 'dropped', 'interfered', 'error', 'browser'];
export const OPEN = 1;
export const BROWSER = 6;

// Services people often need, checked in the summary.
export const SERVICES = [
  ['FTP', 21, 'tcp'], ['SSH', 22, 'tcp'], ['Telnet', 23, 'tcp'], ['SMTP', 25, 'tcp'],
  ['DNS', 53, 'tcp'], ['DNS', 53, 'udp'], ['HTTP', 80, 'tcp'], ['POP3', 110, 'tcp'],
  ['NTP', 123, 'udp'], ['IMAP', 143, 'tcp'], ['HTTPS', 443, 'tcp'], ['HTTP/3 (QUIC)', 443, 'udp'],
  ['SMB', 445, 'tcp'], ['SMTPS', 465, 'tcp'], ['IPsec IKE', 500, 'udp'], ['SMTP submission', 587, 'tcp'],
  ['DNS over TLS', 853, 'tcp'], ['IMAPS', 993, 'tcp'], ['POP3S', 995, 'tcp'], ['OpenVPN', 1194, 'tcp'],
  ['OpenVPN', 1194, 'udp'], ['L2TP', 1701, 'udp'], ['PPTP', 1723, 'tcp'], ['MySQL', 3306, 'tcp'],
  ['RDP', 3389, 'tcp'], ['STUN/TURN', 3478, 'udp'], ['IPsec NAT-T', 4500, 'udp'], ['SIP', 5060, 'tcp'],
  ['SIP', 5060, 'udp'], ['PostgreSQL', 5432, 'tcp'], ['HTTP alt', 8080, 'tcp'], ['HTTPS alt', 8443, 'tcp'],
  ['WireGuard', 51820, 'udp'],
].map(([name, port, protocol]) => ({ name, port, protocol }));

// parsePorts parses a list of ports and ranges such as "22,80,1000-2000"
// into sorted ports without duplicates, like the command line client's
// -port. An empty list means every port.
export function parsePorts(text) {
  if (text.trim() === '') return allPorts();
  const set = new Set();
  for (let item of text.split(',')) {
    item = item.trim();
    if (item === '') continue;
    const m = /^(\d+)\s*(?:-\s*(\d+))?$/.exec(item);
    if (!m) throw new Error(`invalid port entry "${item}"`);
    const first = Number(m[1]);
    const last = m[2] === undefined ? first : Number(m[2]);
    if (first < 1 || last > MAX_PORT) throw new Error(`invalid port entry "${item}": ports are 1 to ${MAX_PORT}`);
    if (first > last) throw new Error(`invalid port entry "${item}": start of range is greater than end`);
    for (let p = first; p <= last; p++) set.add(p);
  }
  if (set.size === 0) throw new Error(`no ports in "${text}"`);
  return [...set].sort((a, b) => a - b);
}

export function allPorts() {
  return Array.from({ length: MAX_PORT }, (_, i) => i + 1);
}

// formatPorts formats sorted ports as a list of ports and ranges.
export function formatPorts(ports) {
  const parts = [];
  for (let i = 0; i < ports.length;) {
    let j = i;
    while (j + 1 < ports.length && ports[j + 1] === ports[j] + 1) j++;
    parts.push(j > i ? `${ports[i]}-${ports[j]}` : `${ports[i]}`);
    i = j + 1;
  }
  return parts.join(',');
}

// Results holds the status of each port tested.
export class Results {
  constructor(ports) {
    this.ports = ports;
    this.status = new Uint8Array(MAX_PORT + 1);
    this.counts = new Array(STATUSES.length + 1).fill(0);
    this.rtts = [];
    this.done = 0;
  }

  set(port, status, rtt) {
    const code = STATUSES.indexOf(status) + 1;
    if (code === 0 || this.status[port] !== 0) return;
    this.status[port] = code;
    this.counts[code]++;
    this.done++;
    if (code === OPEN && rtt !== undefined) this.rtts.push(rtt);
  }

  count(status) {
    return this.counts[STATUSES.indexOf(status) + 1];
  }

  // ranges returns runs of consecutive ports with the same status, as
  // {first, last, status}, for the ports with a result.
  ranges() {
    const out = [];
    let cur = null;
    for (const p of this.ports) {
      const code = this.status[p];
      if (code === 0) {
        cur = null;
        continue;
      }
      const status = STATUSES[code - 1];
      if (cur && cur.status === status && cur.last + 1 === p) {
        cur.last = p;
      } else {
        cur = { first: p, last: p, status };
        out.push(cur);
      }
    }
    return out;
  }
}

// TimeoutEstimator is the command line client's adaptive timeout: it learns
// how quickly open ports answer and waits a safe multiple of that, between
// floor and max, with the same smoothing as TCP (RFC 6298).
export class TimeoutEstimator {
  constructor(max, adaptive, floor = 500) {
    this.max = max;
    this.adaptive = adaptive;
    this.floor = floor;
    this.samples = 0;
    this.srtt = 0;
    this.rttvar = 0;
    this.timeouts = 0;
    this.misses = 0;
  }

  timeout() {
    if (!this.adaptive || this.samples === 0) return this.max;
    return Math.min(Math.max(this.srtt + 4 * this.rttvar, 2 * this.srtt, this.floor), this.max);
  }

  sample(rtt) {
    if (this.samples === 0) {
      this.srtt = rtt;
      this.rttvar = rtt / 2;
    } else {
      this.rttvar = (3 * this.rttvar + Math.abs(this.srtt - rtt)) / 4;
      this.srtt = (7 * this.srtt + rtt) / 8;
    }
    this.samples++;
  }

  // shouldRecheck reports whether a port that timed out should be checked
  // again with the full timeout: one in 20, while the timeout is shortened.
  shouldRecheck() {
    if (this.timeout() >= this.max) return false;
    this.timeouts++;
    return this.timeouts % 20 === 1;
  }

  // missed records a port found open only with the full timeout.
  missed(rtt) {
    this.misses++;
    this.floor = Math.max(this.floor, 2 * rtt);
    this.sample(rtt);
  }
}

// testPort tests one port and returns {status, rtt}. net does the network
// requests, so tests can fake them:
//   net.request(port, timeout) asks the server's port for the probe and
//     resolves to {type: 'response', body, ms}, {type: 'aborted'} if the
//     timeout passed, or {type: 'failed', ms}
//   net.browserBlocks(port) resolves to whether the browser refuses the port
//   net.answers(port) resolves to whether anything answers HTTP on the port,
//     even without the headers that let the page read the answer
export async function testPort(port, net, { token, estimator, retries }) {
  for (let attempt = 0; attempt <= retries; attempt++) {
    const r = await net.request(port, estimator.timeout());
    if (r.type === 'response') {
      if (r.body !== token) return { status: 'interfered' };
      estimator.sample(r.ms);
      return { status: 'open', rtt: r.ms };
    }
    if (r.type === 'failed') {
      if (await net.browserBlocks(port)) return { status: 'browser' };
      // something refused the connection, or answered without letting the
      // page read the answer, as a proxy or captive portal does
      return { status: (await net.answers(port)) ? 'interfered' : 'rejected' };
    }
  }
  if (estimator.shouldRecheck()) {
    const r = await net.request(port, estimator.max);
    if (r.type === 'response' && r.body === token) {
      estimator.missed(r.ms);
      return { status: 'open', rtt: r.ms, late: true };
    }
  }
  return { status: 'dropped' };
}

// testUDPPort tests one UDP port with a STUN request and returns {status,
// rtt}: open if the server answered, or dropped. A closed UDP port gives a
// browser no other sign, so rejected and dropped cannot be told apart.
//   net.stun(port, timeout) resolves to {type: 'response', ms} or
//     {type: 'aborted'} if the timeout passed
export async function testUDPPort(port, net, { estimator, retries }) {
  for (let attempt = 0; attempt <= retries; attempt++) {
    const r = await net.stun(port, estimator.timeout());
    if (r.type === 'response') {
      estimator.sample(r.ms);
      return { status: 'open', rtt: r.ms };
    }
  }
  if (estimator.shouldRecheck()) {
    const r = await net.stun(port, estimator.max);
    if (r.type === 'response') {
      estimator.missed(r.ms);
      return { status: 'open', rtt: r.ms, late: true };
    }
  }
  return { status: 'dropped' };
}

// blockedCheck decides how to tell the ports the browser refuses. A refused
// port fails at once, before any network traffic, for any address, while a
// port that is not refused waits for an answer. So a port that fails quickly
// against the server is tried again against a documentation address that
// never answers: failing at once there too means the browser refused it.
// That needs the documentation address to really not answer, so the page
// first tries it on its own port, which the browser allows: holdMs is how
// long that request waited before failing or being given up on, at
// holdLimit. blockedMs are how long requests to ports the standard lists
// took to fail there. It returns {threshold} for requests to the
// documentation address that fail within threshold ms meaning blocked, or
// {threshold: null} to use STANDARD_BLOCKED instead, with the reason.
export function blockedCheck(holdMs, holdLimit, blockedMs) {
  if (holdMs < holdLimit) {
    return { threshold: null, reason: 'the test address does not hold connections open on this network' };
  }
  if (blockedMs.length === 0) {
    return { threshold: null, reason: 'this browser blocks none of the standard ports' };
  }
  const threshold = Math.min(Math.max(4 * Math.max(...blockedMs), 50), holdLimit / 2);
  return { threshold };
}

// ETA estimates the time left from the average rate over the whole scan,
// counting down between updates and easing toward new values, like the
// command line client.
export class ETA {
  constructor(total, now) {
    this.total = total;
    this.started = now;
    this.estimate = null;
    this.estimated = 0;
  }

  update(done, now) {
    if (done >= this.total) return 0;
    const elapsed = now - this.started;
    if (done <= 0 || elapsed < 2000) return null;
    const target = (elapsed * (this.total - done)) / done;
    if (this.estimate === null) {
      this.estimate = target;
    } else {
      const dt = now - this.estimated;
      const projected = Math.max(this.estimate - dt, 0);
      this.estimate = projected + (1 - Math.exp(-dt / 5000)) * (target - projected);
    }
    this.estimated = now;
    return this.estimate;
  }
}

// roundETA rounds milliseconds left for display: up to 5 seconds below a
// minute, to 10 seconds below 10 minutes, and to minutes beyond.
export function roundETA(ms) {
  if (ms <= 0) return 0;
  if (ms < 60000) return Math.ceil(ms / 5000) * 5000;
  if (ms < 600000) return Math.round(ms / 10000) * 10000;
  return Math.round(ms / 60000) * 60000;
}

// formatDuration formats milliseconds as "45s", "3m 20s", "12m" or "1h 5m".
export function formatDuration(ms) {
  const s = Math.round(ms / 1000);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return m > 0 ? `${h}h ${m}m` : `${h}h`;
  if (m > 0) return sec > 0 ? `${m}m ${sec}s` : `${m}m`;
  return `${sec}s`;
}

// mapColumns splits ports first to last into count columns and counts the
// ports of each status in each, from ranges. Column i covers ports from
// columnStart(i) up to columnStart(i + 1). For other values than statuses,
// such as a diff's changes, keys lists them and field names them in ranges.
export function mapColumns(ranges, first, last, count, keys = STATUSES, field = 'status') {
  const span = last - first + 1;
  count = Math.max(1, Math.min(count, span));
  const start = (i) => first + Math.floor((span * i) / count);
  const cols = Array.from({ length: count }, () => new Array(keys.length).fill(0));
  let col = 0;
  for (const r of ranges) {
    let port = Math.max(r.first, first);
    const end = Math.min(r.last, last);
    const s = keys.indexOf(r[field]);
    if (s < 0) continue;
    while (port <= end) {
      while (col + 1 < count && start(col + 1) <= port) col++;
      const colEnd = col + 1 < count ? start(col + 1) - 1 : last;
      const upTo = Math.min(end, colEnd);
      cols[col][s] += upTo - port + 1;
      port = upTo + 1;
    }
  }
  return { cols, start };
}

// segmentHeights returns the height of each status's segment in a map
// column: in proportion, except that every status present gets at least
// minimum so a few ports among many stay visible, taken from the tallest.
export function segmentHeights(counts, size, height, minimum) {
  const h = counts.map((n) => (n > 0 && size > 0 ? Math.max((height * n) / size, minimum) : 0));
  const excess = h.reduce((a, b) => a + b, 0) - height;
  if (excess > 0) {
    let tallest = 0;
    h.forEach((v, i) => { if (v > h[tallest]) tallest = i; });
    h[tallest] = Math.max(h[tallest] - excess, minimum);
  }
  return h;
}

// largestClosed returns the largest runs of consecutive closed ports, not
// counting ports the browser refused, as [{first, last, status}] with status
// null for runs of mixed statuses, largest first.
export function largestClosed(ranges, limit) {
  const runs = [];
  for (const r of ranges) {
    if (r.status === 'open' || r.status === 'browser') continue;
    const prev = runs[runs.length - 1];
    if (prev && prev.last + 1 === r.first) {
      prev.last = r.last;
      if (prev.status !== r.status) prev.status = null;
    } else {
      runs.push({ ...r });
    }
  }
  return runs.sort((a, b) => (b.last - b.first) - (a.last - a.first) || a.first - b.first).slice(0, limit);
}

// rttStats summarizes response times like the command line client's report.
export function rttStats(rtts) {
  if (rtts.length === 0) return undefined;
  const sorted = [...rtts].sort((a, b) => a - b);
  const pct = (p) => sorted[Math.max(Math.ceil(p * sorted.length) - 1, 0)];
  const ms = (v) => Math.round(v * 10) / 10;
  return { min: ms(sorted[0]), median: ms(pct(0.5)), p95: ms(pct(0.95)), max: ms(sorted[sorted.length - 1]) };
}

// buildReport returns the scan's report in the command line client's -json
// format, with the browser's additions: "client_address", the client's
// address as the server saw it, and "browser_blocked".
// kinds are the kinds tested, each {kind, results, estimator, retries}.
export function buildReport({ info, host, ports, kinds, started, finished, canceled, options, detection, scheme = 'http' }) {
  const seconds = (finished - started) / 1000;
  const done = kinds.reduce((sum, k) => sum + k.results.done, 0);
  return {
    version: info.version,
    client: 'browser',
    client_address: info.client,
    host,
    started: new Date(started).toISOString(),
    finished: new Date(finished).toISOString(),
    duration_seconds: Math.round(seconds * 1000) / 1000,
    tested_per_second: seconds > 0 ? Math.round((done / seconds) * 10) / 10 : 0,
    canceled,
    options: {
      ports: formatPorts(ports),
      tcp: kinds.some((k) => k.kind === 'tcp'),
      udp: kinds.some((k) => k.kind === 'udp'),
      ipv4: false,
      ipv6: false,
      timeout_ms: options.timeout,
      adaptive: options.adaptive,
      retries: -1,
      parallel: options.parallel,
      rate: options.rate,
      multi: 1,
    },
    browser_blocked_detection: detection,
    // how TCP ports were tested: "http" or "https" requests
    tcp_requests: scheme,
    kinds: kinds.map(({ kind, results, estimator, retries }) => {
      const n = (s) => results.count(s);
      return {
        kind,
        address: info.server,
        tested: results.done,
        untested: results.ports.length - results.done,
        open: n('open'),
        rejected: n('rejected'),
        dropped: n('dropped'),
        interfered: n('interfered'),
        errors: n('error'),
        browser_blocked: n('browser'),
        late: estimator.misses,
        retries,
        timeout_ms: Math.round(estimator.timeout() * 10) / 10,
        open_rtt_ms: rttStats(results.rtts),
        ranges: results.ranges(),
      };
    }),
  };
}

// parseReport parses a report as JSON, as written by any portquiz client,
// and checks its ranges: in order, without overlaps, and of valid ports.
export function parseReport(text) {
  let r;
  try {
    r = JSON.parse(text);
  } catch (e) {
    throw new Error(`not a portquiz report: ${e.message}`);
  }
  if (r === null || typeof r !== 'object' || Array.isArray(r)) throw new Error('not a portquiz report');
  if ('a' in r && 'b' in r) throw new Error('this is a comparison of two reports, not a report');
  if (!Array.isArray(r.kinds)) throw new Error('not a portquiz report: it has no kinds');
  for (const k of r.kinds) {
    if (k === null || typeof k !== 'object' || !Array.isArray(k.ranges ?? [])) throw new Error('not a portquiz report: bad kind');
    let prev = 0;
    for (const x of k.ranges ?? []) {
      if (!Number.isInteger(x?.first) || !Number.isInteger(x.last) || typeof x.status !== 'string' ||
          x.first <= prev || x.last > MAX_PORT || x.first > x.last) {
        throw new Error(`not a portquiz report: bad range ${x?.first}-${x?.last} in kind "${k.kind}"`);
      }
      prev = x.last;
    }
  }
  return r;
}

// Comparing two runs. This mirrors scan/diff.go, which the command line
// client and the Android app use, and gives the same JSON; core_test.mjs
// checks both diff scan/testdata/diff the same way.

// The ways a port's result can compare between two runs, A and B.
export const CHANGES = ['opened', 'closed', 'reason_changed', 'unchanged', 'only_a', 'only_b'];

// kindKey is the protocol and IP version of a kind: from the kind's name,
// such as "tcp6", or for a kind named just "tcp" from its address.
export function kindKey(k) {
  let protocol = k.kind ?? '';
  let version = '';
  if (protocol.length > 3) {
    version = protocol.slice(3);
    protocol = protocol.slice(0, 3);
  } else if (k.address) {
    version = k.address.includes(':') ? '6' : '4';
  }
  return protocol + version;
}

// diffReports compares report a with report b, both as parseReport returns
// them: per kind, matched by protocol and IP version, the ports whose status
// changed from a to b.
export function diffReports(a, b) {
  const keys = [...new Set([...a.kinds, ...b.kinds].map(kindKey))].sort();
  return {
    a: runInfo(a),
    b: runInfo(b),
    kinds: keys.map((key) => diffKind(key, testedKind(a, key), testedKind(b, key))),
  };
}

function runInfo(r) {
  const out = { host: r.host ?? '' };
  for (const f of ['label', 'client', 'client_address', 'tcp_requests', 'version']) {
    if (r[f]) out[f] = r[f];
  }
  return { ...out, started: r.started, finished: r.finished, canceled: !!r.canceled };
}

// testedKind returns the kind of r matching key, or null, with only the
// ranges of ports with a result, and "closed" for closed ports when the
// client cannot tell how they were closed: a browser cannot tell a rejected
// UDP port from a dropped one.
function testedKind(r, key) {
  const k = r.kinds.find((x) => kindKey(x) === key);
  if (!k) return null;
  const unknown = r.client === 'browser' && k.kind.startsWith('udp');
  const ranges = (k.ranges ?? [])
    .filter((x) => x.status !== 'browser')
    .map((x) => (unknown && x.status !== 'open' ? { ...x, status: 'closed' } : x));
  return { ...k, ranges };
}

// diffKind compares kind a with kind b, as testedKind returns them: either
// may be null.
function diffKind(key, a, b) {
  const ar = a?.ranges ?? [];
  const br = b?.ranges ?? [];
  const open = (ranges) => ranges.reduce((n, r) => n + (r.status === 'open' ? r.last - r.first + 1 : 0), 0);
  const counts = Object.fromEntries(CHANGES.map((c) => [c, 0]));
  const changes = [];
  const ranges = [];
  // walk the ports with a result in either run, a stretch at a time up to
  // where a range of either starts or ends
  let i = 0;
  let j = 0;
  for (let port = 1; ;) {
    while (i < ar.length && ar[i].last < port) i++;
    while (j < br.length && br[j].last < port) j++;
    if (i === ar.length && j === br.length) break;
    let next = MAX_PORT;
    if (i < ar.length) next = Math.min(next, Math.max(port, ar[i].first));
    if (j < br.length) next = Math.min(next, Math.max(port, br[j].first));
    port = next;
    let sa = '';
    let sb = '';
    let last = MAX_PORT;
    if (i < ar.length) {
      if (ar[i].first <= port) {
        sa = ar[i].status;
        last = Math.min(last, ar[i].last);
      } else {
        last = Math.min(last, ar[i].first - 1);
      }
    }
    if (j < br.length) {
      if (br[j].first <= port) {
        sb = br[j].status;
        last = Math.min(last, br[j].last);
      } else {
        last = Math.min(last, br[j].first - 1);
      }
    }
    const change = compareStatus(sa, sb);
    counts[change] += last - port + 1;
    const r = ranges[ranges.length - 1];
    if (r && r.last + 1 === port && r.change === change) r.last = last;
    else ranges.push({ first: port, last, change });
    if (change === 'opened' || change === 'closed' || change === 'reason_changed') {
      const c = changes[changes.length - 1];
      if (c && c.last + 1 === port && c.a === sa && c.b === sb) c.last = last;
      else changes.push({ first: port, last, a: sa, b: sb });
    }
    port = last + 1;
  }
  const out = {
    kind: key,
    a_kind: a?.kind ?? '',
    b_kind: b?.kind ?? '',
    a_address: a?.address ?? '',
    b_address: b?.address ?? '',
    compared: counts.unchanged + counts.opened + counts.closed + counts.reason_changed,
    unchanged: counts.unchanged,
    opened: counts.opened,
    closed: counts.closed,
    reason_changed: counts.reason_changed,
    only_a: counts.only_a,
    only_b: counts.only_b,
    a_open: open(ar),
    b_open: open(br),
  };
  if (a?.open_rtt_ms) out.a_open_rtt_ms = a.open_rtt_ms;
  if (b?.open_rtt_ms) out.b_open_rtt_ms = b.open_rtt_ms;
  return { ...out, changes, ranges };
}

// compareStatus returns how status a compares with status b, either of
// which is '' for a port without a result in that run.
function compareStatus(a, b) {
  if (b === '') return 'only_a';
  if (a === '') return 'only_b';
  if (a === b) return 'unchanged';
  if (b === 'open') return 'opened';
  if (a === 'open') return 'closed';
  // closed in both, and how is only known for one
  if (a === 'closed' || b === 'closed') return 'unchanged';
  return 'reason_changed';
}

// changed counts the ports whose status changed in a kind's diff.
export function changed(k) {
  return k.opened + k.closed + k.reason_changed;
}
