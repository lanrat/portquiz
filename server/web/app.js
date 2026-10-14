// The browser test page: tests TCP ports with HTTP requests to the server,
// and UDP ports with STUN requests through WebRTC, since the server answers
// both on every port, and summarizes the results. The logic that does not
// need a browser is in core.js.
import * as core from './core.js';

const $ = (id) => document.getElementById(id);
const numbers = new Intl.NumberFormat();
const decimals = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });
const percent = new Intl.NumberFormat(undefined, { style: 'percent', maximumFractionDigits: 1 });
const dates = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' });

// the server's name or address as the page was loaded, with brackets around
// an IPv6 address, so it can go straight into URLs
const host = location.hostname;
// TCP ports are tested over HTTPS from a page loaded over HTTPS, since such
// a page may not make plain HTTP requests, and over HTTP otherwise
const scheme = location.protocol === 'https:' ? 'https' : 'http';
const pagePort = Number(location.port) || (scheme === 'https' ? 443 : 80);
// documentation addresses that never answer, see core.blockedCheck
const testAddress = host.includes(':') ? '[2001:db8::1]' : '192.0.2.1';
const holdLimit = 1000;
// how many result lines are shown; the saved report has all of them
const maxLines = 2000;
// how many STUN checks one WebRTC connection makes before it is replaced:
// each check leaves resources behind until the connection is closed
const stunChecksPerConnection = 50;
// how many changed ranges a comparison lists; its saved JSON has all of them
const maxChanges = 200;

const statusLabels = {
  open: 'open',
  rejected: 'rejected',
  dropped: 'dropped',
  interfered: 'interfered',
  error: 'errors',
  browser: 'blocked by the browser',
};

// label names a status; a UDP port that is not open just did not answer
function label(kind, status) {
  return kind === 'udp' && status === 'dropped' ? 'no answer' : statusLabels[status];
}

// how a comparison names changes
const changeNames = {
  opened: 'opened',
  closed: 'closed',
  reason_changed: 'closed differently',
  unchanged: 'unchanged',
  only_a: 'only tested in A',
  only_b: 'only tested in B',
};

let info = null;
let scan = null;

// past runs, newest first, as {id, added, report}; see loadHistory
const historyKey = 'portquiz-history';
const historyLimit = 20;
let history = [];
// whether past runs can be kept in this browser's storage
let historyStored = true;
// the ids of the runs checked for comparing, and of the runs compared
const selected = new Set();
let compared = null;

init();

async function init() {
  $('form').addEventListener('submit', (e) => {
    e.preventDefault();
    if (scan?.running) stop();
    else start();
  });
  for (const input of document.querySelectorAll('input[name="filter"]')) {
    input.addEventListener('change', renderResults);
  }
  $('save').addEventListener('click', save);
  $('compare').addEventListener('click', compareSelected);
  $('import-button').addEventListener('click', () => $('import').click());
  $('import').addEventListener('change', importFiles);
  $('clear-history').addEventListener('click', clearHistory);
  window.addEventListener('storage', (e) => {
    if (e.key === historyKey) {
      history = loadHistory();
      renderHistory();
    }
  });
  history = loadHistory();
  renderHistory();
  document.addEventListener('visibilitychange', () => {
    $('background').hidden = !(scan?.running && document.hidden);
  });
  try {
    const res = await fetch('/info', { cache: 'no-store' });
    info = await res.json();
  } catch {
    showError('Could not reach the portquiz server. Reload the page to try again.');
    return;
  }
  $('server').textContent = host;
  $('mode').textContent = scheme === 'https'
    ? 'TCP ports are tested with encrypted (HTTPS) requests.'
    : 'TCP ports are tested with plain web (HTTP) requests.';
  if (info.https && scheme === 'http') {
    $('other').replaceChildren('There is also ', el('a', 'an HTTPS version', { href: `https://${info.https}/` }), ' that tests with encrypted requests.');
  } else if (scheme === 'https') {
    // ?http has the server send browsers that upgrade the link to HTTPS
    // back to HTTP, see keepHTTP in web.go
    $('other').replaceChildren('There is also ', el('a', 'a plain HTTP version', { href: `http://${host}/?http` }), ' that tests with unencrypted requests.');
  }
  $('version').textContent = info.version;
  const udpUnavailable = !info.udp
    ? 'This server does not answer UDP.'
    : typeof RTCPeerConnection === 'undefined' ? 'UDP needs WebRTC, which this browser has turned off.' : '';
  if (udpUnavailable) {
    $('udp').checked = false;
    $('udp').disabled = true;
    $('udp-note').textContent = udpUnavailable;
  }
  $('start').disabled = false;
}

function showError(text) {
  $('error').textContent = text;
  $('error').hidden = !text;
}

function number(id, fallback, min, max) {
  const v = Number($(id).value);
  return Number.isFinite(v) && $(id).value.trim() !== '' ? Math.min(Math.max(Math.round(v), min), max) : fallback;
}

