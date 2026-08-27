
import {
  ACTIONS, ACTION_NAMES, CHAIN, MESSAGES, actionsBy, admissionCommand,
  admissionProposal, explainRefusal, groupPreamble, landTx, officeProposal,
  officeTx, personalMessage, proposalMetadata,
} from './registrar.js';
import {
  CallFailed, WalletRefused, aliasOf, addressOfAlias, blockTime, connectWallet,
  head, idFromResponse, landOrNull, landQuery, signAndSend, txsOf,
} from './chain.js';
import { GROUP_SUBMIT_PROPOSAL, any } from './proto.js';

// ---------------------------------------------------------------------------
// Reading the chain.
//
// THE READS MOVED, AND THIS IS THE ONE THING TO KNOW ABOUT THIS PAGE.
//
// Every query here used to go to /api/rest/yamale/blockchain/land/v1/…, and on
// the live deployment every single one of them answered 401 with
// `WWW-Authenticate: Basic realm="Yamale — supervisor access"`. The proxy
// allowlists REST paths per module and denies by default; x/land's prefix is
// not on that list. So the register whose entire premise is that reading it is
// public showed a browser login box on every screen, and the banner below —
// written to explain exactly this — was the only thing that ever rendered.
//
// The identical queries answer unauthenticated over the node's ABCI interface
// at /api/rpc/, which the proxy does publish. chain.js speaks it, proto.js
// decodes it, and the shape of what comes back is the same as the REST
// gateway's JSON on purpose: uint64 as a decimal string, enums as their names,
// absent repeated fields as empty arrays. Every comparison already written on
// this page against `'STATUS_FROZEN'` or `completed_at !== '0'` therefore still
// means what it meant.
//
// Every failure still names the call that failed. "Something went wrong" tells
// a clerk nothing, and tells somebody standing in front of a seller even less:
// they need to know whether the register said "no objection" or whether the
// register did not answer, because those are opposite facts.
// ---------------------------------------------------------------------------

const cache = new Map();
const once = (key, fn) => {
  if (!cache.has(key)) cache.set(key, fn().catch((e) => { cache.delete(key); throw e; }));
  return cache.get(key);
};

/**
 * Finding a parcel by the reference printed on somebody's paper.
 *
 * The module's primary lookup — "how people actually search", as the keeper
 * puts it — and the one that used to be impossible to make at all. Every real
 * cadastral reference contains slashes (ACC/GA/2019/00412) and the REST route
 * bound the reference to a single path segment, so the node answered 501; the
 * query then moved into the query string, and then the whole REST prefix went
 * behind a credential.
 *
 * Over ABCI the reference is a protobuf string field, where a slash is an
 * ordinary byte and no encoding question arises. The problem is gone rather
 * than worked around.
 */
const parcelByRef = (ref) => landOrNull('ParcelByRef', ref);

/** The uniqueness check. Same reasoning: a survey hash may be any string the
 *  surveyor chose, and a protobuf field does not care what is in it. */
const parcelByGeometry = (hash) => landOrNull('ParcelByGeometry', hash);

/**
 * The registry's standing permission to fractionalise one parcel, and whether
 * the *chain* considers it live.
 *
 * `live` is read rather than computed. It depends on the block time, on
 * whether the office withdrew the permission, and on whether the parcel has
 * since acquired a restriction forbidding fractionalisation — and a page that
 * worked any of that out for itself would eventually disagree with the keeper.
 * The moment it does is the moment somebody pays for shares the chain will
 * refuse to mint.
 */
const fractionalisationAuthority = (parcelId) =>
  landOrNull('FractionalisationAuthority', parcelId);

const params = () => once('params', async () => (await landQuery('Params')).params);
const authorities = () => once('offices', async () =>
  (await landQuery('Authorities')).authorities || []);

// ---------------------------------------------------------------------------
// Naming things the way the people reading this name them.
// ---------------------------------------------------------------------------

const OFFICE = new Map();   // address -> Authority
const PERSON = new Map();   // address -> user ID, or null

async function loadOffices() {
  (await authorities()).forEach((a) => OFFICE.set(a.address, a));
}

/**
 * An office has a name and a jurisdiction, both on-chain. Showing the name is
 * not decoration: "attested by the Highland Lands Office" is a fact a reader
 * can check against the world, and a bech32 string is not.
 */
function officeLabel(addr) {
  if (!addr) return '—';
  const o = OFFICE.get(addr);
  if (o) return o.name || o.jurisdiction || shortRef(addr);
  return `an office no longer in the register (${shortRef(addr)})`;
}
const officeWhere = (addr) => OFFICE.get(addr)?.jurisdiction || '';

/**
 * A person is a user ID where they have one, and otherwise an account
 * reference shown short. Names are deliberately absent from this chain — a
 * public list of who owns what with names attached is a targeting list — so
 * this is as far as the register goes, and the page says so rather than
 * implying the holder is anonymous by accident.
 */
async function personLabel(addr) {
  if (!addr) return '—';
  if (OFFICE.has(addr)) return officeLabel(addr);
  if (!PERSON.has(addr)) {
    PERSON.set(addr, (async () => {
      try {
        return await aliasOf(addr);
      } catch { return null; }
    })());
  }
  const id = await PERSON.get(addr);
  return id ? `User ${groupUserId(id)}` : `Holder ${shortRef(addr)}`;
}

/**
 * NGK3M97QRT5 → NG-K3M9-7QRT-5.
 *
 * Display only. The country prefix is the first group because the reason it is
 * in the identifier at all is that a registrar should see which national
 * perimeter a holder belongs to at a glance, and run together with the payload
 * it is two more characters nobody parses.
 *
 * The check character is deliberately *not* recomputed here: this page is a
 * standalone file with no bundler, and a second implementation of the chain's
 * validation would be one that can disagree with it. Anything typed into the
 * search box is resolved by the chain, which normalises and checks it there.
 */
function groupUserId(id) {
  const n = String(id).toUpperCase();
  if (n.length < 4) return n;
  const groups = [n.slice(0, 2)];
  const payload = n.slice(2, -1);
  for (let i = 0; i < payload.length; i += 4) groups.push(payload.slice(i, i + 4));
  groups.push(n.slice(-1));
  return groups.join('-');
}

const shortRef = (a) => (a && a.length > 16 ? `${a.slice(0, 8)}…${a.slice(-5)}` : a || '—');
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) =>
  ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

const dateOf = (iso) => iso
  ? new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' })
  : null;
const dateTimeOf = (unix) => new Date(Number(unix) * 1000)
  .toLocaleString(undefined, { year: 'numeric', month: 'long', day: 'numeric',
                               hour: '2-digit', minute: '2-digit' });

/** Durations in the words a clerk uses, not in seconds. */
function span(seconds) {
  const s = Math.abs(Math.round(seconds));
  if (s >= 172800) return `${Math.round(s / 86400)} days`;
  if (s >= 7200) return `${Math.round(s / 3600)} hours`;
  if (s >= 120) return `${Math.round(s / 60)} minutes`;
  return `${s} seconds`;
}

const live = (list) => (list || []).filter((x) => !x.released && !x.lifted);
const KIND = {
  mortgage: 'Mortgage', lien: 'Lien', 'right-of-way': 'Right of way', caveat: 'Caveat',
  agricultural_use_only: 'Agricultural use only', no_fractionalisation: 'May not be fractionalised',
  foreign_ownership_capped: 'Foreign ownership capped', heritage_protected: 'Heritage protected',
  minimum_parcel_size: 'Minimum parcel size', customary_tenure: 'Customary tenure',
  grant: 'Grant', sale: 'Deed of sale', inheritance: 'Succession', survey: 'Survey',
  'court order': 'Court order',
};
const kindLabel = (k) => KIND[k] || String(k || '').replace(/[_-]/g, ' ')
  .replace(/^./, (c) => c.toUpperCase()) || 'Entry';

// ---------------------------------------------------------------------------
// The verdict.
//
// One function, used by the search results and by the record, so a title cannot
// read as clean in a list and encumbered on its own page. The order is by how
// badly it should stop somebody: a freeze and a dispute stop everything, an
// open transfer means the land may not be the seller's for much longer, and a
// live claim means the buyer inherits it.
// ---------------------------------------------------------------------------

function verdict(parcel, transfers) {
  const claims = live(parcel.encumbrances);
  const limits = live(parcel.restrictions);
  const open = (transfers || []).find((t) => t.completed_at === '0' && !t.objected_by);
  const objected = (transfers || []).find((t) => t.objected_by);
  const why = [];

  let tone = 'ok';
  let head = 'This title is clear';
  let lead = 'One holder, no claims against it, and nothing in progress.';

  if (parcel.status === 'STATUS_FROZEN') {
    tone = 'bad'; head = 'This title is frozen';
    lead = 'The registry office has stopped all dealings with this land. Nothing can be transferred until the freeze is lifted.';
  } else if (parcel.status === 'STATUS_DISPUTED') {
    tone = 'bad'; head = 'This title is disputed';
    lead = 'Somebody objected to a transfer of this land. The register has stopped and a court decides — not the register.';
    if (objected) why.push(`The objection reads: “${esc(objected.objection_reason)}”`);
  } else if (open) {
    tone = 'warn'; head = 'A transfer of this land is under way';
    lead = 'Somebody is already in the process of acquiring this parcel. Do not pay for it before reading the transfer.';
  } else if (claims.length) {
    tone = 'warn'; head = 'This title carries claims against it';
    lead = 'The holder owns the land, but somebody else has a right over it that a buyer would inherit.';
  }

  claims.forEach((c) => why.push(
    `${kindLabel(c.kind)} — ${esc(c.detail || 'no detail recorded')}`));
  // Restrictions are listed but do not change the verdict. They are not a
  // defect in the title — heritage protection is a fact about the land, not a
  // warning about the seller — and colouring them like a lien would teach a
  // reader to ignore the colour.
  if (limits.length) why.push(
    `Limits on what may be done with the land: ${limits.map((r) => kindLabel(r.kind)).join(', ')}.`);

  return { tone, head, lead, why, open, claims, limits };
}

// The chip is not decoration and it is not the colour again. A reader who cannot
// tell this page's green from its amber — one man in twelve — gets the verdict
// from the heading and from this, and never from the border alone.
const VERDICT_WORD = { ok: 'nothing against it', warn: 'read this before you pay', bad: 'stop' };

const verdictHtml = (v) => `
  <div class="verdict verdict--${v.tone}">
    <div class="row" style="align-items:flex-start">
      <div class="verdict__head">${esc(v.head)}</div>
      <span class="pill p-${v.tone}">${esc(VERDICT_WORD[v.tone] ?? v.tone)}</span>
    </div>
    <p class="verdict__why">${esc(v.lead)}</p>
    ${v.why.length ? `<ul class="verdict__why">${v.why.map((w) => `<li>${w}</li>`).join('')}</ul>` : ''}
  </div>`;

const statusPill = (s) => {
  const t = String(s).replace('STATUS_', '').toLowerCase().replace(/_/g, ' ');
  const cls = { registered: 'p-ok', 'transfer pending': 'p-warn',
                disputed: 'p-bad', frozen: 'p-bad' }[t] || 'p-mute';
  return `<span class="pill ${cls}">${t}</span>`;
};

// ---------------------------------------------------------------------------
// Where a transfer has got to.
//
// The four steps are the module's anti-bribery shape, and the page's job is to
// make the *next* one obvious: an office opening this should see what it is
// waiting for without reading five fields and doing the arithmetic itself.
// ---------------------------------------------------------------------------

