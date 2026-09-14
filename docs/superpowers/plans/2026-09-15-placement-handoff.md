# Placement Handoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a newly created account reach "usable" — the site stops offering a seed-phrase wallet to consumers, a foundation administrator can finish the form, the institution field names a real participant, and there is a screen on the institution's side that consumes the request.

**Architecture:** No chain change. Every fault is an interface fault against rules `x/alias` and `x/paymsg` already enforce. Three surfaces move: the site menu (static HTML), the wallet's placement step (React + the `@yamale/chain` SDK), and a new single-page console `clients/institution/` in the house style — no build step, no key, prints the command the institution's own group signs.

**Tech Stack:** Static HTML for the site and the console; React + TypeScript + Vite for the wallet; `@yamale/chain` (`clients/sdk`) for chain reads; `node --test` for all tests; bash for the devnet governance scripts.

## Global Constraints

- **No chain change.** Nothing in this plan is consensus-breaking. If a task seems to need a new message, stop and re-read the spec.
- **The console holds no key and composes no transaction.** It prints commands. Same rule as `clients/syndicate` and `clients/livestock`.
- **Read over REST for alias and paymsg.** Measured 2026-09-15: `GET /api/rest/yamale/blockchain/paymsg/v1/approved_participant` and `GET /api/rest/yamale/blockchain/alias/v1/jurisdiction/{address}` both return `200`. Do not use `abci_query` for these; do not "fix" this without re-measuring.
- **Five languages.** Any new copy key must be added to all five catalogues in `clients/sdk/src/catalogues.ts`: `en` (line ~11), `fr` (~1058), `pt` (~2071), `sw` (~3083), `ar` (~4097). A key present in one and missing in another is a runtime gap.
- **An absence is not a failure.** `{"code":5,"message":"this account has no recorded jurisdiction"}` means *unplaced*, which is the normal state of the account this whole flow exists to serve. Never render it as an error.
- **Foundation administrators have no jurisdiction record.** `Keeper.CountryOf` derives `ZZ` from the role grant. `MsgSetJurisdiction` refuses `ZZ`. Never write a task that records a country for them.
- **Commit messages:** no `Co-Authored-By` trailer, no emoji. Subject line states the fact, not the change type.
- **Do not run `git commit` with `-i`,** and do not push. Commit locally only.
- Run `node --test` from the package directory, not the repo root.

---

## File Structure

| file | responsibility |
|---|---|
| `clients/site/index.html` | The menu. Modify the three `menu__group` blocks at lines 51–112. |
| `clients/sdk/src/placement.ts` | Placement verdicts and the request document. Gains the administrator path. |
| `clients/sdk/src/placement.test.ts` | Tests for the above. |
| `clients/sdk/src/client.ts` | `ChainClient`. Gains `approvedParticipants()`. |
| `clients/sdk/src/catalogues.ts` | Copy, five languages. |
| `clients/wallet/src/pages/Create.tsx` | The placement step: the fork and the dropdown. |
| `clients/institution/placement.js` | **New.** Pure functions: parse a pasted request, judge it against chain facts. No fetching. |
| `clients/institution/placement.test.js` | **New.** Tests for the above. |
| `clients/institution/chain.js` | **New.** REST reads. Thin. |
| `clients/institution/index.html` | **New.** The three screens. |
| `clients/institution/package.json`, `serve.mjs` | **New.** Copied from `clients/syndicate`, names changed. |
| `deploy/stage-site.sh:57` | The console list. Add `institution`. |
| `scripts/devnet/admit-participant.sh` | **New.** The two governance proposals of B3. |

---

### Task 1: The menu says which audience each surface is for

**Files:**
- Modify: `clients/site/index.html:51-112`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing other tasks read. Task 5 adds one more link to the same region.

- [ ] **Step 1: Read the current three groups**

Run: `sed -n '51,112p' clients/site/index.html`

Confirm you see `<h4>For anyone</h4>`, `<h4>For institutions</h4>`, `<h4>For supervisors and operators</h4>`, and that `/wallet/` and `/wallet/faucet` are inside the first.

- [ ] **Step 2: Remove the wallet link from "For anyone"**

Delete exactly these three lines from the *For anyone* group (leave the Faucet link where it is):

```html
            <a class="app-link" href="/wallet/">
              <strong>Wallet</strong><span>Create or load an account, see balances</span>
            </a>
```

- [ ] **Step 3: Add it to "For institutions"**

Insert immediately after the `<h4>For institutions</h4>` line:

```html
            <a class="app-link" href="/wallet/">
              <strong>Wallet</strong><span>Hold your own key and sign for your institution — lose it and the account is gone</span>
            </a>
```

- [ ] **Step 4: Add it to "For supervisors and operators"**

Insert immediately after the `<h4>For supervisors and operators</h4>` line:

```html
            <a class="app-link" href="/wallet/">
              <strong>Wallet</strong><span>Hold your own key and sign as the authority you operate</span>
            </a>
```

