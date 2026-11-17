# Rename loyalty → traceability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The jewel app and everything named after it become *traceability*:
- the site is `https://traceability.yamaletech.com`;
- the code, deploy and live server names change;
- an account created at `loyalty.yamaletech.com` follows its owner to the new address;
- every old link keeps working.

**Architecture:**
- **Code and deploy files:** a mechanical rename in the Blockchain repo (directories, identifiers, translation keys, the request-signing domain, the API path), with every test kept green.
- **Accounts:** a browser keeps an account under the site's own address, so the old host serves a tiny **move page**. It hands the password-sealed account to the new app's `/move-in` route in the URL fragment, which is never sent to a server, then continues to the page originally asked for.
- **On yamale-us:** a one-off script renames the service user, directories, token and sponsor key. The installers then lay down the new units.
- **yamale-platform:** gains the new Caddy site, the move site, the ports table and the monitor entries.

**Tech Stack:** Go (traceability-api), TypeScript/React + Vite (app, SDK), bash, nginx (the gate in Docker), Caddy and systemd on yamale-us.

**Decided with the user (2026-11-16/17):**
- the new host is `traceability.yamaletech.com`, spelled that way;
- **every** name is renamed, not just the address;
- unchanged: x/jewel and every chain message, the product name "Yamale Jewels" / "Bijoux Yamale", ports 8082 and 8095, and the texts of proposals already on the chain;
- no jewel is registered on devnet-2 yet, so nothing on the chain carries the old URL.

## Global Constraints

**Process**
- Commit messages carry **no Co-Authored-By trailer**.
- Local branches only. **No `git push`, no pull request,** in either repository. Blockchain work goes on branch `jewel-devnet2`, in the worktree `D:\Sermium\Yamale\Blockchain\.claude\worktrees\jewel-devnet2`. yamale-platform work goes on its current local branch, in `D:\Sermium\Yamale\yamale-platform`.
- Only Task 5 touches a host, and only yamale-us. Production databases are never touched.
- Never print a secret: no private key, mnemonic, sealed account, claim link, keyring file, operator token or password.
- Never delete data on a host. Anything replaced is moved aside to `<name>.prev-<UTC date>`.
- On a host, `journalctl` is always bounded (`-n 200` at most).
- Never use a bare `git stash`.
- Commands handed to the user survive PowerShell: one line, at most one outer pair of double quotes, no nested quotes.

**Windows (the development machine)**
- Every shell needs Go on its PATH: `export PATH="/c/Program Files/Go/bin:$PATH"`.
- `make` is not installed.
- Application control sometimes refuses Go test binaries with `fork/exec … stratégie de contrôle`. That is policy, not a failure: rerun.
- `*.sh` and `deploy/yamale-us/traceability/jewel-admit` are LF. Check that no shipped script contains a CR byte.
- `$SCRATCH` is the session scratchpad directory.

**Names after the rename. These are exact, and every task uses them:**

| Before | After |
|---|---|
| `loyalty.yamaletech.com` | `traceability.yamaletech.com` |
| `clients/loyalty`, package `@yamale/loyalty`, workspace entry `loyalty` | `clients/traceability`, `@yamale/traceability`, `traceability` |
| `tools/loyaltyapi`, binary `yamale-loyaltyapi` | `tools/traceabilityapi`, `yamale-traceabilityapi` |
| `clients/sdk/src/loyaltyauth.ts` (+ `.test.ts`), `loyaltySignBytes`, `LoyaltyPurpose` | `traceabilityauth.ts`, `traceabilitySignBytes`, `TraceabilityPurpose` |
| request-signing domain `yamale-loyalty/v1` | `yamale-traceability/v1` |
| translation keys `loyalty.*` | `traceability.*` |
| API path `/api/loyalty/` (gate, `config.ts` `API`, vite proxy, env `LOYALTY_API`) | `/api/traceability/`, env `TRACEABILITY_API` |
| browser storage keys `yamale.loyalty.account`, `.theme`, `.link.` | `yamale.traceability.account`, `.theme`, `.link.` |
| `deploy/yamale-us/loyalty/`, `install-loyalty.sh` | `deploy/yamale-us/traceability/`, `install-traceability.sh` |
| gate `loyalty.conf`, zones `yamale_loyalty_write` and `yamale_loyalty_sponsor`, maps `$loyalty_write_key` and `$loyalty_sponsor_key` | `traceability.conf`, `yamale_traceability_write`, `yamale_traceability_sponsor`, `$traceability_write_key`, `$traceability_sponsor_key` |
| `scripts/devnet/loyalty-admit.sh`, `loyalty-local.sh` | `traceability-admit.sh`, `traceability-local.sh` (`jewel-admit` and `jewel-lib.sh` keep their names) |
| unit `yamale-loyalty.service`, user and group `yamale-loyalty` | `yamale-traceability.service`, `yamale-traceability` |
| `yamale-loyalty-backup.service` and `.timer`, archives `loyalty-<date>.tgz` | `yamale-traceability-backup.*`, `traceability-<date>.tgz` |
| `/opt/yamale/loyalty` (sponsor keyring), `/var/lib/yamale-loyalty`, `/var/backups/yamale-loyalty`, `/srv/loyalty` | `/opt/yamale/traceability`, `/var/lib/yamale-traceability`, `/var/backups/yamale-traceability`, `/srv/traceability` |
| `/etc/yamale/loyalty-operator-token` | `/etc/yamale/traceability-operator-token` |
| sponsor key name `loyalty-sponsor` | `traceability-sponsor` (same key, same address) |
| prose "loyalty-api", "the loyalty app" | "traceability-api", "the traceability app" |

**Not renamed:**
- **Dated history:** `docs/superpowers/plans/*` and every spec dated before 2026-11-16. Branch names inside history (`loyalty-app`). Commit messages.
- **`proto/blockchain/jewel/v1/events.proto`:** its one comment mentions "the loyalty app". Changing a proto comment means regenerating `*.pb.go` and `docs/reference`, a toolchain this Windows machine lacks. It stays and is listed in gaps.md.
- **`/opt/yamale/jewel-office`:** the office keyring, which was never named loyalty.

---

### Task 1: Rename the code (Blockchain repo)

