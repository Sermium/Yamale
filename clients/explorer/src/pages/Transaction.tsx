import { useQuery } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';
import {
  formatCoins,
  formatNumber,
  formatPercent,
  formatTimestamp,
  shortTypeUrl,
  timeAgo,
  type DecodedMessage,
  type Transaction, t,} from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import {
  AddressLink,
  Card,
  Copyable,
  ErrorState,
  Loading,
  MessageIcon,
  RawJson,
  StatusBadge,
} from '../components.tsx';

/**
 * A single transaction, told two ways.
 *
 * Simple: did it work, what moved, between whom, when. The hash is present but
 * secondary — it is a receipt number, not the subject.
 *
 * Detailed: all of the above plus gas, fee, memo, every message including the
 * ones the simple view filters out, and the untouched payload.
 */
export function TransactionPage() {
  const { hash = '' } = useParams();
  const { mode } = useViewMode();

  const names = useQuery({ queryKey: ['validatorNames'], queryFn: () => client.validatorNames(), refetchInterval: 60_000 });
  const tx = useQuery({
    queryKey: ['tx', hash, names.data],
    queryFn: () => client.transaction(hash, { names: names.data }),
    // A confirmed transaction never changes, so there is nothing to poll for.
    refetchInterval: false,
  });

  if (tx.isPending) return <Loading label="Looking up the transaction" />;
  if (tx.isError) return <ErrorState error={tx.error} what="transaction" />;

  const data = tx.data!;
  return mode === 'simple' ? <SimpleTransaction tx={data} /> : <ExpertTransaction tx={data} />;
}