async function start() {
  let ports;
  try {
    ports = core.parsePorts($('ports').value);
  } catch (e) {
    showError(`Ports: ${e.message}`);
    return;
  }
  const protocols = ['tcp', 'udp'].filter((p) => $(p).checked && !$(p).disabled);
  if (protocols.length === 0) {
    showError('Choose TCP, UDP or both.');
    return;
  }
  showError('');
  const options = {
    timeout: number('timeout', 2000, 100, 60000),
    adaptive: $('adaptive').checked,
    parallel: number('parallel', 128, 1, 256),
    udpParallel: number('udp-parallel', 32, 1, 64),
    rate: number('rate', 500, 0, 100000),
  };
  const kinds = protocols.map((kind) => ({
    kind,
    results: new core.Results(ports),
    estimator: new core.TimeoutEstimator(options.timeout, options.adaptive),
    // like the command line client: UDP has no retransmission of its own,
    // so it is retried once; TCP once with the adaptive timeout, and not
    // without it, where the full timeout covers a lost connection
    retries: kind === 'udp' || options.adaptive ? 1 : 0,
  }));
  scan = {
    running: true,
    stopped: false,
    ports,
    options,
    kinds,
    total: ports.length * kinds.length,
    controllers: new Set(),
    checkers: [],
    nextSlot: 0,
    nonce: Math.random().toString(36).slice(2),
    detection: null,
    started: Date.now(),
    eta: null,
  };
  $('start').textContent = 'Stop';
  $('summary').hidden = true;
  $('save').hidden = true;
  $('progress-section').hidden = false;
  $('results-section').hidden = false;
  $('progress').value = 0;
  $('status').textContent = protocols.includes('tcp') ? 'Checking which ports this browser blocks…' : 'Starting…';
  $('eta').textContent = '';
  for (const input of document.querySelectorAll('#form input')) input.disabled = true;
  renderResults();

  const s = scan;
  s.detection = protocols.includes('tcp') ? await detectBlocked(s) : { threshold: null, reason: 'TCP was not tested' };
  if (s.stopped) return;
  s.started = Date.now();
  s.eta = new core.ETA(s.total, performance.now());
  const ticker = setInterval(() => render(s), 250);
  const lister = setInterval(renderResults, 1000);

  await Promise.all(kinds.map((k) => runKind(s, k)));
  for (const c of s.checkers) c.close();
  clearInterval(ticker);
  clearInterval(lister);
  finish(s, s.stopped);
}

// runKind tests every port of one kind, several at once.
async function runKind(s, k) {
  const net = network(s);
  let next = 0;
  const worker = async (test) => {
    while (!s.stopped && next < s.ports.length) {
      const port = s.ports[next++];
      const r = await test(port);
      if (s.stopped) return;
      k.results.set(port, r.status, r.rtt);
    }
  };
  const opts = { token: info.token, estimator: k.estimator, retries: k.retries };
  if (k.kind === 'tcp') {
    const n = Math.min(s.options.parallel, s.ports.length);
    await Promise.all(Array.from({ length: n }, () => worker((port) => core.testPort(port, net, opts))));
  } else {
    const n = Math.min(s.options.udpParallel, s.ports.length);
    await Promise.all(Array.from({ length: n }, () => {
      const checker = new StunChecker(s);
      s.checkers.push(checker);
      const udpNet = { stun: (port, timeout) => checker.check(port, timeout) };
      return worker((port) => core.testUDPPort(port, udpNet, opts));
    }));
  }
}

function stop() {
  const s = scan;
  s.stopped = true;
  for (const c of s.controllers) c.abort();
  for (const c of s.checkers) c.abort();
  if (s.detection === null) finish(s, true);
}

function finish(s, canceled) {
  if (!s.running) return;
  s.running = false;
  s.canceled = canceled;
  // stopped while checking which ports the browser blocks
  s.detection ??= { threshold: null, reason: 'the test was stopped before it could check' };
  s.finished = Date.now();
  $('start').textContent = 'Start';
  for (const input of document.querySelectorAll('#form input')) input.disabled = false;
  if (!info.udp || typeof RTCPeerConnection === 'undefined') $('udp').disabled = true;
  $('background').hidden = true;
  render(s);
  $('eta').textContent = '';
  renderResults();
  renderSummary(s);
  s.report = buildReport(s);
  $('save').hidden = false;
  if (done(s) > 0) addRun(s.report);
}

// network returns the requests core.testPort makes, for scan s.
function network(s) {
  return {
    request: async (port, timeout) => {
      if (s.stopped) return { type: 'aborted' };
      await pace(s);
      return timed(s, `${scheme}://${host}:${port}/probe?n=${s.nonce}`, 'cors', timeout);
    },
    browserBlocks: async (port) => {
      if (s.stopped) return false;
      if (s.detection.threshold === null) return core.STANDARD_BLOCKED.has(port);
      const r = await timed(s, `${scheme}://${testAddress}:${port}/`, 'no-cors', s.detection.threshold);
      return r.type === 'failed';
    },
    answers: async (port) => {
      if (s.stopped) return false;
      const r = await timed(s, `${scheme}://${host}:${port}/probe?n=${s.nonce}&opaque`, 'no-cors', s.kinds[0].estimator.max);
      return r.type === 'response';
    },
  };
}

