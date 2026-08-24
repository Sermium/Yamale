/**
 * The liveness strip: the answer to the first question this audience asks.
 *
 * A central bank opens an explorer to answer two things — is the chain healthy,
 * and what happened to this money. The second one had a page; the first one had
 * `block 94,652 · just now` in 10px grey in the corner of a card, which is the
 * one live fact on the whole surface rendered as a footnote.
 *
 * So it is furniture now: on every route, above everything, at a size that
 * matches its importance. Four figures — height, block time, validators, bonded
 * stake — and a state that is carried in **form as well as number**, because
 * "the chain has stopped" has to read at a glance and not from arithmetic on a
 * timestamp. The state has a word, a glyph and a border weight; never a colour
 * alone.
 *
 * Every figure is measured rather than configured. The block time is the median
 * of the last twenty gaps, not the value in the genesis file — a chain
 * *configured* for five-second blocks and *producing* one a minute is precisely
 * the condition this strip exists to make visible.
 */

import { useQuery } from '@tanstack/react-query';
import {
  formatNumber,
  formatPercent,
  t,
  timeAgo,
  type StakingOverview,
} from '@yamale/chain';

import { client, probeRest } from './chain.ts';
import { assessHealth, faultTolerance, type ChainHealth, type ChainState } from './health.ts';
import { displayAmount, shareOf } from './amount.ts';

/** The word for each state, and the shape that carries it without colour. */
const STATE_WORD: Record<ChainState, string> = {
  live: 'xp.health.live',
  slow: 'xp.health.slow',
  stopped: 'xp.health.stopped',
  'catching-up': 'xp.health.catchingUp',
  unreachable: 'xp.health.unreachable',
  unknown: 'xp.health.unknown',
};

/**
 * A distinct glyph per state.
 *
 * Not a traffic light in three colours: roughly one man in twelve cannot
 * separate the green from the red reliably, and "has the payments network
 * halted" is not a question to answer in hue.
 */
const STATE_GLYPH: Record<ChainState, string> = {
  live: '●',
  slow: '◐',
  stopped: '■',
  'catching-up': '◑',
  unreachable: '✕',
  unknown: '○',
};

export function StatusStrip() {
  // The RPC answers while the chain is halted, which is exactly when this has to
  // work, so height and time come from there rather than from a state query.
  const status = useQuery({
    queryKey: ['status'],
    queryFn: () => client.status(),
    refetchInterval: 5000,
    retry: false,
  });

  // Measured, one round trip: CometBFT returns twenty block metas per call.
  const performance = useQuery({
    queryKey: ['performance'],
    queryFn: () => client.performance(20),
    refetchInterval: 20_000,
    retry: false,
  });

  const staking = useQuery({
    queryKey: ['stakingOverview'],
    queryFn: () => client.stakingOverview(),
    refetchInterval: 30_000,
    retry: false,
  });

  // Whether state queries are answering, and from what height. The RPC being up
  // says nothing about this: the node's REST sits behind a deny-by-default gate
  // here, and a misconfigured one looks exactly like a halted chain.
  const rest = useQuery({
    queryKey: ['restProbe'],
    queryFn: () => probeRest(),
    refetchInterval: 15_000,
    retry: false,
  });

  const health: ChainHealth = assessHealth({
    status: status.data ?? null,
    blockSeconds: performance.data?.blockSeconds ?? null,
    stalledAt: rest.data?.stalledAt ?? null,
  });

  const state: ChainState = status.isPending && !status.data ? 'unknown' : health.state;

  return (
    <div className={`strip strip--${state}`}>
      <div className="strip__inner">
        <div className="strip__state">
          <span className="strip__glyph" aria-hidden="true">
            {STATE_GLYPH[state]}
          </span>
          <span className="strip__word">{t(STATE_WORD[state])}</span>
          <span className="strip__chain y-mono">{status.data?.chainId ?? '—'}</span>
        </div>

        <dl className="strip__figures">
          <Figure
            label={t('xp.status.height')}
            value={health.height === null ? '—' : formatNumber(health.height)}
            note={
              health.ageSeconds === null
                ? t('xp.health.noBlockTime')
                : t('xp.health.lastBlock', { age: timeAgo(status.data?.latestTime ?? '') })
            }
          />
          <Figure
            label={t('xp.status.blockTime')}
            value={
              performance.data
                ? t('xp.status.seconds', { value: performance.data.blockSeconds.toFixed(1) })
                : '—'
            }
            note={
              performance.data
                ? t('xp.status.median', { count: String(performance.data.sampled) })
                : undefined
            }
          />
          <Validators overview={staking.data} />
          <Bonded overview={staking.data} />
        </dl>
      </div>

      <StateBanner health={health} state={state} />
    </div>
  );
}

