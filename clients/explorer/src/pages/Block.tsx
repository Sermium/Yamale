import { useQuery } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';
import { formatNumber, formatTimestamp, timeAgo, t} from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import {
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
 * Blocks are a technical concept, so the simple view treats one as "everything
 * that happened at a moment in time" rather than as a data structure. Somebody
 * only lands here from a link; nothing in the simple explorer sends them.
 */
export function BlockPage() {
  const { height = '' } = useParams();
  const { mode } = useViewMode();
  const heightNum = Number(height);

  const names = useQuery({ queryKey: ['validatorNames'], queryFn: () => client.validatorNames(), refetchInterval: 60_000 });
  const block = useQuery({
    queryKey: ['block', heightNum],
    queryFn: () => client.block(heightNum),
    refetchInterval: false,
  });
  const txs = useQuery({
    queryKey: ['blockTxs', heightNum, names.data],
    queryFn: () => client.transactionsInBlock(heightNum, 50, { names: names.data }),
    refetchInterval: false,
  });

  if (block.isPending) return <Loading label="Fetching block" />;
  if (block.isError) return <ErrorState error={block.error} what="block" />;

  const data = block.data!;

  return (
    <>
      <div className="spread" style={{ marginBottom: '0.75rem' }}>
        <h1 style={{ margin: 0 }}>
          {mode === 'simple' ? 'Activity' : `Block ${formatNumber(data.height)}`}
        </h1>
        <span className="inline small">
          <Link to={`/block/${data.height - 1}`}>← previous</Link>
          <Link to={`/block/${data.height + 1}`}>next →</Link>
        </span>
      </div>

      <p className="lede">
        {mode === 'simple'
          ? `Everything that happened ${timeAgo(data.timestamp)}.`
          : `Produced ${timeAgo(data.timestamp)} with ${data.txCount} transaction${data.txCount === 1 ? '' : 's'}.`}
      </p>

      {mode === 'expert' ? (
        <Card title="Block details">
          <dl className="defs">
            <dt>{t('col.height')}</dt>
            <dd className="mono">{formatNumber(data.height)}</dd>
            <dt>{t('col.timestamp')}</dt>
            <dd>{formatTimestamp(data.timestamp)}</dd>
            <dt>{t('col.hash')}</dt>
            <dd>
              <Copyable value={data.hash} />
            </dd>
            <dt>{t('col.proposer')}</dt>
            <dd className="mono">{data.proposer || <span className="faint">unknown</span>}</dd>
            <dt>{t('col.transactions')}</dt>
            <dd>{data.txCount}</dd>
          </dl>
        </Card>
      ) : null}

      <Card title="Transactions" flush>
        {txs.isPending ? (
          <Loading label="Fetching transactions" />
        ) : txs.isError ? (
          <ErrorState error={txs.error} what="transactions" />
        ) : (txs.data ?? []).length === 0 ? (
          <Empty
            title="This block is empty"
            hint="The network kept producing blocks, but nothing needed recording at this moment."
          />
        ) : (
          <ul className="rows">
            {(txs.data ?? []).map((tx) => {
              const primary = tx.messages[0];
              return (
                <li className="row" key={tx.hash}>
                  {primary ? <MessageIcon message={primary} failed={!tx.succeeded} /> : null}
                  <div className="row__main">
                    <p className="row__summary">{tx.summary}</p>
                    <div className="row__meta">
                      <RelativeTime value={tx.timestamp} /> · <TxLink hash={tx.hash} />
                    </div>
                  </div>
                  <div className="row__side">
                    <StatusBadge ok={tx.succeeded} compact />
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </Card>
    </>
  );
}