The same href appears twice on purpose: both audiences hold their own keys, and a link is not a resource. The two descriptions differ because the two audiences do.

- [ ] **Step 5: Verify the menu guard still passes**

Run:

```bash
bash -n deploy/stage-site.sh && grep -c 'href="/wallet/"' clients/site/index.html
```

Expected: no syntax error, and the count is `2`.

- [ ] **Step 6: Commit**

```bash
git add clients/site/index.html
git commit -m "The menu offered a seed-phrase wallet to anyone who arrived"
```

---

### Task 2: A foundation administrator can finish the form

**Files:**
- Modify: `clients/sdk/src/placement.ts` (append a new exported function at the end)
- Modify: `clients/sdk/src/placement.test.ts`
- Modify: `clients/sdk/src/catalogues.ts` (five catalogues)

**Interfaces:**
- Consumes: `FOUNDATION_COUNTRY` from `./alias.ts`, already imported at `placement.ts:1`.
- Produces: `foundationPlacement(args: { address: string; chainId?: string; bin?: string }): { address: string; marker: string; document: string; command: string }` — Task 3 renders it.

- [ ] **Step 1: Write the failing test**

Append to `clients/sdk/src/placement.test.ts`:

```typescript
test('a foundation administrator is told the role is the placement', () => {
  const r = foundationPlacement({ address: 'yml1abc', chainId: 'yamale-devnet-2' });

  // ZZ is DERIVED from the grant and never recorded. Keeper.CountryOf falls
  // through to hasFoundationGrant and answers ZZ; MsgSetJurisdiction refuses ZZ
  // outright so the marker cannot be handed to an ordinary account.
  assert.equal(r.marker, 'ZZ');
  assert.ok(!/set-jurisdiction/.test(r.command),
    'a participant command here would be refused by the chain and is the wrong ask');
  assert.ok(/grant-role/.test(r.command));
  assert.ok(/governance/i.test(r.document));
});

test('the administrator document does not claim the account gets no user ID', () => {
  // The ordinary placement copy says exactly that, and for this account it is
  // false: once the grant exists, CountryOf answers ZZ and an identifier issues
  // with a ZZ prefix.
  const r = foundationPlacement({ address: 'yml1abc' });
  assert.ok(!/no user ID/i.test(r.document));
  assert.ok(/ZZ/.test(r.document));
});
```

Add `foundationPlacement` to the import list at the top of that file.

- [ ] **Step 2: Run it and watch it fail**

Run: `cd clients/sdk && npx vitest run src/placement.test.ts`
Expected: FAIL — `foundationPlacement is not exported` (or `is not a function`).

If this repo's SDK tests run under `node --test` instead, use `node --test src/placement.test.ts`. Check `clients/sdk/package.json` `scripts.test` and use whatever it names.

- [ ] **Step 3: Implement it**

Append to `clients/sdk/src/placement.ts`:

```typescript
/**
 * What a foundation administrator needs instead of a placement request.
 *
 * Not a country, and not a participant's command. Such an account holds NO
 * jurisdiction record at all: `Keeper.CountryOf` finds none, falls through to
 * `hasFoundationGrant`, and answers `ZZ` — derived from the grant, never
 * stored. `MsgSetJurisdiction` refuses `ZZ` outright, precisely so the marker
 * cannot be handed to an ordinary account.
 *
 * So the honest output is the governance path. Once the role is granted the
 * ordinary identifier claim works and issues a `ZZ`-prefixed ID; until then
 * there is nothing the holder or any institution can do, because chain-wide
 * scope may be granted by governance and by nobody else.
 */
export function foundationPlacement(args: {
  address: string;
  chainId?: string;
  bin?: string;
}): { address: string; marker: string; document: string; command: string } {
  const address = (args.address ?? '').trim();
  const bin = args.bin ?? 'blockchaind';
  const chainId = args.chainId ?? '';

  const document = [
    'FOUNDATION ADMINISTRATOR PLACEMENT',
    '',
    `account: ${address}`,
    `marker:  ${FOUNDATION_COUNTRY}`,
    chainId ? `chain:   ${chainId}` : '',
    '',
    'This account is not placed in a country and must not be. The marker',
    `${FOUNDATION_COUNTRY} is derived from holding ROLE_FOUNDATION_ADMINISTRATOR and is never`,
    'recorded; the chain refuses to record it as a jurisdiction.',
    '',
    'No institution can do this. The role is chain-wide, and a chain-wide scope',
    'may be granted by governance and by nobody else. Once the grant passes,',
    'this account can claim its identifier in the ordinary way and it will carry',
    `the ${FOUNDATION_COUNTRY} prefix.`,
  ].filter((line) => line !== '').join('\n');

  const command = [
    `${bin} tx alias grant-role ${address} ROLE_FOUNDATION_ADMINISTRATOR \\`,
    `  --from <a governance proposer>${chainId ? ` --chain-id ${chainId}` : ''} \\`,
    '  --fees 500uyml --yes',
  ].join('\n');

  return { address, marker: FOUNDATION_COUNTRY, document, command };
}
```

