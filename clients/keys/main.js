// Wiring for the sealed-accounts page.
//
// Two jobs, kept apart because they fail differently and a reader has to be
// able to tell which one broke:
//
//   the demonstration  runs locally, needs nothing but this page
//   the evidence       needs the chain
//
// A page that collapsed both into one "loading..." would report a dead node as
// a broken claim.
//
// The demonstration used to load a 19 MB WebAssembly module to run a threshold
// signing protocol. It now runs the arithmetic directly, in about a
// millisecond, because the arithmetic that matters is small - see sharing.js
// for why there is a copy of it here and what holds it honest.

import {
  NotHeld,
  Unreachable,
  balances,
  disclosureCount,
  disclosureTerms,
  earliestBlock,
  height,
  transaction,
} from './threshold.js';
import { combine, fromHex, same, split, toHex } from './sharing.js';

const status = document.getElementById('status');
const verdict = document.getElementById('verdict');
const parts = document.getElementById('parts');
const goButton = document.getElementById('go');
const oneButton = document.getElementById('one');

/** The account the two real payments came from, and those payments. */
const ACCOUNT = 'yml1ael7jxwlvacc3daawzc2kpd6lst6w8nmml6a97';
const PAYMENTS = [
  {
    hash: 'A8F18CABC7572BAAC39D108674ED1AA650CA44F8C4F63DB828C556DCBEB15C3C',
    signers: 'device + custodian',
    note: 'the original shares',
  },
  {
    hash: '6C784D06E3339BBBDCCE0F656E667DEB70D75A5E24A400F24B2C75EFE94AFCFE',
    signers: 'device + custodian',
    note: 'shares created by a password reset',
  },
];

function say(text) {
  status.textContent = text;
}

function show(kind, head, body) {
  verdict.innerHTML = '';
  const box = document.createElement('div');
  box.className = `verdict verdict--${kind}`;
  const h = document.createElement('div');
  h.className = 'head';
  h.textContent = head;
  const p = document.createElement('p');
  p.style.margin = '0';
  p.className = 'note';
  p.textContent = body;
  box.append(h, p);
  verdict.append(box);
}

/**
 * The real values this page demonstrates with.
 *
 * vectors.json is a copy of recovery/testdata/vectors.json - the file the Go
 * implementation and the wallet's are both tested against - and
 * sharing.test.js fails if the copy drifts. So the sealed blob shown here is
 * not a plausible string of bytes written for a demonstration; it is the
 * output of the code that runs in production, for a password and a key that
 * are printed on this page.
 */
let vectorsPromise = null;
function demoVectors() {
  if (!vectorsPromise) {
    vectorsPromise = (async () => {
      const res = await fetch('./vectors.json');
      if (!res.ok) throw new Error(`the demonstration values are not deployed (${res.status})`);
      const all = await res.json();
      const v = all.seal[0];
      return {
        password: v.password,
        key: fromHex(v.secret),
        sealed: fromHex(v.expect),
        time: v.time,
        memory: v.memory,
      };
    })();
  }
  return vectorsPromise;
}

/** Short enough to read, long enough to see that two parts differ. */
function abbreviate(bytes, keep = 16) {
  const hex = toHex(bytes);
  return hex.length <= keep * 2 ? hex : `${hex.slice(0, keep * 2)}...`;
}

function clearParts() {
  parts.innerHTML = '';
}

/**
 * One row of the demonstration.
 *
 * `held` is who holds it, and it carries the party colour - the one
 * categorical scale this page defines for itself, because the shared system
 * has only a semantic one and colouring the custodian "ok" would say the
 * custodian is the good one. It is not; it is one of three.
 */
function addPart(held, label, value, note) {
  const row = document.createElement('div');
  row.className = 'wire';
  const who = document.createElement('span');
  who.className = held ? `tag tag--${held}` : 'r';
  who.textContent = label;
  const bytes = document.createElement('span');
  bytes.className = 'b';
  bytes.textContent = value;
  const hint = document.createElement('span');
  hint.className = 'r';
  hint.textContent = note ?? '';
  row.append(who, bytes, hint);
  parts.append(row);
}

/** The three parts, kept between the buttons so the second follows the first. */
let currentParts = null;

