import { useQuery } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import {
  formatAmount,
  formatCoins,
  resolveDenom,
  timeUntil,
  type Coin,
  type Commitment,
  type Transaction,
} from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import {
  Amount,
  Card,
  Copyable,
  Empty,
  ErrorState,
  Loading,
  MessageIcon,
  RelativeTime,
  StatusBadge,
  TxLink,
} from '../components.tsx';

/**
 * An account page answers "what do I have, and what has happened to it?"
 *
 * Both explorers lead with the balance, because that is the question. They
 * differ in what counts as history: the simple view shows money in and out with
 * direction made obvious, the detailed view shows every transaction the account
 * signed regardless of whether value moved.
 */
export function AccountPage() {
  const { address = '' } = useParams();
  const { mode } = useViewMode();

  const names = useQuery({ queryKey: ['validatorNames'], queryFn: () => client.validatorNames(), refetchInterval: 60_000 });
  const balances = useQuery({ queryKey: ['balances', address], queryFn: () => client.balances(address) });
  const commitments = useQuery({
    queryKey: ['commitments', address],
    queryFn: () => client.commitmentsTo(address),
    refetchInterval: 30_000,
  });

  const sent = useQuery({
    queryKey: ['sent', address, names.data],
    queryFn: () => client.transactionsSentBy(address, 25, { names: names.data }),
  });
  const received = useQuery({
    queryKey: ['received', address, names.data],
    queryFn: () => client.transactionsReceivedBy(address, 25, { names: names.data }),
  });

  const history = mergeHistory(sent.data ?? [], received.data ?? []);
  const loading = sent.isPending || received.isPending;

  return (
    <>
      <h1>{mode === 'simple' ? 'Account' : 'Account detail'}</h1>
      <p className="lede mono" style={{ overflowWrap: 'anywhere' }}>
        <Copyable value={address} />
      </p>

      <Card title={mode === 'simple' ? 'Balance' : 'Balances'}>
        {balances.isPending ? (
          <Loading label="Fetching balance" />
        ) : balances.isError ? (
          <ErrorState error={balances.error} what="balance" />
        ) : (balances.data ?? []).length === 0 ? (
          <Empty
            title="This account is empty"
            hint="It holds nothing right now. It may be new, or everything may have been sent on."
          />
        ) : (
          <BalanceList balances={balances.data ?? []} detailed={mode === 'expert'} />
        )}
      </Card>

      <Commitments list={commitments.data ?? []} expert={mode === 'expert'} />

      <Card title={mode === 'simple' ? 'Money in and out' : 'Transactions'} flush>
        {loading ? (
          <Loading label="Fetching history" />
        ) : sent.isError && received.isError ? (
          <ErrorState error={sent.error} what="history" />
        ) : history.length === 0 ? (
          <Empty
            title="No activity yet"
            hint="Nothing has been sent to or from this account."
          />
        ) : (
          <History entries={history} address={address} mode={mode} />
        )}
      </Card>
    </>
  );
}

/**
 * Money a treasury has committed to this account.
 *
 * It appears in no balance, which is exactly why it has to appear here.
 * Committed funds have left the treasury's spendable balance and have not yet
 * arrived in the beneficiary's, so somebody with a vesting grant who looks at
 * their account sees nothing at all — at the moment they most want to know
 * where they stand.
 */
