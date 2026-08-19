import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { formatCoins, formatNumber, timeAgo, type Transaction, t} from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import {
  Amount,
  Card,
  Empty,
  ErrorState,
  Loading,
  MessageIcon,
  RelativeTime,
  Stat,
  StatusBadge,
  TxLink,
} from '../components.tsx';

/**
 * The front page answers a different question in each mode.
 *
 * Simple: "is anything happening, and can I find my payment?" — recent activity
 * as sentences, nothing else.
 *
 * Detailed: "is the chain healthy?" — height, block times, supply, validator
 * count, and the block stream alongside the transactions.
 */
export function HomePage() {
  const { mode } = useViewMode();

  const status = useQuery({ queryKey: ['status'], queryFn: () => client.status() });
  const names = useQuery({ queryKey: ['validatorNames'], queryFn: () => client.validatorNames(), refetchInterval: 60_000 });
  const supply = useQuery({ queryKey: ['supply'], queryFn: () => client.totalSupply(), refetchInterval: 30_000 });
  const blocks = useQuery({ queryKey: ['recentBlocks'], queryFn: () => client.recentBlocks(10) });

  const activity = useQuery({
    queryKey: ['recentActivity', names.data],
    queryFn: () => client.searchTransactions('tx.height>0', 25, { names: names.data }),
  });

  return (
    <>
      {mode === 'simple' ? (
        <>
          <h1>{t('exp.whatHappening')}</h1>
          <p className="lede">
            Recent payments and transfers on the Yamale network. Search above for an account or a payment
            reference to find a specific one.
          </p>
        </>
      ) : (
        <>
          <h1>{status.data?.chainId ?? 'Yamale'}</h1>
          <p className="lede">
            Chain overview, recent blocks and the full transaction stream.
          </p>
        </>
      )}

      {mode === 'expert' ? <ChainStats status={status} supply={supply} /> : null}

      <Card
        title={mode === 'simple' ? 'Recent activity' : 'Transactions'}
        actions={
          status.data ? (
            <span className="small faint">
              block {formatNumber(status.data.latestHeight)} · <RelativeTime value={status.data.latestTime} />
            </span>
          ) : null
        }
        flush
      >
        {activity.isPending ? (
          <Loading label="Fetching recent activity" />
        ) : activity.isError ? (
          <ErrorState error={activity.error} what="recent activity" />
        ) : (
          <ActivityList transactions={activity.data ?? []} mode={mode} />
        )}
      </Card>

      {mode === 'expert' ? (
        <Card title="Recent blocks" flush>
          {blocks.isPending ? (
            <Loading label="Fetching blocks" />
          ) : blocks.isError ? (
            <ErrorState error={blocks.error} what="blocks" />
          ) : (
            <div className="scroll-x">
              <table className="grid">
                <thead>
                  <tr>
                    <th>{t('col.height')}</th>
                    <th>{t('col.time')}</th>
                    <th>{t('col.transactions')}</th>
                    <th>{t('col.hash')}</th>
                  </tr>
                </thead>
                <tbody>
                  {(blocks.data ?? []).map((b) => (
                    <tr key={b.height}>
                      <td>
                        <Link to={`/block/${b.height}`} className="mono">
                          {formatNumber(b.height)}
                        </Link>
                      </td>
                      <td className="muted">{timeAgo(b.timestamp)}</td>
                      <td>{b.txCount === 0 ? <span className="faint">empty</span> : b.txCount}</td>
                      <td className="mono faint small">{b.hash.slice(0, 16)}…</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      ) : null}
    </>
  );
}

function ChainStats({
  status,
  supply,
}: {
  status: ReturnType<typeof useQuery<any>>;
  supply: ReturnType<typeof useQuery<any>>;
}) {
  return (
    <div className="stats" style={{ marginBottom: '1rem' }}>
      <Stat
        label="Block height"
        value={status.data ? formatNumber(status.data.latestHeight) : '—'}
        note={status.data ? <RelativeTime value={status.data.latestTime} /> : null}
      />
      <Stat label="Chain" value={status.data?.chainId ?? '—'} note={status.data ? `node ${status.data.nodeVersion}` : null} />
      <Stat
        label="Total supply"
        value={supply.data ? formatCoins(supply.data, { maxDecimals: 0 }) : '—'}
        note="minted since genesis"
      />
      <Stat
        label="Sync"
        value={status.data ? (status.data.catchingUp ? 'Catching up' : 'In sync') : '—'}
        note={status.data?.catchingUp ? 'data may be behind' : 'live'}
      />
    </div>
  );
}

/**
 * The activity list is where the two explorers most obviously diverge.
 *
 * In simple mode it shows only what the SDK marks as everyday — transfers,
 * payments, staking, treasury movements — because somebody checking whether
 * their rent arrived should not have to scroll past parameter changes and
 * authorisation grants to find it.
 */
function ActivityList({ transactions, mode }: { transactions: Transaction[]; mode: 'simple' | 'expert' }) {
  const rows =
    mode === 'simple'
      ? transactions
          .filter((tx) => tx.succeeded && tx.messages.some((m) => m.everyday))
          .map((tx) => ({ tx, messages: tx.messages.filter((m) => m.everyday) }))
      : transactions.map((tx) => ({ tx, messages: tx.messages }));

  if (rows.length === 0) {
    return (
      <Empty
        title={mode === 'simple' ? 'Nothing has happened yet' : 'No transactions yet'}
        hint={
          mode === 'simple'
            ? 'When money moves on this network, it will show up here.'
            : 'The chain is producing blocks, but no transactions have been included.'
        }
      />
    );
  }

  return (
    <ul className="rows">
      {rows.map(({ tx, messages }) =>
        messages.map((message, i) => (
          <li className="row" key={`${tx.hash}-${i}`}>
            <MessageIcon message={message} failed={!tx.succeeded} />
            <div className="row__main">
              <p className="row__summary">{message.summary}</p>
              <div className="row__meta">
                <RelativeTime value={tx.timestamp} />
                {mode === 'expert' ? (
                  <>
                    {' · '}
                    <TxLink hash={tx.hash} />
                    {' · block '}
                    <Link to={`/block/${tx.height}`}>{formatNumber(tx.height)}</Link>
                    {' · '}
                    {message.typeUrl}
                  </>
                ) : (
                  <>
                    {' · '}
                    <TxLink hash={tx.hash}>details</TxLink>
                  </>
                )}
              </div>
            </div>
            <div className="row__side">
              {tx.succeeded ? <Amount coins={message.coins} /> : <StatusBadge ok={false} />}
            </div>
          </li>
        )),
      )}
    </ul>
  );
}