goButton.addEventListener('click', async () => {
  goButton.disabled = true;
  oneButton.disabled = true;
  verdict.innerHTML = '';
  clearParts();
  try {
    const v = await demoVectors();

    addPart(null, 'the key', abbreviate(v.key), `${v.key.length} bytes`);
    addPart(
      null,
      'sealed',
      abbreviate(v.sealed),
      `${v.sealed.length} bytes - Argon2id, ${Math.round(v.memory / 1024)} MiB`,
    );

    // Split live. Fresh randomness every time, which is why pressing this twice
    // gives three different parts for the same blob - and why one part tells an
    // attacker nothing about the other two.
    const started = performance.now();
    currentParts = split(v.sealed, 3, 2);
    const took = performance.now() - started;

    const held = ['device', 'custodian', 'recovery'];
    currentParts.forEach((part, i) => {
      addPart(held[i], held[i], abbreviate(part.y), `part ${part.x} of 3`);
    });

    say(`Split in ${took.toFixed(1)} ms. Press it again and the three parts are different.`);
    show(
      'ok',
      'Sealed first, then split three ways.',
      'The key was encrypted under the password before anything was divided. What the three ' +
        'parties hold are parts of the CIPHERTEXT, not parts of the key - which is what the ' +
        'next button is about.',
    );
  } catch (err) {
    say('');
    show('no', 'The demonstration could not run.', String(err.message || err));
  } finally {
    goButton.disabled = false;
    oneButton.disabled = false;
  }
});

oneButton.addEventListener('click', async () => {
  goButton.disabled = true;
  oneButton.disabled = true;
  verdict.innerHTML = '';
  try {
    const v = await demoVectors();
    if (!currentParts) currentParts = split(v.sealed, 3, 2);

    // Parts 2 and 3: the custodian and the recovery service. Both run by the
    // same operator, which is the whole reason this is the interesting case.
    const services = [currentParts[1], currentParts[2]];
    const rebuilt = combine(services);

    clearParts();
    addPart('custodian', 'custodian', abbreviate(services[0].y), 'part 2 of 3');
    addPart('recovery', 'recovery', abbreviate(services[1].y), 'part 3 of 3');
    addPart(null, 'together', abbreviate(rebuilt), `${rebuilt.length} bytes`);
    addPart(null, 'the key', abbreviate(v.key), `${v.key.length} bytes`);

    const isSealed = same(rebuilt, v.sealed);
    const isKey = same(rebuilt, v.key);

    say('');
    if (isSealed && !isKey) {
      show(
        'no',
        'Both services together get a ciphertext they cannot open.',
        'They rebuilt the sealed blob exactly - and it is not the key, as the two rows above ' +
          'show. Opening it needs the password, which neither service holds in any form. That ' +
          'is the difference between "no single service can move your money" and "not even ' +
          'both of them can", and only the second survives one company running both.',
      );
    } else {
      // Stated rather than hidden. If this ever fires the page is wrong and a
      // reader should be told, not shown a confident box.
      show(
        'no',
        'That did not come out as expected.',
        `The two service parts rebuilt ${isKey ? 'THE KEY ITSELF' : 'neither the sealed blob nor the key'}. ` +
          'This page is wrong and it is worth reporting.',
      );
    }
  } catch (err) {
    say('');
    show('no', 'Could not run the demonstration.', String(err.message || err));
  } finally {
    goButton.disabled = false;
    oneButton.disabled = false;
  }
});

// --------------------------------------------------------------- evidence

function cell(text, className) {
  const td = document.createElement('td');
  if (className) td.className = className;
  td.textContent = text;
  return td;
}

async function loadEvidence() {
  const body = document.getElementById('chain');
  const account = document.getElementById('account');
  const caption = document.getElementById('chain-caption');
  try {
    const at = await height();
    // Asked for alongside the height, because it is what decides whether a
    // "not found" here is a missing transaction or a missing block.
    const earliest = await earliestBlock().catch(() => null);
    const rows = await Promise.all(PAYMENTS.map((p) => transaction(p.hash).then(
      (tx) => ({ ...p, ...tx }),
      (err) => ({ ...p, error: err }),
    )));
    body.innerHTML = '';
    let notHeld = 0;
    for (const row of rows) {
      const tr = document.createElement('tr');
      const hash = document.createElement('td');
      hash.className = 'mono addr';
      hash.textContent = `${row.hash.slice(0, 16)}…`;
      tr.append(hash);
      if (row.error) {
        // Three facts, three renderings. "This node no longer holds the block"
        // and "this node did not answer" were one sentence, and a reader taking
        // the first for the second concludes the chain is broken — on the page
        // whose entire argument is that its claims can be checked.
        const held = row.error instanceof NotHeld;
        if (held) notHeld += 1;
        tr.append(cell('—', 'num gone'));
        tr.append(cell(`${row.signers} — ${row.note}`));
        tr.append(cell(
          held ? 'outside this node\'s history' : 'the chain did not answer',
          'gone',
        ));
      } else {
        tr.append(cell(row.height.toLocaleString(), 'num'));
        tr.append(cell(`${row.signers} — ${row.note}`));
        tr.append(cell(row.code === 0 ? 'accepted' : `refused, code ${row.code}`));
      }
      body.append(tr);
    }

    if (caption) {
      caption.textContent = notHeld === 0
        ? 'Read by hash from the public node, which returns the delivered code as well as the height — a hash '
          + 'on its own says a transaction landed in a block, not that it succeeded.'
        : `${notHeld === rows.length ? 'Neither payment is' : 'One payment is'} readable here, and that is a `
          + `property of the node rather than of the transaction. The public node was state-synced${
            earliest ? `, so the oldest block it holds is ${earliest.toLocaleString()}` : ''} — it keeps no `
          + 'index of anything earlier, and answers "not found" for a transaction it never downloaded. The '
          + 'account below is still readable, because a balance is current state rather than history.';
    }

    let holding = '';
    try {
      const { coins } = await balances(ACCOUNT);
      holding = coins.length
        ? ` It currently holds ${coins.map((c) => `${Number(c.amount).toLocaleString()} ${c.denom}`).join(', ')}.`
        : ' It currently holds nothing.';
    } catch {
      holding = '';
    }
    account.textContent =
      `Account ${ACCOUNT}. Both payments left the same address, and the key behind it has ` +
      `never existed in one piece.${holding} Read at block ${at.toLocaleString()}.`;
  } catch (err) {
    body.innerHTML = '';
    const tr = document.createElement('tr');
    const td = cell(
      err instanceof Unreachable
        ? 'The chain is not answering, so nothing is shown here. These are facts about a running network and a stale copy of them would be worth less than none.'
        : String(err.message || err),
      'gone',
    );
    td.colSpan = 4;
    tr.append(td);
    body.append(tr);
  }
}

