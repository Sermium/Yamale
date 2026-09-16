// Wiring for the threshold-accounts page.
//
// Two jobs, kept apart because they fail differently and a reader has to be
// able to tell which one broke:
//
//   the demonstration  runs locally, needs the WebAssembly module and no chain
//   the evidence       needs the chain and no WebAssembly
//
// A page that collapsed both into one "loading…" would report a dead node as a
// broken protocol.

import {
  NotHeld,
  Unreachable,
  balances,
  demonstrate,
  digestOf,
  disclosureCount,
  disclosureTerms,
  earliestBlock,
  height,
  loadProtocol,
  refuseWithOneShare,
  transaction,
} from './threshold.js';

const status = document.getElementById('status');
const verdict = document.getElementById('verdict');
const wires = document.getElementById('wires');
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

function clearWires() {
  wires.innerHTML = '';
}

function addWire(message) {
  const row = document.createElement('div');
  row.className = 'wire';
  const r = document.createElement('span');
  r.className = 'r';
  r.textContent = `${message.round}`;
  const what = document.createElement('span');
  const from = document.createElement('span');
  from.className = `tag tag--${message.from}`;
  from.textContent = message.from;
  const arrow = document.createTextNode(' → ');
  const to = document.createElement('span');
  to.className = `tag tag--${message.to}`;
  to.textContent = message.to;
  what.append(from, arrow, to);
  if (message.broadcast) {
    const b = document.createElement('span');
    b.className = 'r';
    b.textContent = '  (broadcast)';
    what.append(b);
  }
  const bytes = document.createElement('span');
  bytes.className = 'b';
  bytes.textContent = `${message.bytes} B`;
  row.append(r, what, bytes);
  wires.append(row);
  wires.scrollTop = wires.scrollHeight;
}

// The throwaway shares this page demonstrates with. Fetched rather than
// embedded so the page itself stays readable, and so the fact that they are
// ordinary published files is obvious to anybody who looks.
let sharesPromise = null;
function demoShares() {
  if (!sharesPromise) {
    sharesPromise = (async () => {
      // TEXT, never res.json().
      //
      // Go marshals a big.Int as a bare JSON NUMBER, and JSON.parse turns any
      // number into a float64. A 2048-bit Paillier modulus becomes
      // 1.7489051584485197e+308 and the share is destroyed — which the protocol
      // then reports as "cannot unmarshal into a *big.Int", a long way from the
      // line that broke it. So the share text is carried to WebAssembly
      // untouched and JavaScript never looks inside it.
      const load = async (role) => {
        const res = await fetch(`./demo/${role}.share.json`);
        if (!res.ok) throw new Error(`the ${role} share is not deployed (${res.status})`);
        return res.text();
      };
      const [device, custodian] = await Promise.all([load('device'), load('custodian')]);
      return { device, custodian };
    })();
  }
  return sharesPromise;
}

async function ready() {
  const mpc = await loadProtocol('./mpc.wasm', (stage) => {
    if (stage === 'fetching') say('Fetching the protocol module…');
    if (stage === 'instantiating') say('Starting it…');
  });
  const shares = await demoShares();
  return { mpc, shares };
}

goButton.addEventListener('click', async () => {
  goButton.disabled = true;
  oneButton.disabled = true;
  verdict.innerHTML = '';
  clearWires();
  try {
    const { mpc, shares } = await ready();
    say('Running the protocol. Each round is one message neither party could have produced alone.');
    const digest = await digestOf(`a demonstration at ${new Date().toISOString()}`);
    const started = performance.now();
    const result = await demonstrate(mpc, digest, shares.device, shares.custodian, addWire);
    const took = Math.round(performance.now() - started);
    say(`Done in ${took} ms, over ${result.transcript.length} messages.`);
    show(
      'ok',
      'A signature exists, and neither party could have made it.',
      `Both parties independently computed ${result.agreed ? 'the same' : 'DIFFERENT'} bytes. ` +
        `Signature: ${result.signature.slice(0, 44)}…`,
    );
    if (!result.agreed) {
      show('no', 'The two parties disagree about the signature.',
        'That should never happen and is worth reporting.');
    }
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
    const { mpc, shares } = await ready();
    const digest = await digestOf('one share, trying');
    const refusal = refuseWithOneShare(mpc, digest, shares.custodian);
    say('');
    if (refusal) {
      show('no', 'Refused — and this refusal is the product.', refusal);
    } else {
      show('no', 'It was NOT refused.',
        'One share produced a session, which would mean the whole design is not doing what it claims. Worth reporting.');
    }
  } catch (err) {
    say('');
    show('no', 'Could not run the refusal.', String(err.message || err));
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
