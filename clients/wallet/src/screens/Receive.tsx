/**
 * How money gets in.
 *
 * Two answers, and they are different in kind rather than in degree: somebody
 * else pays you, which needs them to know what to address the payment to; or
 * the devnet faucet hands you something worthless to test with. They share a
 * screen because the question "how do I get money into this" has those two
 * answers and no third one, and because separating them put the faucet in the
 * navigation of a wallet whose users mostly are not testers.
 */
import { useState } from 'react';
import { formatUserId, resolveDenom, t } from '@yamale/chain';
import { useQuery } from '@tanstack/react-query';

import { client } from '../chain.ts';
import { Identifier } from '../Identifier.tsx';
import { ClaimUserId } from '../ClaimUserId.tsx';
import { CLAIMABLE, CLAIM_GROUPS, type Denom } from '../claimable.ts';
import { vaultSummary } from '../vault.ts';
import { Amount, Back, Note, ScreenTitle } from '../ui.tsx';

export function ReceiveScreen({ onAttach }: { onAttach: () => void }) {
  const [faucet, setFaucet] = useState(false);
  const account = vaultSummary();

  const userId = useQuery({
    queryKey: ['userId', account?.address],
    queryFn: () => client.userIdOf(account!.address),
    enabled: Boolean(account?.address),
  });

  if (!account) {
    return (
      <>
        <ScreenTitle lede={t('wal.receiveNoAccount')}>{t('action.receive')}</ScreenTitle>
        <button type="button" className="wal-btn" onClick={onAttach}>{t('wal.attachTitle')}</button>
      </>
    );
  }

  if (faucet) return <Faucet address={account.address} label={account.label} onBack={() => setFaucet(false)} />;

  return (
    <>
      <ScreenTitle lede={t('wal.receiveLede')}>{t('action.receive')}</ScreenTitle>

      {/* The user ID first, because it is the thing a payer can read back down
          a telephone and the thing that carries a check character. The address
          is underneath: correct, unreadable, and the only one that always
          exists. */}
      <h2 className="y-screen__sub">{t('wal.yourUserId')}</h2>
      <div className="y-card y-card--money">
        {userId.isPending ? (
          <p className="wal-muted">{t('acct.reading')}</p>
        ) : userId.data ? (
          <>
            <p className="wal-idBig y-mono">{formatUserId(userId.data)}</p>
            <p className="wal-small wal-muted">{t('wal.userIdBody')}</p>
          </>
        ) : (
          <ClaimUserId address={account.address} label={account.label} />
        )}
      </div>

      <h2 className="y-screen__sub">{t('watch.address')}</h2>
      <div className="y-card">
        <Identifier value={account.address} name={false} />
        <p className="wal-small wal-muted">{t('wal.addressBody')}</p>
      </div>

      {/* No heading above it. The row's own title is the heading — a section
          called "Get test funds" containing one row called "Get test funds"
          said the same thing twice and cost a line of a 390px screen. */}
      <button type="button" className="y-row wal-faucetOpen" onClick={() => setFaucet(true)}>
        <span className="y-row__main">
          <span className="y-row__title">{t('wal.faucetRow')}</span>
          <span className="y-row__sub">{t('wal.faucetRowSub')}</span>
        </span>
        <svg className="y-row__chev" viewBox="0 0 20 20" width="16" height="16" fill="none"
             stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"
             aria-hidden="true"><path d="M8 4 L14 10 L8 16" /></svg>
      </button>
    </>
  );
}

/* ----------------------------------------------------------------- faucet */

/** One grant waits a block so the faucet's transactions cannot collide. */
const SECONDS_PER_GRANT = 7;

type Status =
  | { state: 'idle' }
  | { state: 'busy' }
  | { state: 'sent'; sent: string; txHash: string }
  | { state: 'error'; message: string; retryAfter?: number };

/**
 * Claiming test funds, on a phone.
 *
 * Forty-five currencies do not fit on a 390px screen as a grid of panels, so
 * they are a searchable list of rows grouped by region — the grouping is the
 * answer to "which of these is my country's", which is the question actually
 * being asked.
 *
 * The destination is always this device's account. The desktop page let you
 * type somewhere else, which on a phone is a blank address box shown to
 * somebody whose address the app already knows: an invitation to paste the
 * wrong thing, on the input method worst suited to catching it.
 */