function stage(transfer, parcel, p) {
  const quorum = Number(p.attestation_quorum);
  const got = (transfer.attestors || []).length;
  const quorumAt = Number(transfer.quorum_at);
  const window = Number(p.challenge_window);
  const closesAt = quorumAt ? quorumAt + window : 0;
  const now = Math.floor(Date.now() / 1000);

  if (transfer.objected_by) {
    return { key: 'objected', tone: 'bad', got, quorum, closesAt,
             waiting: 'Stopped. Somebody objected, and the parcel is disputed until a court decides.' };
  }
  if (transfer.completed_at !== '0') {
    return { key: 'done', tone: 'ok', got, quorum, closesAt,
             waiting: 'Complete. The land has changed hands.' };
  }
  if (!transfer.validated) {
    return { key: 'validate', tone: 'warn', got, quorum, closesAt,
             waiting: `Waiting for ${officeLabel(parcel?.authority)} — the office holding this parcel's file — to validate it.` };
  }
  if (got < quorum) {
    return { key: 'attest', tone: 'warn', got, quorum, closesAt,
             waiting: `Waiting for ${quorum - got} more independent ${quorum - got === 1 ? 'office' : 'offices'} to attest. ${got} of ${quorum} so far.` };
  }
  if (now < closesAt) {
    return { key: 'window', tone: 'warn', got, quorum, closesAt,
             waiting: `Open to objection until ${dateTimeOf(closesAt)} — ${span(closesAt - now)} from now. Anybody may object; one objection stops it.` };
  }
  return { key: 'complete', tone: 'ok', got, quorum, closesAt,
           waiting: 'Everything is satisfied and the window has closed. Anyone at all may now complete it — that step is mechanical, so no official can withhold it.' };
}

async function stepsHtml(transfer, parcel, p, st) {
  const proposedOn = dateOf(await blockTime(transfer.proposed_at));
  const attestors = await Promise.all((transfer.attestors || [])
    .map(async (a) => `${officeLabel(a)}${officeWhere(a) ? ` (${esc(officeWhere(a))})` : ''}`));
  const couldAttest = (await authorities())
    .filter((a) => a.active && a.address !== parcel?.authority
                && !(transfer.attestors || []).includes(a.address))
    .map((a) => a.name);

  const mark = (done, now, stop) =>
    stop ? 'step--stop' : done ? 'step--done' : now ? 'step--now' : '';
  const glyph = (done, now, stop) => (stop ? '×' : done ? '✓' : now ? '…' : '');

  const order = ['validate', 'attest', 'window', 'complete', 'done', 'objected'];
  const at = order.indexOf(st.key);
  const reached = (k) => at > order.indexOf(k) || st.key === 'done' || st.key === 'complete';

  const holderName = await personLabel(transfer.from);
  const toName = await personLabel(transfer.to);

  return `<ol class="steps">
    <li class="step step--done">
      <span class="step__mark">✓</span>
      <div class="step__body">
        <div class="step__title">The holder consented</div>
        <p class="muted small" style="margin:.2rem 0">${esc(holderName)} proposed this transfer to ${esc(toName)}${
          proposedOn ? ` on ${esc(proposedOn)}` : ` in block ${esc(transfer.proposed_at)}`}${
          transfer.price ? `, at a declared price of ${esc(transfer.price)}` : ''}.
          No office can start the sale of somebody's land; this step has to come first.</p>
      </div>
    </li>
    <li class="step ${mark(transfer.validated, st.key === 'validate', false)}">
      <span class="step__mark">${glyph(transfer.validated, st.key === 'validate', false)}</span>
      <div class="step__body">
        <div class="step__title">The office in charge validated it</div>
        <p class="muted small" style="margin:.2rem 0">${
          transfer.validated
            ? `${esc(officeLabel(transfer.validated_by))} confirmed the seller against the paper file it holds.`
            : `Only ${esc(officeLabel(parcel?.authority))} may do this — it is the office holding this parcel's file.`}</p>
      </div>
    </li>
    <li class="step ${mark(st.got >= st.quorum, st.key === 'attest', false)}">
      <span class="step__mark">${glyph(st.got >= st.quorum, st.key === 'attest', false)}</span>
      <div class="step__body">
        <div class="step__title">Independent offices attested — ${st.got} of ${st.quorum}</div>
        ${attestors.length
          ? `<ul class="muted small" style="margin:.2rem 0 .2rem 1rem;padding:0">${
              attestors.map((a) => `<li>${a}</li>`).join('')}</ul>`
          : '<p class="muted small" style="margin:.2rem 0">Nobody has attested yet.</p>'}
        ${st.got < st.quorum && couldAttest.length && st.key !== 'objected' && st.key !== 'done'
          ? `<p class="muted small" style="margin:.2rem 0">Still able to attest: ${esc(couldAttest.join(', '))}.
             ${esc(officeLabel(parcel?.authority))} cannot — an attestor from the parcel's own office is not independent.</p>`
          : st.key === 'objected' && st.got < st.quorum
            ? `<p class="muted small" style="margin:.2rem 0">It never reached quorum, and now cannot:
               the objection below stopped the transfer, so no further attestation is possible.</p>`
            : ''}
      </div>
    </li>
    <li class="step ${transfer.objected_by ? 'step--stop'
                    : mark(reached('window'), st.key === 'window', false)}">
      <span class="step__mark">${transfer.objected_by ? '×'
                    : glyph(reached('window'), st.key === 'window', false)}</span>
      <div class="step__body">
        <div class="step__title">${transfer.objected_by ? 'Objected to' : 'The challenge window'}</div>
        <p class="muted small" style="margin:.2rem 0">${
          transfer.objected_by
            ? `${esc(await personLabel(transfer.objected_by))} objected: “${esc(transfer.objection_reason)}”. One objection is enough — the register stops and a court decides.`
            : st.closesAt
              ? `Runs from the moment quorum was reached — ${esc(dateTimeOf(Number(transfer.quorum_at)))} — for ${esc(span(Number(p.challenge_window)))}, closing ${esc(dateTimeOf(st.closesAt))}. Anybody at all may object during it, with no credentials and no standing to prove.`
              : `Has not started. The clock runs from quorum, not from the proposal, so the public clock only starts once the transfer is real.`}</p>
      </div>
    </li>
    <li class="step ${transfer.completed_at !== '0' ? 'step--done'
                    : st.key === 'complete' ? 'step--now' : ''}">
      <span class="step__mark">${transfer.completed_at !== '0' ? '✓' : st.key === 'complete' ? '…' : ''}</span>
      <div class="step__body">
        <div class="step__title">Completion</div>
        <p class="muted small" style="margin:.2rem 0">${
          transfer.completed_at !== '0'
            ? `Completed ${esc(dateTimeOf(Number(transfer.completed_at)))}. The land is now held by ${esc(toName)}.`
            : 'Mechanical, and callable by anyone. If only an official could finalise a transfer, an official could refuse to — and refusal is leverage worth paying for.'}</p>
      </div>
    </li>
  </ol>`;
}

// ---------------------------------------------------------------------------
// Composing an action, rather than performing it. See registrar.js.
// ---------------------------------------------------------------------------

// Copy targets live in a registry and the click is handled once, on the
// document. Wiring each button as it is built looked simpler and was wrong:
// the markup is composed into a string and inserted by the caller, so anything
// that reaches for the element while composing finds nothing there yet — and a
// copy button that silently does nothing is worse than no copy button, because
// the clerk believes they have the command on their clipboard.
const COPY = new Map();
let seq = 0;

/**
 * Anything a person copies into a shell goes through this.
 *
 * A carriage return inside a pasted command gives `$'\r': command not found`,
 * an error that names neither the cause nor the file and appears only for the
 * person whose machine served the page. `clients/land/**` is pinned to LF in
 * .gitattributes for the same reason — this is the second line of defence, and
 * it is here because this file is *deployed by copying the working tree*, so a
 * checkout's line endings are the bytes a clerk's browser gets.
 */
const shellSafe = (s) => String(s ?? '').replace(/\r/g, '');

/**
 * An identifier: truncated, in mono, with the whole of it one click away and a
 * copy button beside it.
 *
 * A survey hash is sixty-four characters and an account reference is
 * thirty-nine, and printed in full in the middle of a sentence neither is read —
 * they are skipped, which on this page means a buyer has not in fact checked
 * that the hash on their surveyor's document is the hash the register holds.
 * Truncated to head and tail they are read, because comparing ten characters
 * against ten is something a person will actually do at a counter. The whole of
 * it stays one click away, always: this is the page a court reads.
 */
function idHtml(value, label) {
  const full = String(value ?? '');
  if (!full) return '<span class="muted">—</span>';
  const brief = full.length > 18 ? `${full.slice(0, 12)}…${full.slice(-6)}` : full;
  const what = label ? `${label}: ` : '';
  return `<span class="y-id"><button type="button" class="y-id__text"
      data-full="${esc(full)}" data-brief="${esc(brief)}" title="${esc(full)}"
      aria-label="${esc(what)}${esc(brief)} — show all of it"
    >${esc(brief)}</button><button type="button" class="y-id__copy"
      data-copy-text="${esc(full)}"
      aria-label="Copy ${esc(what)}${esc(full)}">copy</button></span>`;
}

document.addEventListener('click', async (e) => {
  // Revealing an identifier in place, rather than in a tooltip nobody on a
  // touch screen can open.
  const reveal = e.target.closest?.('.y-id__text');
  if (reveal) {
    const open = reveal.closest('.y-id').classList.toggle('y-id--open');
    reveal.textContent = open ? reveal.dataset.full : reveal.dataset.brief;
    return;
  }

  const btn = e.target.closest?.('[data-copy], [data-copy-text]');
  if (!btn) return;
  const text = btn.dataset.copyText !== undefined
    ? btn.dataset.copyText
    : COPY.get(btn.dataset.copy);
  if (text === undefined) return;
  const was = btn.dataset.label ?? btn.textContent;
  try { await navigator.clipboard.writeText(shellSafe(text)); btn.textContent = 'copied'; }
  catch { btn.textContent = 'select the text and copy it'; }
  setTimeout(() => { btn.textContent = was; }, 2500);
});

/**
 * One thing to run, composed for this parcel and this office.
 *
 * x/land has a CLI now, so this is the real `blockchaind tx land …` command
 * rather than a transaction document assembled by hand in a browser. The
 * difference is not cosmetic: a document this page builds is checked by
 * nothing until it reaches a block, whereas a command is parsed by the binary
 * that owns the message — a wrong field fails at the terminal, named, in front
 * of the person who can correct it.
 *
 * `office` decides which of two shapes is printed. An office is a group
 * account and cannot broadcast: its command is generated unsigned and
 * submitted as a proposal its registrars vote on. Printing the broadcast form
 * for an office would suggest one clerk can move land alone, which is the
 * thing the module is built to prevent.
 */
function action(title, note, spec, office = false) {
  const id = `act${seq++}`;
  const cmd = office ? officeTx(spec) : landTx(spec);
  COPY.set(`${id}-cmd`, cmd);
  return `<div class="card">
    <div class="row"><h3>${esc(title)}</h3></div>
    <p class="muted small" style="margin:.1rem 0 .5rem">${note}</p>
    <div class="row" style="margin-bottom:.2rem">
      <span class="muted small">run this where the key is</span>
      <button class="copy" data-copy="${id}-cmd" data-label="copy the command">copy the command</button>
    </div>
    <pre>${esc(cmd)}</pre>
  </div>`;
}

// ===========================================================================
// SIGNING.
//
// The console now signs three of x/land's twelve messages, proposes seven to
// an office, composes one as a command and explains the last as a governance
// vote. Which is which — and the reasoning for each — is in registrar.js, and
// this section is only the plumbing.
//
// The rule that keeps it honest is that nothing below chooses. Every entry
// point goes through `personalMessage` or `officeProposal`, both of which throw
// on a message whose shape does not permit them. A screen written in a hurry
// therefore cannot decide for itself that an office message is signable; it
// gets an exception instead of a signature.
// ===========================================================================

/**
 * Who is signing, remembered for the length of a visit and no longer.
 *
 * Not persisted. A land register is consulted on shared and borrowed devices —
 * a phone passed across a counter is the normal case — and an address left in
 * localStorage is the next person's starting point. Reconnecting costs one
 * click and the wallet remembers its own unlock.
 */
let SIGNER = null;

const signerBar = () => document.getElementById('signer');

function renderSigner() {
  const el = signerBar();
  if (!el) return;
  el.innerHTML = SIGNER
    ? `<span class="pill p-ok">signed in</span>
       <span class="small">${esc(SIGNER.label ?? '')}</span>
       ${idHtml(SIGNER.address, 'your account')}
       <button class="copy" id="signer-out">sign out</button>`
    : `<button class="btn" id="signer-in">Connect your wallet</button>
       <span class="muted small">only needed to act — reading needs nothing</span>`;
  const inBtn = document.getElementById('signer-in');
  if (inBtn) inBtn.onclick = () => connect().then(renderSigner).catch(() => renderSigner());
  const outBtn = document.getElementById('signer-out');
  if (outBtn) outBtn.onclick = () => { SIGNER = null; renderSigner(); route(); };
}