// timed requests url and reports how it went: see core.testPort.
async function timed(s, url, mode, timeout) {
  const ctrl = new AbortController();
  s.controllers.add(ctrl);
  const timer = setTimeout(() => ctrl.abort(), timeout);
  const t0 = performance.now();
  try {
    const res = await fetch(url, { mode, signal: ctrl.signal, cache: 'no-store', credentials: 'omit', referrerPolicy: 'no-referrer' });
    const body = mode === 'cors' ? await res.text() : '';
    return { type: 'response', body, ms: performance.now() - t0 };
  } catch {
    return ctrl.signal.aborted ? { type: 'aborted' } : { type: 'failed', ms: performance.now() - t0 };
  } finally {
    clearTimeout(timer);
    s.controllers.delete(ctrl);
  }
}

// StunChecker tests UDP ports one at a time: browsers cannot send raw UDP,
// but WebRTC asks a STUN server for the browser's public address while it
// gathers ways to connect, and the server answers STUN on every port. One
// connection is reused, pointed at each port in turn with an ICE restart,
// since browsers only allow a few hundred connections; each restart has a
// new ufrag, so a late answer for an earlier port is never counted.
class StunChecker {
  constructor(s) {
    this.s = s;
    this.pc = null;
    this.uses = 0;
    this.waiting = null;
  }

  async open() {
    this.pc?.close();
    // browsers limit how many connections a page has, counting closed ones
    // until they are garbage collected, so wait for room if needed
    for (;;) {
      try {
        this.pc = new RTCPeerConnection({ iceServers: [] });
        break;
      } catch {
        if (this.s.stopped) return;
        await new Promise((r) => setTimeout(r, 500));
      }
    }
    this.pc.createDataChannel('portquiz');
    this.pc.onicecandidate = (e) => {
      const w = this.waiting;
      if (w && e.candidate?.type === 'srflx' && e.candidate.usernameFragment === w.ufrag) w.resolve({ type: 'response', ms: performance.now() - w.t0 });
    };
    this.uses = 0;
  }

  async check(port, timeout) {
    if (this.s.stopped) return { type: 'aborted' };
    await pace(this.s);
    if (!this.pc || this.uses >= stunChecksPerConnection) await this.open();
    if (this.s.stopped) return { type: 'aborted' };
    this.uses++;
    try {
      this.pc.setConfiguration({ iceServers: [{ urls: `stun:${host}:${port}` }] });
      this.pc.restartIce();
      const offer = await this.pc.createOffer({ iceRestart: true });
      const ufrag = /a=ice-ufrag:(\S+)/.exec(offer.sdp)?.[1];
      const result = new Promise((resolve) => {
        this.waiting = { ufrag, resolve, t0: 0 };
      });
      this.waiting.t0 = performance.now();
      await this.pc.setLocalDescription(offer);
      const timer = setTimeout(() => this.waiting?.resolve({ type: 'aborted' }), timeout);
      const r = await result;
      clearTimeout(timer);
      this.waiting = null;
      return r;
    } catch {
      // start over with a new connection
      this.uses = stunChecksPerConnection;
      this.waiting = null;
      return { type: 'aborted' };
    }
  }

  abort() {
    this.waiting?.resolve({ type: 'aborted' });
  }

  close() {
    this.pc?.close();
    this.pc = null;
  }
}

// pace waits for the next connection slot under the rate limit, shared by
// TCP and UDP.
async function pace(s) {
  if (s.options.rate <= 0) return;
  const now = performance.now();
  const slot = Math.max(s.nextSlot, now);
  s.nextSlot = slot + 1000 / s.options.rate;
  if (slot > now) await new Promise((r) => setTimeout(r, slot - now));
}

// detectBlocked works out how to tell which ports the browser refuses, see
// core.blockedCheck.
async function detectBlocked(s) {
  const hold = await timed(s, `${scheme}://${testAddress}:${pagePort}/`, 'no-cors', holdLimit);
  const holdMs = hold.type === 'failed' ? hold.ms : holdLimit;
  const blockedMs = [];
  for (const p of [1, 7, 9]) {
    if (s.stopped) break;
    const r = await timed(s, `${scheme}://${testAddress}:${p}/`, 'no-cors', holdLimit);
    if (r.type === 'failed') blockedMs.push(r.ms);
  }
  return core.blockedCheck(holdMs, holdLimit, blockedMs);
}

function done(s) {
  return s.kinds.reduce((sum, k) => sum + k.results.done, 0);
}

function render(s) {
  const d = done(s);
  $('progress').value = s.total ? d / s.total : 0;
  const open = s.kinds.map((k) => `${numbers.format(k.results.count('open'))} ${k.kind.toUpperCase()} open`).join(' · ');
  $('status').textContent = `${numbers.format(d)} / ${numbers.format(s.total)} tested · ${open}`;
  if (s.running && s.eta) {
    const left = s.eta.update(d, performance.now());
    $('eta').textContent = left === null ? '' : `about ${core.formatDuration(core.roundETA(left))} left`;
  }
}

function selectedFilter() {
  return document.querySelector('input[name="filter"]:checked').value;
}