- [ ] **Step 4: Run the test again**

Run: the same command as Step 2.
Expected: PASS, and every pre-existing test in the file still passes.

- [ ] **Step 5: Verify the grant-role command name is real**

Run: `grep -n 'GrantRole' -A 4 x/alias/module/autocli.go | head -12`

Read the `Use:` line. If the command is not spelled `grant-role [account] [role]`, correct the string in Step 3 to match exactly, and re-run the test. **Do not skip this** — a printed command that does not exist is worse than no command.

- [ ] **Step 6: Add the copy keys, all five languages**

In `clients/sdk/src/catalogues.ts`, add these three keys to **each** of `en`, `fr`, `pt`, `sw`, `ar`, beside the existing `'placement.institution'` key in each block:

```typescript
  'placement.who': 'Who is being placed?',
  'placement.byInstitution': 'An institution onboarded me',
  'placement.byFoundation': 'This is a foundation administrator account',
```

Translate for the other four. If you are unsure of a translation, use the English string rather than inventing one — a wrong translation is harder to find than an untranslated one.

- [ ] **Step 7: Verify all five have it**

Run: `grep -c "'placement.who'" clients/sdk/src/catalogues.ts`
Expected: `5`

- [ ] **Step 8: Commit**

```bash
git add clients/sdk/src/placement.ts clients/sdk/src/placement.test.ts clients/sdk/src/catalogues.ts
git commit -m "The placement form had no path for the one account the chain exempts"
```

---

### Task 3: The wallet asks who is being placed, and offers real institutions

**Files:**
- Modify: `clients/sdk/src/client.ts` (append a method to `ChainClient`)
- Modify: `clients/wallet/src/pages/Create.tsx:193-245`

**Interfaces:**
- Consumes: `foundationPlacement` from Task 2.
- Produces: `ChainClient.approvedParticipants(): Promise<{ participant: string; code: string; name: string }[]>` — Task 6 uses it too.

- [ ] **Step 1: Write the failing test for the client method**

Create `clients/sdk/src/participants.test.ts`:

```typescript
import { describe, expect, test } from 'vitest';
import { ChainClient } from './client.ts';

const clientWith = (body: unknown) =>
  new ChainClient({
    restUrl: '/api/rest',
    rpcUrl: '/api/rpc',
    fetchImpl: async () =>
      new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json' } }),
  });

test('an empty register is an empty list, not a failure', async () => {
  // This is the LIVE state of devnet-2, measured 2026-09-15. A dropdown that
  // threw here would show a loading error on the one screen that most needs to
  // say something true.
  const c = clientWith({ approved_participant: [], pagination: { next_key: null, total: '0' } });
  expect(await c.approvedParticipants()).toEqual([]);
});

test('participants come back with the three fields a dropdown needs', async () => {
  const c = clientWith({
    approved_participant: [
      { participant: 'yml1bank', code: 'SN-CBAO', name: 'CBAO Senegal', payload_store_url: '' },
    ],
    pagination: { next_key: null, total: '1' },
  });
  expect(await c.approvedParticipants()).toEqual([
    { participant: 'yml1bank', code: 'SN-CBAO', name: 'CBAO Senegal' },
  ]);
});
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd clients/sdk && npx vitest run src/participants.test.ts`
Expected: FAIL — `c.approvedParticipants is not a function`.

- [ ] **Step 3: Implement the method**

Add to `ChainClient` in `clients/sdk/src/client.ts`, beside `userIdOf`:

```typescript
  /**
   * The institutions that may record where an account belongs.
   *
   * Read over REST rather than abci_query: measured 2026-09-15, this prefix
   * answers 200 on the deployment's proxy while several other modules' do not.
   *
   * An empty list is the honest live answer on devnet-2 and must reach the
   * caller as an empty array. A caller that cannot tell "nobody is approved"
   * from "the read failed" will render one as the other.
   */
  async approvedParticipants(): Promise<{ participant: string; code: string; name: string }[]> {
    const data = await this.get<any>(this.rest, '/yamale/blockchain/paymsg/v1/approved_participant');
    const rows = data?.approved_participant ?? [];
    return rows.map((r: any) => ({
      participant: r.participant ?? '',
      code: r.code ?? '',
      name: r.name ?? '',
    }));
  }
```

- [ ] **Step 4: Run the test again**

Run: `cd clients/sdk && npx vitest run src/participants.test.ts`
Expected: PASS, both tests.

- [ ] **Step 5: Add the fork to the wallet form**

In `clients/wallet/src/pages/Create.tsx`, add to the state block at line ~30:

```typescript
  const [placing, setPlacing] = useState<'institution' | 'foundation'>('institution');
  const [participants, setParticipants] = useState<
    { participant: string; code: string; name: string }[] | null
  >(null);
```

Add the import at the top:

```typescript
import { foundationPlacement } from '@yamale/chain';
import { client } from '../chain.ts';
```

Add an effect that loads the list once the placement step is reached:

```typescript
  useEffect(() => {
    if (!written || !address) return;
    let cancelled = false;
    void client.approvedParticipants()
      .then((rows) => !cancelled && setParticipants(rows))
      .catch(() => !cancelled && setParticipants([]));
    return () => { cancelled = true; };
  }, [written, address]);
```

Import `useEffect` alongside `useState` on line 2.

- [ ] **Step 6: Render the fork and the dropdown**

Inside the `{written && address && (` section, immediately after `<p className="notice">{t('placement.cannotSelf')}</p>`, insert:

```tsx
          <fieldset className="field">
            <legend>{t('placement.who')}</legend>
            <label>
              <input type="radio" name="placing" checked={placing === 'institution'}
                     onChange={() => setPlacing('institution')} />
              {' '}{t('placement.byInstitution')}
            </label>
            <label>
              <input type="radio" name="placing" checked={placing === 'foundation'}
                     onChange={() => setPlacing('foundation')} />
              {' '}{t('placement.byFoundation')}
            </label>
          </fieldset>
```

Wrap the existing country `<div className="placement__country">` block, the country problem notice, and the institution `<label className="field">` in `{placing === 'institution' && ( … )}`.

Replace the institution `<input>` with a select:

```tsx
            {participants === null ? (
              <p className="muted">Reading the register…</p>
            ) : participants.length === 0 ? (
              <p className="notice">
                No institution on this chain can onboard an account yet. The register of approved
                participants is empty, so there is nobody to hand this request to. That is what the
                chain says, not a fault in this page.
              </p>
            ) : (
              <select value={institution} onChange={(e) => setInstitution(e.target.value)}>
                <option value="">—</option>
                {participants.map((p) => (
                  <option key={p.participant} value={p.name}>{p.name} ({p.code})</option>
                ))}
              </select>
            )}
```

- [ ] **Step 7: Render the foundation branch**

After the `{placing === 'institution' && ( … )}` block, add:

```tsx
          {placing === 'foundation' && (() => {
            const f = foundationPlacement({ address, chainId: import.meta.env.VITE_CHAIN_ID ?? 'yamale-devnet-2' });
            return (
              <>
                <p className="notice">
                  This account is not placed in a country and must not be. The marker{' '}
                  <span className="y-mono">{f.marker}</span> is derived from holding the role and is
                  never recorded — the chain refuses to record it as a jurisdiction.
                </p>
                <pre>{f.document}</pre>
                <pre>{f.command}</pre>
              </>
            );
          })()}
```

Leave the existing `<PlacementRequest … />` inside the institution branch only.

- [ ] **Step 8: Build the wallet and check it compiles**

Run: `cd clients/wallet && npx tsc --noEmit`
Expected: no errors. If `import.meta.env.VITE_CHAIN_ID` is not typed, add it to `clients/wallet/src/vite-env.d.ts` beside `VITE_RPC_URL`.

- [ ] **Step 9: Commit**

```bash
git add clients/sdk/src/client.ts clients/sdk/src/participants.test.ts clients/wallet/src/pages/Create.tsx clients/wallet/src/vite-env.d.ts
git commit -m "The institution field invited a name the chain would never honour"
```

---

### Task 4: The console can read a pasted request and judge it

**Files:**
- Create: `clients/institution/placement.js`
- Create: `clients/institution/placement.test.js`
- Create: `clients/institution/package.json`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `parseRequest(text: string): { address, country, institution } | { problem: string }`
  - `judge(parsed, facts: { exists: boolean, jurisdiction: {country, recorded_by} | null, participants: {name, code, participant}[] }): { ok: boolean, blocking: string[], warnings: string[], action: 'record' | 'refuse' }`

  Task 5 renders both.

- [ ] **Step 1: Write the package manifest**

Create `clients/institution/package.json`:

```json
{
  "name": "@yamale/institution",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "description": "The institution console: take a placement request, check it against the chain, and print the command that records where an account belongs.",
  "scripts": {
    "test": "node --test placement.test.js",
    "serve": "node serve.mjs"
  }
}
```

- [ ] **Step 2: Write the failing tests**

Create `clients/institution/placement.test.js`:

```javascript
import test from 'node:test';
import assert from 'node:assert/strict';

import { judge, parseRequest } from './placement.js';

/** A request exactly as clients/sdk/src/placement.ts prints one. */
const REQUEST = [
  'PLACEMENT REQUEST',
  '',
  'account:      yml1holder',
  'country:      SN (Senegal)',
  'onboarded by: CBAO Senegal',
  'chain:        yamale-devnet-2',
  '',
  'I am asking the institution that onboarded me to record this account as',
  'being in SN.',
].join('\n');

const PARTICIPANTS = [{ name: 'CBAO Senegal', code: 'SN-CBAO', participant: 'yml1bank' }];

test('a request yields the three facts the institution has to act on', () => {
  assert.deepEqual(parseRequest(REQUEST), {
    address: 'yml1holder',
    country: 'SN',
    institution: 'CBAO Senegal',
  });
});

test('a paste that is not a placement request is refused, not half-read', () => {
  // Half-reading is the dangerous failure: a screen that recovered an address
  // from arbitrary text would offer to place an account nobody requested.
  const r = parseRequest('hello, please place my account in SN');
  assert.ok('problem' in r);
});

test('an unplaced account with an approved institution is ready to record', () => {
  const v = judge(parseRequest(REQUEST), {
    exists: true, jurisdiction: null, participants: PARTICIPANTS,
  });
  assert.equal(v.ok, true);
  assert.equal(v.action, 'record');
  assert.deepEqual(v.blocking, []);
});

test('an account that already has a jurisdiction is a correction, and not this institution’s to make', () => {
  const v = judge(parseRequest(REQUEST), {
    exists: true,
    jurisdiction: { country: 'NG', recorded_by: 'yml1someone' },
    participants: PARTICIPANTS,
  });
  assert.equal(v.ok, false);
  assert.equal(v.action, 'refuse');
  assert.ok(v.blocking.some((b) => /foundation administrator/i.test(b)),
    'a participant who could rewrite a recorded country could move a customer out from under an authority');
});

test('an institution that is not an approved participant is refused', () => {
  const v = judge(parseRequest(REQUEST), {
    exists: true, jurisdiction: null, participants: [],
  });
  assert.equal(v.ok, false);
  assert.ok(v.blocking.some((b) => /approved participant/i.test(b)));
});

test('a reserved country code is refused, naming the reason', () => {
  const zz = parseRequest(REQUEST.replace('SN (Senegal)', 'ZZ (reserved)').replace('being in SN', 'being in ZZ'));
  const v = judge(zz, { exists: true, jurisdiction: null, participants: PARTICIPANTS });
  assert.equal(v.ok, false);
  assert.ok(v.blocking.some((b) => /foundation/i.test(b)),
    'ZZ is the marker foundation administrators carry and must not be handed to an ordinary account');
});

test('an account the chain has never seen is an absence, reported as one', () => {
  const v = judge(parseRequest(REQUEST), {
    exists: false, jurisdiction: null, participants: PARTICIPANTS,
  });
  assert.equal(v.ok, false);
  assert.ok(v.blocking.some((b) => /never/i.test(b) || /no such account/i.test(b)));
});

test('judging a failed parse does not throw', () => {
  const v = judge({ problem: 'not a placement request' }, {
    exists: false, jurisdiction: null, participants: [],
  });
  assert.equal(v.ok, false);
});
```

- [ ] **Step 3: Run them and watch them fail**

Run: `cd clients/institution && node --test placement.test.js`
Expected: FAIL — cannot find module `./placement.js`.

- [ ] **Step 4: Implement**

Create `clients/institution/placement.js`:

```javascript
/**
 * Reading a placement request, and deciding whether this institution may act.
 *
 * Pure: nothing here fetches. The chain facts arrive as an argument, which is
 * what makes every refusal below testable without a node.
 *
 * The refusals are the product. A screen that printed a command for every paste
 * would send an institution to the chain to be refused, and the chain's refusal
 * arrives without the sentence explaining which of several rules was broken.
 */

/** ISO 3166-1 permanently reserves ZZ; x/alias uses it as the foundation marker. */
export const FOUNDATION_MARKER = 'ZZ';

const FIELD = (text, label) => {
  const m = new RegExp(`^${label}:\\s*(.+)$`, 'm').exec(text);
  return m ? m[1].trim() : '';
};

/**
 * Parse the document `clients/sdk/src/placement.ts` prints.
 *
 * The header is required. Recovering an address out of arbitrary text would let
 * this screen offer to place an account nobody requested, which is the one
 * mistake an institution cannot detect by reading the screen.
 */
export function parseRequest(text) {
  const body = String(text ?? '');
  if (!/^PLACEMENT REQUEST\s*$/m.test(body)) {
    return { problem: 'That is not a placement request. Paste the whole document the holder gave you, header included.' };
  }
  const address = FIELD(body, 'account');
  const countryField = FIELD(body, 'country');
  const institution = FIELD(body, 'onboarded by');

  const country = (countryField.split(/\s+/)[0] ?? '').toUpperCase();
  if (!address) return { problem: 'The request names no account.' };
  if (!/^[A-Z]{2}$/.test(country)) return { problem: 'The request names no country, or one that is not two letters.' };

  return { address, country, institution: institution === '(name the institution)' ? '' : institution };
}

/**
 * What this institution may do about a parsed request, given what the chain says.
 *
 * `blocking` stops the command being printed at all. `warnings` do not — they
 * are things worth saying beside a command that will work.
 */
export function judge(parsed, facts) {
  const blocking = [];
  const warnings = [];

  if (!parsed || parsed.problem) {
    return { ok: false, action: 'refuse', blocking: [parsed?.problem ?? 'Nothing to judge.'], warnings };
  }

  if (!facts.exists) {
    blocking.push('The chain has never seen this account. It holds no balance and has signed nothing, so there is nothing yet to place.');
  }

  if (parsed.country === FOUNDATION_MARKER) {
    blocking.push(
      `${FOUNDATION_MARKER} is the marker a foundation administrator carries, derived from the role and never recorded. `
      + 'The chain refuses it as a jurisdiction, so that it cannot be handed to an ordinary account.',
    );
  }

  if (facts.jurisdiction) {
    blocking.push(
      `This account is already recorded in ${facts.jurisdiction.country}, by ${facts.jurisdiction.recorded_by}. `
      + 'Changing it is a correction, and a correction is a foundation administrator’s act rather than yours: '
      + 'a participant able to rewrite a country it had already recorded could move a customer out from under an '
      + 'authority mid-investigation.',
    );
  }

  const named = (parsed.institution ?? '').trim().toLowerCase();
  if (!named) {
    warnings.push('The request names no institution. Check it is yours to act on before you sign anything.');
  } else if (!facts.participants.some((p) => p.name.trim().toLowerCase() === named)) {
    blocking.push(
      `"${parsed.institution}" is not an approved participant on this chain. Only an approved participant may `
      + 'record where an account belongs, so this recording would be refused.',
    );
  }

  return {
    ok: blocking.length === 0,
    action: blocking.length === 0 ? 'record' : 'refuse',
    blocking,
    warnings,
  };
}
```