/**
 * Connect, and find out what the chain already knows about this account.
 *
 * The user ID is looked up so the page can say "User NG-K3M9-7QRT-5" rather
 * than forty characters of bech32 — the same courtesy every other name on this
 * page gets, applied to the reader's own.
 */
async function connect() {
  const account = await connectWallet();
  const id = await aliasOf(account.address);
  SIGNER = { ...account, label: id ? `User ${groupUserId(id)}` : '' };
  return SIGNER;
}

/**
 * Sign something, put it in a block, and say what happened.
 *
 * `where` is the element the outcome is drawn into. The three states are drawn
 * in it in turn — waiting for the wallet, waiting for the block, and the
 * result — because the worst version of this screen is the one that goes blank
 * while a popup is open and leaves somebody unsure whether they have just sold
 * their land.
 */
async function submit(where, messageAnys, memo, { onDone } = {}) {
  const el = typeof where === 'string' ? document.getElementById(where) : where;
  const say = (html) => { el.innerHTML = html; };

  if (!SIGNER) {
    try {
      say(`<p class="spin">Waiting for the wallet window…</p>`);
      await connect();
      renderSigner();
    } catch (e) {
      say(walletFailure(e));
      return null;
    }
  }

  say(`<div class="card card--waiting"><p class="lede">Check this in your wallet</p>
    <p class="muted small">The wallet's own window shows what is about to be signed. Nothing
      is sent until you approve it there — and this page cannot approve it for you, which is
      the point of it being a separate window.</p></div>`);

  let outcome;
  try {
    outcome = await signAndSend(SIGNER, messageAnys, memo);
  } catch (e) {
    say(walletFailure(e));
    return null;
  }

  if (!outcome.accepted) {
    say(refusalHtml(outcome, 'Nothing was recorded. The node refused this before it '
      + 'reached a block, so the register is exactly as it was.'));
    return outcome;
  }
  if (!outcome.delivered) {
    say(`<div class="card card--warn">
      <p class="lede">Sent, and still waiting for a block</p>
      <p class="muted small">The transaction is signed and the node has it. This page stopped
        watching after half a minute, which is not the same as it having failed — do not send
        it again. Reload in a moment and the register will show whether it ran.</p>
      <p class="small">Transaction ${idHtml(outcome.hash, 'the transaction')}</p></div>`);
    return outcome;
  }
  if (outcome.code) {
    say(refusalHtml(outcome, 'It reached a block and the chain refused it there, so nothing '
      + 'changed in the register.'));
    return outcome;
  }

  say(`<div class="card card--ok">
    <p class="lede">Done, and in the register</p>
    <p class="muted small">Written in block ${esc(String(outcome.height))}. Everything on this
      page is read back from the register itself, so reload to see it.</p>
    <p class="small">Transaction ${idHtml(outcome.hash, 'the transaction')}</p>
    ${onDone ? onDone(outcome) : ''}</div>`);
  return outcome;
}

/**
 * A refusal, translated, with the chain's own words kept beside it.
 *
 * An operator who cannot see the original stops trusting the translation, and
 * is right to. A refusal this page has not learned yet is shown as itself
 * rather than smoothed into one it has — that is how "your objection was not
 * recorded" comes to read as though everything is fine.
 */
function refusalHtml(outcome, lead) {
  const r = explainRefusal(outcome);
  return `<div class="card card--bad">
    <p class="lede">The chain refused this</p>
    <p class="small">${esc(lead)}</p>
    ${r.known
      ? `<p style="margin:.5rem 0 0">${esc(r.says)}</p>`
      : `<p style="margin:.5rem 0 0">This console has not learned what that refusal means.
           The chain's own words are below, unaltered.</p>`}
    <details><summary>What the chain said</summary>
      <pre>${esc(outcome.log || '(no message)')}</pre>
      <p class="muted small">${esc(outcome.codespace || 'no codespace')} · code ${
        esc(String(outcome.code ?? '?'))}</p></details>
  </div>`;
}

/** The wallet declining is ordinary. Everything else is worth naming. */
function walletFailure(e) {
  if (e instanceof WalletRefused && e.declined) {
    return `<div class="card"><p class="muted small" style="margin:0">Nothing was signed and
      nothing was sent. ${esc(e.message)}</p></div>`;
  }
  if (e instanceof CallFailed && e.status === 'unknown') {
    return `<div class="card card--warn"><p class="lede">That account is not on the chain yet</p>
      <p class="muted small">An account comes into existence when something is first sent to
        it, and until then it cannot sign. Ask whoever is helping you to send it any amount —
        the fee for these messages is zero on this network, so the amount does not matter.</p>
      </div>`;
  }
  return `<div class="card card--bad"><p class="lede">This could not be signed</p>
    <p class="small"><code>${esc(e.message)}</code></p>
    <p class="muted small">Nothing was sent. If the wallet window did not open, allow pop-ups
      for this site.</p></div>`;
}

/**
 * A card for a message this console signs from a person's own key.
 *
 * `terminal` is not decoration. Two of the three signable messages cannot be
 * undone — an objection stops a transfer for good and marks the parcel
 * disputed, and a completion moves title — and the sentence before the button
 * is the only protection a person gets. So the catalogue's `terminal` text is
 * rendered as a distinct block above the control rather than as one more line
 * of small print, and the button says what it does rather than "confirm".
 */
function signCard(name, { heading, body, verb, fields, memo, gather, disabled }) {
  const spec = ACTIONS[name];
  const id = `sign${seq++}`;
  return {
    html: `<div class="card ${spec.undo === '' ? 'card--grave' : 'card--waiting'}">
      <div class="row"><h3>${esc(heading ?? spec.title)}</h3>
        <span class="pill p-brass">you sign this</span></div>
      ${body ?? ''}
      <p class="muted small" style="margin:.4rem 0 0"><strong>Who may:</strong> ${esc(spec.who)}.
        ${esc(spec.why)}</p>
      ${spec.undo === ''
        ? `<p class="badbox" style="margin-top:.6rem"><strong>This cannot be undone.</strong>
             ${esc(spec.terminal)}</p>`
        : `<p class="muted small" style="margin:.3rem 0 0"><strong>Afterwards:</strong> ${
             esc(spec.undo)}</p>`}
      <p style="margin:.8rem 0 0"><button class="primary" id="${id}-go"${
        disabled ? ' disabled' : ''}>${esc(verb ?? spec.title)}</button></p>
      <div id="${id}-out"></div>
    </div>`,
    wire: () => {
      const btn = document.getElementById(`${id}-go`);
      if (!btn) return;
      btn.onclick = async () => {
        const out = document.getElementById(`${id}-out`);
        let values;
        try {
          values = gather ? gather() : (fields ?? {});
        } catch (e) {
          // A validation refusal, phrased as the chain would refuse it. Better
          // caught here: the chain's version costs a signature and a block.
          out.innerHTML = `<p class="err small" style="margin-top:.7rem">${esc(e.message)}</p>`;
          return;
        }
        // Connected before the message is composed, not inside `submit`: the
        // signer's own address IS a field of every one of these messages, so a
        // message built before connecting would name `undefined` as its
        // creator and be refused after a signature had already been given.
        if (!SIGNER) {
          out.innerHTML = '<p class="spin">Waiting for the wallet window…</p>';
          try { await connect(); renderSigner(); }
          catch (e) { out.innerHTML = walletFailure(e); return; }
        }
        btn.disabled = true;
        try {
          await submit(out, [personalMessage(name, { ...values, creator: SIGNER.address })],
            memo ?? '');
        } finally {
          btn.disabled = false;
        }
      };
    },
  };
}

/**
 * A card for a message only an office can send.
 *
 * The button does not sign the land message — it cannot, and the whole module
 * depends on that. It signs `MsgSubmitProposal` in the registrar's own name,
 * carrying the land message to the office's group policy, where the office's
 * own M-of-N then votes. The card says so above the button, because a control
 * labelled "Validate" that in fact opens a vote would be lying about what the
 * click did.
 *
 * The command is kept beside it, in a disclosure. A registrar with the office's
 * keyring on a machine may well prefer it, and an operator who cannot see what
 * the button would send stops trusting the button.
 */
function proposeCard(name, { office, officeName, heading, body, fields, summary, cli, disabled }) {
  const spec = ACTIONS[name];
  const id = `prop${seq++}`;
  const cmd = cli ? officeTx({ ...cli, from: 'your-office-key' }) : null;
  if (cmd) COPY.set(`${id}-cmd`, cmd);
  return {
    html: `<div class="card card--waiting">
      <div class="row"><h3>${esc(heading ?? spec.title)}</h3>
        <span class="pill p-mute">put to ${esc(officeName ?? 'the office')}</span></div>
      ${body ?? ''}
      <p class="muted small" style="margin:.4rem 0 0"><strong>Who may:</strong> ${esc(spec.who)}.
        ${esc(spec.why)}</p>
      <p class="warnbox" style="margin-top:.6rem">${esc(groupPreamble(officeName ?? 'This office'))}
        The button below does not act as the office and cannot: it signs a
        <em>proposal</em> with your own key, and ${esc(officeName ?? 'the office')}'s registrars
        vote on it afterwards.</p>
      <p class="muted small" style="margin:.3rem 0 0"><strong>Afterwards:</strong> ${
        esc(spec.undo)}</p>
      <p style="margin:.8rem 0 0"><button class="primary" id="${id}-go"${
        disabled ? ' disabled' : ''}>Put this to ${esc(officeName ?? 'the office')}</button></p>
      <div id="${id}-out"></div>
      ${cmd ? `<details><summary>Or run it where the office's keyring is</summary>
        <div class="cmdhead"><span class="muted small">generated unsigned, then submitted as
          the same proposal</span>
          <button class="copy" data-copy="${id}-cmd" data-label="copy the command">copy the
            command</button></div>
        <pre>${esc(cmd)}</pre></details>` : ''}
    </div>`,
    wire: () => {
      const btn = document.getElementById(`${id}-go`);
      if (!btn) return;
      btn.onclick = async () => {
        const out = document.getElementById(`${id}-out`);
        if (!SIGNER) {
          try { await connect(); renderSigner(); }
          catch (e) { out.innerHTML = walletFailure(e); return; }
        }
        let proposal;
        try {
          proposal = officeProposal(name, {
            office,
            proposer: SIGNER.address,
            fields: typeof fields === 'function' ? fields() : fields,
            metadata: proposalMetadata(name, summary),
          });
        } catch (e) {
          out.innerHTML = `<p class="err small" style="margin-top:.7rem">${esc(e.message)}</p>`;
          return;
        }
        btn.disabled = true;
        try {
          await submit(out, [any(proposal.typeUrl, proposal.bytes)], '', {
            onDone: () => `<p class="small" style="margin:.5rem 0 0">This is now a proposal
              inside ${esc(officeName ?? 'the office')}, not a change to the register. It takes
              effect only when enough of its registrars have voted for it.</p>`,
          });
        } finally {
          btn.disabled = false;
        }
      };
    },
  };
}

/** Insert a card and wire its button. Composed-then-inserted, so wiring cannot
 *  happen while composing — see the note above `COPY`. */
function place(el, cards) {
  const list = [].concat(cards);
  el.innerHTML = list.map((c) => (typeof c === 'string' ? c : c.html)).join('');
  list.forEach((c) => { if (typeof c !== 'string') c.wire(); });
}

// ---------------------------------------------------------------------------
// Screens.
// ---------------------------------------------------------------------------

const view = document.getElementById('view');
const banner = document.getElementById('banner');
/**
 * The banner, and what a failure here actually means now.
 *
 * The 401 this used to explain is gone, because the reads moved off the REST
 * surface that produced it — see the note at the top of this script, and
 * chain.js. It was worth explaining while it lasted: the proxy allowlists the
 * node's REST paths per module and denies by default, x/land's prefix was not
 * on that list, and the symptom was a browser login box with no credentials to
 * type into it. That reads as "you are not allowed to see the register" rather
 * than as "this deployment has not published it yet", and those are opposite
 * facts about a register whose whole design is that reading it is public.
 *
 * What is left is a genuine failure to reach the node, and the thing a reader
 * has to be told about one is that nothing below it is a statement about any
 * title. "No objection against this land" and "this page could not ask" look
 * identical on a screen.
 */
const setBanner = (e) => {
  if (!e) { banner.innerHTML = ''; return; }
  banner.innerHTML = `<p class="err">
    <strong>The register did not answer.</strong> <code>${esc(e.message)}</code>
    <br>Nothing on this page is a statement about any title while this is showing. The
    register is read over the node's public interface at <code>/api/rpc/</code>, which
    needs no account and no password, so this is the node being unreachable from here
    rather than a register you are barred from.</p>`;
};

/**
 * Four, and the fourth one is new.
 *
 * "Register a parcel" and "Fractionalisation" left the masthead and did not
 * leave the console: both are office acts, and a top-level tab for each made
 * the navigation of a public register read as the navigation of a staff tool.
 * They are reached from "What can be done", which lists all twelve of x/land's
 * messages with who may send each — and which is the honest front page for a
 * chain with no offices and no parcels on it yet, where every other screen can
 * only say "none".
 */
const ROUTES = [
  ['#/', 'Search a title'],
  ['#/pending', 'Transfers under way'],
  ['#/offices', 'Registry offices'],
  ['#/actions', 'What can be done'],
];

function renderNav(hash) {
  document.getElementById('nav').innerHTML = ROUTES.map(([h, label]) => {
    const on = h === '#/'
      ? (hash === '#/' || hash === '' || hash.startsWith('#/parcel') || hash.startsWith('#/transfer'))
      : h === '#/actions'
        ? (hash.startsWith('#/actions') || hash === '#/register' || hash === '#/authorisations')
        : hash.startsWith(h);
    return `<a href="${h}"${on ? ' aria-current="page"' : ''}>${label}</a>`;
  }).join('');
}

// --- 1. Search ------------------------------------------------------------

async function screenSearch(query) {
  view.innerHTML = `
    <h2>Who owns this land, and is the title clean?</h2>
    <form class="searchbar" id="sf">
      <input id="q" value="${esc(query || '')}" autofocus
             placeholder="Cadastral reference, parcel number, user ID, or a survey hash">
      <button class="primary" type="submit">Search the register</button>
    </form>
    <p class="muted small" style="margin:.5rem 0 0">
      No account is needed and nobody is told that you looked. Reading the register is
      public on purpose: the secrecy that protects a corrupt registry is the same
      secrecy that stops a buyer discovering the land was already sold.</p>
    <div id="results"></div>
    <div id="recent"></div>`;

  document.getElementById('sf').onsubmit = (e) => {
    e.preventDefault();
    const v = document.getElementById('q').value.trim();
    location.hash = v ? `#/?q=${encodeURIComponent(v)}` : '#/';
  };

  if (query) await runSearch(query);
  else await recentlyRegistered();
}