function renderResults() {
  if (!scan) return;
  const filter = selectedFilter();
  const ranges = scan.kinds
    .flatMap((k) => k.results.ranges().map((r) => ({ ...r, kind: k.kind })))
    .filter((r) => filter === 'all' || (filter === 'open') === (r.status === 'open'))
    .sort((a, b) => a.first - b.first || a.kind.localeCompare(b.kind));
  const items = ranges.slice(0, maxLines).map((r) => {
    const li = document.createElement('li');
    li.className = r.status;
    const ports = r.first === r.last ? `${r.first}` : `${r.first}-${r.last}`;
    li.textContent = `${r.status === 'open' ? 'OPEN' : 'CLOSED'} ${r.kind} ${ports}${r.status === 'open' ? '' : ` (${label(r.kind, r.status)})`}`;
    return li;
  });
  $('results').replaceChildren(...items);
  $('more').textContent = ranges.length > maxLines
    ? `…and ${numbers.format(ranges.length - maxLines)} more ranges, all in the saved report.`
    : (done(scan) > 0 && ranges.length === 0 ? 'No ranges match the filter.' : '');
}

// el creates an element with text or children.
function el(tag, content, attrs = {}) {
  const e = document.createElement(tag);
  if (Array.isArray(content)) e.append(...content);
  else if (content !== undefined) e.textContent = content;
  Object.assign(e, attrs);
  return e;
}

// portCount formats a count of ports: "1 port", "1,200 ports".
function portCount(count) {
  return `${numbers.format(count)} ${count === 1 ? 'port' : 'ports'}`;
}

function portRange(r) {
  return r.first === r.last ? `${r.first}` : `${r.first}–${r.last}`;
}

function renderSummary(s) {
  const out = [el('h2', 'Summary')];
  const seconds = (s.finished - s.started) / 1000;
  const total = done(s);
  const server = host.replace(/^\[|\]$/g, '') === info.server ? host : `${host} (${info.server})`;
  out.push(el('p', `${server} from ${info.client} · tested ${numbers.format(total)} in ${core.formatDuration(seconds * 1000)}${s.kinds.some((k) => k.kind === 'tcp') ? ` · TCP over ${scheme.toUpperCase()}` : ''}`));
  if (s.canceled) out.push(el('p', `Stopped early: ${numbers.format(s.total - total)} not tested.`, { className: 'warning' }));

  for (const k of s.kinds) {
    const r = k.results;
    const tested = r.done - r.count('browser');
    out.push(el('p', `${k.kind.toUpperCase()}: ${numbers.format(r.count('open'))} of ${portCount(tested)} tested open (${percent.format(tested ? r.count('open') / tested : 0)}).`));
    if (r.count('browser') > 0) {
      out.push(el('p', `${portCount(r.count('browser'))} not tested because this browser refuses to use them. ${detectionText(s.detection)}`, { className: 'muted' }));
    }
    if (k.kind === 'tcp' && s.detection.threshold !== null) out.push(...listDifferences(s, k));
    if (k.kind === 'udp' && r.done > 0 && r.count('open') === 0) {
      out.push(el('p', 'No UDP port answered. Either this network blocks UDP to the server, or this browser hides WebRTC answers, as some privacy settings and extensions do; the command line client or Android app can tell which.', { className: 'warning' }));
    }
  }

  if (total >= 100) {
    out.push(el('h2', 'Port map'));
    for (const k of s.kinds) out.push(...portMap(s, k));
  }

  out.push(el('h2', 'How closed ports were closed'));
  const explain = {
    rejected: 'Rejected: actively refused, by a reset or a firewall set to reject.',
    dropped: 'Dropped: nothing came back before the timeout, as when a firewall silently discards traffic.',
    interfered: 'Interfered: something other than the portquiz server answered, as a proxy, captive portal or filter on the path does.',
    error: 'Errors: the browser could not make the request.',
  };
  const shown = new Set();
  for (const k of s.kinds) {
    const r = k.results;
    const closed = ['rejected', 'dropped', 'interfered', 'error'].filter((st) => r.count(st) > 0);
    const name = k.kind.toUpperCase();
    out.push(el('p', closed.length === 0 ? `${name}: no closed ports.` : `${name}: ${closed.map((st) => `${numbers.format(r.count(st))} ${label(k.kind, st)}`).join(', ')}`));
    if (k.kind === 'tcp') closed.forEach((st) => shown.add(st));
    const interfered = r.ranges().filter((x) => x.status === 'interfered');
    if (interfered.length > 0) {
      const list = interfered.slice(0, 20).map(portRange).join(', ');
      out.push(el('p', `${name} interfered ports: ${list}${interfered.length > 20 ? `, …and ${interfered.length - 20} more` : ''}`, { className: 'warning' }));
    }
  }
  for (const st of shown) out.push(el('p', explain[st], { className: 'muted' }));
  if (s.kinds.some((k) => k.kind === 'udp' && k.results.count('dropped') > 0)) {
    out.push(el('p', 'UDP: a port that is not open gives a browser no sign of why, so rejected and dropped UDP ports both show as no answer.', { className: 'muted' }));
  }

  out.push(el('h2', 'Common services'));
  const rows = core.SERVICES.flatMap((svc) => {
    const k = s.kinds.find((x) => x.kind === svc.protocol);
    const code = k?.results.status[svc.port];
    if (!code) return [];
    const st = core.STATUSES[code - 1];
    return [el('tr', [el('td', svc.name), el('td', `${svc.port}/${svc.protocol}`), el('td', label(k.kind, st), { className: st === 'open' ? 'open' : '' })])];
  });
  if (rows.length === 0) out.push(el('p', 'None of these ports were tested.'));
  else out.push(el('table', [el('thead', [el('tr', [el('th', 'Service'), el('th', 'Port'), el('th', 'Result')])]), el('tbody', rows)]));

  out.push(el('h2', 'Largest blocked ranges'));
  for (const k of s.kinds) {
    const runs = core.largestClosed(k.results.ranges(), 5);
    const name = k.kind.toUpperCase();
    if (runs.length === 0) {
      out.push(el('p', `${name}: no blocked ranges.`));
      continue;
    }
    out.push(el('p', name));
    out.push(el('ul', runs.map((x) => el('li', `${portRange(x)} · ${portCount(x.last - x.first + 1)} · ${x.status === null ? 'mixed' : label(k.kind, x.status)}`))));
  }

  out.push(el('h2', 'Speed and timing'));
  out.push(el('p', `Tested ${numbers.format(total)} in ${core.formatDuration(seconds * 1000)}, ${decimals.format(seconds > 0 ? total / seconds : 0)} per second on average.`));
  for (const k of s.kinds) {
    const name = k.kind.toUpperCase();
    const rtt = core.rttStats(k.results.rtts);
    out.push(el('p', rtt
      ? `${name}: open ports answered in ${decimals.format(rtt.median)} ms (median), ${decimals.format(rtt.p95)} ms (95th percentile), ${decimals.format(rtt.min)} to ${decimals.format(rtt.max)} ms (fastest to slowest).`
      : `${name}: no open ports, so no response times.`));
    const e = k.estimator;
    out.push(el('p', s.options.adaptive
      ? `${name}: timeout ${decimals.format(e.timeout())} ms at the end, adapted from up to ${numbers.format(e.max)} ms. Ports that did not answer were retried: ${k.retries}.`
      : `${name}: timeout ${numbers.format(e.max)} ms. Ports that did not answer were retried: ${k.retries}.`, { className: 'muted' }));
    if (e.misses > 0) out.push(el('p', `${name}: ${e.misses === 1 ? '1 open port' : `${e.misses} open ports`} only answered on a slower recheck, so ports that did not answer may be open.`, { className: 'warning' }));
  }

  $('summary').replaceChildren(...out);
  $('summary').hidden = false;
}