**Files:**
- Move:
  - `clients/loyalty/` → `clients/traceability/`
  - `tools/loyaltyapi/` → `tools/traceabilityapi/`
  - `clients/sdk/src/loyaltyauth.ts` → `clients/sdk/src/traceabilityauth.ts`
  - `clients/sdk/src/loyaltyauth.test.ts` → `clients/sdk/src/traceabilityauth.test.ts`
- Modify:
  - every file under the two moved trees;
  - `clients/package.json` and `clients/package-lock.json` (regenerated by `npm install`);
  - `clients/sdk/src/catalogues.ts`, `index.ts`, `jewel.test.ts`, `jewelreads.ts`;
  - `x/jewel/keeper/events_test.go`, `x/jewel/keeper/fixture_test.go`, `x/jewel/types/errors_needles_test.go`, `x/jewel/types/types_test.go`;
  - `app/profile_upgrades_on.go:402` (comment only).
- Regenerate: `tools/traceabilityapi/testdata/vectors.json`.

**Interfaces:**
- Produces:
  - SDK exports `traceabilitySignBytes`, `signedHeaders`, `sha256Hex` and `type TraceabilityPurpose` from `./traceabilityauth.ts`;
  - the signing domain is `yamale-traceability/v1:<purpose>:<chain id>:<subject>:<issued>`;
  - `clients/traceability/src/config.ts` exports `API = '/api/traceability'`;
  - `clients/traceability/src/account/device.ts` uses the storage key `'yamale.traceability.account'`, and exports `readAccount(store?)` (existing) and a new `writeImportedAccount(acct: StoredAccount, store?)` (Task 2 adds it; not here).

- [ ] **Step 1: Record the starting point**

```bash
cd /d/Sermium/Yamale/Blockchain/.claude/worktrees/jewel-devnet2
export PATH="/c/Program Files/Go/bin:$PATH"
git status --short   # must be empty
git grep -I -c -i loyalty -- . ':!docs/superpowers' ':!clients/package-lock.json' | awk -F: '{s+=$2} END {print s}'
```

Expected: an empty status, and a count above 3,000. Write the count into the task report.

- [ ] **Step 2: Move the files with git**

```bash
git mv clients/loyalty clients/traceability
git mv tools/loyaltyapi tools/traceabilityapi
git mv clients/sdk/src/loyaltyauth.ts clients/sdk/src/traceabilityauth.ts
git mv clients/sdk/src/loyaltyauth.test.ts clients/sdk/src/traceabilityauth.test.ts
```

- [ ] **Step 3: Replace the text, in this order (longest first)**

Restrict the edit to the files named under **Files** above. Never touch `docs/`, `proto/`, `deploy/`, `scripts/`, `.gitattributes`, `docs/scope/gaps.md` or `clients/README.md`: Tasks 3 and 6 own those.

```bash
FILES=$(git grep -I -l -i loyalty -- clients/traceability tools/traceabilityapi clients/sdk/src clients/package.json x/jewel app/profile_upgrades_on.go)
for f in $FILES; do
  sed -i \
    -e 's#loyalty\.yamaletech\.com#traceability.yamaletech.com#g' \
    -e 's#yamale-loyalty/v1#yamale-traceability/v1#g' \
    -e 's#yamale-loyaltyapi#yamale-traceabilityapi#g' \
    -e 's#loyaltyapi#traceabilityapi#g' \
    -e 's#loyaltyauth#traceabilityauth#g' \
    -e 's#loyaltySignBytes#traceabilitySignBytes#g' \
    -e 's#LoyaltyPurpose#TraceabilityPurpose#g' \
    -e 's#LOYALTY_#TRACEABILITY_#g' \
    -e 's#Loyalty#Traceability#g' \
    -e 's#loyalty#traceability#g' \
    "$f"
done
```

`sed -i` keeps each file's existing line endings when the file is LF. Check with `git diff --stat` that no file shows every line changed. If one does, it was CRLF: restore it from git and edit it with a CRLF-preserving method (memory: editing-files-in-this-repo).

- [ ] **Step 4: Inspect what the substitution produced**

```bash
git grep -n -i loyalty -- clients/traceability tools/traceabilityapi clients/sdk/src clients/package.json x/jewel app/profile_upgrades_on.go
git grep -n -E "Traceability[A-Z]|traceabilitytraceability" -- clients tools x app | head
```

Expected:
- the first command prints nothing;
- the second prints only legitimate identifiers. Read every match, and fix any awkward compound by hand (e.g. a sentence reading "a traceability app's backend" is fine; a doubled word is not).

Read `clients/sdk/src/catalogues.ts` in two or three places. Keys must read `'traceability.title'`, and the **values** must not have changed meaning. The French value of the old `loyalty.title` was "Bijoux Yamale", and it must stay that. The sed only touches the literal word "loyalty", which appears in no translated value. Confirm with:

```bash
git diff clients/sdk/src/catalogues.ts | grep "^+" | grep -v -E "^\+\s+'traceability\." | head
```

Expected: no output beyond the `+++` header. Every changed line is a key change.

- [ ] **Step 5: Regenerate the signature vectors**

The domain string changed, so every signature in the vectors changed.

```bash
go test ./tools/traceabilityapi -run TestVectors -update-vectors
git diff --stat tools/traceabilityapi/testdata/vectors.json
grep -c "yamale-traceability/v1" tools/traceabilityapi/testdata/vectors.json
```

Expected: the file changed, and the count equals the number of vectors (4 today). Then run without the flag:

```bash
go test ./tools/traceabilityapi -run TestVectors
```

Expected: PASS.

- [ ] **Step 6: Reinstall workspaces, so the package name resolves**

```bash
cd clients && npm install && cd ..
git diff --stat clients/package-lock.json
```

Expected:
- the lock file names `@yamale/traceability` and `traceability`, and no longer `@yamale/loyalty`;
- `git grep -c loyalty clients/package-lock.json` prints `0` or nothing.

- [ ] **Step 7: Run every affected test suite**

```bash
go build ./...
go test ./tools/traceabilityapi/... ./x/jewel/...
cd clients
npx vitest run --root sdk
npx vitest run --root traceability
npm run build --workspace @yamale/traceability
cd ..
```

Expected:
- every command exits 0;
- the Go needle test `TestTheTraceabilityAppsErrorNeedlesAreRegisteredTexts` reads `../../../clients/traceability/src/actions/errors.ts` and passes;
- SDK tests: 236 passing;
- app tests: 46 passing, the same counts as before the rename. Record the counts in the report.