/**
 * One box, because a citizen holding a piece of paper does not know which of
 * the register's four identifiers is written on it. The query is shaped and
 * every plausible lookup is tried; what was tried and what answered is shown,
 * so "not found" is a statement about the register rather than about the query.
 */
async function runSearch(q) {
  const out = document.getElementById('results');
  out.innerHTML = '<p class="spin">Searching…</p>';
  const tried = [];
  const found = new Map();
  const failures = [];

  const attempt = async (what, ask, pick) => {
    tried.push(what);
    try {
      const r = await ask();
      if (r) pick(r);
    } catch (e) { failures.push(`${what} — ${e.message}`); }
  };

  const add = (p) => { if (p) found.set(p.id, p); };

  // Parcel numbers start at 1, and TRANSFER numbers start at 0. The two are
  // not the same rule and the difference is in the keeper, not in taste:
  // x/land/keeper/msg_server_parcel.go calls `NextParcelID.Next` and then, if
  // it got 0, calls it again — because a zero id is indistinguishable from an
  // unset protobuf field, and x/tokenisation says "this vehicle is over land"
  // by carrying a parcel id, so a parcel 0 would make every warehouse receipt
  // on the chain look like a vehicle over the first field this registry ever
  // recorded. msg_server_transfer.go does no such thing, so transfer 0 is
  // real and `#/transfer/0` must work. A "0" typed into this box is therefore
  // a reference, not a parcel number, and falls through to the lookups below.
  if (/^[1-9][0-9]*$/.test(q)) {
    await attempt('parcel number', () => landOrNull('Parcel', q), (r) => add(r.parcel));
  }
  if (/^[0-9a-f]{32,}$/i.test(q)) {
    await attempt('survey hash', () => parcelByGeometry(q), (r) => add(r.parcel));
  }
  if (/^yml1[0-9a-z]{20,}$/.test(q)) {
    await attempt('account reference', () => landOrNull('ParcelsByHolder', q),
      (r) => (r.parcels || []).forEach(add));
  } else if (/^[A-Za-z0-9-]{4,24}$/.test(q)) {
    // A user ID resolves to an account, and the account is then asked what it
    // holds. Shaped first: a cadastral reference has slashes in it, and asking
    // the alias module about "NR/KAN/2019/00412" produces a 501 that reads on
    // screen as the register failing when nothing failed at all.
    tried.push('user ID');
    try {
      const address = await addressOfAlias(q);
      if (address) {
        const r = await landOrNull('ParcelsByHolder', address);
        (r?.parcels || []).forEach(add);
      }
    } catch (e) { failures.push(`user ID — ${e.message}`); }
  }
  if (!/^[1-9][0-9]*$/.test(q)) {
    tried.push('cadastral reference');
    try { add((await parcelByRef(q))?.parcel); }
    catch (e) { failures.push(`cadastral reference — ${e.message}`); }
  }

  const parcels = [...found.values()];
  if (!parcels.length) {
    out.innerHTML = `
      <div class="card">
        <h3>Nothing in the register matches “${esc(q)}”</h3>
        <p class="muted small">Looked up as: ${esc(tried.join(', ') || 'nothing — the query matched no known form')}.
           A cadastral reference must match exactly, including its slashes.</p>
        ${failures.length ? `<p class="err small" style="margin-top:.6rem">Some lookups did not complete, so this
           is not proof the land is unregistered:<br>${failures.map(esc).join('<br>')}</p>` : `
        <p class="muted small">Every lookup answered, so the register really does hold no such title.
           If somebody is selling you this land on a paper reference, that is worth knowing before you pay.</p>`}
      </div>`;
    return;
  }

  const cards = await Promise.all(parcels.map(async (p) => {
    const ts = await landOrNull('TransfersByParcel', p.id);
    const v = verdict(p, ts?.transfers || []);
    return `<a class="card hit verdict verdict--${v.tone}" href="#/parcel/${p.id}">
      <div class="row">
        <span class="ref">${esc(p.cadastral_ref)}</span>
        ${statusPill(p.status)}
      </div>
      <div class="verdict__head" style="font-size:var(--step-1);margin:.35rem 0 .15rem">${
        esc(v.head)}</div>
      <p class="muted small" style="margin:0">${esc(v.lead)}</p>
      <p class="small" style="margin:.45rem 0 0">
        Held by ${esc(await personLabel(p.holder))} ·
        ${esc(officeLabel(p.authority))}${officeWhere(p.authority) ? `, ${esc(officeWhere(p.authority))}` : ''} ·
        parcel ${esc(p.id)}</p>
    </a>`;
  }));

  out.innerHTML = `<h2>${parcels.length} matching ${parcels.length === 1 ? 'title' : 'titles'}</h2>${cards.join('')}`
    + (failures.length ? `<p class="err small">Some lookups did not complete: ${failures.map(esc).join('; ')}</p>` : '');
}

async function recentlyRegistered() {
  const el = document.getElementById('recent');
  el.innerHTML = '<h2>Recently registered</h2><p class="spin">Reading the transaction log…</p>';
  let rows;
  try {
    rows = await txsOf(MESSAGES.register, 12);
  } catch (e) {
    el.innerHTML = `<h2>Recently registered</h2>
      <div class="card"><p class="muted small">The register cannot be listed — there is no
      "every parcel" query, deliberately, because a public list of who owns what is a
      targeting list. This panel is instead reconstructed from the chain's transaction
      log, and that log is not being served here.</p>
      <p class="err small" style="margin-top:.5rem"><code>${esc(e.message)}</code>
      ${e.status === 401 ? '<br>A 401 here is the operator\'s basic-auth gate on transaction search, which is a deliberate configuration rather than a fault. Search above still works without it.' : ''}</p>
      </div>`;
    return;
  }
  const ok = rows.filter((r) => r.code === 0);
  if (!ok.length) {
    el.innerHTML = `<h2>Recently registered</h2>
      <div class="card"><p class="muted small">No parcel has been registered on this chain yet.
        The register is empty — not unreachable. An office can open the first title from
        <a href="#/register">Register a parcel</a>.</p></div>`;
    return;
  }
  const items = await Promise.all(ok.slice(0, 10).map(async (r) => {
    const when = dateOf(await blockTime(r.height));
    return `<li class="is-ok">
      <div class="tl__when">${esc(when || `block ${r.height}`)}</div>
      <div class="tl__what">${esc(r.msg.cadastral_ref)}</div>
      <div class="muted small">First registration by ${esc(officeLabel(r.msg.creator))},
        to ${esc(await personLabel(r.msg.holder))}</div>
    </li>`;
  }));
  el.innerHTML = `<h2>Recently registered</h2>
    <div class="card">
      <ul class="tl">${items.join('')}</ul>
      <p class="muted small" style="margin-top:.6rem">Reconstructed from the chain's
        transaction log, not from the register itself: there is no "list every parcel"
        query, because a public list of who owns what is a targeting list.</p>
      <p class="muted small" style="margin-top:.4rem"><strong>Refused registrations are not in
        this list</strong>, and cannot be. A message the chain rejects — a second title over
        ground already titled, a cadastral reference already in use — never emits the event the
        transaction log is indexed by, so the refusal is in the block but not in the search index.
        The count above is registrations that <em>succeeded</em>, not attempts.</p>
    </div>`;
}

// --- 2. A parcel's full record --------------------------------------------