function detectionText(d) {
  return d.threshold !== null
    ? 'They were detected by testing in this browser.'
    : `They are the ports browsers are expected to refuse, since they could not be detected here: ${d.reason}.`;
}

// listDifferences notes where the browser's blocked ports, as detected,
// differ from the standard list, among the ports tested.
function listDifferences(s, k) {
  const r = k.results;
  const extra = [];
  const allowed = [];
  for (const p of s.ports) {
    const code = r.status[p];
    if (code === 0) continue;
    const blocked = code === core.BROWSER;
    if (blocked && !core.STANDARD_BLOCKED.has(p)) extra.push(p);
    if (!blocked && core.STANDARD_BLOCKED.has(p)) allowed.push(p);
  }
  const out = [];
  if (extra.length > 0) out.push(el('p', `Blocked here but not on the standard list: ${extra.slice(0, 30).join(', ')}${extra.length > 30 ? ', …' : ''}`, { className: 'muted' }));
  if (allowed.length > 0) out.push(el('p', `On the standard list but allowed by this browser: ${allowed.slice(0, 30).join(', ')}${allowed.length > 30 ? ', …' : ''}`, { className: 'muted' }));
  return out;
}

// portMap draws one kind's results across the ports tested, see mapView.
function portMap(s, k) {
  const legend = el('div', core.STATUSES.filter((st) => k.results.count(st) > 0).map((st) => {
    const item = el('span', `${numbers.format(k.results.count(st))} ${label(k.kind, st)}`);
    item.style.setProperty('--swatch', `var(--map-${st})`);
    return item;
  }), { className: 'legend' });
  const first = s.ports[0];
  const last = s.ports[s.ports.length - 1];
  const span = core.formatPorts(s.ports).replaceAll(',', ', ').replaceAll('-', '–');
  return [
    el('p', `${k.kind.toUpperCase()}, ports ${span.length > 80 ? portRange({ first, last }) : span}`, { className: 'muted' }),
    ...mapView({
      ports: s.ports,
      ranges: k.results.ranges(),
      keys: core.STATUSES,
      field: 'status',
      color: (st) => `--map-${st}`,
      name: (st) => label(k.kind, st),
      title: k.kind.toUpperCase(),
      legend,
    }),
  ];
}

