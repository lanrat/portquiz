// The browser test page: tests TCP ports with HTTP requests to the server,
// which answers on every port, and summarizes the results. The logic that
// does not need a browser is in core.js.
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

const statusLabels = {
  open: 'open',
  rejected: 'rejected',
  dropped: 'dropped',
  interfered: 'interfered',
  error: 'errors',
  browser: 'blocked by the browser',
};

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
  showError('');
  const options = {
    timeout: number('timeout', 2000, 100, 60000),
    adaptive: $('adaptive').checked,
    parallel: number('parallel', 128, 1, 256),
    rate: number('rate', 500, 0, 100000),
  };
  scan = {
    running: true,
    stopped: false,
    ports,
    options,
    results: new core.Results(ports),
    estimator: new core.TimeoutEstimator(options.timeout, options.adaptive),
    // like the command line client: one retry with the adaptive timeout,
    // none without it, where the full timeout covers a lost connection
    retries: options.adaptive ? 1 : 0,
    controllers: new Set(),
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
  $('status').textContent = 'Checking which ports this browser blocks…';
  $('eta').textContent = '';
  for (const input of document.querySelectorAll('#form input')) input.disabled = true;
  renderResults();

  const s = scan;
  s.detection = await detectBlocked(s);
  if (s.stopped) return;
  s.started = Date.now();
  s.eta = new core.ETA(ports.length, performance.now());
  const ticker = setInterval(() => render(s), 250);
  const lister = setInterval(renderResults, 1000);

  let next = 0;
  const worker = async () => {
    while (!s.stopped && next < ports.length) {
      const port = ports[next++];
      const r = await core.testPort(port, network(s), { token: info.token, estimator: s.estimator, retries: s.retries });
      if (s.stopped) return;
      s.results.set(port, r.status, r.rtt);
    }
  };
  await Promise.all(Array.from({ length: Math.min(s.options.parallel, ports.length) }, worker));
  clearInterval(ticker);
  clearInterval(lister);
  finish(s, s.stopped);
}

function stop() {
  const s = scan;
  s.stopped = true;
  for (const c of s.controllers) c.abort();
  if (s.detection === null) finish(s, true);
}