async function screenParcel(id) {
  view.innerHTML = '<p class="spin">Reading the title…</p>';
  // Refused before the call rather than after it. The register has no parcel 0
  // and never will — see the note in runSearch — so asking for one and
  // reporting "the register answered" would be reporting something that did
  // not happen.
  if (!/^[1-9][0-9]*$/.test(id)) {
    view.innerHTML = `<div class="card"><h3>No parcel ${esc(id)}</h3>
      <p class="muted small">Parcel numbers start at 1.</p>
      <p><a href="#/">Search by cadastral reference instead</a></p></div>`;
    return;
  }
  const res = await landOrNull('Parcel', id);
  if (!res) {
    view.innerHTML = `<div class="card"><h3>No parcel ${esc(id)}</h3>
      <p class="muted small">The register answered, and holds no title with that number.</p>
      <p><a href="#/">Search by cadastral reference instead</a></p></div>`;
    return;
  }
  const p = res.parcel;
  const [ts, prm] = await Promise.all([
    landOrNull('TransfersByParcel', p.id).catch(() => null),
    params(),
  ]);
  const transfers = ts?.transfers || [];
  const v = verdict(p, transfers);

  const holder = await personLabel(p.holder);
  const registeredOn = dateOf(await blockTime(p.registered_at));

  view.innerHTML = `
    ${verdictHtml(v)}
    <div id="why-frozen"></div>
    <div class="card">
      <div class="row">
        <div>
          <div class="label">Cadastral reference</div>
          <!-- The reference off somebody's paper file, in mono: it is compared
               character by character against a document, which is not something
               a proportional face lets a reader do. -->
          <div class="stat y-mono" style="font-size:var(--step-2)">${esc(p.cadastral_ref)}</div>
        </div>
        ${statusPill(p.status)}
      </div>
      <dl>
        <dt>Held by</dt><dd>${esc(holder)}</dd>
        <dt>Registry office</dt><dd>${esc(officeLabel(p.authority))}${
          officeWhere(p.authority) ? ` — ${esc(officeWhere(p.authority))}` : ''}</dd>
        <dt>First registered</dt><dd>${esc(registeredOn || `block ${p.registered_at} (this node no longer serves that block, so the date cannot be shown)`)}</dd>
        <dt>Parcel number</dt><dd>${esc(p.id)}</dd>
      </dl>
      <details>
        <summary>The evidence behind this record</summary>
        <dl style="margin-top:.5rem">
          <dt>Survey hash</dt><dd>${idHtml(p.geometry_hash, 'the survey hash')}</dd>
          <dt>Holder's account</dt><dd>${idHtml(p.holder, "the holder's account")}</dd>
          <dt>Office's account</dt><dd>${idHtml(p.authority, "the office's account")}</dd>
        </dl>
        <p class="muted small" style="margin-top:.5rem">The survey itself is not on the chain —
          it is too large for a block and usually carries somebody's personal details. The hash
          proves which survey this title refers to, and the registry serves the document.
          This hash is also the uniqueness constraint: a second title over it is refused.</p>
      </details>
    </div>

    <h2>Claims against this land</h2>
    <div id="claims"></div>

    <h2>Limits on what may be done with it</h2>
    <div id="limits"></div>

    <h2>Chain of title</h2>
    <p class="muted small" style="margin:0 0 .5rem">Every deed, every claim, and every transfer
      with the offices that signed it. This is the receipt a dispossessed owner does not
      currently get, and it is why nothing here is ever deleted — a released mortgage stays
      on the record, marked released.</p>
    <div id="history"><p class="spin">Assembling the history…</p></div>`;

  // Claims -----------------------------------------------------------------
  const claimsEl = document.getElementById('claims');
  const encs = p.encumbrances || [];
  claimsEl.innerHTML = encs.length ? (await Promise.all(encs.map(async (c, i) => `
      <div class="claim ${c.released ? 'claim--gone' : ''}">
        <div class="row">
          <strong>${esc(kindLabel(c.kind))}</strong>
          <span class="pill ${c.released ? 'p-mute' : 'p-warn'}">${c.released ? 'released' : 'live'}</span>
        </div>
        <p class="small" style="margin:.25rem 0 0">${esc(c.detail || 'No detail recorded.')}</p>
        <p class="muted small" style="margin:.2rem 0 0">In favour of ${esc(await personLabel(c.holder))}
          · recorded ${esc(dateOf(await blockTime(c.recorded_at)) || `in block ${c.recorded_at}`)}
          · entry ${i}</p>
      </div>`))).join('')
    : `<div class="card"><p class="muted small" style="margin:0">Nothing is charged against this land.
        No mortgage, no lien, no right of way — the register was asked and answered.</p></div>`;

  // Limits -----------------------------------------------------------------
  const limitsEl = document.getElementById('limits');
  const rs = p.restrictions || [];
  limitsEl.innerHTML = rs.length ? (await Promise.all(rs.map(async (r, i) => `
      <div class="claim ${r.lifted ? 'claim--gone' : 'claim--stop'}">
        <div class="row">
          <strong>${esc(kindLabel(r.kind))}${r.value ? ` — ${esc(r.value)}` : ''}</strong>
          <span class="pill ${r.lifted ? 'p-mute' : 'p-bad'}">${r.lifted ? 'lifted' : 'in force'}</span>
        </div>
        <p class="small" style="margin:.25rem 0 0">${esc(r.detail || 'No detail recorded.')}</p>
        <p class="muted small" style="margin:.2rem 0 0">Imposed by ${esc(officeLabel(r.imposed_by))}
          · ${esc(dateOf(await blockTime(r.imposed_at)) || `block ${r.imposed_at}`)} · entry ${i}</p>
      </div>`))).join('')
    : `<div class="card"><p class="muted small" style="margin:0">No restriction is recorded.
        The land may be used and dealt with under the ordinary law.</p></div>`;

  document.getElementById('history').innerHTML = await historyHtml(p, transfers, prm);
  if (p.status === 'STATUS_FROZEN') await whyFrozen(p);
}

/**
 * Why the land was frozen.
 *
 * A freeze stops everything a holder can do with their land, and the keeper
 * refuses one without a reason. That reason is now kept on the parcel, so this
 * reads it from the register rather than reconstructing it from the
 * transaction log — which mattered: the log is pruned, is often behind an
 * operator's basic-auth gate, and answered "no freeze order found" for a
 * parcel that was demonstrably frozen. Somebody told their land is stopped and
 * given no grounds cannot tell a fraud inquiry from an extortion.
 *
 * The whole history is shown, not only the order in force. A parcel frozen,
 * released and frozen again by a different office is exactly the record
 * somebody contesting the second freeze needs.
 */
async function whyFrozen(p) {
  const el = document.getElementById('why-frozen');
  const freezes = p.freezes || [];
  const live = [...freezes].reverse().find((f) => !f.lifted);

  el.innerHTML = `<div class="card" style="border-inline-start:3px solid var(--bad)">
    <h3>Why this land was frozen</h3>
    ${live
      ? `<p style="margin:.2rem 0 .3rem">“${esc(live.reason)}”</p>
         <p class="muted small" style="margin:0">Imposed by ${esc(officeLabel(live.imposed_by))}
           on ${esc(dateOf(await blockTime(live.imposed_at)) || `block ${live.imposed_at}`)}.</p>`
      : `<p class="muted small" style="margin:.2rem 0">This parcel is frozen but carries no
           recorded order. That means the freeze predates the register keeping its grounds — the
           reason exists only in the transaction that imposed it. Nothing has been lost from the
           record since; a freeze imposed today is recorded with its grounds.</p>`}
    ${freezes.length > 1 || (freezes.length === 1 && !live)
      ? `<h3 style="margin:.9rem 0 .3rem">Earlier orders on this land</h3>
         ${(await Promise.all(freezes.filter((f) => f !== live).map(async (f) => `
           <p class="muted small" style="margin:.3rem 0">“${esc(f.reason)}” — ${esc(officeLabel(f.imposed_by))},
             ${esc(dateOf(await blockTime(f.imposed_at)) || `block ${f.imposed_at}`)}.
             ${f.lifted
               ? `Lifted by ${esc(officeLabel(f.lifted_by))}${f.lift_reason ? `: “${esc(f.lift_reason)}”` : '.'}`
               : ''}</p>`))).join('')}`
      : ''}
    <p class="muted small" style="margin:.6rem 0 0">Read from the register itself. A freeze needs
      the office and its own quorum, but the holder's remedy against a captured office is an
      objection and a court, not the chain.</p>
  </div>`;
}

/**
 * The chain of title as a timeline, oldest first, because that is how a
 * conveyancer reads one: you start at the grant and follow it forward looking
 * for the step that does not fit.
 */
async function historyHtml(p, transfers, prm) {
  const events = [];

  events.push({ h: Number(p.registered_at), tone: 'ok',
    what: 'First registration',
    detail: `${officeLabel(p.authority)} opened this title over survey hash `
          + `${shortRef(p.geometry_hash)}, to ${await personLabel(p.holder)}. `
          + `Seeding a register is a political act, and it is recorded here so the `
          + `initial allocation is as auditable as everything after it.` });

  for (const d of p.deeds || []) {
    events.push({ h: Number(d.recorded_at), tone: 'ok',
      what: `${kindLabel(d.kind)} recorded`,
      detail: `${d.reference ? `Registry reference ${esc(d.reference)}` : 'No registry reference'}`
            + `${d.issued_on ? `, issued ${esc(d.issued_on)}` : ''}. `
            + `${d.uri ? `The registry serves the document at <a href="${esc(d.uri)}" rel="noreferrer">${esc(d.uri)}</a>. ` : ''}`
            + `<br><span class="muted">Document hash ${
                 idHtml(d.document_hash, 'the document hash')}</span>`,
      raw: true });
  }
  for (const c of p.encumbrances || []) {
    events.push({ h: Number(c.recorded_at), tone: 'warn',
      what: `${kindLabel(c.kind)} recorded${c.released ? ' (since released)' : ''}`,
      detail: `In favour of ${await personLabel(c.holder)}. ${c.detail || ''}` });
  }
  for (const r of p.restrictions || []) {
    events.push({ h: Number(r.imposed_at), tone: 'bad',
      what: `${kindLabel(r.kind)} imposed${r.lifted ? ' (since lifted)' : ''}`,
      detail: `${officeLabel(r.imposed_by)} — ${r.detail || 'no detail recorded'}` });
  }
  // Freezes belong in the timeline and not only in the banner above it. The
  // banner appears while a parcel is frozen; a freeze that was imposed and then
  // released would otherwise leave no trace on the page at all, even though the
  // register keeps it — and "this land was stopped for eleven months in 2026,
  // by that office, on those grounds" is exactly the sort of fact somebody
  // buying it is entitled to see.
  for (const f of p.freezes || []) {
    events.push({ h: Number(f.imposed_at), tone: 'bad',
      what: `Frozen by ${officeLabel(f.imposed_by)}${f.lifted ? ' (since lifted)' : ''}`,
      detail: `“${esc(f.reason)}”`, raw: true });
    if (f.lifted) {
      events.push({ h: Number(f.lifted_at), tone: 'ok',
        what: `Freeze lifted by ${officeLabel(f.lifted_by)}`,
        detail: f.lift_reason
          ? `“${esc(f.lift_reason)}”`
          : '<span class="muted">No grounds were given for the release.</span>',
        raw: true });
    }
  }

  for (const t of transfers) {
    const st = stage(t, p, prm);
    const attestors = (t.attestors || []).map((a) => officeLabel(a));
    const bits = [
      `Proposed by ${await personLabel(t.from)} to ${await personLabel(t.to)}`
      + (t.price ? ` at a declared price of ${esc(t.price)}` : ''),
      t.validated ? `Validated by ${officeLabel(t.validated_by)}`
                  : 'Not validated by the office in charge',
      attestors.length ? `Attested by ${attestors.join(', ')}`
                       : 'No independent office has attested',
      t.objected_by ? `Objected to by ${await personLabel(t.objected_by)}: “${esc(t.objection_reason)}”` : null,
      t.completed_at !== '0' ? `Completed ${dateTimeOf(Number(t.completed_at))}` : null,
    ].filter(Boolean);
    events.push({ h: Number(t.proposed_at),
      tone: t.objected_by ? 'bad' : t.completed_at !== '0' ? 'ok' : 'warn',
      what: `Transfer ${t.id} — ${t.objected_by ? 'objected to'
              : t.completed_at !== '0' ? 'completed' : st.key === 'window' ? 'open to objection'
              : 'under way'}`,
      detail: `${bits.join('.<br>')}.<br><a href="#/transfer/${t.id}">Follow this transfer →</a>`,
      raw: true });
  }

  events.sort((a, b) => a.h - b.h);
  const items = await Promise.all(events.map(async (e) => {
    const when = dateOf(await blockTime(e.h));
    return `<li class="is-${e.tone}">
      <div class="tl__when">${esc(when || `block ${e.h}`)}</div>
      <div class="tl__what">${esc(e.what)}</div>
      <div class="muted small">${e.raw ? e.detail : esc(e.detail)}</div>
    </li>`;
  }));
  return `<div class="card"><ul class="tl">${items.join('')}</ul>
    <p class="muted small" style="margin-top:.7rem">Dates are the time of the block each entry
      was written in, read back from the chain — not estimated from an average block time.
      Where a node no longer serves an old block the block number is shown instead of a guess.</p>
  </div>`;
}

// --- 3. The transfer workflow ---------------------------------------------

