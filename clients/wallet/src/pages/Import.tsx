import { t } from '@yamale/chain';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { DirectSecp256k1HdWallet } from '@cosmjs/proto-signing';

import { createVault, vaultSummary } from '../vault.ts';

/**
 * Loading an existing account from its recovery phrase.
 *
 * The counterpart to Create, and the screen people reach on their worst day —
 * new laptop, lost phone, cleared browser. So it does the checking *before*
 * asking for a password: deriving the address first means somebody can confirm
 * they typed the right phrase by recognising the address, rather than finding
 * out after they have committed a password to it.
 *
 * The phrase is validated by actually deriving from it. BIP-39 carries a
 * checksum, so a single mistyped or transposed word fails here rather than
 * silently producing a valid-but-different empty account — which is the failure
 * that makes people think their money is gone.
 */
export function ImportPage() {
  const [phrase, setPhrase] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [derived, setDerived] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  const existing = vaultSummary();
  const words = phrase.trim().split(/\s+/).filter(Boolean);
  const countLooksRight = words.length === 12 || words.length === 24;

  async function check() {
    setBusy(true);
    setError(null);
    setDerived(null);
    try {
      const wallet = await DirectSecp256k1HdWallet.fromMnemonic(words.join(' '), {
        prefix: 'yml',
      });
      const [account] = await wallet.getAccounts();
      setDerived(account!.address);
    } catch {
      setError(
        'That phrase is not valid. Check for a mistyped or swapped word — the phrase carries a checksum, which is what just failed.',
      );
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    setBusy(true);
    setError(null);
    try {
      await createVault(words.join(' '), password);
      setDone(true);
      setPhrase('');
      setPassword('');
      setConfirm('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save.');
    } finally {
      setBusy(false);
    }
  }

  if (done && derived) {
    return (
      <>
        <h1>{t('import.loaded')}</h1>
        <section className="card">
          <p>
            <code className="address">{derived}</code> is on this device, encrypted under your
            password.
          </p>
          <p>
            <Link to={`/a/${derived}`}>Watch this account →</Link> ·{' '}
            <Link to="/faucet">Get test funds →</Link>
          </p>
        </section>
      </>
    );
  }

  return (
    <>
      <h1>{t('import.title')}</h1>
      <p className="lede">
        Restore an account you already have, from its 12 or 24 word recovery phrase.
      </p>

      {/* Said before the phrase is typed, not after. A vault is overwritten in
          place, and somebody restoring a second account on a device that already
          holds one would otherwise destroy the first with no warning and no
          undo. */}
      {existing && (
        <div className="notice notice--bad">
          <strong>This device already holds “{existing.label}”</strong> ({existing.address}).
          Loading another account replaces it. If you do not have that one's phrase written down,
          it cannot be recovered afterwards.
        </div>
      )}

      <section className="card">
        <h2>{t('import.phrase')}</h2>
        <label className="field">
          <span>The words, in order, separated by spaces</span>
          <textarea
            value={phrase}
            onChange={(e) => {
              setPhrase(e.target.value);
              setDerived(null);
            }}
            rows={4}
            spellCheck={false}
            autoComplete="off"
            placeholder="nuclear thrive identify time …"
          />
        </label>
        <p className="small muted">
          {words.length === 0
            ? 'Nobody legitimate will ever ask you for this phrase.'
            : `${words.length} words${countLooksRight ? '' : ' — expected 12 or 24'}`}
        </p>
        {error && <div className="notice notice--bad">{error}</div>}
        {!derived && (
          <button type="button" onClick={check} disabled={!countLooksRight || busy}>
            {busy ? 'Checking…' : 'Check this phrase'}
          </button>
        )}
      </section>

      {derived && (
        <section className="card">
          <h2>{t('import.rightAccount')}</h2>
          <p>
            <code className="address">{derived}</code>
          </p>
          <p className="small muted">
            If that is not the address you expected, the phrase is valid but not the one you meant —
            go back and check the words rather than continuing.
          </p>

          <label className="field">
            <span>Choose a password for this device</span>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </label>
          <label className="field">
            <span>Again</span>
            <input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
          </label>
          {password !== '' && password.length < 10 && (
            <p className="notice">
              Ten characters at least. This password is the only thing between anyone using this
              computer and your money.
            </p>
          )}
          {confirm !== '' && password !== confirm && (
            <p className="notice notice--bad">{t('msg.noMatch')}</p>
          )}
          <button
            type="button"
            onClick={save}
            disabled={busy || password.length < 10 || password !== confirm}
          >
            {busy ? 'Saving…' : 'Load this account'}
          </button>
        </section>
      )}
    </>
  );
}
