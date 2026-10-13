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

// Services people often need, checked in the summary. Only TCP is tested
// from a browser.
export const SERVICES = [
  ['FTP', 21], ['SSH', 22], ['Telnet', 23], ['SMTP', 25], ['DNS', 53], ['HTTP', 80],
  ['POP3', 110], ['IMAP', 143], ['HTTPS', 443], ['SMB', 445], ['SMTPS', 465],
  ['SMTP submission', 587], ['DNS over TLS', 853], ['IMAPS', 993], ['POP3S', 995],
  ['OpenVPN', 1194], ['PPTP', 1723], ['MySQL', 3306], ['RDP', 3389], ['SIP', 5060],
  ['PostgreSQL', 5432], ['HTTP alt', 8080], ['HTTPS alt', 8443],
];

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
// columnStart(i) up to columnStart(i + 1).
export function mapColumns(ranges, first, last, count) {
  const span = last - first + 1;
  count = Math.max(1, Math.min(count, span));
  const start = (i) => first + Math.floor((span * i) / count);
  const cols = Array.from({ length: count }, () => new Array(STATUSES.length).fill(0));
  let col = 0;
  for (const r of ranges) {
    let port = Math.max(r.first, first);
    const end = Math.min(r.last, last);
    const s = STATUSES.indexOf(r.status);
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
// format, with the browser's additions: "client" and "browser_blocked".
export function buildReport({ info, host, results, started, finished, canceled, options, estimator, retries, detection }) {
  const ranges = results.ranges();
  const seconds = (finished - started) / 1000;
  const n = (s) => results.count(s);
  return {
    version: info.version,
    client: 'browser',
    host,
    started: new Date(started).toISOString(),
    finished: new Date(finished).toISOString(),
    duration_seconds: Math.round(seconds * 1000) / 1000,
    tested_per_second: seconds > 0 ? Math.round((results.done / seconds) * 10) / 10 : 0,
    canceled,
    options: {
      ports: formatPorts(results.ports),
      tcp: true,
      udp: false,
      ipv4: false,
      ipv6: false,
      timeout_ms: options.timeout,
      adaptive: options.adaptive,
      retries,
      parallel: options.parallel,
      rate: options.rate,
      multi: 1,
    },
    browser_blocked_detection: detection,
    kinds: [{
      kind: 'tcp',
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
      ranges,
    }],
  };
}