async function screenTransfer(id) {
  view.innerHTML = '<p class="spin">Reading the transfer…</p>';
  const res = await landOrNull('Transfer', id);
  if (!res) {
    view.innerHTML = `<div class="card"><h3>No transfer ${esc(id)}</h3>
      <p class="muted small">The register answered, and holds no transfer with that number.</p></div>`;
    return;
  }
  const t = res.transfer;
  const [pr, prm] = await Promise.all([
    landOrNull('Parcel', t.parcel_id), params(),
  ]);
  const p = pr?.parcel;
  const st = stage(t, p, prm);

  view.innerHTML = `
    <div class="verdict verdict--${st.tone}">
      <div class="label">Transfer ${esc(t.id)} · ${
        p ? esc(p.cadastral_ref) : `parcel ${esc(t.parcel_id)}`}</div>
      <div class="row" style="align-items:flex-start">
        <div class="verdict__head" style="margin-top:.2rem">What this is waiting for</div>
        <span class="pill p-${st.tone}">${esc(
          st.key === 'done' ? 'complete' : st.key === 'objected' ? 'stopped' : st.key)}</span>
      </div>
      <p class="verdict__why">${esc(st.waiting)}</p>
    </div>
    ${p ? `<p class="muted small" style="margin:-.3rem 0 1rem">
      <a href="#/parcel/${p.id}">Read the full record for ${esc(p.cadastral_ref)} →</a></p>` : ''}

    <div class="card">
      <h3>The four steps</h3>
      <p class="muted small" style="margin:.1rem 0 .6rem">No party controls two of them. To move
        this land against the holder's wishes somebody would have to buy the holder's consent,
        the office holding the file, and ${esc(prm.attestation_quorum)} offices elsewhere with no
        relationship to them — and then survive ${esc(span(Number(prm.challenge_window)))} of
        public objection.</p>
      <div id="steps"><p class="spin">…</p></div>
    </div>

    <h2>Acting on this transfer</h2>
    <div id="acts"></div>

    <h2>Object to this transfer</h2>
    <div id="objection"></div>`;

  document.getElementById('steps').innerHTML = await stepsHtml(t, p, prm, st);
  document.getElementById('acts').innerHTML = await actionsFor(t, p, prm, st);
  // Wired after insertion, not while composing — see the note above `action`.
  const cmp = document.getElementById('cmp-go');
  if (cmp) cmp.onclick = () => {
    const who = document.getElementById('cmp-who').value.trim();
    document.getElementById('cmp-out').innerHTML = action(
      `Complete transfer ${t.id}`,
      'The chain checks every condition and applies the result. Nothing here is discretionary.',
      { sub: 'complete-transfer', args: [t.id], from: who || 'your-key' });
  };
  renderObjection(t, st);
}

async function actionsFor(t, p, prm, st) {
  if (st.key === 'objected') {
    return `<div class="card"><p class="muted small" style="margin:0">Nothing can be done to this
      transfer on the chain. It is stopped, the parcel is disputed, and the register's job now is
      to preserve the evidence rather than to decide who is right. That belongs to a court.</p></div>`;
  }
  if (st.key === 'done') {
    return `<div class="card"><p class="muted small" style="margin:0">This transfer is complete
      and closed. It is kept rather than deleted: the record of who signed what and when is worth
      more than the bytes it costs.</p></div>`;
  }

  const out = [];
  if (st.key === 'validate' && p) {
    out.push(action(
      `Validate — ${officeLabel(p.authority)} only`,
      groupPreamble(officeLabel(p.authority)) + ' Validating means the office has checked the seller against the paper file it holds.',
      { sub: 'validate-transfer', args: [t.id], from: 'your-office-key' }, true));
  }
  if (st.key === 'attest' && p) {
    const eligible = (await authorities()).filter((a) => a.active
      && a.address !== p.authority && !(t.attestors || []).includes(a.address));
    if (!eligible.length) {
      out.push(`<div class="card"><p class="err small" style="margin:0">This transfer cannot reach
        quorum. It needs ${esc(prm.attestation_quorum)} independent attestations, and after
        excluding ${esc(officeLabel(p.authority))} there are only
        ${esc((await authorities()).filter((a) => a.active).length - 1)} other active offices in the
        register. That is a configuration problem for governance, not something an office can fix
        by signing.</p></div>`);
    }
    eligible.forEach((a) => out.push(action(
      `Attest — ${a.name}`,
      groupPreamble(a.name) + ` ${esc(officeLabel(p.authority))} cannot attest its own transfer; that is what "independent" means here, and the chain refuses it.`,
      { sub: 'attest-transfer', args: [t.id], from: 'your-office-key' }, true)));
  }
  if (st.key === 'window') {
    out.push(`<div class="card"><p class="muted small" style="margin:0">Nothing to sign. Every
      office that had to act has acted, and the transfer is now simply waiting out its challenge
      window. It can be completed after ${esc(dateTimeOf(st.closesAt))}.</p></div>`);
  }
  if (st.key === 'complete') {
    // An input rather than a placeholder in the document: the one field that
    // changes per sender is the sender, and a document that has to be
    // hand-edited before it will sign is a document somebody edits wrongly.
    out.push(`<div class="card">
      <h3>Complete this transfer — anyone may</h3>
      <p class="muted small" style="margin:.1rem 0 .3rem">Not an official act, and not restricted to
        one. The chain checks the conditions and applies the result, so this can be sent by the
        buyer, the seller, or a stranger. That is deliberate: if only an official could finalise a
        transfer, an official could refuse to, and refusal is leverage.</p>
      <label for="cmp-who">The account you will sign from</label>
      <input id="cmp-who" placeholder="yml1…">
      <p style="margin:.7rem 0 0"><button class="primary" id="cmp-go">Compose the completion</button></p>
      <div id="cmp-out"></div>
    </div>`);
  }
  return out.join('');
}

function renderObjection(t, st) {
  const el = document.getElementById('objection');
  if (st.key === 'objected' || st.key === 'done') {
    el.innerHTML = `<div class="card"><p class="muted small" style="margin:0">${
      st.key === 'objected'
        ? 'This transfer has already been objected to. One objection is enough.'
        : 'This transfer is complete. An objection can no longer stop it — a court can still undo it, and the record above is what that court would read.'}</p></div>`;
    return;
  }
  el.innerHTML = `
    <div class="card">
      <p class="muted small" style="margin:0 0 .3rem">Anybody may object, and no standing has to be
        proved. The person being robbed is usually the person with no official relationships, and
        requiring standing would exclude exactly them. One objection stops the transfer dead and
        marks the parcel disputed; the chain then preserves the evidence and a court decides.</p>
      <label for="obj-why">Why you are objecting — this goes on the permanent record</label>
      <textarea id="obj-why" placeholder="e.g. My late father's estate has not been distributed. The seller is one of four heirs and cannot convey the whole parcel alone. Succession case HC/PR/2024/118."></textarea>
      <label for="obj-who">Your account reference</label>
      <input id="obj-who" placeholder="yml1…">
      <p style="margin:.7rem 0 0"><button class="primary" id="obj-go">Compose the objection</button></p>
      <div id="obj-out"></div>
    </div>`;
  document.getElementById('obj-go').onclick = () => {
    const why = document.getElementById('obj-why').value.trim();
    const who = document.getElementById('obj-who').value.trim();
    const out = document.getElementById('obj-out');
    if (!why) {
      out.innerHTML = `<p class="err small" style="margin-top:.7rem">An objection must give a
        reason — the chain refuses one without it (<code>land error 23</code>). The reason is the
        whole point: it is what a court reads afterwards.</p>`;
      return;
    }
    out.innerHTML = action('Your objection',
      'Run this wherever your key is. It halts the transfer in the block it lands in.',
      { sub: 'object', args: [t.id, why], from: who || 'your-key' });
  };
}

// --- 4a. Transfers under way ----------------------------------------------

async function screenPending() {
  view.innerHTML = `<h2>Transfers under way</h2>
    <p class="muted small" style="margin:0 0 .8rem">Every transfer that has not completed and has
      not been objected to. This list is what makes the challenge window mean anything: nobody can
      object to a transfer they cannot see.</p>
    <div id="list"><p class="spin">Reading…</p></div>`;
  const el = document.getElementById('list');
  let data, prm;
  try {
    [data, prm] = await Promise.all([landQuery('PendingTransfers'), params()]);
  } catch (e) {
    el.innerHTML = `<p class="err">Could not read the pending transfers. <code>${esc(e.message)}</code><br>
      This list being empty and this list being unavailable are different facts, so nothing is
      shown rather than an empty list.</p>`;
    return;
  }
  const ts = data.transfers || [];
  if (!ts.length) {
    el.innerHTML = `<div class="card"><p class="muted small" style="margin:0">No transfer is under
      way. The register answered — this is an empty list, not a failed call.</p></div>`;
    return;
  }
  const cards = await Promise.all(ts.map(async (t) => {
    const pr = await landOrNull('Parcel', t.parcel_id);
    const p = pr?.parcel;
    const st = stage(t, p, prm);
    return `<a class="card hit verdict verdict--${st.tone}" href="#/transfer/${t.id}">
      <div class="row">
        <span class="ref">${p ? esc(p.cadastral_ref) : `parcel ${esc(t.parcel_id)}`}</span>
        <span class="pill p-${st.tone === 'ok' ? 'ok' : st.tone === 'bad' ? 'bad' : 'warn'}">${
          st.key === 'window' ? 'open to objection'
          : st.key === 'complete' ? 'ready to complete'
          : st.key === 'attest' ? `${st.got} of ${st.quorum} attested`
          : 'awaiting validation'}</span>
      </div>
      <p class="small" style="margin:.4rem 0 0">${esc(st.waiting)}</p>
      <p class="muted small" style="margin:.3rem 0 0">
        ${esc(await personLabel(t.from))} → ${esc(await personLabel(t.to))}${
          t.price ? ` · declared price ${esc(t.price)}` : ''} · transfer ${esc(t.id)}</p>
    </a>`;
  }));
  el.innerHTML = cards.join('');
}

// --- 4b. Register a parcel ------------------------------------------------

async function screenRegister() {
  const offices = await authorities();
  view.innerHTML = `
    <h2>Register a parcel</h2>
    <p class="muted small" style="margin:0 0 .8rem">First registration: opening a title over ground
      that has none. The chain refuses it if the survey is already titled, or if the cadastral
      reference is already used — that refusal is what makes a parcel impossible to own twice.
      Check both before you compose anything.</p>

    <div class="card">
      <h3>The survey</h3>
      <p class="muted small" style="margin:.1rem 0 .3rem">The survey document never leaves this
        machine. Its hash is computed here, in the browser, and only the hash goes on the chain —
        the document is megabytes and usually carries somebody's personal details.</p>
      <label for="file">Choose the survey file (GeoJSON, or the scanned cadastral document)</label>
      <input type="file" id="file">
      <label for="geom">…or paste the survey hash if you already have it</label>
      <input id="geom" placeholder="64 hexadecimal characters">
      <p id="geom-note" class="muted small" style="margin:.5rem 0 0"></p>
    </div>

    <div class="card">
      <h3>The title</h3>
      <label for="ref">Cadastral reference — the number on the paper file</label>
      <input id="ref" placeholder="NR/KAN/2019/00412">
      <p id="ref-note" class="muted small" style="margin:.4rem 0 0"></p>
      <label for="holder">The holder's account reference</label>
      <input id="holder" placeholder="yml1…">
      <p class="muted small" style="margin:.3rem 0 0">Exactly one holder. Co-ownership is expressed
        by that account being a group account — a legal arrangement between people — not by the
        register holding several owners and having to rank them.</p>
      <label for="office">Registering office</label>
      <select id="office">${offices.map((o) => `<option value="${esc(o.address)}"${
        o.active ? '' : ' disabled'}>${esc(o.name)} — ${esc(o.jurisdiction)}${
        o.active ? '' : ' (not active)'}</option>`).join('')}</select>
      <p style="margin:.9rem 0 0"><button class="primary" id="reg-go">Check the register, then compose</button></p>
    </div>
    <div id="reg-out"></div>`;

  const geomEl = document.getElementById('geom');
  const geomNote = document.getElementById('geom-note');

  document.getElementById('file').onchange = async (e) => {
    const f = e.target.files?.[0];
    if (!f) return;
    geomNote.textContent = `Hashing ${f.name}…`;
    try {
      const buf = await f.arrayBuffer();
      const digest = await crypto.subtle.digest('SHA-256', buf);
      const hex = [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, '0')).join('');
      geomEl.value = hex;
      geomNote.innerHTML = `SHA-256 of <strong>${esc(f.name)}</strong>, computed here.
        The file was not uploaded anywhere.`;
      await checkGeometry(hex);
    } catch (err) {
      geomNote.innerHTML = `<span class="err">Could not hash the file — ${esc(err.message)}.
        Hashing needs a secure context (https, or localhost).</span>`;
    }
  };
  geomEl.onchange = () => checkGeometry(geomEl.value.trim());
  document.getElementById('ref').onchange = (e) => checkRef(e.target.value.trim());
  document.getElementById('reg-go').onclick = composeRegistration;
}