- [ ] **Step 5: Run the tests again**

Run: `cd clients/institution && node --test placement.test.js`
Expected: all 8 PASS.

- [ ] **Step 6: Commit**

```bash
git add clients/institution/placement.js clients/institution/placement.test.js clients/institution/package.json
git commit -m "Reading a placement request, and the four reasons to refuse one"
```

---

### Task 5: The console, and the site links to it

**Files:**
- Create: `clients/institution/chain.js`, `clients/institution/index.html`, `clients/institution/serve.mjs`
- Modify: `deploy/stage-site.sh:57`
- Modify: `clients/site/index.html` (the *For institutions* group)

**Interfaces:**
- Consumes: `parseRequest`, `judge` from Task 4.
- Produces: a deployed console at `/institution/`.

- [ ] **Step 1: Copy the server, unchanged but for its name**

Run:

```bash
cp clients/syndicate/serve.mjs clients/institution/serve.mjs
```

Then edit `clients/institution/serve.mjs`: change the port to `8151`, and change the two occurrences of the word `syndicate` in its banner string and its served path to `institution`.

Run: `cd clients/institution && node -e "import('./serve.mjs')" ` is **not** a check — it starts a server. Instead run `node --check serve.mjs`. Expected: no output.

- [ ] **Step 2: Write the chain reader**

Create `clients/institution/chain.js`:

```javascript
/**
 * Reading what the institution needs, over REST.
 *
 * REST and not abci_query, which is what the other consoles use. Measured
 * 2026-09-15 against the deployment's proxy: both prefixes below answer 200,
 * while several other modules' are denied. REST returns JSON, so this console
 * transcribes no protobuf at all. Re-measure before changing this.
 *
 * READ ONLY. This console holds no key and composes no transaction.
 */

const REST = '/api/rest';
const TIMEOUT_MS = 12000;

async function get(path) {
  const res = await fetch(`${REST}${path}`, { signal: AbortSignal.timeout(TIMEOUT_MS) });
  const body = await res.json().catch(() => null);
  return { status: res.status, body };
}

/** The institutions that may record where an account belongs. */
export async function participants() {
  const { status, body } = await get('/yamale/blockchain/paymsg/v1/approved_participant');
  if (status !== 200) throw new Error(`the register did not answer (${status})`);
  return (body?.approved_participant ?? []).map((r) => ({
    participant: r.participant ?? '',
    code: r.code ?? '',
    name: r.name ?? '',
  }));
}

/**
 * The jurisdiction recorded against an account, or null.
 *
 * Null is an ANSWER and not a failure: the chain replies
 * `{"code":5,"message":"this account has no recorded jurisdiction"}` for an
 * account nobody has placed, which is the normal state of every account this
 * console exists to serve. Reporting it as an error would tell an institution
 * the chain was broken at the exact moment it was working.
 */
export async function jurisdiction(address) {
  const { status, body } = await get(`/yamale/blockchain/alias/v1/jurisdiction/${encodeURIComponent(address)}`);
  if (status === 200 && body?.jurisdiction) return body.jurisdiction;
  if (body?.code === 5) return null;
  throw new Error(body?.message ?? `the chain did not answer (${status})`);
}

/** Whether the chain has ever seen this account. */
export async function accountExists(address) {
  const { status } = await get(`/cosmos/auth/v1beta1/accounts/${encodeURIComponent(address)}`);
  return status === 200;
}

/** The identifier issued to an account, or null. */
export async function userId(address) {
  const { status, body } = await get(`/yamale/blockchain/alias/v1/alias_of/${encodeURIComponent(address)}`);
  if (status === 200 && body?.alias?.id) return body.alias.id;
  return null;
}
```