function Faucet({ address, label, onBack }: { address: string; label: string; onBack: () => void }) {
  const [status, setStatus] = useState<Record<string, Status>>({});
  const [allBusy, setAllBusy] = useState(false);
  const [progress, setProgress] = useState(0);
  const [find, setFind] = useState('');

  async function claim(denom: Denom): Promise<void> {
    setStatus((s) => ({ ...s, [denom]: { state: 'busy' } }));
    try {
      const response = await fetch('/api/faucet/', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        // `denom` names an *additional* currency the faucet was configured to
        // hand out. YML is the default grant and is not in that list, so asking
        // for it by name is refused — the field has to be omitted instead.
        body: JSON.stringify(denom === 'uyml' ? { address } : { address, denom }),
      });
      const body = await response.json();
      setStatus((s) => ({
        ...s,
        [denom]: body.error
          ? { state: 'error', message: body.error, retryAfter: body.retry_after }
          : { state: 'sent', sent: body.sent, txHash: body.tx_hash },
      }));
    } catch {
      setStatus((s) => ({ ...s, [denom]: { state: 'error', message: t('wal.faucetUnreachable') } }));
    }
  }

  /**
   * Sequential, not parallel. The faucet holds a lock for one block per grant
   * so its transactions cannot collide on the account sequence — firing
   * forty-five at once would queue on that lock behind forty-five spinners and
   * no progress. One at a time makes the waiting legible.
   */
  async function claimAll() {
    setAllBusy(true);
    for (const [i, denom] of CLAIMABLE.entries()) {
      setProgress(i + 1);
      await claim(denom as Denom);
    }
    setAllBusy(false);
    setProgress(0);
  }

  const needle = find.trim().toLowerCase();
  const minutes = Math.round((CLAIMABLE.length * SECONDS_PER_GRANT) / 60);

  return (
    <>
      <Back onClick={onBack}>{t('action.receive')}</Back>
      <ScreenTitle lede={t('wal.faucetLede')}>{t('faucet.title')}</ScreenTitle>

      <div className="y-card">
        <span className="wal-fact__label">{t('faucet.sendingTo')}</span>
        <p className="wal-row__strong">{label}</p>
        <Identifier value={address} name={false} />
      </div>

      {/* Said before anything is tapped, not discovered halfway through. The
          wait is inherent, so the honest move is to state the cost up front
          rather than leave somebody watching a spinner wondering if it hung. */}
      <button type="button" className="wal-btn" onClick={claimAll} disabled={allBusy}>
        {allBusy
          ? t('wal.faucetClaiming', { done: String(progress), total: String(CLAIMABLE.length) })
          : t('wal.faucetClaimAll', { count: String(CLAIMABLE.length), minutes: String(minutes) })}
      </button>
      <p className="wal-small wal-muted">
        {allBusy
          ? t('wal.faucetLeft', { seconds: String(Math.max(0, (CLAIMABLE.length - progress) * SECONDS_PER_GRANT)) })
          : t('wal.faucetWhySlow')}
      </p>

      <input className="wal-input" value={find} onChange={(e) => setFind(e.target.value)}
             placeholder={t('wal.faucetFind')} spellCheck={false} autoComplete="off"
             aria-label={t('wal.faucetFind')} />

      {CLAIM_GROUPS.map((group) => {
        const rows = group.denoms.filter((d) => {
          if (!needle) return true;
          const info = resolveDenom(d);
          return info.symbol.toLowerCase().includes(needle) || info.name.toLowerCase().includes(needle);
        });
        if (rows.length === 0) return null;
        return (
          <details key={group.id} className="wal-group" open={Boolean(needle) || group.id === CLAIM_GROUPS[0]?.id}>
            <summary>
              {group.title}
              <span className="wal-group__count">{rows.length}</span>
            </summary>
            <ul className="y-cards wal-list">
              {rows.map((d) => (
                <FaucetRow key={d} denom={d as Denom} status={status[d] ?? { state: 'idle' }}
                           disabled={allBusy} onClaim={() => claim(d as Denom)} />
              ))}
            </ul>
          </details>
        );
      })}
    </>
  );
}

function FaucetRow({ denom, status, disabled, onClaim }: {
  denom: Denom; status: Status; disabled: boolean; onClaim: () => void;
}) {
  const info = resolveDenom(denom);
  return (
    <li className="y-card wal-faucetRow">
      <div className="y-card__head">
        <span className="y-row__main">
          <span className="y-row__title">{info.symbol}</span>
          <span className="y-row__sub">{info.name}</span>
        </span>
        <button type="button" className="wal-chip" onClick={onClaim}
                disabled={disabled || status.state === 'busy'}>
          {status.state === 'busy' ? t('wal.faucetAsking') : t('wal.faucetClaim')}
        </button>
      </div>

      {/* The fee point belongs beside the currencies it applies to, and is not
          worth saying under YML — "an account holding only YML can move it
          without holding YML" is what the shared wording produced. */}
      <p className="y-card__note">{denom === 'uyml' ? t('msg.paysFee') : t('msg.feeAllowance')}</p>

      {status.state === 'sent' && (
        <Note tone="ok">
          {/* Through Amount, so the grant is printed at the precision the
              chain moved it at — see the note in ui.tsx about XOF. */}
          <Amount amount={status.sent.replace(/[a-z]+$/, '')} denom={denom} />{' '}
          {t('wal.faucetSent')}
        </Note>
      )}
      {status.state === 'error' && (
        <Note tone="bad">
          {/* The faucet's own message already ends with "Try again in 6h0m0s".
              Appending a humanised wait on top printed it twice, in two
              formats. Strip the machine one, keep the readable one. */}
          {status.message.replace(/\s*Try again in [^.]*\.\s*$/i, '')}
          {status.retryAfter ? ` ${t('wal.faucetRetry', { wait: humanWait(status.retryAfter) })}` : ''}
        </Note>
      )}
    </li>
  );
}

/**
 * A wait, in words somebody can act on.
 *
 * Rounding seconds to minutes produced "try again in 0m" for a 22-second wait,
 * which reads as broken rather than as nearly ready — so each scale keeps a
 * unit that cannot round to zero.
 */
function humanWait(seconds: number): string {
  if (seconds < 60) return t('wal.waitSeconds', { n: String(Math.max(1, Math.round(seconds))) });
  if (seconds < 3600) return t('wal.waitMinutes', { n: String(Math.round(seconds / 60)) });
  return t('wal.waitHours', { n: String(Math.round(seconds / 360) / 10) });
}