function Commitments({ list, expert }: { list: Commitment[]; expert: boolean }) {
  const active = list.filter((c) => c.active);
  if (active.length === 0) return null;

  return (
    <Card title="Committed to this account">
      <p className="small muted" style={{ marginTop: 0 }}>
        A treasury has set these aside for this account. Nobody — not an administrator, not a vote — can
        spend them elsewhere. They become claimable over time.
      </p>

      <ul className="rows">
        {active.map((c) => {
          const claimable = BigInt(c.claimable) > 0n;
          return (
            <li key={c.id} className="row">
              <div className="row__main">
                <div className="spread" style={{ gap: '0.6rem' }}>
                  <strong>{formatAmount(c.total, c.denom)} committed</strong>
                  <span className={claimable ? 'badge badge--ok' : 'badge'}>
                    {claimable ? `${formatAmount(c.claimable, c.denom)} claimable now` : 'Nothing claimable yet'}
                  </span>
                </div>

                <p className="small muted" style={{ margin: '0.25rem 0 0' }}>
                  {c.vesting ? 'Vesting over time' : 'Released in full later'}
                  {c.endTime > 0 ? ` · fully released ${timeUntil(new Date(c.endTime * 1000))}` : ''}
                  {' · '}
                  {formatAmount(c.released, c.denom)} already taken
                  {c.revocable ? ' · the treasury may still cancel what has not vested' : ''}
                </p>

                {expert ? (
                  <p className="small faint mono" style={{ margin: '0.25rem 0 0' }}>
                    lock #{c.id} · treasury #{c.treasuryId} · remaining {c.remaining} {c.denom}
                  </p>
                ) : null}
              </div>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

function BalanceList({ balances, detailed }: { balances: Coin[]; detailed: boolean }) {
  return (
    <dl className="defs">
      {balances.map((coin) => {
        const info = resolveDenom(coin.denom);
        return (
          <div key={coin.denom} style={{ display: 'contents' }}>
            <dt>{info.name}</dt>
            <dd>
              <strong>{formatAmount(coin.amount, coin.denom)}</strong>
              {detailed ? (
                <span className="faint mono small">
                  {' '}
                  ({coin.amount} {coin.denom})
                </span>
              ) : null}
            </dd>
          </div>
        );
      })}
    </dl>
  );
}

interface HistoryEntry {
  tx: Transaction;
  /** Whether this account was on the receiving end. */
  incoming: boolean;
}

/**
 * The chain indexes sent and received separately, so an account's history is
 * the union of two queries. Deduplicating matters: a self-transfer, or a
 * treasury payment where the signer is also the recipient, appears in both.
 */
function mergeHistory(sent: Transaction[], received: Transaction[]): HistoryEntry[] {
  const byHash = new Map<string, HistoryEntry>();

  for (const tx of received) byHash.set(tx.hash, { tx, incoming: true });
  for (const tx of sent) {
    // Being the signer is the stronger statement about what happened, so it
    // wins when a transaction appears on both sides.
    byHash.set(tx.hash, { tx, incoming: false });
  }

  return [...byHash.values()].sort((a, b) => b.tx.height - a.tx.height);
}

function History({
  entries,
  address,
  mode,
}: {
  entries: HistoryEntry[];
  address: string;
  mode: 'simple' | 'expert';
}) {
  const rows =
    mode === 'simple'
      ? entries.filter((e) => e.tx.succeeded && e.tx.messages.some((m) => m.everyday))
      : entries;

  if (rows.length === 0) {
    return (
      <Empty
        title="Nothing to show here"
        hint="This account has activity, but none of it involved money moving. Switch to Detailed to see everything."
      />
    );
  }

  return (
    <ul className="rows">
      {rows.map(({ tx, incoming }) => {
        const messages = mode === 'simple' ? tx.messages.filter((m) => m.everyday) : tx.messages;
        const primary = messages[0] ?? tx.messages[0];
        if (!primary) return null;

        return (
          <li className="row" key={tx.hash}>
            <MessageIcon message={primary} failed={!tx.succeeded} />
            <div className="row__main">
              <p className="row__summary">{tx.summary}</p>
              <div className="row__meta">
                <RelativeTime value={tx.timestamp} />
                {' · '}
                <TxLink hash={tx.hash}>{mode === 'expert' ? undefined : 'details'}</TxLink>
                {mode === 'expert' ? ` · ${primary.typeUrl}` : null}
              </div>
            </div>
            <div className="row__side">
              {tx.succeeded ? (
                <Amount coins={primary.coins} direction={incoming ? 'in' : 'out'} />
              ) : (
                <StatusBadge ok={false} />
              )}
            </div>
          </li>
        );
      })}
    </ul>
  );
}
