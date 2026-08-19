import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { describeThroughput, finality, headroom, t} from '@yamale/chain';

import { client } from '../chain.ts';
import { Card, ErrorState, Loading, Stat } from '../components.tsx';

/**
 * What this chain does, and what it is doing right now.
 *
 * Two halves, and the order matters. The measurements come first because they
 * are checkable — every figure is computed from block headers this node served
 * seconds ago, so a reader who doubts the page can verify it. The capability
 * list comes second, and each entry carries its own live count.
 *
 * Nothing here is a claim. A capability with nothing behind it says "none yet"
 * rather than being quietly omitted: on a new network that is the true and more
 * useful statement, and hiding it would make the whole page the kind of feature
 * grid nobody believes.
 */
export function CapabilitiesPage() {
  // Two rhythms, because the two halves move at different speeds. Block times
  // shift within a minute and are worth re-reading; the number of currencies on
  // the chain does not change while somebody reads a paragraph, and polling it
  // every thirty seconds spends a node's bandwidth to redraw the same figure.
  const performance = useQuery({
    queryKey: ['performance'],
    queryFn: () => client.performance(),
    refetchInterval: 20_000,
    staleTime: 15_000,
  });
  const capabilities = useQuery({
    queryKey: ['capabilities'],
    queryFn: () => client.capabilities(),
    refetchInterval: 120_000,
    staleTime: 60_000,
  });

  if (performance.isPending) return <Loading label="Measuring the chain" />;
  if (performance.isError) return <ErrorState error={performance.error} what="chain measurements" />;

  const p = performance.data;
  const c = capabilities.data;

  return (
    <>
      <h1>{t('exp.whatChainDoes')}</h1>
      <p className="lede">
        Every figure on this page is measured from blocks this node just served, or counted from
        its state. Nothing here is a specification.
      </p>

      {p && (
        <>
          <div className="stat-row">
            <Stat
              label="Block time"
              value={`${p.blockSeconds.toFixed(2)}s`}
              note={`median of ${p.sampled} blocks · ${p.fastestSeconds.toFixed(2)}–${p.slowestSeconds.toFixed(2)}s`}
            />
            <Stat
              label="Settlement"
              value={finality(p.blockSeconds).split('—')[0]!.trim()}
              note="one block, final — no confirmations to wait for"
            />
            <Stat
              label="Blocks per day"
              value={p.blocksPerDay.toLocaleString()}
              note="at the observed rate"
            />
          </div>

          <Card title="Throughput">
            {p.idle ? (
              // The honest version. A chain with no traffic has a block time
              // and no throughput, and printing "0.0 TPS" beside a capability
              // claim would be lying by layout.
              <p className="muted">
                Nothing was submitted during the {p.sampled} blocks measured, so there is no
                throughput to report. The interval above is the chain keeping time, not the chain
                working.
              </p>
            ) : (
              <p>
                <strong>{p.transactions.toLocaleString()}</strong> transactions in the last{' '}
                {p.sampled} blocks — <strong>{describeThroughput(p)}</strong>.
              </p>
            )}

            <Headroom blockSeconds={p.blockSeconds} transactions={p.transactions} blocks={p.sampled} />
          </Card>
        </>
      )}

      <h2 className="section">Supported natively</h2>
      <p className="muted">
        Built into the chain itself — not contracts deployed on top, which means the rules cannot be
        redeployed differently tomorrow.
      </p>

      <div className="grid">
        <Capability
          title="Payments"
          count={c ? `${c.participants} approved institutions` : undefined}
          empty="No institutions approved yet."
        >
          Credit transfers shaped like ISO 20022. The end-to-end reference and purpose code are
          first-class fields, so a payment arrives already matched to what it was for. Only approved
          institutions may route them, and only for customers they have registered.
        </Capability>

        <Capability
          title="Currencies"
          count={c ? `${c.currencies} issuable, ${c.pricedCurrencies} priced` : undefined}
          empty="No currencies registered yet."
        >
          Fiat-referenced tokens with exactly one approved issuer each — including every ISO 4217
          currency in use across Africa. Only that issuer can mint or redeem, and the permission is a
          governance decision on the chain rather than a setting in a config file.
        </Capability>

        <Capability
          title="Treasuries"
          count={c ? `${c.treasuries} open` : undefined}
          empty="No treasuries yet."
        >
          Shared funds with roles, spending limits and time locks. Money committed to somebody — a
          vesting grant, a scheduled disbursement — leaves the spendable balance entirely and cannot
          be redirected by a later proposal, even one that clears the signing threshold.
        </Capability>

        <Capability
          title="Prices"
          count={c ? `${c.pricedCurrencies} agreed rates` : undefined}
          to="/prices"
          empty="No rates agreed yet."
        >
          Exchange rates agreed by the validators and weighted by stake, so moving one costs as much
          as attacking the network. Every rate carries its age, and one too old to trust is refused
          rather than quietly returned.
        </Capability>

        <Capability
          title="Trading"
          count={c ? `${c.pools} liquidity pools` : undefined}
          to="/trade"
          empty="No pools yet — anyone may open one."
        >
          Liquidity pools anyone may open or add to, so a holder of one currency can move into
          another without leaving the network. Swaps carry a slippage floor the trader sets, and it
          is a floor rather than a hint.
        </Capability>

        <Capability
          title="Recovery"
          count={c ? `${c.enforcementCases} cases opened` : undefined}
          to="/enforcement"
          empty="No cases opened."
        >
          A validator can freeze a stolen balance in the block they see it move. Taking it needs two
          thirds of the validator set and can only ever go to one address set by governance. Every
          case is public, including the ones that failed.
        </Capability>

        <Capability
          title="Governance"
          count={c ? `${c.proposals} proposals` : undefined}
          to="/governance"
          empty="No proposals yet."
        >
          Who may validate, who may issue, who may route payments — each is a vote of everyone who
          has staked, with the decision and its reasoning kept next to what it authorised.
        </Capability>

        <Capability title="Sponsored fees">
          An institution can pay the network fee for its customers, so somebody holding only their
          own currency can still transact. The allowance is a capped budget with an optional expiry,
          revocable at any moment.
        </Capability>
      </div>

      <p className="small muted foot-note">
        Measured continuously. If a number here disagrees with what you see elsewhere, the chain is
        right and this page is stale — reload it.
      </p>
    </>
  );
}