Run: `cd clients/institution && node --check chain.js`. Expected: no output.

- [ ] **Step 3: Write the page**

Create `clients/institution/index.html`. Copy the `<style>` block wholesale from `clients/syndicate/index.html` (everything between `<style>` and `</style>`) so the console matches the house look, then write the body:

```html
<title>Yamale · Institution</title>
<header class="topbar">
  <a class="back" href="/">← Yamale</a>
  <strong>Institution</strong>
</header>
<main id="view"><p class="spin">…</p></main>

<script type="module">
import { accountExists, jurisdiction, participants, userId } from './chain.js';
import { judge, parseRequest } from './placement.js';

const esc = (s) => String(s ?? '').replace(/[&<>"']/g,
  (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

const view = document.getElementById('view');

view.innerHTML = `
  <h2>Record where an account belongs</h2>
  <p class="muted">The holder cannot record it themselves. You performed the checks, so you are the
    only party that knows the answer — and the chain enforces that by accepting this recording only
    from an approved participant.</p>
  <p class="muted">This page holds no key and signs nothing. It checks the request against the chain
    and prints the command your own group signs.</p>
  <form id="take">
    <label class="field"><span>Paste the placement request</span>
      <textarea id="req" rows="12" spellcheck="false"></textarea></label>
    <p><button class="btn" type="submit">Check it</button></p>
  </form>
  <div id="verdict"></div>`;

document.getElementById('take').addEventListener('submit', async (e) => {
  e.preventDefault();
  const out = document.getElementById('verdict');
  out.innerHTML = '<p class="spin">Asking the chain…</p>';

  const parsed = parseRequest(document.getElementById('req').value);
  if (parsed.problem) {
    out.innerHTML = `<div class="badbox"><p>${esc(parsed.problem)}</p></div>`;
    return;
  }

  try {
    const [exists, j, ps] = await Promise.all([
      accountExists(parsed.address),
      jurisdiction(parsed.address).catch(() => null),
      participants(),
    ]);
    const v = judge(parsed, { exists, jurisdiction: j, participants: ps });

    out.innerHTML = `
      <div class="card">
        <p class="label">Account</p><p class="stat">${esc(parsed.address)}</p>
        <p class="muted">to be recorded in <strong>${esc(parsed.country)}</strong>,
          onboarded by ${esc(parsed.institution || '(not named)')}</p>
      </div>
      ${v.blocking.map((b) => `<div class="badbox"><p>${esc(b)}</p></div>`).join('')}
      ${v.warnings.map((w) => `<div class="warnbox"><p>${esc(w)}</p></div>`).join('')}
      ${v.ok ? `
        <div class="card card--ok">
          <p>Nothing on the chain refuses this. Your group signs:</p>
          <pre>blockchaind tx alias set-jurisdiction ${esc(parsed.address)} ${esc(parsed.country)} \\
  --from &lt;your key&gt; --chain-id yamale-devnet-2 --fees 500uyml --yes</pre>
          <p class="muted">Afterwards the account can claim its user ID, and it will carry the
            ${esc(parsed.country)} prefix.</p>
        </div>` : ''}
      <div class="card">
        <p class="label">Identifier today</p>
        <p class="stat">${esc((await userId(parsed.address)) ?? '—')}</p>
        <p class="muted">An account with no recorded country holds no identifier, and nobody can
          address a payment to it.</p>
      </div>`;
  } catch (err) {
    out.innerHTML = `<div class="err"><p><strong>The chain did not answer.</strong>
      <code>${esc(err.message)}</code></p></div>`;
  }
});
</script>
```

- [ ] **Step 4: Check it parses and renders**

Run: `cd clients/institution && node serve.mjs` in one shell. In the Browser pane, open `http://localhost:8151/institution/`, paste a request built by hand for an address with no jurisdiction, and confirm the blocking message about the empty participant register appears (devnet-2 has none approved until Task 6).

Read the browser console for errors: there must be none.

- [ ] **Step 5: Add the console to the staging list**

In `deploy/stage-site.sh` line 57, add `institution` to the list:

```bash
for console in keys demo markets oversight land livestock syndicate institution governance foundation validator; do
```

- [ ] **Step 6: Link it from the site menu**

In `clients/site/index.html`, inside the *For institutions* group, after the Wallet link added in Task 1:

```html
            <a class="app-link" href="/institution/">
              <strong>Onboarding</strong><span>Record where an account belongs, and hand back its user ID</span>
            </a>
```

- [ ] **Step 7: Verify the menu guard**

Run: `bash deploy/stage-site.sh --help 2>/dev/null || grep -n 'institution' deploy/stage-site.sh`
Expected: `institution` appears in the console list. The guard added on 2026-09-13 refuses a bundle whose menu links to a directory that is not staged; this task adds both halves together, which is what that guard exists to enforce.

- [ ] **Step 8: Commit**

```bash
git add clients/institution deploy/stage-site.sh clients/site/index.html
git commit -m "The placement request had nowhere to go"
```

---