If a vitest root flag differs in this repo, use the command `clients/package.json` defines for that workspace, and say which in the report.

- [ ] **Step 8: Commit**

```bash
git add -A clients tools x/jewel app/profile_upgrades_on.go
git status --short   # only the paths above, nothing under deploy/ scripts/ docs/
git commit -m "rename: loyalty becomes traceability in the app, the SDK, traceability-api and the tests

The request-signing domain is yamale-traceability/v1 (vectors regenerated),
the API path /api/traceability, the storage keys yamale.traceability.*,
translation keys traceability.*. Chain messages and the product name are unchanged."
```

---

### Task 2: Carry accounts across: the `/move-in` route

**Files:**
- Create: `clients/traceability/src/account/movein.ts`
- Create: `clients/traceability/src/account/movein.test.ts`
- Create: `clients/traceability/src/account/MoveIn.tsx`
- Modify: `clients/traceability/src/account/device.ts` (add `writeImportedAccount`)
- Modify: `clients/traceability/src/App.tsx` (add the route)
- Modify: `clients/sdk/src/catalogues.ts` (three keys, in all seven languages)

**Interfaces:**
- Consumes:
  - `StoredAccount`, `KeyStore` and `readAccount(store?)` from `device.ts`, exactly as they exist;
  - the storage key is `'yamale.traceability.account'` (Task 1).
- Produces: the URL contract the move page (Task 4) writes, which is exact:

  ```
  https://traceability.yamaletech.com/move-in#move=<base64url of the old localStorage value, UTF-8>&to=<base64url of path+query+fragment, UTF-8>
  ```

  `move` is absent when the old origin held no account; then the move page redirects straight to `to` and never calls `/move-in`.
- `acceptMove(fragment: string, store: KeyStore): MoveOutcome`, where:

  ```ts
  export type MoveOutcome =
    | { kind: 'stored'; address: string; to: string }
    | { kind: 'same'; address: string; to: string }
    | { kind: 'other'; here: string; incoming: string; to: string }
    | { kind: 'invalid'; to: string };
  ```

- `safeTarget(raw: string | null): string` returns `raw` only if it starts with `/` and not with `//` or `/\`; otherwise `'/'`.

- [ ] **Step 1: Write the failing tests**

`clients/traceability/src/account/movein.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { acceptMove, safeTarget } from './movein.ts';
import type { KeyStore, StoredAccount } from './device.ts';

const KEY = 'yamale.traceability.account';
const memory = (init: Record<string, string> = {}): KeyStore & { data: Record<string, string> } => {
  const data = { ...init };
  return { data, getItem: (k) => data[k] ?? null, setItem: (k, v) => { data[k] = v; }, removeItem: (k) => { delete data[k]; } };
};
const b64u = (s: string) => Buffer.from(s, 'utf8').toString('base64url');
const acct = (address: string): StoredAccount => ({
  address, email: 'a@example.com', createdAt: '2026-11-16T00:00:00.000Z', protection: 'none', sponsored: true,
  sealed: { salt: 'c2FsdA', nonce: 'bm9uY2U', ciphertext: 'Y2lwaGVy', time: 3, memory: 65536, threads: 1 },
});
const A = 'yml1dzwpk73ntdyve2vx49flv0dxxguy6a5ygp2zv7';
const B = 'yml1rjgw9548jhz9e87me4zja37sym0nrt3n3huu2q';

describe('acceptMove', () => {
  it('stores the incoming account when this origin holds none, and keeps the target', () => {
    const s = memory();
    const out = acceptMove(`#move=${b64u(JSON.stringify(acct(A)))}&to=${b64u('/claim?x=1#k=abc')}`, s);
    expect(out).toEqual({ kind: 'stored', address: A, to: '/claim?x=1#k=abc' });
    expect(JSON.parse(s.data[KEY]!).address).toBe(A);
  });
  it('does nothing when the same account is already here', () => {
    const s = memory({ [KEY]: JSON.stringify(acct(A)) });
    const before = s.data[KEY];
    expect(acceptMove(`#move=${b64u(JSON.stringify(acct(A)))}&to=${b64u('/')}`, s)).toEqual({ kind: 'same', address: A, to: '/' });
    expect(s.data[KEY]).toBe(before);
  });
  it('never overwrites a different account', () => {
    const s = memory({ [KEY]: JSON.stringify(acct(B)) });
    expect(acceptMove(`#move=${b64u(JSON.stringify(acct(A)))}&to=${b64u('/mine')}`, s))
      .toEqual({ kind: 'other', here: B, incoming: A, to: '/mine' });
    expect(JSON.parse(s.data[KEY]!).address).toBe(B);
  });
  it('refuses a payload that is not a sealed account', () => {
    const s = memory();
    for (const bad of ['not-base64!', b64u('{}'), b64u('{"address":"cosmos1x","sealed":{}}'), b64u(JSON.stringify({ ...acct(A), sealed: { salt: 'x' } }))]) {
      expect(acceptMove(`#move=${bad}&to=${b64u('/')}`, s).kind).toBe('invalid');
    }
    expect(s.data[KEY]).toBeUndefined();
  });
  it('an unsafe target becomes /', () => {
    const s = memory();
    expect(acceptMove(`#move=${b64u(JSON.stringify(acct(A)))}&to=${b64u('//evil.example/x')}`, s).to).toBe('/');
  });
});

describe('safeTarget', () => {
  it.each([['/j/1/1', '/j/1/1'], ['/claim#k=1', '/claim#k=1'], ['//x.example', '/'], ['/\\x.example', '/'], ['https://x.example', '/'], [null, '/'], ['', '/']])(
    '%s → %s', (raw, want) => expect(safeTarget(raw)).toBe(want));
});
```

- [ ] **Step 2: Run them to see them fail**

```bash
cd clients && npx vitest run --root traceability src/account/movein.test.ts; cd ..
```

Expected: FAIL, "Failed to resolve import './movein.ts'".

- [ ] **Step 3: Add `writeImportedAccount` to `device.ts`**

Add this below `readAccount`. It reuses the module's `KEY` and `browserStore()`, and validates through the same parser `readAccount` uses, so an account that `readAccount` would reject can never be written.

```ts
/**
 * Stores an account carried over from the app's previous address
 * (loyalty.yamaletech.com, see movein.ts). It is the sealed form only: the
 * password never travelled. Refuses to replace an account already here.
 */