/**
 * How much of the block interval the work actually took.
 *
 * Framed as spare capacity rather than as a throughput headline, because the
 * honest claim is "execution is not the constraint here" — the interval is a
 * deliberate wait, and saying "capable of N thousand TPS" from that would be
 * arithmetic dressed up as a benchmark.
 */
function Headroom({
  blockSeconds,
  transactions,
  blocks,
}: {
  blockSeconds: number;
  transactions: number;
  blocks: number;
}) {
  const perBlock = blocks > 0 ? transactions / blocks : 0;
  const { usedSeconds, sharePercent } = headroom(blockSeconds, perBlock);

  return (
    <p className="small muted">
      Executing a block of this size takes roughly{' '}
      <strong>{(usedSeconds * 1000).toFixed(0)} ms</strong> — about{' '}
      {sharePercent < 1 ? 'under 1%' : `${sharePercent.toFixed(0)}%`} of the{' '}
      {blockSeconds.toFixed(1)}-second interval. The rest is the chain waiting on purpose, so
      execution is nowhere near the limit.
    </p>
  );
}

function Capability({
  title,
  count,
  to,
  empty,
  children,
}: {
  title: string;
  count?: string;
  /** Where in this explorer somebody can go and look at it, if anywhere. */
  to?: string;
  empty?: string;
  children: React.ReactNode;
}) {
  // A count of zero is shown as the empty sentence rather than "0", because
  // "No pools yet — anyone may open one" tells somebody what to do and "0"
  // tells them the feature is broken.
  const isEmpty = count !== undefined && /^0\b/.test(count);

  return (
    <article className="card capability">
      <h3>{title}</h3>
      {count !== undefined && (
        <p className={isEmpty ? 'capability__count capability__count--none' : 'capability__count'}>
          {isEmpty ? (empty ?? 'None yet.') : count}
        </p>
      )}
      <p>{children}</p>
      {/* Linked only where this explorer actually has somewhere to go. A "learn
          more" that lands on the same page is worse than no link. */}
      {to && (
        <p className="small">
          <Link to={to} className="capability__link">
            See it →
          </Link>
        </p>
      )}
    </article>
  );
}
