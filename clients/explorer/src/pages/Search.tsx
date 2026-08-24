import { Navigate, Link, useSearchParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { formatUserId, resolveDenom, t } from '@yamale/chain';

import { client } from '../chain.ts';
import { classifySearch } from '../search.ts';
import { displayAmount } from '../amount.ts';
import { useDenomRegistry } from '../Feed.tsx';
import { Card, Empty, ErrorState, Loading } from '../components.tsx';

/**
 * One destination for the search box.
 *
 * Everything the box accepts arrives here, and this page decides what it was.
 * Two reasons it is not done in the header: half the kinds need the chain's own
 * lists to recognise at all — a currency code, a validator's name — and a user
 * ID has to be resolved to an address before there is anywhere to go. Neither
 * can happen synchronously in a submit handler, and both need somewhere to show
 * a pending state and a failure.
 *
 * The three kinds decidable from the string alone still redirect immediately, so
 * pasting an address is as fast as it was.
 */
export function SearchPage() {
  const [params] = useSearchParams();
  const term = params.get('q') ?? '';

  const registry = useDenomRegistry();

  const monikers = useQuery({
    queryKey: ['validatorNames'],
    queryFn: () => client.validatorNames(),
    staleTime: 60_000,
  });

  // Classified twice on purpose. The first pass needs nothing and answers for
  // the shapes that are self-describing; the second waits for the chain's lists
  // and answers for the ones that are not. Waiting for the lists before the
  // first pass would put a spinner in front of a pasted address.
  const quick = classifySearch(term);
  const listsReady = registry !== undefined && monikers.data !== undefined;
  const guess =
    quick.kind === 'unknown' && listsReady
      ? classifySearch(term, { registry, monikers: monikers.data })
      : quick;

  if (guess.kind === 'address') return <Navigate to={`/account/${guess.value}`} replace />;
  if (guess.kind === 'tx') return <Navigate to={`/tx/${guess.value}`} replace />;
  if (guess.kind === 'height') return <Navigate to={`/block/${guess.value}`} replace />;
  if (guess.kind === 'empty') return <Navigate to="/" replace />;

  return (
    <>
      <h1>{t('xp.search.looking', { term })}</h1>

      {guess.kind === 'userId' ? <UserIdResult id={guess.value} /> : null}
      {guess.kind === 'denom' ? <DenomResult denom={guess.value} /> : null}
      {guess.kind === 'validator' ? (
        <ValidatorResult operator={guess.value} moniker={monikers.data?.[guess.value]} />
      ) : null}
      {guess.kind === 'unknown' ? (
        listsReady ? (
          <NothingMatched />
        ) : (
          <Card flush>
            <Loading label={t('xp.search.looking', { term })} />
          </Card>
        )
      ) : null}
    </>
  );
}

/**
 * A user ID is the only identifier most people on this chain have ever seen, and
 * the old search box rejected every one of them.
 */
function UserIdResult({ id }: { id: string }) {
  const lookup = useQuery({
    queryKey: ['addressOfUserId', id],
    queryFn: () => client.addressOfUserId(id),
    retry: false,
  });

  if (lookup.isPending) {
    return (
      <Card flush>
        <Loading label={formatUserId(id)} />
      </Card>
    );
  }
  if (lookup.isError) return <ErrorState error={lookup.error} what="that user ID" />;
  if (lookup.data) return <Navigate to={`/account/${lookup.data}`} replace />;

  // The ID is well formed and its check character agrees, so this is a real
  // answer about the chain rather than a rejected input: nobody holds it.
  return (
    <Card>
      <Empty
        title={t('xp.search.noAccountForId', { id: formatUserId(id) })}
        hint={t('xp.search.idNeverRepointed')}
      />
    </Card>
  );
}

function DenomResult({ denom }: { denom: string }) {
  const registry = useDenomRegistry();
  const info = resolveDenom(denom, registry);

  const supply = useQuery({
    queryKey: ['supply'],
    queryFn: () => client.totalSupply(),
    staleTime: 30_000,
  });

  const held = supply.data?.find((c) => c.denom === denom);
  const amount = held ? displayAmount(held.amount, denom, registry) : null;

  return (
    <Card title={`${info.name} · ${info.symbol}`}>
      <dl className="defs">
        <dt>{t('xp.search.currency')}</dt>
        <dd>
          {info.name} (<span className="y-mono">{info.symbol}</span>)
        </dd>
        <dt>{t('xp.search.storedAs')}</dt>
        <dd className="y-mono">
          {info.base} · {t('xp.search.decimals', { count: String(info.exponent) })}
        </dd>
        <dt>{t('xp.search.supply')}</dt>
        <dd className="y-num y-mono">
          {supply.isPending
            ? '…'
            : amount
              ? `${amount.value} ${amount.symbol}`
              : t('xp.search.noneIssued')}
        </dd>
      </dl>
      <p style={{ marginTop: '1rem' }}>
        <Link to="/prices">{t('xp.search.viewPrices')}</Link>
      </p>
    </Card>
  );
}

function ValidatorResult({ operator, moniker }: { operator: string; moniker?: string }) {
  const staking = useQuery({
    queryKey: ['stakingOverview'],
    queryFn: () => client.stakingOverview(),
    staleTime: 30_000,
  });

  const validator = staking.data?.validators.find((v) => v.operatorAddress === operator);

  return (
    <Card title={moniker ?? validator?.moniker ?? t('xp.search.validator')}>
      <dl className="defs">
        <dt>{t('xp.search.operator')}</dt>
        <dd>
          <span className="y-mono y-addr">{operator}</span>
        </dd>
        {validator ? (
          <>
            <dt>{t('col.shareOfStake')}</dt>
            <dd className="y-num">{(validator.votingPower * 100).toFixed(2)}%</dd>
            <dt>{t('col.theyKeep')}</dt>
            <dd className="y-num">{(validator.commission * 100).toFixed(1)}%</dd>
            <dt>{t('col.status')}</dt>
            <dd>
              {validator.jailed ? (
                <span className="y-chip y-chip--bad">{t('xp.search.jailed')}</span>
              ) : (
                <span className="y-chip y-chip--ok">{t('xp.search.signing')}</span>
              )}
            </dd>
          </>
        ) : null}
      </dl>
      <p style={{ marginTop: '1rem' }}>
        <Link to="/staking">{t('xp.search.viewValidator')}</Link>
      </p>
    </Card>
  );
}

/**
 * The empty state that matters most, because it is the one somebody reaches
 * when they are already lost.
 *
 * The old copy was "That does not look like an account, a transaction or a block
 * number" — true, and no help at all. This says what the box does take, with an
 * example of the one form nobody can guess.
 */
function NothingMatched() {
  return (
    <Card>
      <div className="state">
        <div className="state__title">{t('xp.search.notFound')}</div>
        <p className="small" style={{ margin: '0.5rem auto 0', maxWidth: '46ch' }}>
          {t('xp.search.notFoundHint')}
        </p>
        <ul className="search-kinds">
          <li>{t('xp.search.kindAddress')}</li>
          <li>{t('xp.search.kindUserId')}</li>
          <li>{t('xp.search.kindTx')}</li>
          <li>{t('xp.search.kindHeight')}</li>
          <li>{t('xp.search.kindDenom')}</li>
          <li>{t('xp.search.kindValidator')}</li>
        </ul>
      </div>
    </Card>
  );
}