function Figure({
  label,
  value,
  note,
  tone,
}: {
  label: string;
  value: string;
  note?: string;
  tone?: 'warn';
}) {
  return (
    <div className={tone ? `figure figure--${tone}` : 'figure'}>
      <dt className="y-label">{label}</dt>
      <dd className="figure__value y-num">{value}</dd>
      {note ? <dd className="figure__note">{note}</dd> : null}
    </div>
  );
}

/**
 * The validator count, and what it survives.
 *
 * A count on its own is a number nobody can act on. Losing one node stopped this
 * chain when it had three validators, and the reason — two thirds of voting
 * power is what a commit needs, and stake here is not evenly held — is not
 * something a reader should have to reconstruct.
 */
function Validators({ overview }: { overview: StakingOverview | undefined }) {
  if (!overview) return <Figure label={t('xp.status.validators')} value="—" />;

  const shares = overview.validators.map((v) => v.votingPower);
  const { spare, fragile } = faultTolerance(shares);
  const concentrated = overview.validators.some((v) => v.concerningPower);

  return (
    <Figure
      label={t('xp.status.validators')}
      value={formatNumber(overview.validators.length)}
      note={
        fragile
          ? t('xp.status.fragile')
          : concentrated
            ? t('xp.status.concentrated')
            : t('xp.status.spare', { count: String(spare) })
      }
      tone={fragile || concentrated ? 'warn' : undefined}
    />
  );
}

function Bonded({ overview }: { overview: StakingOverview | undefined }) {
  // The supply, for the share. Not taken from `overview.bondedRatio`: the SDK
  // computes that with an integer division by ten thousand before converting to
  // a number, which quantises every share to four decimal places and turns this
  // chain's 0.0175% into 0.010%.
  const supply = useQuery({
    queryKey: ['supply'],
    queryFn: () => client.totalSupply(),
    refetchInterval: 60_000,
    retry: false,
  });

  if (!overview) return <Figure label={t('xp.status.bonded')} value="—" />;

  // Base units, converted at the edge and never through a float: bonded stake on
  // a national chain is the figure most likely to exceed what a double holds.
  const amount = displayAmount(overview.bonded, 'uyml');
  const issued = supply.data?.find((c) => c.denom === 'uyml');
  const share = issued ? shareOf(overview.bonded, issued.amount) : null;

  return (
    <Figure
      label={t('xp.status.bonded')}
      value={`${amount.value} ${amount.symbol}`}
      note={share === null ? undefined : t('xp.status.ofSupply', { percent: sharePercent(share) })}
    />
  );
}

/**
 * A share, printed to enough places to be non-zero.
 *
 * Measured on this chain: 174,900 YML of 997,766,566 is 0.0175%, which at one
 * decimal place prints as "0.0% of supply" — a figure that reads as *nothing is
 * staked* when the truth is *very little is*. Those are different statements and
 * only one of them is true.
 */
function sharePercent(ratio: number): string {
  const percent = ratio * 100;
  if (percent >= 1) return formatPercent(ratio, 1);
  if (percent >= 0.1) return formatPercent(ratio, 2);
  return formatPercent(ratio, 3);
}

/**
 * Said on every screen while something is wrong, because every figure below it
 * is historical and none of them look it.
 */
function StateBanner({ health, state }: { health: ChainHealth; state: ChainState }) {
  if (state === 'live' || state === 'unknown') return null;

  if (state === 'unreachable') {
    return (
      <Banner tone="bad" title={t('xp.unreachable.title')} body={t('xp.unreachable.body')} />
    );
  }

  if (state === 'catching-up') {
    return (
      <Banner
        tone="warn"
        title={t('xp.catchingUp.title')}
        body={t('xp.catchingUp.body', { height: formatNumber(health.height ?? 0) })}
      />
    );
  }

  if (state === 'slow') return null;

  // Stopped. Two different sentences, because the two situations have different
  // consequences: a node pinned to a past height is serving accurate history,
  // and a node whose tip has simply gone cold is serving the tip.
  return (
    <Banner
      tone="bad"
      title={t('xp.stopped.title')}
      body={
        health.readingHistory
          ? t('xp.stopped.body', { height: formatNumber(health.stalledAt ?? 0) })
          : t('xp.stopped.bodyPlain', {
              age: health.ageSeconds === null ? '—' : timeAgo(new Date(Date.now() - health.ageSeconds * 1000)),
            })
      }
    />
  );
}

function Banner({ tone, title, body }: { tone: 'warn' | 'bad'; title: string; body: string }) {
  return (
    <div className={`strip__banner strip__banner--${tone}`} role="status">
      <span className="strip__banner-glyph" aria-hidden="true">
        {tone === 'bad' ? '■' : '◐'}
      </span>
      <p>
        <strong>{title}</strong> {body}
      </p>
    </div>
  );
}
