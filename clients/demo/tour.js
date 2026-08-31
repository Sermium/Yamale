// Drawing the tour.
//
// The rendering rule this file follows everywhere: a mechanism's proof panel is
// drawn from a tagged value and never from a raw number, so there is no
// template branch where an unfetched figure and a genuine nought produce the
// same pixels. See format.js — that separation is the point of both files.
//
// Every proof is fetched independently and drawn as it lands. One module being
// unreachable must not blank the other sixteen: on a chain that is scheduled to
// halt mid-demonstration, "all or nothing" means nothing.

import { blockTime, head } from './chain.js';
import { isProven, secondsPerBlock, when } from './format.js';
import { ACTS, MECHANISMS, NOT_BUILT, SURFACES } from './mechanisms.js';

/* ------------------------------------------------------------------ DOM */

const el = (tag, className, text) => {
  const node = document.createElement(tag);
  if (className) node.className = className;
  // textContent throughout. Every string drawn here — an objection's reason, a
  // treasury's name, a validator's moniker — is written by somebody else and
  // arrives from the chain. innerHTML on any of them is a stored cross-site
  // scripting hole with a governance proposal as the injection vector.
  if (text !== undefined && text !== null) node.textContent = String(text);
  return node;
};

const put = (parent, ...children) => { children.forEach((c) => c && parent.appendChild(c)); return parent; };

/* ---------------------------------------------------------------- theme */

/**
 * The theme switch, stamping the attribute yamale.css's third state reads.
 *
 * Three states, not two: unstamped follows the system, and the two stamped
 * values override it in either direction. A toggle that only ever stamps
 * 'dark' cannot return a reader whose system is dark to light.
 */
function wireTheme() {
  const button = document.getElementById('theme');
  const root = document.documentElement;
  let stored = null;
  try { stored = localStorage.getItem('yamale.theme'); } catch { /* private window */ }
  if (stored === 'dark' || stored === 'light') root.setAttribute('data-theme', stored);

  button.addEventListener('click', () => {
    const systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    const current = root.getAttribute('data-theme') ?? (systemDark ? 'dark' : 'light');
    const next = current === 'dark' ? 'light' : 'dark';
    root.setAttribute('data-theme', next);
    try { localStorage.setItem('yamale.theme', next); } catch { /* private window */ }
  });
}

/* ----------------------------------------------------------- the surfaces */

function drawSurfaces() {
  const grid = document.getElementById('surface-grid');
  for (const [key, s] of Object.entries(SURFACES)) {
    const card = el('a', 'surface');
    card.href = s.href;
    card.dataset.surface = key;
    put(card,
      put(el('div', 'surface__top'),
        el('strong', 'surface__label', s.label),
        el('span', `y-chip ${s.status === 'live' ? 'y-chip--ok' : 'y-chip--warn'}`,
          s.status === 'live' ? 'live' : 'being built')),
      el('span', 'surface__blurb', s.blurb),
      el('span', 'surface__href y-mono', s.href));
    if (s.status !== 'live') card.classList.add('surface--building');
    grid.appendChild(card);
  }
}

/* --------------------------------------------------------------- the acts */

function drawActs() {
  const host = document.getElementById('acts');
  const nav = document.querySelector('.masthead__nav');

  ACTS.forEach((act, index) => {
    const link = el('a', 'masthead__navlink', act.title);
    link.href = `#act-${act.id}`;
    nav.appendChild(link);

    const section = el('section', 'act');
    section.id = `act-${act.id}`;

    const inner = el('div', 'wrap');
    put(inner,
      put(el('header', 'act__head'),
        el('p', 'y-eyebrow', `Act ${index + 1} of ${ACTS.length}`),
        el('h2', null, act.title),
        el('p', 'section__lede', act.lede)));

    const list = el('div', 'mech-list');
    MECHANISMS.filter((m) => m.act === act.id).forEach((m) => list.appendChild(drawMechanism(m)));
    inner.appendChild(list);
    section.appendChild(inner);
    host.appendChild(section);
  });
}

function drawMechanism(m) {
  const surface = SURFACES[m.surface];
  const card = el('article', 'mech');
  card.id = `m-${m.id}`;

  put(card,
    put(el('header', 'mech__head'),
      el('p', 'y-eyebrow', m.module),
      el('h3', null, m.name)));

  const prose = el('div', 'mech__prose');
  put(prose,
    put(el('div', 'mech__block'),
      el('p', 'y-label', 'What it does'),
      el('p', 'mech__does', m.does)),
    put(el('div', 'mech__block mech__block--refuses'),
      el('p', 'y-label', 'What it refuses'),
      el('p', 'mech__refuses', m.refuses)));

  const go = el('a', 'mech__go');
  go.href = surface.href;
  put(go,
    el('span', null, surface.status === 'live'
      ? `Open ${surface.label}`
      : `${surface.label} — not deployed yet`),
    el('span', 'mech__go-arrow', '→'));
  if (surface.status !== 'live') go.classList.add('mech__go--building');
  prose.appendChild(go);

  const proof = el('div', 'mech__proof');
  proof.dataset.proof = m.id;
  put(proof,
    put(el('div', 'proof__head'),
      el('p', 'y-label', 'Proof, read from the running chain'),
      el('span', 'y-chip y-chip--mute proof__chip', 'reading…')),
    el('p', 'proof__watch', m.watch),
    el('div', 'proof__body'));

  put(card, put(el('div', 'mech__body'), prose, proof));
  return card;
}