async function checkGeometry(hash) {
  const note = document.getElementById('geom-note');
  if (!hash) return;
  try {
    const r = await parcelByGeometry(hash);
    if (r?.parcel) {
      note.innerHTML = `<span class="pill p-bad">already titled</span>
        This ground is parcel ${esc(r.parcel.id)}, <a href="#/parcel/${r.parcel.id}">${esc(r.parcel.cadastral_ref)}</a>,
        held by ${esc(await personLabel(r.parcel.holder))}. The chain will refuse a second title over it.`;
    } else {
      note.innerHTML = `<span class="pill p-ok">not titled</span> No title exists over this survey.
        Note the limit of that answer: the chain compares hashes, so a <em>different</em> survey
        describing overlapping ground looks like different land to it. Only a surveyor can see an
        overlap, and the way to raise one is an objection, not a query.`;
    }
  } catch (e) {
    note.innerHTML = `<span class="err">Could not check the survey hash — ${esc(e.message)}.
      Do not treat that as "not titled".</span>`;
  }
}

async function checkRef(ref) {
  const note = document.getElementById('ref-note');
  if (!ref) { note.textContent = ''; return; }
  try {
    const r = await parcelByRef(ref);
    note.innerHTML = r?.parcel
      ? `<span class="pill p-bad">reference in use</span> Already parcel ${esc(r.parcel.id)} —
         <a href="#/parcel/${r.parcel.id}">open it</a>. Two records claiming to be the same paper
         file makes reconciliation guesswork, so the chain refuses it.`
      : `<span class="pill p-ok">free</span> No title uses this reference.`;
  } catch (e) {
    note.innerHTML = `<span class="err">Could not check the reference — ${esc(e.message)}.</span>`;
  }
}

function composeRegistration() {
  const geom = document.getElementById('geom').value.trim();
  const ref = document.getElementById('ref').value.trim();
  const holder = document.getElementById('holder').value.trim();
  const office = document.getElementById('office').value;
  const out = document.getElementById('reg-out');

  const missing = [];
  if (!geom) missing.push('a survey hash — choose the survey file, or paste the hash');
  if (!ref) missing.push('the cadastral reference');
  if (!holder) missing.push("the holder's account reference");
  if (missing.length) {
    out.innerHTML = `<p class="err">The chain would refuse this. Still needed:
      ${missing.map(esc).join('; ')}.</p>`;
    return;
  }
  out.innerHTML = action(
    `Register ${ref}`,
    groupPreamble(officeLabel(office)) + ' Nothing is sent from this page — the command below is what the office runs.',
    // The three positionals in the order autocli declares them: survey hash,
    // cadastral reference, holder. Getting that order wrong would register the
    // reference as the survey and the survey as the reference, and the chain
    // would accept it — both are free-form strings to the keeper.
    { sub: 'register-parcel', args: [geom, ref, holder], from: 'your-office-key' },
    true);
}

// --- 5. Fractionalisation --------------------------------------------------

async function screenAuthorisations() {
  view.innerHTML = `
    <h2>Fractionalisation authorisations</h2>
    <p class="muted small" style="margin:0 0 .8rem">Selling a share of what land earns — a lease, a
      revenue share, an exploitation right — is legitimate financing, and often the only credit
      available to somebody whose one asset is land they cannot borrow against. What must not
      happen is fractionalisation the registry cannot see. So an office authorises it, naming what
      may be sold, a ceiling, and an expiry; the title itself never leaves the register.</p>

    <div class="card">
      <h3>Check one parcel</h3>
      <p class="muted small" style="margin:.1rem 0 .3rem">This is the check to run before paying
        for a share in somebody's land. It asks the register directly and needs nothing else to be
        working.</p>
      <label for="auth-id">Parcel number</label>
      <input id="auth-id" inputmode="numeric" placeholder="7">
      <p style="margin:.7rem 0 0"><button class="primary" id="auth-go">Ask the register</button></p>
      <div id="auth-one"></div>
    </div>

    <h2>Authorisations on this chain</h2>
    <div id="auths"><p class="spin">Reading…</p></div>`;

  document.getElementById('auth-go').onclick = async () => {
    const el = document.getElementById('auth-one');
    const raw = document.getElementById('auth-id').value.trim();
    // Parcel 0 is never issued — a zero id is what an unset protobuf field
    // looks like, and x/tokenisation says "this vehicle is over land" by
    // carrying one. Treating 0 as a parcel would make every warehouse receipt
    // on the chain look like a vehicle over the first field ever registered.
    if (!/^[1-9][0-9]*$/.test(raw)) {
      el.innerHTML = `<p class="err small" style="margin-top:.7rem">Parcel numbers start at 1.</p>`;
      return;
    }
    el.innerHTML = '<p class="spin">Asking…</p>';
    try {
      el.innerHTML = await authorisationCard(raw, await fractionalisationAuthority(raw));
    } catch (e) {
      el.innerHTML = `<p class="err small" style="margin-top:.7rem">The register did not answer.
        <code>${esc(e.message)}</code></p>`;
    }
  };

  const el = document.getElementById('auths');
  let rows;
  try {
    rows = (await txsOf(MESSAGES.fractionalise, 50)).filter((r) => r.code === 0);
  } catch (e) {
    el.innerHTML = `<div class="card">
      <h3>The list cannot be assembled here</h3>
      <p class="err small"><code>${esc(e.message)}</code></p>
      <p class="muted small">${e.status === 401
        ? "A 401 is the operator's basic-auth gate on transaction search — a deliberate configuration, not a fault."
        : 'The transaction log did not answer.'}</p>
      ${discoveryNote()}</div>`;
    return;
  }
  if (!rows.length) {
    el.innerHTML = `<div class="card">
      <p class="muted small" style="margin:0">No office has authorised fractionalisation over any
        parcel. The transaction log answered — this is genuinely none, not a failed call.</p>
      ${discoveryNote()}</div>`;
    return;
  }

  // The log is used to learn *which* parcels to ask about and for nothing else.
  // One entry per parcel, because the register holds one authorisation per
  // parcel: granting again replaces the terms rather than accumulating them.
  const ids = [...new Set(rows.map((r) => String(r.msg.parcel_id)))].filter((id) => id !== '0');

  const cards = await Promise.all(ids.map(async (id) => {
    try {
      return await authorisationCard(id, await fractionalisationAuthority(id));
    } catch (e) {
      return `<div class="card"><strong>parcel ${esc(id)}</strong>
        <p class="err small" style="margin:.3rem 0 0">The register did not answer for this parcel.
          <code>${esc(e.message)}</code></p></div>`;
    }
  }));
  el.innerHTML = cards.join('') + discoveryNote();
}

/**
 * One authorisation, as the register holds it.
 *
 * `live` comes from the keeper and is not recomputed here. It folds in three
 * things a page cannot reliably know: the chain's block time rather than the
 * reader's clock, whether the office has withdrawn the permission, and whether
 * the parcel has since acquired a restriction forbidding fractionalisation.
 * This page used to work the first of those out from `Date.now()`, which meant
 * a device with a wrong clock — or a chain whose blocks had stopped — would
 * show as live a permission x/tokenisation would refuse to issue against. The
 * chain and the console must not disagree about what live means.
 */
async function authorisationCard(parcelId, res) {
  const pr = await landOrNull('Parcel', parcelId);
  const p = pr?.parcel;
  const title = p
    ? `<a href="#/parcel/${p.id}">${esc(p.cadastral_ref)}</a>`
    : `parcel ${esc(parcelId)}`;

  if (!res) {
    return `<div class="card">
      <div class="row"><strong>${title}</strong><span class="pill p-mute">none</span></div>
      <p class="muted small" style="margin:.4rem 0 0">The register holds no fractionalisation
        authorisation for this parcel. x/tokenisation refuses to open a vehicle without one, so
        anything being sold as a share of this land is not something the chain will mint.</p>
    </div>`;
  }

  const a = res.authorisation;
  const live = Boolean(res.live);
  const expires = Number(a.expires_at);
  const why = live
    ? `Stands until ${esc(dateTimeOf(expires))}.`
    : a.withdrawn
      ? 'Withdrawn by the office. New issuance stops; existing holders are not expropriated by the registry — that would be a taking, and it belongs to a court.'
      : `Not live. It has run out — it was granted until ${esc(dateTimeOf(expires))} — or the
         parcel now carries a restriction forbidding fractionalisation. The register folds both
         into the one answer, because both have the same effect: nothing more can be issued.`;

  return `<div class="card">
    <div class="row">
      <strong>${title}</strong>
      <span class="pill ${live ? 'p-ok' : 'p-mute'}">${live ? 'live' : 'not live'}</span>
    </div>
    <dl>
      <dt>What may be sold</dt><dd>${esc(a.right || 'not stated')} — never the title itself</dd>
      <dt>Ceiling</dt><dd>${(Number(a.max_share_bps) / 100).toFixed(2)}% of that right</dd>
      <dt>Authorised by</dt><dd>${esc(officeLabel(a.granted_by))}</dd>
      <dt>Vehicle opened</dt><dd>${p && p.vehicle_id !== '0'
        ? `yes — vehicle ${esc(p.vehicle_id)}`
        : 'none yet'}</dd>
    </dl>
    <p class="muted small" style="margin:.5rem 0 0">${why}</p>
  </div>`;
}

/**
 * Named rather than hidden, and much smaller than it used to be.
 *
 * Everything shown above is state the register holds and serves. What the
 * register does not offer is a list — there is no "every authorisation" query,
 * for the same reason there is no "every parcel" one — so the transaction log
 * supplies the parcel numbers to ask about and nothing more. A permission
 * granted before the log's retention window is missing from this list and still
 * answers correctly when asked for by parcel number, which is what the box at
 * the top is for.
 */
const discoveryNote = () => `<div class="card" style="border-inline-start:3px solid var(--warn)">
  <h3>What this list can and cannot show</h3>
  <p class="muted small" style="margin:.1rem 0">Every fact above — the right, the ceiling, the
    expiry, and whether the permission is live — is read from the register. Only the question of
    <em>which parcels to ask about</em> comes from the chain's transaction log, because x/land
    publishes no list of authorisations any more than it publishes a list of owners.</p>
  <p class="muted small" style="margin:.4rem 0 0">So this list can be short where the log has been
    pruned or is gated, and it is never wrong about a parcel it does show. To check one parcel,
    ask for it by number above — that path does not touch the log at all.</p>
</div>`;

// --- 6. Offices ------------------------------------------------------------

async function screenOffices() {
  view.innerHTML = `<h2>Registry offices</h2>
    <p class="muted small" style="margin:0 0 .8rem">Admitted by the chain's governance, never by
      each other — an office that could admit offices could manufacture the independent attestors
      a transfer's quorum depends on. Every one of them is a group account, so its decisions
      already need several registrars to agree before the office signs anything.</p>
    <div id="offs"><p class="spin">Reading…</p></div>`;
  const el = document.getElementById('offs');
  let list, prm;
  try { [list, prm] = await Promise.all([authorities(), params()]); }
  catch (e) {
    el.innerHTML = `<p class="err">Could not read the offices. <code>${esc(e.message)}</code></p>`;
    return;
  }
  const active = list.filter((o) => o.active).length;
  const quorum = Number(prm.attestation_quorum);
  el.innerHTML = `
    ${active <= quorum ? `<p class="err">The register has ${active} active
      ${active === 1 ? 'office' : 'offices'} and a transfer needs ${quorum} independent
      attestations. Since the parcel's own office may not attest, no transfer can ever reach
      quorum. This is a governance problem — admit more offices, or lower the quorum knowing what
      that costs.</p>` : ''}
    ${list.length ? '' : `<div class="card">
      <p class="lede">No office has been admitted to the register.</p>
      <p class="muted small">The register answered — this is an empty list, not a failed call.
        Until governance admits an office nothing can be registered at all: a first registration
        is an office's act, and a transfer needs ${esc(String(quorum))} more offices than that to
        attest it. Offices are admitted by the chain's governance and never by each other, which
        is why this list cannot be filled in from here.</p>
      <p class="small"><a href="/governance/">Governance is where an office is admitted →</a></p>
    </div>`}
    <div class="grid">${list.map((o) => `
      <div class="card">
        <strong style="display:block">${esc(o.name || 'unnamed office')}</strong>
        <p class="muted small" style="margin:.3rem 0 .45rem">${esc(o.jurisdiction || 'no jurisdiction recorded')}</p>
        <span class="pill ${o.active ? 'p-ok' : 'p-mute'}">${o.active ? 'active' : 'not active'}</span>
        <details><summary>Account reference</summary>
          ${idHtml(o.address, "the office's account")}</details>
      </div>`).join('')}
    </div>
    <div class="card" style="margin-top:.8rem">
      <h3>The rules currently in force</h3>
      <dl>
        <dt>Independent attestations</dt><dd>${esc(prm.attestation_quorum)} offices must attest a
          transfer before it can complete</dd>
        <dt>Challenge window</dt><dd>${esc(span(Number(prm.challenge_window)))} from the moment
          quorum is reached, during which anybody may object</dd>
        <dt>Own office may attest</dt><dd>${prm.same_authority_attestation
          ? '<span class="pill p-bad">yes — the independence rule is switched off</span>'
          : 'no — an attestor from the parcel\'s own office is not independent'}</dd>
      </dl>
      <p class="muted small" style="margin:.6rem 0 0">These belong to governance. A chain where an
        official can lower the quorum is a chain with no quorum.</p>
    </div>`;
}