// mapView draws ranges of ports, each with one of keys in its field, such
// as a status: each column covers a run of ports and is split by how many
// had each key, in color(key). The ports are laid out side by side, so a
// list such as 1-1024,8000-8080 is not mostly the gap between them.
// Pointing at the map shows the ports under the pointer. It returns the
// map, the legend and the line that shows what is pointed at.
function mapView({ ports, ranges, keys, field, color, name, title, legend }) {
  const index = new Int32Array(core.MAX_PORT + 1);
  ports.forEach((p, i) => { index[p] = i; });
  const byIndex = ranges.map((x) => ({ first: index[x.first], last: index[x.last], [field]: x[field] }));
  const count = ports.length;
  const canvas = el('canvas', undefined, { className: 'map' });
  canvas.setAttribute('role', 'img');
  canvas.setAttribute('aria-label', `${title} map: ${legend.textContent}`);
  const inspect = el('p', 'Point at the map to see the ports under it.', { className: 'inspect' });

  let layout = null;
  const draw = () => {
    const width = canvas.clientWidth;
    if (width === 0) return;
    const ratio = window.devicePixelRatio || 1;
    canvas.width = Math.round(width * ratio);
    canvas.height = Math.round(64 * ratio);
    const ctx = canvas.getContext('2d');
    const style = getComputedStyle(document.documentElement);
    const colors = keys.map((key) => style.getPropertyValue(color(key)).trim());
    const h = canvas.height;
    ctx.fillStyle = style.getPropertyValue('--map-track').trim();
    ctx.fillRect(0, 0, canvas.width, h);
    const { cols, start: colStart } = core.mapColumns(byIndex, 0, count - 1, Math.floor(width / 2), keys, field);
    layout = { cols, start: colStart };
    const colW = canvas.width / cols.length;
    cols.forEach((counts, i) => {
      const size = (i + 1 < cols.length ? colStart(i + 1) : count) - colStart(i);
      const heights = core.segmentHeights(counts, size, h, 3 * ratio);
      let bottom = h;
      heights.forEach((seg, key) => {
        if (seg <= 0) return;
        ctx.fillStyle = colors[key];
        ctx.fillRect(Math.floor(i * colW), bottom - seg, Math.ceil(colW), seg);
        bottom -= seg;
      });
    });
  };
  const point = (e) => {
    if (!layout) return;
    const rect = canvas.getBoundingClientRect();
    const i = Math.min(Math.max(Math.floor(((e.clientX - rect.left) / rect.width) * layout.cols.length), 0), layout.cols.length - 1);
    const from = ports[layout.start(i)];
    const to = ports[i + 1 < layout.cols.length ? layout.start(i + 1) - 1 : count - 1];
    const parts = layout.cols[i].map((c, key) => (c > 0 ? `${numbers.format(c)} ${name(keys[key])}` : null)).filter(Boolean);
    inspect.textContent = `${title} ports ${portRange({ first: from, last: to })}: ${parts.join(', ') || 'not tested'}`;
  };
  canvas.addEventListener('pointermove', point);
  canvas.addEventListener('pointerdown', point);
  new ResizeObserver(draw).observe(canvas);
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', draw);
  return [canvas, legend, inspect];
}

// buildReport returns scan s's report, in the command line client's -json
// format.
function buildReport(s) {
  return core.buildReport({
    info,
    host,
    ports: s.ports,
    kinds: s.kinds,
    started: s.started,
    finished: s.finished,
    canceled: s.canceled,
    options: s.options,
    detection: s.detection.threshold !== null ? 'runtime' : `standard list: ${s.detection.reason}`,
    scheme,
  });
}

// save downloads the last run's report, with the label it has been given
// since, if any.
function save() {
  const kept = history.find((e) => e.report.started === scan.report.started && e.report.client === 'browser');
  saveReport(kept?.report ?? scan.report);
}

function saveReport(report) {
  const name = (report.host || 'report').replace(/[^A-Za-z0-9.-]/g, '_');
  download(report, `portquiz-${name}-${stamp(report.started)}.json`);
}