/* -------------------------------------------------------------- the proofs */

/**
 * Draw one proof.
 *
 * The two branches produce structurally different panels on purpose. An unread
 * proof gets a sentence and no data rows at all — not a row of dashes, which
 * the eye reads as a measured value of nothing.
 */
function drawProof(mechanismId, proof) {
  const panel = document.querySelector(`[data-proof="${mechanismId}"]`);
  if (!panel) return;
  const chip = panel.querySelector('.proof__chip');
  const body = panel.querySelector('.proof__body');
  body.replaceChildren();

  if (!isProven(proof)) {
    const tone = proof.reason === 'unreachable' ? 'bad' : proof.reason === 'denied' ? 'warn' : 'mute';
    const label = proof.reason === 'unreachable'
      ? 'cannot reach the chain'
      : proof.reason === 'denied' ? 'gateway denies this path' : 'no such record';
    chip.className = `y-chip y-chip--${tone} proof__chip`;
    chip.textContent = label;
    panel.classList.add('mech__proof--unread');
    body.appendChild(el('p', 'proof__failure', proof.detail));
    return;
  }

  panel.classList.remove('mech__proof--unread');
  chip.className = 'y-chip y-chip--ok proof__chip';
  chip.textContent = proof.height ? `read at height ${proof.height.toLocaleString('en')}` : 'read live';

  const dl = el('dl', 'proof__rows');
  proof.rows.forEach((row) => {
    const dt = el('dt', 'proof__label', row.label);
    const dd = el('dd', 'proof__value', row.value);
    if (row.mono) dd.classList.add('y-mono');
    if (row.full) dd.classList.add('proof__value--full', 'y-addr');
    if (row.emphasis) dd.classList.add('proof__value--emphasis');
    if (row.quote) dd.classList.add('proof__value--quote');
    put(dl, dt, dd);
  });
  body.appendChild(dl);
  if (proof.note) body.appendChild(el('p', 'proof__note', proof.note));
}

/* ---------------------------------------------------------------- the head */

function drawHead(state, text, title) {
  const box = document.getElementById('head');
  box.querySelector('.head__dot').dataset.state = state;
  box.querySelector('.head__text').textContent = text;
  if (title) box.title = title;
}

/**
 * Read the head, and measure the block interval rather than assuming it.
 *
 * The interval is used to render every "N blocks" parameter as a duration —
 * how long a provisional freeze lasts, how long a seizure waits. A hard-coded
 * seven seconds would have been wrong today: this chain has been running at
 * about five and a third, and the freeze duration would have been overstated
 * by a third on the one number a supervisor would check.
 *
 * Measured across two thousand blocks, not two. Adjacent blocks differ by
 * whatever the proposer's clock did.
 */
async function readHead() {
  const now = await head();
  drawHead(now.catchingUp ? 'warn' : 'ok',
    `${now.network} · height ${now.height.toLocaleString('en')}`,
    `${when(now.at) ?? ''}${now.catchingUp ? ' — this node is still catching up' : ''}`);

  document.getElementById('hero-network').textContent = now.network;
  document.getElementById('hero-height').textContent = now.height.toLocaleString('en');

  const span = Math.min(2000, now.height - 1);
  let interval = null;
  if (span >= 100) {
    const at = await blockTime(now.height - span);
    interval = secondsPerBlock({ height: now.height - span, at }, now);
  }
  document.getElementById('hero-blocktime').textContent =
    interval ? `${interval.toFixed(1)} s` : 'not measurable';
  return { head: now, secondsPerBlock: interval };
}

/* ------------------------------------------------------------------- boot */

async function main() {
  wireTheme();
  drawSurfaces();
  drawActs();
  document.getElementById('hero-count').textContent = String(MECHANISMS.length);
  const notBuilt = document.getElementById('notbuilt-list');
  NOT_BUILT.forEach((line) => notBuilt.appendChild(el('li', null, line)));

  // The head first, because the block interval it measures is an input to
  // several proofs. If it fails, the proofs still run: a mechanism whose
  // parameters are readable should not be blanked because /status timed out.
  let ctx = {};
  try {
    ctx = await readHead();
  } catch {
    drawHead('bad', 'cannot reach the chain', 'The node did not answer /status.');
    ['hero-network', 'hero-height', 'hero-blocktime'].forEach((id) => {
      document.getElementById(id).textContent = 'unreachable';
    });
  }

  // Fetched independently and drawn as each lands, so the page fills in rather
  // than waiting on its slowest query — and so one halted module does not take
  // the other sixteen with it.
  await Promise.all(MECHANISMS.map(async (m) => {
    try {
      drawProof(m.id, await m.read(ctx));
    } catch {
      // read() catches its own failures; reaching here means the catalogue
      // entry itself threw. Still not allowed to render as a number.
      drawProof(m.id, { state: 'unread', reason: 'unreachable', detail: 'Cannot reach the chain.' });
    }
  }));
}

main();