### Task 6: Somebody on devnet-2 can actually onboard an account

**Files:**
- Create: `scripts/devnet/admit-participant.sh`

**Interfaces:**
- Consumes: nothing in code.
- Produces: a non-empty `ListApprovedParticipant` on devnet-2, which Tasks 3 and 5 render.

**This task submits governance proposals and STOPS. The votes are the user's to cast — see the `let-the-user-drive-governance` memory. Do not vote.**

- [ ] **Step 1: Find out who may admit a participant**

Run:

```bash
grep -n "ROLE_PAYMENTS_AUTHORITY" -B 3 -A 8 proto/blockchain/alias/v1/role.proto
grep -rn "ROLE_PAYMENTS_AUTHORITY" x/paymsg/keeper/*.go | head
```

Write down: which message admits a participant, and which role its signer must hold. The plan assumes `ROLE_PAYMENTS_AUTHORITY`; if the code says otherwise, follow the code and say so in the commit message.

- [ ] **Step 2: Confirm the current state**

Run:

```bash
ssh -i ~/.ssh/yamale_oracle ubuntu@92.4.151.72 \
  "/opt/yamale/bin/blockchaind query alias chain-wide-grants --home /opt/yamale/node --node http://127.0.0.1:26657 -o json"
curl -s https://pay.yamalelegal.com/api/rest/yamale/blockchain/paymsg/v1/approved_participant
```

Expected: two `ROLE_FOUNDATION_ADMINISTRATOR` grants and no payments authority; an empty participant list.

- [ ] **Step 3: Write the script**

Create `scripts/devnet/admit-participant.sh`. It must:

1. Resolve the gov module address the way `scripts/devnet/seed-deal-papers.sh` resolves the operator — by query, never hard-coded.
2. Submit proposal one: grant `ROLE_PAYMENTS_AUTHORITY` to a named account, scoped to `SN`.
3. Print the vote command and **exit**, so the user casts it.
4. Take a `--step 2` flag that, once the role exists, submits proposal two: admit a participant with a real `name` and `code`.

Model the proposal JSON on `scripts/devnet/` precedent and on the `syndicate-documents` proposal in this session's history: a top-level `messages` array, `metadata`, `deposit`, `title`, `summary`.

- [ ] **Step 4: Syntax-check it**

Run: `bash -n scripts/devnet/admit-participant.sh`
Expected: no output.

- [ ] **Step 5: Run step one and hand over the vote**

Run: `ssh ... bash /tmp/admit-participant.sh` after `scp`-ing it.
Expected: a proposal id, and the vote command printed.

**Stop here.** Report the proposal id and the vote command to the user. Do not cast the vote.

- [ ] **Step 6: After the user votes and it passes, run step two**

Run: `ssh ... bash /tmp/admit-participant.sh --step 2`
Then the same hand-over: report the id, print the vote command, stop.

- [ ] **Step 7: Verify the dropdown is no longer empty**

Run: `curl -s https://pay.yamalelegal.com/api/rest/yamale/blockchain/paymsg/v1/approved_participant`
Expected: one row with the name and code you chose.

- [ ] **Step 8: Perform one real placement end to end**

Create an account in `/wallet/`, choose the institution from the dropdown, take the printed request into `/institution/`, and run the `set-jurisdiction` command as the participant. Then confirm the account can claim a user ID carrying the right prefix.

Report what the chain said at each step, including anything that was refused.

- [ ] **Step 9: Commit**

```bash
git add scripts/devnet/admit-participant.sh
git commit -m "Nobody on devnet-2 could onboard an account, so the dropdown had nothing in it"
```

---

## Self-Review

**Spec coverage.** C1 → Task 1. C2 → Task 2 (SDK + copy) and Task 3 steps 5–7 (the form). B1 → Task 3 steps 1–6. B2 → Tasks 4 and 5. B3 → Task 6. The spec's Testing section is satisfied by Task 4's eight tests and Task 5 step 7's guard check.

**Known gap, stated rather than hidden.** Task 1 has no automated test. The site menu is static HTML with no test harness in this repo, and inventing one for three `<a>` tags would cost more than it catches; the `stage-site.sh` guard plus a browser look is the verification, and Task 5 step 7 exercises the guard that actually matters — a menu link to a directory the bundle lacks, which is the failure that shipped on 2026-09-13.

**Type consistency.** `approvedParticipants()` returns `{participant, code, name}` in Task 3 and is consumed with those names in Task 3 step 6 and Task 5's `chain.js`. `judge()` takes `{exists, jurisdiction, participants}` in Task 4 and is called with exactly those in Task 5. `foundationPlacement()` returns `{address, marker, document, command}` in Task 2 and is destructured as `f.marker`, `f.document`, `f.command` in Task 3.

**One assumption flagged for the implementer.** Task 6 step 1 exists because the admitting role was not verified against `x/paymsg`'s keeper while this plan was written — only `role.proto`'s description of `ROLE_PAYMENTS_AUTHORITY` was read. Check the code before submitting a proposal that grants the wrong role.