export function writeImportedAccount(acct: StoredAccount, store: KeyStore = browserStore()): void {
  if (store.getItem(KEY) !== null) throw new Error(ACCOUNT_EXISTS);
  const text = JSON.stringify(acct);
  store.setItem(KEY, text);
  if (readAccount(store)?.address !== acct.address) {
    store.removeItem(KEY);
    throw new Error('the carried account does not read back');
  }
}
```

- [ ] **Step 4: Write `movein.ts`**

```ts
/**
 * The app moved from loyalty.yamaletech.com to traceability.yamaletech.com.
 * A browser keeps an account under the site's own address, so the old address
 * serves a page (yamale-platform www/loyalty-move) that reads the sealed
 * account there and opens /move-in#move=…&to=… here. The fragment is never
 * sent to any server. The account is sealed under its owner's password
 * (device.ts), so what travels cannot be opened without it.
 */
import { readAccount, writeImportedAccount, type KeyStore, type StoredAccount } from './device.ts';

export type MoveOutcome =
  | { kind: 'stored'; address: string; to: string }
  | { kind: 'same'; address: string; to: string }
  | { kind: 'other'; here: string; incoming: string; to: string }
  | { kind: 'invalid'; to: string };

export function safeTarget(raw: string | null): string {
  if (!raw || !raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return '/';
  return raw;
}

function fromB64u(s: string | null): string | null {
  if (!s || !/^[A-Za-z0-9_-]+$/.test(s)) return null;
  try {
    const bin = atob(s.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (s.length % 4)) % 4));
    return new TextDecoder('utf-8', { fatal: true }).decode(Uint8Array.from(bin, (c) => c.charCodeAt(0)));
  } catch { return null; }
}

function looksSealed(v: unknown): v is StoredAccount {
  const a = v as StoredAccount;
  const w = a?.sealed;
  return typeof a?.address === 'string' && /^yml1[02-9ac-hj-np-z]{38}$/.test(a.address)
    && typeof w?.salt === 'string' && typeof w?.nonce === 'string' && typeof w?.ciphertext === 'string'
    && Number.isInteger(w?.time) && Number.isInteger(w?.memory) && Number.isInteger(w?.threads);
}