// stamp formats a time as 20261013-0930, for file names.
function stamp(time) {
  const d = new Date(time);
  if (Number.isNaN(d.getTime())) return 'unknown';
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`;
}

// download saves value as a JSON file.
function download(value, name) {
  const blob = new Blob([JSON.stringify(value, null, 2) + '\n'], { type: 'application/json' });
  const a = el('a', undefined, { href: URL.createObjectURL(blob), download: name });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 10000);
}

// Past runs are kept in this browser's local storage, so they are only on
// this device, and pages loaded over HTTP and HTTPS each have their own.

// loadHistory returns the runs kept, or none if storage is not available.
function loadHistory() {
  try {
    const list = JSON.parse(localStorage.getItem(historyKey) ?? '[]');
    return Array.isArray(list) ? list.filter((e) => typeof e?.id === 'string' && Array.isArray(e.report?.kinds)) : [];
  } catch {
    return [];
  }
}

// updateHistory applies change to the runs kept, read afresh in case
// another tab changed them, and stores the result: the newest runs up to
// the limit, or fewer if storage is full. Without storage the runs are only
// kept until the page is closed.
function updateHistory(change) {
  let list = change(historyStored ? loadHistory() : history).slice(0, historyLimit);
  for (;;) {
    try {
      localStorage.setItem(historyKey, JSON.stringify(list));
      historyStored = true;
      break;
    } catch {
      if (list.length <= 1) {
        historyStored = false;
        break;
      }
      list = list.slice(0, -1);
    }
  }
  history = list;
  for (const id of selected) if (!history.some((e) => e.id === id)) selected.delete(id);
  if (compared && !compared.every((id) => history.some((e) => e.id === id))) compared = null;
  renderHistory();
  renderComparison();
}

// startedTime returns when a report's run started, for ordering runs.
function startedTime(report) {
  return new Date(report.started).getTime() || 0;
}

function newID() {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

function addRun(report) {
  updateHistory((list) => [{ id: newID(), added: Date.now(), report }, ...list]);
}

// sameRun reports whether two reports are of the same run.
function sameRun(a, b) {
  return a.started === b.started && a.finished === b.finished && a.host === b.host && (a.client ?? '') === (b.client ?? '');
}

async function importFiles() {
  const files = [...$('import').files];
  $('import').value = '';
  const errors = [];
  const added = [];
  for (const file of files) {
    try {
      const report = core.parseReport(await file.text());
      if (history.some((e) => sameRun(e.report, report)) || added.some((r) => sameRun(r, report))) {
        errors.push(`${file.name}: already in the list`);
        continue;
      }
      added.push(report);
    } catch (e) {
      errors.push(`${file.name}: ${e.message}`);
    }
  }
  showError(errors.join('. '));
  if (added.length > 0) {
    // the oldest first, so the list stays in the order they ran
    added.sort((a, b) => startedTime(a) - startedTime(b));
    updateHistory((list) => [...added.reverse().map((report) => ({ id: newID(), added: Date.now(), report })), ...list]);
  }
}

function clearHistory() {
  if (!confirm('Remove all past runs from this browser?')) return;
  selected.clear();
  updateHistory(() => []);
}

function formatDate(time) {
  const d = new Date(time);
  return Number.isNaN(d.getTime()) ? 'unknown time' : dates.format(d);
}

// clientName names the client that ran a report.
function clientName(r) {
  switch (r.client) {
    case 'browser': return r.tcp_requests ? `browser (${r.tcp_requests.toUpperCase()})` : 'browser';
    case 'cli': return 'command line';
    case 'android': return 'Android';
    default: return r.client ?? '';
  }
}

// kindName names a kind such as "tcp", "udp6" or a diff's "tcp4".
function kindName(kind) {
  const protocol = kind.slice(0, 3).toUpperCase();
  const version = kind.slice(3);
  return version ? `${protocol} over IPv${version}` : protocol;
}

// runDetail describes a run on one line: where, how and what it found.
function runDetail(r) {
  const parts = [r.host];
  const client = clientName(r);
  if (client) parts.push(r.client_address ? `${client} from ${r.client_address}` : client);
  for (const k of r.kinds) {
    const tested = (k.tested ?? 0) - (k.browser_blocked ?? 0);
    parts.push(`${kindName(k.kind)} ${numbers.format(k.open ?? 0)} of ${numbers.format(tested)} open`);
  }
  if (r.canceled) parts.push('stopped early');
  return parts.filter(Boolean).join(' · ');
}

function renderHistory() {
  const items = history.map((entry) => {
    const r = entry.report;
    const box = el('input', undefined, { type: 'checkbox', checked: selected.has(entry.id) });
    box.addEventListener('change', () => {
      if (box.checked) selected.add(entry.id);
      else selected.delete(entry.id);
      updateCompareButton();
    });
    const title = el('span', `${formatDate(r.started)}${r.label ? ` · ${r.label}` : ''}`, { className: 'run-title' });
    const actions = el('div', [
      button('Rename', () => rename(entry, actions)),
      button('Save', () => saveReport(entry.report)),
      button('Delete', () => updateHistory((list) => list.filter((e) => e.id !== entry.id))),
    ], { className: 'run-actions' });
    return el('li', [
      el('label', [box, title], { className: 'check' }),
      el('div', runDetail(r), { className: 'run-detail muted' }),
      actions,
    ]);
  });
  $('history').replaceChildren(...items);
  $('clear-history').hidden = history.length === 0;
  $('history-note').textContent = !historyStored
    ? 'This browser does not let the page keep past runs, so they are only kept until you close it.'
    : history.length === 0
      ? 'Finished runs are kept here, the last 20, to compare. You can also import reports saved by any portquiz client, such as the command line client\'s -json.'
      : 'This browser keeps your last 20 runs. Check two to compare them. You can also import reports saved by any portquiz client.';
  updateCompareButton();
}

function updateCompareButton() {
  $('compare').disabled = selected.size !== 2;
  $('compare').textContent = selected.size === 2 || selected.size === 0 ? 'Compare' : `Compare (${selected.size} of 2 checked)`;
}

function button(text, onClick) {
  const b = el('button', text, { type: 'button', className: 'link' });
  b.addEventListener('click', onClick);
  return b;
}

// rename lets the user edit a run's label in place of its actions.
function rename(entry, actions) {
  const input = el('input', undefined, { value: entry.report.label ?? '', placeholder: 'e.g. home wifi', maxLength: 100 });
  input.setAttribute('aria-label', 'Label');
  let ended = false;
  const end = (keep) => {
    if (ended) return;
    ended = true;
    if (!keep) {
      renderHistory();
      return;
    }
    const text = input.value.trim();
    updateHistory((list) => list.map((e) => {
      if (e.id !== entry.id) return e;
      const report = { ...e.report, label: text };
      if (!text) delete report.label;
      return { ...e, report };
    }));
  };
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') end(true);
    if (e.key === 'Escape') end(false);
  });
  input.addEventListener('blur', () => end(true));
  actions.replaceChildren(input);
  input.focus();
}

// compareSelected compares the two checked runs, the earlier one as A.
function compareSelected() {
  const runs = history.filter((e) => selected.has(e.id));
  if (runs.length !== 2) return;
  runs.sort((x, y) => startedTime(x.report) - startedTime(y.report));
  compared = runs.map((e) => e.id);
  renderComparison();
  $('comparison').scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function renderComparison() {
  const runs = compared?.map((id) => history.find((e) => e.id === id));
  if (!runs || runs.some((e) => !e)) {
    $('comparison').hidden = true;
    $('comparison').replaceChildren();
    return;
  }
  const [a, b] = runs.map((e) => e.report);
  const d = core.diffReports(a, b);
  const swap = el('button', 'Swap A and B', { type: 'button', className: 'secondary' });
  swap.addEventListener('click', () => {
    compared.reverse();
    renderComparison();
  });
  const saveDiff = el('button', 'Save JSON', { type: 'button', className: 'secondary' });
  saveDiff.addEventListener('click', () => download(d, `portquiz-compare-${stamp(a.started)}-${stamp(b.started)}.json`));
  const close = el('button', 'Close', { type: 'button', className: 'secondary' });
  close.addEventListener('click', () => {
    compared = null;
    renderComparison();
  });
  const out = [el('div', [el('h2', 'Comparison'), swap, saveDiff, close], { className: 'compare-head' })];
  for (const [tag, r] of [['A', a], ['B', b]]) {
    out.push(el('p', [el('span', tag, { className: 'run-tag' }), `${formatDate(r.started)}${r.label ? ` · ${r.label}` : ''} · ${runDetail(r)}`]));
  }
  if (a.host !== b.host) out.push(el('p', `These runs tested different servers: ${a.host} and ${b.host}.`, { className: 'warning' }));
  if ((a.client === 'browser') !== (b.client === 'browser')) {
    out.push(el('p', 'One run is from a browser, which cannot test the ports browsers block and tells less about how closed ports were closed, so those ports are not compared or compared as just closed.', { className: 'muted' }));
  }
  if (d.kinds.every((k) => core.changed(k) === 0) && d.kinds.some((k) => k.compared > 0)) {
    out.push(el('p', 'No port\'s result changed.'));
  }

  for (const k of d.kinds) {
    out.push(el('h3', kindName(k.kind)));
    if (!k.a_kind || !k.b_kind) {
      out.push(el('p', `Only tested in ${k.a_kind ? 'A' : 'B'}.`, { className: 'muted' }));
      continue;
    }
    const counts = [`${numbers.format(k.opened)} opened`, `${numbers.format(k.closed)} closed`, `${numbers.format(k.reason_changed)} closed differently`, `${numbers.format(k.unchanged)} unchanged`];
    if (k.only_a + k.only_b > 0) counts.push(`${portCount(k.only_a + k.only_b)} tested in only one run`);
    out.push(el('p', counts.join(', ') + '.'));
    const rtt = (x) => (x ? `${decimals.format(x.median)} ms` : 'none');
    const times = k.a_open_rtt_ms || k.b_open_rtt_ms
      ? ` Median response time of open ports: ${rtt(k.a_open_rtt_ms)} in A, ${rtt(k.b_open_rtt_ms)} in B.`
      : '';
    out.push(el('p', `Open: ${numbers.format(k.a_open)} in A, ${numbers.format(k.b_open)} in B.${times}`, { className: 'muted' }));
    if (k.a_address !== k.b_address) out.push(el('p', `Server address: ${k.a_address} in A, ${k.b_address} in B.`, { className: 'muted' }));
    if (k.ranges.length > 0) {
      const ports = k.ranges.flatMap((x) => Array.from({ length: x.last - x.first + 1 }, (_, i) => x.first + i));
      const present = core.CHANGES.filter((c) => k[c] > 0);
      const legend = el('div', present.map((c) => {
        const item = el('span', `${numbers.format(k[c])} ${changeNames[c]}`);
        item.style.setProperty('--swatch', `var(--change-${c})`);
        return item;
      }), { className: 'legend' });
      out.push(...mapView({
        ports,
        ranges: k.ranges,
        keys: core.CHANGES,
        field: 'change',
        color: (c) => `--change-${c}`,
        name: (c) => changeNames[c],
        title: kindName(k.kind),
        legend,
      }));
    }
    const protocol = k.kind.slice(0, 3);
    const lines = k.changes.slice(0, maxChanges).map((c) => {
      const services = core.SERVICES.filter((x) => x.protocol === protocol && x.port >= c.first && x.port <= c.last).map((x) => x.name);
      const names = [...new Set(services)];
      const size = c.last - c.first + 1;
      return el('li', `${portRange(c)}${names.length ? ` (${names.slice(0, 3).join(', ')}${names.length > 3 ? ', …' : ''})` : ''}: ${c.a} → ${c.b}${size > 1 ? ` · ${portCount(size)}` : ''}`);
    });
    if (lines.length > 0) out.push(el('ul', lines));
    if (k.changes.length > maxChanges) out.push(el('p', `…and ${numbers.format(k.changes.length - maxChanges)} more changed ranges, all in the saved comparison.`, { className: 'muted' }));
  }
  $('comparison').replaceChildren(...out);
  $('comparison').hidden = false;
}
