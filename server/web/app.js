// The browser test page: tests TCP ports with HTTP requests to the server,
// and UDP ports with STUN requests through WebRTC, since the server answers
// both on every port, and summarizes the results. The logic that does not
// need a browser is in core.js.
import * as core from './core.js';

const $ = (id) => document.getElementById(id);
const numbers = new Intl.NumberFormat();
const decimals = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });
const percent = new Intl.NumberFormat(undefined, { style: 'percent', maximumFractionDigits: 1 });

// the server's name or address as the page was loaded, with brackets around
// an IPv6 address, so it can go straight into URLs
const host = location.hostname;
const pagePort = Number(location.port) || 80;
// documentation addresses that never answer, see core.blockedCheck
const testAddress = host.includes(':') ? '[2001:db8::1]' : '192.0.2.1';
const holdLimit = 1000;
// how many result lines are shown; the saved report has all of them
const maxLines = 2000;
// how many STUN checks one WebRTC connection makes before it is replaced:
// each check leaves resources behind until the connection is closed
const stunChecksPerConnection = 50;

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

let info = null;
let scan = null;

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
  s.finished = Date.now();
  $('start').textContent = 'Start';
  for (const input of document.querySelectorAll('#form input')) input.disabled = false;
  if (!info.udp || typeof RTCPeerConnection === 'undefined') $('udp').disabled = true;
  $('background').hidden = true;
  render(s);
  $('eta').textContent = '';
  renderResults();
  renderSummary(s);
  $('save').hidden = false;
}

// network returns the requests core.testPort makes, for scan s.
function network(s) {
  return {
    request: async (port, timeout) => {
      if (s.stopped) return { type: 'aborted' };
      await pace(s);
      return timed(s, `http://${host}:${port}/probe?n=${s.nonce}`, 'cors', timeout);
    },
    browserBlocks: async (port) => {
      if (s.stopped) return false;
      if (s.detection.threshold === null) return core.STANDARD_BLOCKED.has(port);
      const r = await timed(s, `http://${testAddress}:${port}/`, 'no-cors', s.detection.threshold);
      return r.type === 'failed';
    },
    answers: async (port) => {
      if (s.stopped) return false;
      const r = await timed(s, `http://${host}:${port}/probe?n=${s.nonce}&opaque`, 'no-cors', s.kinds[0].estimator.max);
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
  const hold = await timed(s, `http://${testAddress}:${pagePort}/`, 'no-cors', holdLimit);
  const holdMs = hold.type === 'failed' ? hold.ms : holdLimit;
  const blockedMs = [];
  for (const p of [1, 7, 9]) {
    if (s.stopped) break;
    const r = await timed(s, `http://${testAddress}:${p}/`, 'no-cors', holdLimit);
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
  out.push(el('p', `${server} from ${info.client} · tested ${numbers.format(total)} in ${core.formatDuration(seconds * 1000)}`));
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
  if (s.kinds.some((k) => k.kind === 'udp')) {
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

// portMap draws one kind's results across the ports tested: each column
// covers a run of ports and is split by how many had each status. Pointing
// at it shows the ports under the pointer.
function portMap(s, k) {
  const ranges = k.results.ranges();
  const first = s.ports[0];
  const last = s.ports[s.ports.length - 1];
  // lay the map out over the ports tested, side by side, so a list such as
  // 1-1024,8000-8080 is not mostly the gap between them
  const index = new Int32Array(core.MAX_PORT + 1);
  s.ports.forEach((p, i) => { index[p] = i; });
  const byIndex = ranges.map((x) => ({ first: index[x.first], last: index[x.last], status: x.status }));
  const count = s.ports.length;
  const canvas = el('canvas', undefined, { className: 'map' });
  canvas.setAttribute('role', 'img');
  const inspect = el('p', 'Point at the map to see the ports under it.', { className: 'inspect' });
  const legend = el('div', core.STATUSES.filter((st) => k.results.count(st) > 0).map((st) => {
    const item = el('span', `${numbers.format(k.results.count(st))} ${label(k.kind, st)}`);
    item.style.setProperty('--swatch', `var(--map-${st})`);
    return item;
  }), { className: 'legend' });
  canvas.setAttribute('aria-label', `${k.kind.toUpperCase()} port map: ${legend.textContent}`);

  let layout = null;
  const draw = () => {
    const width = canvas.clientWidth;
    if (width === 0) return;
    const ratio = window.devicePixelRatio || 1;
    canvas.width = Math.round(width * ratio);
    canvas.height = Math.round(64 * ratio);
    const ctx = canvas.getContext('2d');
    const style = getComputedStyle(document.documentElement);
    const colors = core.STATUSES.map((st) => style.getPropertyValue(`--map-${st}`).trim());
    const h = canvas.height;
    ctx.fillStyle = style.getPropertyValue('--map-track').trim();
    ctx.fillRect(0, 0, canvas.width, h);
    const { cols, start: colStart } = core.mapColumns(byIndex, 0, count - 1, Math.floor(width / 2));
    layout = { cols, start: colStart };
    const colW = canvas.width / cols.length;
    cols.forEach((counts, i) => {
      const size = (i + 1 < cols.length ? colStart(i + 1) : count) - colStart(i);
      const heights = core.segmentHeights(counts, size, h, 3 * ratio);
      let bottom = h;
      heights.forEach((seg, st) => {
        if (seg <= 0) return;
        ctx.fillStyle = colors[st];
        ctx.fillRect(Math.floor(i * colW), bottom - seg, Math.ceil(colW), seg);
        bottom -= seg;
      });
    });
  };
  const point = (e) => {
    if (!layout) return;
    const rect = canvas.getBoundingClientRect();
    const i = Math.min(Math.max(Math.floor(((e.clientX - rect.left) / rect.width) * layout.cols.length), 0), layout.cols.length - 1);
    const from = s.ports[layout.start(i)];
    const to = s.ports[i + 1 < layout.cols.length ? layout.start(i + 1) - 1 : count - 1];
    const parts = layout.cols[i].map((c, st) => (c > 0 ? `${numbers.format(c)} ${label(k.kind, core.STATUSES[st])}` : null)).filter(Boolean);
    inspect.textContent = `${k.kind.toUpperCase()} ports ${portRange({ first: from, last: to })}: ${parts.join(', ') || 'not tested'}`;
  };
  canvas.addEventListener('pointermove', point);
  canvas.addEventListener('pointerdown', point);
  new ResizeObserver(draw).observe(canvas);
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', draw);

  const span = core.formatPorts(s.ports).replaceAll(',', ', ').replaceAll('-', '–');
  return [el('p', `${k.kind.toUpperCase()}, ports ${span.length > 80 ? portRange({ first, last }) : span}`, { className: 'muted' }), canvas, legend, inspect];
}

// save downloads the report, in the command line client's -json format.
function save() {
  const s = scan;
  const report = core.buildReport({
    info,
    host,
    ports: s.ports,
    kinds: s.kinds,
    started: s.started,
    finished: s.finished,
    canceled: s.canceled,
    options: s.options,
    detection: s.detection.threshold !== null ? 'runtime' : `standard list: ${s.detection.reason}`,
  });
  const blob = new Blob([JSON.stringify(report, null, 2) + '\n'], { type: 'application/json' });
  const stamp = new Date(s.started).toISOString().slice(0, 16).replace(/[-:]/g, '').replace('T', '-');
  const a = el('a', undefined, { href: URL.createObjectURL(blob), download: `portquiz-${host.replace(/[^A-Za-z0-9.-]/g, '_')}-${stamp}.json` });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 10000);
}
