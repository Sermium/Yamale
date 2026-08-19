import { useQuery } from '@tanstack/react-query';
import {
  describeCase,
  formatCoins,
  requiredPower,
  type EnforcementCase, t,} from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import { AddressLink, Card, Empty, ErrorState, Loading, Meter, RawJson, Stat } from '../components.tsx';

/**
 * Every case this chain has ever opened, including the ones that failed.
 *
 * This page exists because the power it reports on is only defensible if it is
 * watched. A chain that can freeze an account on one validator's word and take
 * its balance on a vote had better show, without anybody having to ask: who was
 * accused, by whom, on what evidence, how the set voted, and what happened to
 * the money. The rejected and withdrawn cases matter most of all — a record
 * that kept only the accusations that succeeded would read like a record of a
 * power that is never misused.
 */
export function EnforcementPage() {
  const { mode } = useViewMode();
  const expert = mode === 'expert';

  const cases = useQuery({
    queryKey: ['enforcement-cases'],
    queryFn: () => client.enforcementCases(),
    refetchInterval: 15_000,
  });
  const totals = useQuery({ queryKey: ['enforcement-totals'], queryFn: () => client.enforcementTotals() });

  if (cases.isPending) return <Loading label="Fetching cases" />;
  if (cases.isError) return <ErrorState error={cases.error} what="enforcement cases" />;

  const list = cases.data ?? [];
  const open = list.filter((c) => c.status === 'voting');
  const resolved = list.filter((c) => c.status !== 'voting');

  return (
    <>
      <h1>{t('exp.enforcement')}</h1>
      <p className="lede">
        A validator can freeze an account the moment they see a theft — money moves in minutes and
        a vote takes hours. Taking anything from it needs two thirds of the validator set, and that
        can only ever go to one address, set by governance. Every case is here, whether it succeeded
        or not.
      </p>

      <div className="stat-row">
        <Stat label="Cases opened" value={totals.data?.casesOpened ?? list.length} />
        <Stat
          label="Cases passed"
          value={totals.data?.casesPassed ?? resolved.filter((c) => c.status === 'passed').length}
          note="the rest were rejected, withdrawn or expired"
        />
        <Stat
          label="Recovered"
          value={totals.data && totals.data.total.length > 0 ? formatCoins(totals.data.total) : 'Nothing'}
          note="sent to the recovery destination"
        />
      </div>

      <Card title={`Open cases${open.length ? ` (${open.length})` : ''}`} flush>
        {open.length === 0 ? (
          <Empty
            title="Nothing is being voted on"
            hint="No account is currently frozen by a case awaiting a decision."
          />
        ) : (
          open.map((c) => <CaseRow key={c.id} enforcementCase={c} expert={expert} />)
        )}
      </Card>

      <Card title={`Closed cases${resolved.length ? ` (${resolved.length})` : ''}`} flush>
        {resolved.length === 0 ? (
          <Empty
            title="No case has been decided yet"
            hint="This is where cases stay after they pass, fail, expire or are withdrawn — permanently."
          />
        ) : (
          resolved.map((c) => <CaseRow key={c.id} enforcementCase={c} expert={expert} />)
        )}
      </Card>

      {expert && <RawJson value={list} label="Raw cases" />}
    </>
  );
}

const STATUS_LABEL: Record<string, string> = {
  voting: 'Being voted on',
  passed: 'Upheld',
  rejected: 'Refused',
  expired: 'Lapsed',
  withdrawn: 'Withdrawn',
  reversed: 'Overturned',
  unknown: 'Unknown',
};

function CaseRow({ enforcementCase: c, expert }: { enforcementCase: EnforcementCase; expert: boolean }) {
  const needed = requiredPower(c.totalPowerAtOpen);

  return (
    <div className="case-row">
      <div className="case-head">
        <span className={`badge badge-${c.status}`}>{STATUS_LABEL[c.status] ?? c.status}</span>
        <span className="case-action">{c.action === 'seize' ? 'Freeze and seize' : 'Freeze only'}</span>
        {c.emergency && (
          // Worth its own badge. "A validator saw this" and "the founders acted
          // directly" are different facts about how the chain is being run, and
          // an interface that showed only the outcome would hide the second.
          <span className="badge badge-emergency">Founders' emergency</span>
        )}
        <span className="case-id">Case {c.id}</span>
      </div>

      <p className="case-target">
        Against <AddressLink address={c.target} />, opened by <AddressLink address={c.opener} />
        {c.emergency && ' (emergency authority)'}
      </p>

      {/* The grounds, quoted rather than summarised: the accused's own words back
          to them are the point, and paraphrasing an accusation is not this
          interface's job. */}
      <blockquote className="case-reason">{c.reason}</blockquote>

      <p className="case-meaning">{describeCase(c)}</p>

      {c.status === 'voting' && (
        <Meter
          value={needed > 0 ? c.yesPower / needed : 0}
          threshold={1}
          label={`${c.yesPower} of ${needed} voting power needed`}
          caption={`${c.yesPower} of the ${needed} voting power this case needs · ${c.noPower} against`}
          tone={c.yesPower >= needed ? 'bad' : 'neutral'}
        />
      )}

      {c.evidenceUri && (
        <p className="case-evidence">
          Evidence:{' '}
          <a href={c.evidenceUri} target="_blank" rel="noreferrer noopener">
            {c.evidenceUri}
          </a>
          {c.evidenceHash && (
            // The hash is shown next to the link deliberately: it is what makes a
            // document that was edited after the case opened provable, and a link
            // on its own proves nothing.
            <>
              {' '}
              <code className="case-hash">{c.evidenceHash}</code>
            </>
          )}
        </p>
      )}

      {c.recovered.length > 0 && (
        <p className="case-recovered">
          Recovered so far: <strong>{formatCoins(c.recovered)}</strong>
          {!c.sweepComplete && ' — more is still unbonding and will be collected as it matures'}
        </p>
      )}

      {expert && (
        <p className="case-meta">
          opened at height {c.openedAtHeight}
          {c.status === 'voting'
            ? ` · voting ends at ${c.votingEndsAtHeight}`
            : ` · resolved at ${c.resolvedAtHeight}`}
          {' · '}
          yes {c.yesPower} / no {c.noPower} / abstain {c.abstainPower} of {c.totalPowerAtOpen}
        </p>
      )}
    </div>
  );
}