function SimpleTransaction({ tx }: { tx: Transaction }) {
  const everyday = tx.messages.filter((m) => m.everyday);
  const shown = everyday.length > 0 ? everyday : tx.messages;

  return (
    <>
      <div className="spread" style={{ marginBottom: '0.75rem' }}>
        <h1 style={{ margin: 0 }}>{tx.succeeded ? 'Confirmed' : 'This did not go through'}</h1>
        <StatusBadge ok={tx.succeeded} />
      </div>

      <p className="lede">
        {tx.summary} — <RelativeWithExact value={tx.timestamp} />
      </p>

      {!tx.succeeded && tx.error ? (
        <div className="notice notice--bad" style={{ marginBottom: '1rem' }}>
          <strong>{tx.error.message}</strong>
          {tx.error.reason ? <p style={{ margin: '0.35rem 0 0' }}>{tx.error.reason}</p> : null}
          {tx.error.nextStep ? (
            <p style={{ margin: '0.35rem 0 0' }} className="small">
              {tx.error.nextStep}
            </p>
          ) : null}
          <p className="small muted" style={{ margin: '0.5rem 0 0' }}>
            No money moved. Anything this transaction would have changed was undone.
          </p>
        </div>
      ) : null}

      <Card flush>
        <ul className="rows">
          {shown.map((message, i) => (
            <li className="row" key={i}>
              <MessageIcon message={message} failed={!tx.succeeded} />
              <div className="row__main">
                <p className="row__summary">{message.summary}</p>
                {message.details && message.details.length > 0 ? (
                  <dl className="defs small" style={{ marginTop: '0.5rem' }}>
                    {message.details
                      .filter((d) => d.value && d.value !== '—')
                      .map((detail) => (
                        <div key={detail.label} style={{ display: 'contents' }}>
                          <dt>{detail.label}</dt>
                          <dd>
                            {detail.address ? <AddressLink address={detail.value} /> : detail.value}
                          </dd>
                        </div>
                      ))}
                  </dl>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      </Card>

      <Card title="Receipt">
        <dl className="defs">
          <dt>When</dt>
          <dd>
            <RelativeWithExact value={tx.timestamp} />
          </dd>
          <dt>{t('term.reference')}</dt>
          <dd>
            <Copyable value={tx.hash} display={`${tx.hash.slice(0, 12)}…`} />
          </dd>
          {tx.memo ? (
            <>
              <dt>{t('col.note')}</dt>
              <dd>{tx.memo}</dd>
            </>
          ) : null}
        </dl>
        <p className="small muted" style={{ marginTop: '0.75rem', marginBottom: 0 }}>
          Switch to <strong>Detailed</strong> above to see the full technical record.
        </p>
      </Card>
    </>
  );
}

function ExpertTransaction({ tx }: { tx: Transaction }) {
  const gasRatio = tx.gasWanted > 0 ? tx.gasUsed / tx.gasWanted : 0;

  return (
    <>
      <div className="spread" style={{ marginBottom: '0.75rem' }}>
        <h1 style={{ margin: 0 }}>Transaction</h1>
        <StatusBadge ok={tx.succeeded} />
      </div>
      <p className="lede mono" style={{ overflowWrap: 'anywhere' }}>
        <Copyable value={tx.hash} />
      </p>

      {!tx.succeeded && tx.error ? (
        <div className="notice notice--bad" style={{ marginBottom: '1rem' }}>
          <strong>
            Failed with code {tx.code}: {tx.error.message}
          </strong>
          <pre className="raw" style={{ marginTop: '0.6rem' }}>
            {tx.error.raw}
          </pre>
        </div>
      ) : null}

      <Card title="Details">
        <dl className="defs">
          <dt>{t('col.block')}</dt>
          <dd>
            <Link to={`/block/${tx.height}`} className="mono">
              {formatNumber(tx.height)}
            </Link>
          </dd>
          <dt>{t('col.timestamp')}</dt>
          <dd>
            {formatTimestamp(tx.timestamp)} <span className="faint">({timeAgo(tx.timestamp)})</span>
          </dd>
          <dt>{t('col.resultCode')}</dt>
          <dd className="mono">{tx.code}</dd>
          <dt>{t('term.fee')}</dt>
          <dd>{formatCoins(tx.fee)}</dd>
          <dt>{t('col.gas')}</dt>
          <dd>
            {formatNumber(tx.gasUsed)} used of {formatNumber(tx.gasWanted)}{' '}
            <span className="faint">({formatPercent(gasRatio, 1)})</span>
          </dd>
          <dt>{t('col.memo')}</dt>
          <dd>{tx.memo || <span className="faint">none</span>}</dd>
          <dt>{t('col.messages')}</dt>
          <dd>{tx.messages.length}</dd>
        </dl>
      </Card>

      {tx.messages.map((message, i) => (
        <ExpertMessage key={i} index={i} message={message} />
      ))}

      <Card title="Raw transaction">
        <p className="small muted" style={{ marginTop: 0 }}>
          Exactly what the node returned, before any interpretation.
        </p>
        <pre className="raw">{JSON.stringify(tx.raw, null, 2)}</pre>
      </Card>
    </>
  );
}

function ExpertMessage({ message, index }: { message: DecodedMessage; index: number }) {
  return (
    <Card
      title={
        <span className="inline">
          <span className="faint mono">#{index}</span> {shortTypeUrl(message.typeUrl)}
        </span>
      }
      actions={<span className="badge">{message.kind}</span>}
    >
      <p style={{ marginTop: 0 }}>{message.summary}</p>

      <dl className="defs">
        <dt>Type URL</dt>
        <dd className="mono">{message.typeUrl}</dd>
        {message.actor ? (
          <>
            <dt>{t('col.signer')}</dt>
            <dd>
              <AddressLink address={message.actor} />
            </dd>
          </>
        ) : null}
        {message.counterparty ? (
          <>
            <dt>{t('col.counterparty')}</dt>
            <dd>
              <AddressLink address={message.counterparty} />
            </dd>
          </>
        ) : null}
        {message.coins && message.coins.length > 0 ? (
          <>
            <dt>Value</dt>
            <dd>{formatCoins(message.coins)}</dd>
          </>
        ) : null}
        {(message.details ?? []).map((detail) => (
          <div key={detail.label} style={{ display: 'contents' }}>
            <dt>{detail.label}</dt>
            <dd>{detail.address ? <AddressLink address={detail.value} /> : detail.value}</dd>
          </div>
        ))}
      </dl>

      <RawJson value={message.raw} label="Raw message" />
    </Card>
  );
}

function RelativeWithExact({ value }: { value: string }) {
  return (
    <time dateTime={value} title={formatTimestamp(value)}>
      {timeAgo(value)}
    </time>
  );
}