export function acceptMove(fragment: string, store: KeyStore): MoveOutcome {
  const params = new URLSearchParams(fragment.replace(/^#/, ''));
  const to = safeTarget(fromB64u(params.get('to')));
  const text = fromB64u(params.get('move'));
  let incoming: unknown;
  try { incoming = text === null ? null : JSON.parse(text); } catch { incoming = null; }
  if (!looksSealed(incoming)) return { kind: 'invalid', to };
  const here = readAccount(store);
  if (here) {
    return here.address === incoming.address
      ? { kind: 'same', address: here.address, to }
      : { kind: 'other', here: here.address, incoming: incoming.address, to };
  }
  try { writeImportedAccount(incoming, store); } catch { return { kind: 'invalid', to }; }
  return { kind: 'stored', address: incoming.address, to };
}
```

- [ ] **Step 5: Run the tests and see them pass**

```bash
cd clients && npx vitest run --root traceability src/account/movein.test.ts; cd ..
```

Expected: PASS, 5 `acceptMove` tests plus the 7 `safeTarget` cases.

- [ ] **Step 6: The route and its screen**

`clients/traceability/src/account/MoveIn.tsx`:

```tsx
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { t } from '@yamale/chain';
import { acceptMove, type MoveOutcome } from './movein.ts';
import { AppBar } from '../ui.tsx';

/** Lands here from loyalty.yamaletech.com with the sealed account in the fragment. */
export function MoveIn() {
  const navigate = useNavigate();
  const [out, setOut] = useState<MoveOutcome | null>(null);
  useEffect(() => {
    const result = acceptMove(window.location.hash, window.localStorage);
    // Take the sealed account out of the address bar and the history entry first.
    window.history.replaceState(null, '', '/move-in');
    if (result.kind === 'stored' || result.kind === 'same' || result.kind === 'invalid') {
      navigate(result.to, { replace: true });
      if (result.to.includes('#')) window.location.hash = result.to.slice(result.to.indexOf('#'));
      return;
    }
    setOut(result);
  }, [navigate]);
  if (!out || out.kind !== 'other') return null;
  return (
    <>
      <AppBar title={t('traceability.moveTitle')} eyebrow={t('traceability.title')} />
      <main className="y-screen stack">
        <p className="note">{t('traceability.moveOther', { here: out.here, incoming: out.incoming })}</p>
        <button className="btn btn--primary" onClick={() => navigate(out.to, { replace: true })}>{t('traceability.moveContinue')}</button>
      </main>
    </>
  );
}
```

Check how `App.tsx` imports `useNavigate`, and match it exactly (react-router-dom or react-router). In `App.tsx`, add `<Route path="/move-in" element={<MoveIn />} />` next to the other top-level routes, with the import.

A fragment that `navigate()` drops is put back with `location.hash`, because claim links (`/claim#…`) depend on it. If the app's router already preserves the hash on `navigate`, remove that line and say so in the report.

- [ ] **Step 7: The four translation keys, in all seven languages**

In `clients/sdk/src/catalogues.ts`, add to each language block, after `traceability.title`:

| key | en | fr |
|---|---|---|
| `traceability.moveTitle` | "Your account moved" | "Votre compte a déménagé" |
| `traceability.moveOther` | "This device already holds account {here} at this address, so the account {incoming} from loyalty.yamaletech.com was not copied. It is still there." | "Cet appareil détient déjà le compte {here} à cette adresse ; le compte {incoming} de loyalty.yamaletech.com n'a donc pas été copié. Il y est toujours." |
| `traceability.moveContinue` | "Continue" | "Continuer" |

For the other five languages (es, pt, ar, sw, zh-HK; take the exact codes from the file), translate the English. That is three keys in seven languages. Then run the catalogue completeness test the SDK already has, which fails when a key is missing in one language:

```bash
cd clients && npx vitest run --root sdk; cd ..
```

Expected: PASS.

- [ ] **Step 8: Build, run all app tests, commit**

```bash
cd clients && npx vitest run --root traceability && npm run build --workspace @yamale/traceability; cd ..
git add clients/traceability clients/sdk/src/catalogues.ts
git commit -m "traceability: /move-in carries the sealed account over from loyalty.yamaletech.com

Never overwrites a different account, refuses anything that is not a sealed
account, accepts only a same-site target, and clears the fragment first."
```

---

### Task 3: Rename the deploy side, and write the one-off migration (Blockchain repo)

**Files:**
- Move:
  - `deploy/yamale-us/loyalty/` → `deploy/yamale-us/traceability/`
  - `deploy/yamale-us/traceability/install-loyalty.sh` → `install-traceability.sh`
  - `deploy/yamale-us/gate/loyalty.conf` → `deploy/yamale-us/gate/traceability.conf`
  - `scripts/devnet/loyalty-admit.sh` → `scripts/devnet/traceability-admit.sh`
  - `scripts/devnet/loyalty-local.sh` → `scripts/devnet/traceability-local.sh`
- Modify:
  - `deploy/yamale-us/gate/{deploy-gate.sh,docker-compose.yml,nginx.conf,traceability.conf}`;
  - `deploy/yamale-us/traceability/{install-traceability.sh,jewel-admit}`;
  - `scripts/devnet/{traceability-admit.sh,traceability-local.sh,measure-amend-gas.sh,seed-jewel.sh,jewel-lib.sh}`, where each mentions loyalty;
  - `.gitattributes` (the `jewel-admit` rule's path and comment).
- Create: `deploy/yamale-us/traceability/migrate-from-loyalty.sh`.

**Interfaces:**
- Consumes: the names table in Global Constraints.
- Produces, used by Task 5:
  - `migrate-from-loyalty.sh [--dry-run]` runs locally and ssh'es to `${TRACEABILITY_HOST:-yamale-us}`. It is idempotent: on a host already migrated, it prints `already migrated` and exits 0.
  - `install-traceability.sh [--api] [--app]` keeps the old installer's behaviour, under the new names.

- [ ] **Step 1: Move the files**

```bash
cd /d/Sermium/Yamale/Blockchain/.claude/worktrees/jewel-devnet2
git mv deploy/yamale-us/loyalty deploy/yamale-us/traceability
git mv deploy/yamale-us/traceability/install-loyalty.sh deploy/yamale-us/traceability/install-traceability.sh
git mv deploy/yamale-us/gate/loyalty.conf deploy/yamale-us/gate/traceability.conf
git mv scripts/devnet/loyalty-admit.sh scripts/devnet/traceability-admit.sh
git mv scripts/devnet/loyalty-local.sh scripts/devnet/traceability-local.sh
```

- [ ] **Step 2: Replace the text in the deploy and script files**

Use the same ordered substitution as Task 1 Step 3, restricted to these paths:

```bash
FILES=$(git grep -I -l -i loyalty -- deploy/yamale-us scripts/devnet .gitattributes)
for f in $FILES; do
  sed -i \
    -e 's#loyalty\.yamaletech\.com#traceability.yamaletech.com#g' \
    -e 's#yamale-loyalty/v1#yamale-traceability/v1#g' \
    -e 's#yamale-loyaltyapi#yamale-traceabilityapi#g' \
    -e 's#loyaltyapi#traceabilityapi#g' \
    -e 's#LOYALTY_#TRACEABILITY_#g' \
    -e 's#Loyalty#Traceability#g' \
    -e 's#loyalty#traceability#g' \
    "$f"
done
git grep -n -i loyalty -- deploy/yamale-us scripts/devnet .gitattributes
```

Expected: the last command prints nothing.

The new migration script in Step 3 deliberately names the **old** paths. Write it after this substitution, so the substitution does not rewrite it.

Then read the full diff of these files: `install-traceability.sh`, `deploy-gate.sh`, `docker-compose.yml`, `nginx.conf` and `traceability.conf`. Check that:
- the unit is `yamale-traceability.service` with `User=yamale-traceability`, `ExecStart=/opt/yamale/bin/yamale-traceabilityapi … --dir /var/lib/yamale-traceability … --from traceability-sponsor --home /opt/yamale/traceability … --operator-token-file /etc/yamale/traceability-operator-token`, and `ReadWritePaths=/var/lib/yamale-traceability /opt/yamale/traceability`;
- the backup unit archives `var/lib/yamale-traceability opt/yamale/traceability` (and `opt/yamale/jewel-office`) into `/var/backups/yamale-traceability/traceability-$(date …).tgz`, and its `find … -name "traceability-*.tgz"` cleanup matches;
- the gate mounts `/srv/traceability:/srv/traceability:ro`, and nginx has `root /srv/traceability;`, `location /api/traceability/`, `location = /api/traceability/v1/sponsor`, `location = /api/traceability/v1/applications`, `location = /api/traceability/v1/health/sponsor { return 404; }`, the zones `yamale_traceability_write` and `yamale_traceability_sponsor`, and the maps `$traceability_write_key` and `$traceability_sponsor_key`;
- `deploy-gate.sh` copies `traceability.conf`, creates `/srv/traceability`, and probes `/api/traceability/v1/health`;
- the installer builds `clients/traceability` with `VITE_PUBLIC_ORIGIN=https://traceability.yamaletech.com`, checks `clients/traceability/.env*`, ships `scripts/devnet/traceability-admit.sh`, and installs it to `/opt/yamale/bin/`;
- `jewel-admit` execs `/opt/yamale/bin/traceability-admit.sh`.

- [ ] **Step 3: Write `migrate-from-loyalty.sh`**

`deploy/yamale-us/traceability/migrate-from-loyalty.sh`, mode 755, LF:

```bash
#!/usr/bin/env bash
# migrate-from-loyalty.sh - one-off: rename the loyalty service on yamale-us to traceability.
#
#   deploy/yamale-us/traceability/migrate-from-loyalty.sh --dry-run   # says what it would do
#   deploy/yamale-us/traceability/migrate-from-loyalty.sh             # does it
#
# Run it BEFORE install-traceability.sh, which then writes the new units over
# the renamed directories. It moves and renames; it deletes nothing. The old
# unit files are moved aside into /root/loyalty-units.prev-<date>. The sponsor
# key is renamed, not replaced, so its address and every fee allowance it
# granted stay as they are. On a host already migrated it says so and exits 0.
set -euo pipefail
HOST="${TRACEABILITY_HOST:-yamale-us}"
DRY=0
[ "${1:-}" = --dry-run ] && DRY=1
ssh "$HOST" "DRY=$DRY;"'set -euo pipefail
run() { if [ "$DRY" = 1 ]; then echo "would: $*"; else echo "+ $*"; sudo "$@"; fi; }
BIN=/opt/yamale/bin/blockchaind
if ! id yamale-loyalty >/dev/null 2>&1 && id yamale-traceability >/dev/null 2>&1; then echo "already migrated"; exit 0; fi
id yamale-loyalty >/dev/null 2>&1 || { echo "no yamale-loyalty user: nothing to migrate"; exit 1; }
for p in /opt/yamale/traceability /var/lib/yamale-traceability /var/backups/yamale-traceability /etc/yamale/traceability-operator-token; do
  if sudo test -e "$p"; then echo "refusing: $p already exists"; exit 1; fi
done
D=$(date -u +%Y%m%d%H%M)
run systemctl stop yamale-loyalty yamale-loyalty-backup.timer
run systemctl disable yamale-loyalty yamale-loyalty-backup.timer
run usermod -l yamale-traceability -d /opt/yamale/traceability yamale-loyalty
run groupmod -n yamale-traceability yamale-loyalty
run mv /opt/yamale/loyalty /opt/yamale/traceability
run mv /var/lib/yamale-loyalty /var/lib/yamale-traceability
run mv /var/backups/yamale-loyalty /var/backups/yamale-traceability
run mv /etc/yamale/loyalty-operator-token /etc/yamale/traceability-operator-token
if [ "$DRY" = 1 ]; then
  echo "would: keys rename loyalty-sponsor traceability-sponsor (in /opt/yamale/traceability)"
else
  sudo -u yamale-traceability $BIN keys rename loyalty-sponsor traceability-sponsor --keyring-backend test --home /opt/yamale/traceability -y >/dev/null
  echo "sponsor key renamed: $(sudo -u yamale-traceability $BIN keys show traceability-sponsor -a --keyring-backend test --home /opt/yamale/traceability)"
fi
run install -d -o root -g root -m 700 /root/loyalty-units.prev-$D
for u in yamale-loyalty.service yamale-loyalty-backup.service yamale-loyalty-backup.timer; do
  run mv /etc/systemd/system/$u /root/loyalty-units.prev-$D/
done
run systemctl daemon-reload
if [ -d /srv/loyalty ]; then run mv /srv/loyalty /srv/loyalty.prev-$D; fi
echo "migrated; now run install-traceability.sh, deploy-gate.sh, then the platform deploy"
'
```

Notes for the reviewer:
- `/srv/loyalty` is moved aside rather than renamed into place, because the installer refills `/srv/traceability` from a fresh build. The gate stays down between this script and `deploy-gate.sh`, which is expected within the Task 5 window.
- Group ownership of `/etc/yamale/traceability-operator-token` follows the renamed group automatically, because `groupmod` keeps the GID.

- [ ] **Step 4: Check every shell file**

```bash
for f in deploy/yamale-us/traceability/*.sh deploy/yamale-us/traceability/jewel-admit deploy/yamale-us/gate/deploy-gate.sh scripts/devnet/traceability-*.sh; do
  bash -n "$f" && [ "$(LC_ALL=C tr -dc '\r' < "$f" | wc -c)" -eq 0 ] && echo "ok $f" || echo "BAD $f"
done
git ls-files -s deploy/yamale-us/traceability/migrate-from-loyalty.sh   # want mode 100755
bash scripts/devnet/test-await-halt.sh >/dev/null && echo "watcher test ok"
```

Expected: `ok` on every file, mode `100755` (set it with `git update-index --chmod=+x` if needed), and `watcher test ok`.

- [ ] **Step 5: Commit**

```bash
git add -A deploy/yamale-us scripts/devnet .gitattributes
git commit -m "rename: deploy and devnet scripts become traceability; one-off migration for yamale-us

The gate serves /srv/traceability with /api/traceability/ and the
yamale_traceability_* zones; the installer lays down yamale-traceability.service
and its backup. migrate-from-loyalty.sh renames the user, directories, token and
sponsor key in place (no deletion; old units moved aside)."
```

---

### Task 4: yamale-platform: the new site, the move page, the ports and the monitor

**Files** (in `D:\Sermium\Yamale\yamale-platform`):
- Move: `caddy/sites/loyalty.yamaletech.com.caddy` → `caddy/sites/traceability.yamaletech.com.caddy`
- Create: `caddy/sites/loyalty.yamaletech.com.caddy` (the move site)
- Create: `www/loyalty-move/index.html`
- Create: `www/loyalty-move/move.js`
- Modify: `PORTS.md` (rows 8082 and 8095)
- Modify: `monitor/yamale-monitor-host.conf` (the unit and the three probes)

**Interfaces:**
- Consumes:
  - the `/move-in` URL contract from Task 2, which is exact;
  - the old storage key `yamale.loyalty.account`.
- Produces:
  - `https://loyalty.yamaletech.com/<anything>` loads the move page, which `location.replace`s to `https://traceability.yamaletech.com/move-in#move=…&to=…` when it holds an account, otherwise straight to `https://traceability.yamaletech.com<path><query><fragment>`;
  - no request carries the account to any server.

- [ ] **Step 1: Rename the site file and edit it**

```bash
cd /d/Sermium/Yamale/yamale-platform
git status --short   # must be empty
git mv caddy/sites/loyalty.yamaletech.com.caddy caddy/sites/traceability.yamaletech.com.caddy
```

Its whole content becomes:

```
# traceability.yamaletech.com: the jewel app (Blockchain repo, clients/traceability), served at the
# root of its own host by the chain gate's second server (127.0.0.1:8082). Not chain_app:
# that snippet redirects / to /<app>/ and this app lives at /.
traceability.yamaletech.com {
	import common
	import access_log traceability.yamaletech.com
	reverse_proxy 127.0.0.1:8082 {
		import visitor
	}
}
```

- [ ] **Step 2: The move page**

`www/loyalty-move/move.js`:

```js
// loyalty.yamaletech.com moved to traceability.yamaletech.com (2026-11-17).
// A browser keeps an account under the site's own address, so the sealed account
// this browser made here is handed to the new address in the URL fragment,
// which no server ever receives. It is sealed under its owner's password.
// The copy here is left in place.
(function () {
  var DEST = 'https://traceability.yamaletech.com';
  var to = location.pathname + location.search + location.hash;
  function b64u(s) {
    var bytes = new TextEncoder().encode(s), bin = '';
    for (var i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
    return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  }
  var acct = null;
  try { acct = localStorage.getItem('yamale.loyalty.account'); } catch (e) { acct = null; }
  location.replace(acct ? DEST + '/move-in#move=' + b64u(acct) + '&to=' + b64u(to) : DEST + to);
})();
```

`www/loyalty-move/index.html`:

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>Yamale Jewels has moved</title>
<style>
  body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: #0d1b2e; color: #e8edf4;
         font: 16px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif; }
  main { max-width: 32rem; padding: 2rem 1rem; }
  a { color: #d9b46a; }
</style>
<script src="/move.js"></script>
</head>
<body>
<main>
  <h1>Yamale Jewels has moved</h1>
  <p>It now lives at <a href="https://traceability.yamaletech.com/">traceability.yamaletech.com</a>. Taking you there…</p>
  <noscript><p>Open <a href="https://traceability.yamaletech.com/">traceability.yamaletech.com</a>. To bring an account made here, allow JavaScript and reload.</p></noscript>
</main>
</body>
</html>
```

- [ ] **Step 3: The move site**

New `caddy/sites/loyalty.yamaletech.com.caddy`. First read how `common` is defined in `caddy/Caddyfile` (or wherever `(common)` lives), and whether it sets `Content-Security-Policy`. Caddy's `header` with a plain name **replaces** the value, so the line below wins either way.

```
# loyalty.yamaletech.com: the jewel app's old address (until 2026-11-17). Every path serves
# the move page, which carries an account made here to traceability.yamaletech.com and
# continues to the same path. Not a 301: a redirect cannot read this address's storage.
loyalty.yamaletech.com {
	import common
	import access_log loyalty.yamaletech.com
	header Content-Security-Policy "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	header Cache-Control "no-store"
	root * /srv/www/loyalty-move
	@page not path /move.js
	rewrite @page /index.html
	file_server
}
```

`deploy.sh` copies `www/` to `/srv/www/` (the line beginning `sudo rsync -a --delete "$T/www/"`), so `/srv/www/loyalty-move` exists after a deploy.

- [ ] **Step 4: Ports table and monitor**

In `PORTS.md`:
- the 8082 row's purpose and host become `chain gate, traceability server (app + /api/traceability)` and `traceability.yamaletech.com`;
- the 8095 row becomes `traceability-api (\`yamale-traceability\`)`.

In `monitor/yamale-monitor-host.conf`:
- in `MONITOR_UNITS`, replace `yamale-loyalty=Loyalty API` with `yamale-traceability=Traceability API`;
- in `MONITOR_PROBES`, replace the three probes with:

```
Traceability=https://traceability.yamaletech.com/j/1/1,Traceability API=https://traceability.yamaletech.com/api/traceability/v1/health,Traceability sponsor=http://127.0.0.1:8095/v1/health/sponsor,Loyalty move page=https://loyalty.yamaletech.com/
```

Then check:

```bash
git grep -n -i loyalty
```

Expected: only the move site's file, `www/loyalty-move/*`, the `Loyalty move page` probe, and the dated comment in the traceability site.

- [ ] **Step 5: Validate without installing, then commit**

```bash
./deploy.sh --check
```

Expected: every check passes, and the new site files validate. Nothing is installed.

```bash
git add -A caddy www PORTS.md monitor
git commit -m "traceability.yamaletech.com replaces loyalty.yamaletech.com; the old address serves a move page

The move page hands a sealed account made at the old address to
traceability.yamaletech.com/move-in in the fragment, which no server receives,
and continues to the same path. Monitor and ports table renamed."
```

---

### Task 5: Switch yamale-us over (controller, live)

This is not for a subagent. The controller runs it, says each step in chat as it goes, and stops at the first unexpected output.

- [ ] **Step 1: Before touching anything**

```bash
ssh yamale-us "systemctl is-active yamale-loyalty caddy yamale-monitor; sudo ls /opt/yamale/loyalty /var/lib/yamale-loyalty | head; curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8095/v1/health/sponsor"
curl -sI https://traceability.yamaletech.com/ | head -3
```

Expected:
- three `active`;
- the directories list;
- sponsor health 200;
- `traceability.yamaletech.com` already answers through Cloudflare, because the wildcard DNS record covers it. It answers from Caddy's catch-all, with no site yet.

If Cloudflare does not answer for the name, stop: the order is in the memory note cloudflare-in-front.

Then run the dry run, and read every `would:` line:

```bash
deploy/yamale-us/traceability/migrate-from-loyalty.sh --dry-run
```

- [ ] **Step 2: Migrate, install, gate, platform: in this order, back to back**

```bash
deploy/yamale-us/traceability/migrate-from-loyalty.sh
bash deploy/yamale-us/traceability/install-traceability.sh --api
bash deploy/yamale-us/gate/deploy-gate.sh
bash deploy/yamale-us/traceability/install-traceability.sh --app
(cd /d/Sermium/Yamale/yamale-platform && ./deploy.sh)
```

**The order is the point:**
- the gate must be redeployed, so that it mounts `/srv/traceability` and serves `/api/traceability/`, **before** the app is installed;
- the app install's `/j/1/1` probe goes through the gate;
- the old container still has the moved-aside `/srv/loyalty.prev-*` mounted, and would answer from it.

Expected:
- the migration prints `sponsor key renamed: yml143pwcf3z73zctk9r4x5vck2jxfg8354f67fpku`, the same address as before;
- `--api` ends with `sponsor health 200` and `ready`. It does not ask the faucet: the balance carried over;
- the gate's probes show `traceability /api/traceability/v1/health 200`. Its `traceability /` line is 403 until the app is in, as on 2026-11-16;
- `--app` ends with `gate probe /j/1/1: 200` and `ready`;
- the platform deploy ends with `deployed and reloaded`.

- [ ] **Step 3: Verify from outside**

```bash
for p in / /j/1/1 /claim /move-in /api/rpc/status /api/traceability/v1/health; do
  curl -s -o /dev/null -w "%{http_code}  $p\n" "https://traceability.yamaletech.com$p"; done
curl -sI https://traceability.yamaletech.com/ | grep -iE "content-security-policy|referrer-policy|strict-transport|x-frame-options|^server"
for p in /api/traceability/v1/health/sponsor /api/traceability/v1/applications /assets/nope.js /api/loyalty/v1/health; do
  curl -s -o /dev/null -w "%{http_code}  $p (want 404)\n" "https://traceability.yamaletech.com$p"; done
curl -s https://loyalty.yamaletech.com/j/1/1 | grep -c "move.js"
curl -sI https://loyalty.yamaletech.com/move.js | grep -iE "^HTTP|content-type"
curl -sI https://loyalty.yamaletech.com/ | grep -i content-security-policy
ssh yamale-us "systemctl is-active yamale-traceability yamale-traceability-backup.timer; systemctl is-enabled yamale-loyalty 2>&1 | head -1; id yamale-loyalty 2>&1 | head -1"
```

Expected:
- 200 on all six paths;
- the same four security headers as before, with `server: cloudflare`;
- 404 on all four paths in the second loop;
- `1` for the move page's script reference;
- `move.js` served as JavaScript;
- the move site's CSP line printed;
- `active` twice, then "not-found" or "No such file" for the old unit, and "no such user" for the old user.

Then the monitor: read `/status.json` on 127.0.0.1:8096 as on 2026-11-16, and filter names containing `raceab` or `Loyalty move`. Expected: `unit:yamale-traceability` and the four probes, all `ok`.

- [ ] **Step 4: The move, in a browser, with the user's own account**

The account the user made (business `yml1dzwpk73ntdyve2vx49flv0dxxguy6a5ygp2zv7`, Maison Diop) lives in **the user's** browser at the old address. Only the user's browser can carry it. Ask the user to open `https://loyalty.yamaletech.com/` in the same browser they used on 2026-11-16, and to report:
- the address bar ends on `traceability.yamaletech.com/` with no fragment;
- the Account tab shows the same address and asks for the same password;
- the Atelier shows Maison Diop.

Meanwhile, in the built-in browser, which holds no account:
- `https://loyalty.yamaletech.com/j/1/1` must land on `https://traceability.yamaletech.com/j/1/1`;
- `https://loyalty.yamaletech.com/claim#k=test` must land on `https://traceability.yamaletech.com/claim#k=test`, with the fragment kept.

Take a screenshot of each landing.

- [ ] **Step 5: Record in the SDD ledger** what ran, its outputs, the sponsor address, and the user's confirmation.

---

### Task 6: Record it

**Files:**
- Modify: `clients/README.md`, `docs/scope/gaps.md`, `docs/superpowers/specs/2026-11-16-loyalty-kyb-design.md`
- Memory, under `C:\Users\chris\.claude\projects\D--Sermium-Yamale-Blockchain\memory\`: `jewel-module-design.md`, `chain-on-yamale-us.md`, `MEMORY.md`

- [ ] **Step 1: `clients/README.md`**

Rename the loyalty section to traceability, with:
- the new paths and commands (`clients/traceability`, `npm run dev --workspace @yamale/traceability`, `scripts/devnet/traceability-local.sh`);
- the line "Public: https://traceability.yamaletech.com (devnet-2); loyalty.yamaletech.com serves a move page that carries accounts across. Deploy with `deploy/yamale-us/traceability/install-traceability.sh` (`--app` for the app alone); the office admits with `jewel-admit` on yamale-us."

- [ ] **Step 2: `docs/scope/gaps.md`**

In the `x/jewel` row:
- the app and API are now `clients/traceability` and `tools/traceabilityapi`, live at https://traceability.yamaletech.com since 2026-11-17;
- `loyalty.yamaletech.com` serves the move page.

Leave older history wording, such as the branch name `loyalty-app` and the commits, as it is.

Add as open, dated 2026-11-17:
- the proto comment in `events.proto` still says "the loyalty app", pending a regeneration;
- the move page and `/move-in` carry only the account. Saved offer links (`yamale.loyalty.link.*`) are not carried; none existed at the switch;
- keep `loyalty.yamaletech.com` until no account can be left there, and revisit it no earlier than 2027-05-17.

- [ ] **Step 3: The KYB spec**

Apply the same names to `docs/superpowers/specs/2026-11-16-loyalty-kyb-design.md`:
- `loyalty-api` → `traceability-api`;
- paths and units;
- `/etc/yamale/loyalty-*` → `/etc/yamale/traceability-*`;
- `yamale_loyalty_*` → `yamale_traceability_*`;
- `/var/lib/yamale-loyalty` → `/var/lib/yamale-traceability`;
- the host.

`git mv` it to `2026-11-16-traceability-kyb-design.md`, and add one line under its title: "Names updated 2026-11-17 for the rename to traceability."

- [ ] **Step 4: Commit**

```bash
git add clients/README.md docs/scope/gaps.md docs/superpowers/specs
git commit -m "docs: the jewel app is traceability.yamaletech.com; KYB spec renamed with it"
```

- [ ] **Step 5: Memory**

- Update `jewel-module-design.md`: the app's host and names, and the move page.
- Update `chain-on-yamale-us.md`: the `yamale-traceability` unit, user and directories, and the sponsor key name.
- Update their `MEMORY.md` lines if the hooks mention loyalty.
- Add one memory, `app-moves-carry-local-accounts.md`: renaming the host of an app that keeps keys in `localStorage` strands every account, so a 301 is wrong; serve a move page that hands the sealed blob over in the fragment. Link it from `MEMORY.md`.

---

## Self-review (done at writing)

- **Spec coverage:**
  - the user's decisions: address and every name → Tasks 1, 3 and 4; the redirect → Task 4, as a move page;
  - the not-renamed list is explicit;
  - accounts carried → Tasks 2 and 4;
  - live switch → Task 5;
  - docs and memory → Task 6.
- **Names:**
  - the `/move-in` contract is the same string in Task 2 (`acceptMove`, the tests) and Task 4 (`move.js`): `#move=<b64u>&to=<b64u>`;
  - the storage keys are the same strings;
  - the sponsor address is the same in Tasks 3 and 5.
- **Order on the host:** migrate → install `--api` → gate → install `--app` → platform. The gate goes before the app, because the app's probe goes through the gate.