function finish(s, canceled) {
  if (!s.running) return;
  s.running = false;
  s.canceled = canceled;
  s.finished = Date.now();
  $('start').textContent = 'Start';
  for (const input of document.querySelectorAll('#form input')) input.disabled = false;
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
      const r = await timed(s, `http://${host}:${port}/probe?n=${s.nonce}&opaque`, 'no-cors', s.estimator.max);
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

// pace waits for the next connection slot under the rate limit.
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

function render(s) {
  const done = s.results.done;
  const total = s.ports.length;
  $('progress').value = total ? done / total : 0;
  $('status').textContent = `${numbers.format(done)} / ${numbers.format(total)} tested · ${numbers.format(s.results.count('open'))} open`;
  if (s.running && s.eta) {
    const left = s.eta.update(done, performance.now());
    $('eta').textContent = left === null ? '' : `about ${core.formatDuration(core.roundETA(left))} left`;
  }
}

function selectedFilter() {
  return document.querySelector('input[name="filter"]:checked').value;
}

function renderResults() {
  const list = $('results');
  if (!scan) return;
  const filter = selectedFilter();
  const ranges = scan.results.ranges().filter((r) =>
    filter === 'all' || (filter === 'open') === (r.status === 'open'));
  const items = ranges.slice(0, maxLines).map((r) => {
    const li = document.createElement('li');
    li.className = r.status;
    const ports = r.first === r.last ? `${r.first}` : `${r.first}-${r.last}`;
    li.textContent = `${r.status === 'open' ? 'OPEN' : 'CLOSED'} tcp ${ports}${r.status === 'open' ? '' : ` (${statusLabels[r.status]})`}`;
    return li;
  });
  list.replaceChildren(...items);
  $('more').textContent = ranges.length > maxLines
    ? `…and ${numbers.format(ranges.length - maxLines)} more ranges, all in the saved report.`
    : (scan.results.done > 0 && ranges.length === 0 ? 'No ranges match the filter.' : '');
}

// el creates an element with text or children.
function el(tag, content, attrs = {}) {
  const e = document.createElement(tag);
  if (Array.isArray(content)) e.append(...content);
  else if (content !== undefined) e.textContent = content;
  Object.assign(e, attrs);
  return e;
}

// ports formats a count of ports: "1 port", "1,200 ports".
function portCount(count) {
  return `${numbers.format(count)} ${count === 1 ? 'port' : 'ports'}`;
}

function portRange(r) {
  return r.first === r.last ? `${r.first}` : `${r.first}–${r.last}`;
}

function renderSummary(s) {
  const r = s.results;
  const ranges = r.ranges();
  const n = (status) => r.count(status);
  const tested = r.done - n('browser');
  const out = [el('h2', 'Summary')];

  const seconds = (s.finished - s.started) / 1000;
  const server = host.replace(/^\[|\]$/g, '') === info.server ? host : `${host} (${info.server})`;
  out.push(el('p', `${server} from ${info.client} · tested ${numbers.format(r.done)} in ${core.formatDuration(seconds * 1000)}`));
  if (s.canceled) out.push(el('p', `Stopped early: ${numbers.format(s.ports.length - r.done)} not tested.`, { className: 'warning' }));
  out.push(el('p', `${numbers.format(n('open'))} of ${portCount(tested)} tested open (${percent.format(tested ? n('open') / tested : 0)}).`));
  if (n('browser') > 0) {
    out.push(el('p', `${portCount(n('browser'))} not tested because this browser refuses to use them. ${detectionText(s.detection)}`, { className: 'muted' }));
  }
  if (s.detection.threshold !== null) out.push(...listDifferences(s));

  if (r.done >= 100) out.push(...portMap(s, ranges));

  out.push(el('h2', 'How closed ports were closed'));
  const closed = ['rejected', 'dropped', 'interfered', 'error'].filter((st) => n(st) > 0);
  if (closed.length === 0) out.push(el('p', 'No closed ports.'));
  else out.push(el('p', closed.map((st) => `${numbers.format(n(st))} ${statusLabels[st]}`).join(', ')));
  const explain = {
    rejected: 'Rejected: actively refused, by a reset or a firewall set to reject.',
    dropped: 'Dropped: nothing came back before the timeout, as when a firewall silently discards traffic.',
    interfered: 'Interfered: something other than the portquiz server answered, as a proxy, captive portal or filter on the path does.',
    error: 'Errors: the browser could not make the request.',
  };
  for (const st of closed) out.push(el('p', explain[st], { className: 'muted' }));
  const interfered = ranges.filter((x) => x.status === 'interfered');
  if (interfered.length > 0) {
    const shown = interfered.slice(0, 20).map(portRange).join(', ');
    out.push(el('p', `Interfered ports: ${shown}${interfered.length > 20 ? `, …and ${interfered.length - 20} more` : ''}`, { className: 'warning' }));
  }

  out.push(el('h2', 'Common services'));
  const rows = core.SERVICES.filter(([, port]) => r.status[port] !== 0).map(([name, port]) => {
    const st = core.STATUSES[r.status[port] - 1];
    return el('tr', [el('td', name), el('td', `${port}/tcp`), el('td', statusLabels[st], { className: st === 'open' ? 'open' : '' })]);
  });
  if (rows.length === 0) out.push(el('p', 'None of these ports were tested.'));
  else out.push(el('table', [el('thead', [el('tr', [el('th', 'Service'), el('th', 'Port'), el('th', 'Result')])]), el('tbody', rows)]));

  out.push(el('h2', 'Largest blocked ranges'));
  const runs = core.largestClosed(ranges, 5);
  if (runs.length === 0) out.push(el('p', 'No blocked ranges.'));
  else out.push(el('ul', runs.map((x) => el('li', `${portRange(x)} · ${portCount(x.last - x.first + 1)} · ${x.status === null ? 'mixed' : statusLabels[x.status]}`))));

  out.push(el('h2', 'Speed and timing'));
  out.push(el('p', `Tested ${numbers.format(r.done)} in ${core.formatDuration(seconds * 1000)}, ${decimals.format(seconds > 0 ? r.done / seconds : 0)} per second on average.`));
  const rtt = core.rttStats(r.rtts);
  out.push(el('p', rtt
    ? `Open ports answered in ${decimals.format(rtt.median)} ms (median), ${decimals.format(rtt.p95)} ms (95th percentile), ${decimals.format(rtt.min)} to ${decimals.format(rtt.max)} ms (fastest to slowest). This includes setting up each connection.`
    : 'No open ports, so no response times.'));
  const e = s.estimator;
  out.push(el('p', s.options.adaptive
    ? `Timeout ${decimals.format(e.timeout())} ms at the end, adapted from up to ${numbers.format(e.max)} ms. Ports that did not answer were retried: ${s.retries}.`
    : `Timeout ${numbers.format(e.max)} ms. Ports that did not answer were retried: ${s.retries}.`, { className: 'muted' }));
  if (e.misses > 0) out.push(el('p', `${e.misses === 1 ? '1 open port' : `${e.misses} open ports`} only answered on a slower recheck, so dropped ports may be open.`, { className: 'warning' }));

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
function listDifferences(s) {
  const r = s.results;
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

// portMap draws the results across the ports tested: each column covers a
// run of ports and is split by how many had each status. Pointing at it
// shows the ports under the pointer.
function portMap(s, ranges) {
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
  const inspect = el('p', 'Point at the map to see the ports under it.', { id: 'inspect' });
  const legend = el('div', core.STATUSES.filter((st) => s.results.count(st) > 0).map((st) => {
    const item = el('span', `${numbers.format(s.results.count(st))} ${statusLabels[st]}`);
    item.style.setProperty('--swatch', `var(--map-${st})`);
    return item;
  }), { className: 'legend' });
  canvas.setAttribute('aria-label', `Port map from ${first} to ${last}: ${legend.textContent}`);

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
    const parts = layout.cols[i].map((c, st) => (c > 0 ? `${numbers.format(c)} ${statusLabels[core.STATUSES[st]]}` : null)).filter(Boolean);
    inspect.textContent = `Ports ${portRange({ first: from, last: to })}: ${parts.join(', ') || 'not tested'}`;
  };
  canvas.addEventListener('pointermove', point);
  canvas.addEventListener('pointerdown', point);
  new ResizeObserver(draw).observe(canvas);
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', draw);

  const span = core.formatPorts(s.ports).replaceAll(',', ', ').replaceAll('-', '–');
  return [el('h2', 'Port map'), el('p', `Ports ${span.length > 80 ? portRange({ first, last }) : span}`, { className: 'muted' }), canvas, legend, inspect];
}

// save downloads the report, in the command line client's -json format.
function save() {
  const s = scan;
  const report = core.buildReport({
    info,
    host,
    results: s.results,
    started: s.started,
    finished: s.finished,
    canceled: s.canceled,
    options: s.options,
    estimator: s.estimator,
    retries: s.retries,
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