// --- 7. What can be done, and by whom -------------------------------------

/**
 * All twelve of x/land's messages, and what this console can do about each.
 *
 * This screen was written because of what the live chain currently holds: zero
 * registry offices and zero parcels. Every other screen can honestly say only
 * "none", and a register that answers "none" four times running looks broken
 * rather than empty. What is true and useful in that state is the shape of the
 * thing — who may do what, in what order, and what is blocking the first step.
 *
 * It is also the page's own audit. The console used to compose two of these
 * twelve as structured messages and hand the other ten over as strings; listing
 * them here means a message that quietly loses its affordance is visible rather
 * than merely absent.
 */
const HOW = {
  sign: { word: 'you can sign this here', cls: 'p-ok' },
  propose: { word: 'you can put this to an office', cls: 'p-brass' },
  command: { word: 'composed, run at the office', cls: 'p-warn' },
  governance: { word: 'a governance vote', cls: 'p-mute' },
};

/** Where each action is actually reachable from, or null when it is not a
 *  screen of its own — the transfer ones live on a transfer. */
const WHERE = {
  RegisterAuthority: ['#/offices', 'Registry offices'],
  RegisterParcel: ['#/register', 'Register a parcel'],
  AuthoriseFractionalisation: ['#/authorisations', 'Fractionalisation'],
  ProposeTransfer: [null, 'a parcel’s record'],
  ValidateTransfer: [null, 'a transfer'],
  AttestTransfer: [null, 'a transfer'],
  Object: [null, 'a transfer'],
  CompleteTransfer: [null, 'a transfer'],
  RecordEncumbrance: [null, 'a parcel’s record'],
  FreezeParcel: [null, 'a parcel’s record'],
  AttachDeed: [null, 'a parcel’s record'],
  SetRestriction: [null, 'a parcel’s record'],
};

async function screenActions() {
  view.innerHTML = `<h2>What can be done to land here, and by whom</h2>
    <p class="muted small" style="margin:0 0 .8rem">Twelve things, and the reason each one is
      shaped the way it is. Almost every protection in this register is a rule about <em>who
      may not</em> do something: an office cannot start the sale of your land, an office
      cannot attest its own transfer, an office cannot admit another office, and no single
      person inside an office can act for it.</p>
    <div id="blocking"></div>
    <div class="acts">${ACTION_NAMES.map((name) => {
      const a = ACTIONS[name];
      const how = HOW[a.how];
      const [href, place] = WHERE[name];
      return `<div class="card act">
        <div class="row" style="align-items:flex-start">
          <h3 style="margin:0">${esc(a.title)}</h3>
          <span class="pill ${how.cls} act__how">${esc(how.word)}</span>
        </div>
        <p class="small" style="margin:0"><strong>Who may:</strong> ${esc(a.who)}.</p>
        <p class="muted small" style="margin:0">${esc(a.why)}</p>
        <p class="muted small" style="margin:0"><strong>Afterwards:</strong> ${
          a.undo === ''
            ? `<span style="color:var(--bad)">nothing — ${esc(a.terminal)}</span>`
            : esc(a.undo)}</p>
        <p class="small" style="margin:auto 0 0">${href
          ? `<a href="${href}">${esc(place)} →</a>`
          : `<span class="muted">From ${esc(place)}.</span>`}</p>
      </div>`;
    }).join('')}</div>

    <h2>The four steps of a transfer</h2>
    <div class="card">
      <p class="muted small" style="margin:0 0 .5rem">No party controls two of them, and that
        is the whole design. The numbers below are read from the chain, not written here.</p>
      <div id="rules"><p class="spin">Reading the rules in force…</p></div>
    </div>`;

  await blockingNotice(document.getElementById('blocking'));

  const el = document.getElementById('rules');
  try {
    const prm = await params();
    const offices = await authorities();
    const active = offices.filter((o) => o.active).length;
    el.innerHTML = `<ol class="steps">
      ${[
        ['The holder consents', 'Signed by whoever holds the parcel. An office cannot start '
          + 'the sale of somebody’s land.'],
        ['The office in charge validates', 'The office whose jurisdiction the parcel falls in '
          + 'checks the seller against the paper file it holds. Exactly one validation is '
          + 'accepted, and it does not count toward the quorum below.'],
        [`${esc(prm.attestation_quorum)} independent offices attest`,
          `Offices <em>other</em> than the one holding the parcel. An attestor from the same `
          + `office is not independent, and allowing it would collapse a quorum of many `
          + `offices back into a single bribe. The register currently has ${
            esc(String(active))} active ${active === 1 ? 'office' : 'offices'}.`],
        [`Anybody may object for ${esc(span(Number(prm.challenge_window)))}`,
          'The clock runs from the moment quorum is reached, not from the proposal, so the '
          + 'public clock only starts once the transfer is real. One objection stops '
          + 'everything and marks the parcel disputed.'],
        ['Anyone completes it', 'Mechanical. If only an official could finalise a transfer, an '
          + 'official could refuse to — and a refusal that costs a seller their sale is '
          + 'leverage worth paying to remove.'],
      ].map(([t, d], i) => `<li class="step">
        <span class="step__mark">${i + 1}</span>
        <div class="step__body"><div class="step__title">${t}</div>
          <p class="muted small" style="margin:.2rem 0">${d}</p></div></li>`).join('')}
    </ol>
    <p class="muted small" style="margin:.6rem 0 0">To move this land against a holder’s
      wishes somebody would have to buy the holder’s consent, the office holding the file,
      and ${esc(prm.attestation_quorum)} offices elsewhere with no relationship to them — and
      then survive ${esc(span(Number(prm.challenge_window)))} of public objection.</p>`;
  } catch (e) {
    el.innerHTML = `<p class="err small">The rules in force could not be read, so the numbers
      above are not shown rather than guessed. <code>${esc(e.message)}</code></p>`;
  }
}

/**
 * What is stopping the register from being used at all, drawn before anything
 * else on the two screens that would otherwise show an empty list.
 *
 * The live chain has no offices and no parcels, and the honest thing to say
 * about that is not "no results". It is that first registration is an office
 * act, that offices are admitted only by governance, and that until one is
 * admitted nothing can be registered by anybody — which tells a visitor what
 * would have to happen next, rather than leaving them to conclude the register
 * is broken.
 *
 * It draws nothing at all once the register is in use. An empty state that
 * lingers as a permanent notice is a banner people stop reading.
 */
async function blockingNotice(el) {
  if (!el) return;
  let offices;
  let prm;
  try { [offices, prm] = await Promise.all([authorities(), params()]); }
  catch (e) {
    el.innerHTML = `<p class="err">The register’s list of offices could not be read, so this
      page cannot tell you whether the register is empty or unreachable.
      <code>${esc(e.message)}</code></p>`;
    return;
  }
  const active = offices.filter((o) => o.active);
  const quorum = Number(prm.attestation_quorum);

  if (!offices.length) {
    el.innerHTML = `<div class="card card--warn">
      <div class="row" style="align-items:flex-start">
        <p class="lede" style="margin:0">No registry office has been admitted, so no land can
          be registered here yet</p>
        <span class="pill p-warn">register empty</span>
      </div>
      <p class="small">The register answered — this is an empty list, not a failed call, and
        not a page that has not loaded. There are no parcels because there is nobody who
        could have registered one: a first registration is an office’s act, and offices are
        admitted by the chain’s governance and never by each other.</p>
      <p class="small">That last rule is the reason this cannot be fixed from this page. An
        office that could admit another office would be able to manufacture the independent
        attestors every transfer’s quorum depends on, so buying one office would buy the
        whole mechanism.</p>
      <p class="small"><strong>What would have to happen, in order:</strong></p>
      <ol class="small" style="margin:.2rem 0 .6rem;padding-inline-start:1.2rem">
        <li>Governance admits at least one office — a group account, so that its own
          decisions already need several registrars to agree.</li>
        <li>That office registers a parcel, which is when this register first holds
          anything.</li>
        <li>Governance admits ${esc(String(quorum))} more offices before any transfer can
          complete, because the parcel’s own office may not attest its own transfer.</li>
      </ol>
      <p class="small"><a href="#/offices">The admission proposal is composed on Registry
        offices →</a></p>
    </div>`;
    return;
  }

  if (active.length <= quorum) {
    el.innerHTML = `<div class="card card--warn">
      <div class="row" style="align-items:flex-start">
        <p class="lede" style="margin:0">No transfer on this register can complete yet</p>
        <span class="pill p-warn">${esc(String(active.length))} of ${esc(String(quorum + 1))} offices</span>
      </div>
      <p class="small">A transfer needs ${esc(String(quorum))} independent attestations, and
        the parcel’s own office may not attest — so ${esc(String(quorum + 1))} active offices
        is the minimum, and the register has ${esc(String(active.length))}. Land can be
        registered and transfers can be proposed; none of them can finish. That is a
        governance matter, not something an office can fix by signing.</p>
    </div>`;
    return;
  }
  el.innerHTML = '';
}

// ---------------------------------------------------------------------------
// Routing.
// ---------------------------------------------------------------------------

async function route() {
  const hash = location.hash || '#/';
  renderNav(hash.split('?')[0]);
  setBanner(null);
  window.scrollTo(0, 0);
  try {
    await loadOffices();
  } catch (e) {
    // Offices failing is survivable — every screen degrades to account
    // references — so it is a banner and not a dead page.
    setBanner(new CallFailed('the register’s list of offices', e.status ?? '?',
      'office names cannot be shown, so accounts appear in their place'));
  }
  const [path, qs] = hash.split('?');
  const q = new URLSearchParams(qs || '').get('q');
  try {
    if (path.startsWith('#/parcel/')) return await screenParcel(path.slice(9));
    if (path.startsWith('#/transfer/')) return await screenTransfer(path.slice(11));
    if (path === '#/pending') return await screenPending();
    if (path === '#/authorisations') return await screenAuthorisations();
    if (path === '#/register') return await screenRegister();
    if (path === '#/offices') return await screenOffices();
    if (path === '#/actions') return await screenActions();
    return await screenSearch(q);
  } catch (e) {
    view.innerHTML = `<p class="err">This screen could not be built.
      <code>${esc(e.message)}</code><br>
      That is the exact call that failed — nothing below it was attempted, so treat the rest of
      this page as unknown rather than as empty.</p>`;
  }
}

window.addEventListener('hashchange', route);
renderSigner();

// The clock is the honest signal that the page is live rather than a cached
// screenshot of a register — which matters when the answer is "no objection".
async function tick() {
  const el = document.getElementById('clock');
  try {
    const h = await head();
    // A node replaying history answers every query correctly for a height that
    // is hours old. A register that says "no objection" as of four hours ago is
    // worse than one that says nothing, so this is surfaced rather than hidden.
    el.innerHTML = h.catchingUp
      ? `<span class="pill p-warn">catching up — answers are ${
          esc(span((Date.now() - h.at.getTime()) / 1000))} behind</span>`
      : `register read at block ${esc(h.height.toLocaleString())} · ${
          esc(new Date().toLocaleTimeString())}`;
  } catch {
    el.innerHTML = '<span class="pill p-bad">not reading the chain</span>';
  }
}

route();
tick();
setInterval(tick, 15000);