/**
 * The terms of the door out, read from the chain rather than written here.
 *
 * The whole point of putting the waiting period and the warning's hash in
 * consensus is that they are not the operator's to adjust. A page that printed
 * "48 hours" as static text would be making exactly the promise the module
 * exists to replace with a fact — so if the chain does not answer, this says so
 * and shows no figure at all.
 */
async function loadDisclosureTerms() {
  const el = document.getElementById('door-terms');
  if (!el) return;
  try {
    const terms = await disclosureTerms();
    const hours = Math.round(terms.delaySeconds / 3600);
    const days = Math.round(terms.expirySeconds / 86400);

    // The count is a separate call and a separate failure. Terms that loaded
    // are worth showing even when the count did not, because the terms are
    // what somebody deciding needs and the count is context.
    let rate = '';
    try {
      const count = await disclosureCount();
      rate = ` So far ${count.disclosed === 0 ? 'no account has' : `${count.disclosed} account${count.disclosed === 1 ? ' has' : 's have'}`}` +
        ` taken their key on this chain, and ${count.pending === 0 ? 'none is' : `${count.pending} ${count.pending === 1 ? 'is' : 'are'}`} inside the waiting period.` +
        ' A count, never a list: a public roll of accounts whose keys are in somebody\'s' +
        ' downloads folder would be a target list, ordered by how easy each one is to rob.';
    } catch { rate = ''; }

    const tile = document.getElementById('kpi-wait');
    if (tile) {
      tile.querySelector('.y-kpi__value').innerHTML =
        `${hours}<span class="y-kpi__unit">hours</span>`;
      tile.querySelector('.y-kpi__hint').textContent =
        `Between asking for your key and confirming, and the request lapses after ${days} days. A chain `
        + 'parameter, not a setting in the platform\'s software.';
    }

    el.innerHTML = `
      <p><strong>In force on this chain, at block ${at(terms.height)}:</strong>
        a wait of <strong>${hours} hours</strong> between asking and confirming, and a
        request lapses after <strong>${days} days</strong> if nobody completes it — a
        request left standing is an authorisation to take a key, waiting for the day
        somebody acquires the device.</p>
      <p style="margin-top:0.7rem">The warning that must be acknowledged is version
        <span class="mono">${escapeText(terms.version)}</span>, sha256
        <span class="mono addr">${escapeText(terms.hash)}</span>. Acknowledged again at
        the confirmation, not just at the request: if governance publishes new wording
        during the wait, the terms have changed and the holder is sent back to read
        them.${rate}</p>`;
  } catch (err) {
    // The tile keeps its en dash. An unread figure and a figure of nothing are
    // different facts, and "0 hours" here would say a key leaves on request.
    const tile = document.getElementById('kpi-wait');
    if (tile) {
      tile.querySelector('.y-kpi__hint').textContent = err instanceof Unreachable
        ? 'Not read — the chain did not answer. The wait is enforced by consensus rather than by this page.'
        : 'Not read. This deployment may have no key-disclosure pathway at all.';
    }
    el.textContent = err instanceof Unreachable
      ? 'The chain is not answering, so the terms are not shown. They are enforced by '
        + 'consensus rather than by this page, and a stale copy of them would be worth '
        + 'less than none.'
      : String(err.message || err);
  }
}

const at = (h) => Number(h).toLocaleString();
const escapeText = (t) => String(t ?? '').replace(/[&<>"']/g,
  (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

loadEvidence();
loadDisclosureTerms();
