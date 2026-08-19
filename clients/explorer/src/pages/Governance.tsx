import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  assessProposal,
  vote as voteMsg,
  formatCoins,
  formatDuration,
  formatPercent,
  timeAgo,
  timeUntil,
  type GovParams,
  type Proposal,
  type ProposalAssessment,
  type VoteOption, t,} from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import { Card, Empty, ErrorState, Loading, Meter, RawJson } from '../components.tsx';
import { SignAction } from '../wallet.tsx';

/**
 * Governance answers "what is being decided, and does it affect me?"
 *
 * Every proposal leads with what it would actually do, in the same decoded
 * sentences the explorer uses elsewhere. A proposal rendered as its protobuf
 * payload asks the reader to be a developer before they are allowed to have an
 * opinion, which is how governance ends up decided by the few people who can
 * read it.
 *
 * It also leads with whether the thing is passing. A tally alone does not
 * answer that: a proposal at 100% yes still fails if too little of the staked
 * supply voted, and one at 60% yes fails if a third of votes are vetoes. Those
 * rules are the chain's, so the interface applies them rather than showing four
 * numbers and hoping.
 */
export function GovernancePage() {
  const { mode } = useViewMode();

  const proposals = useQuery({
    queryKey: ['proposals'],
    queryFn: () => client.proposals(50),
    refetchInterval: 30_000,
  });
  const params = useQuery({ queryKey: ['gov-params'], queryFn: () => client.govParams() });
  const staking = useQuery({ queryKey: ['staking'], queryFn: () => client.stakingOverview() });

  if (proposals.isPending) return <Loading label="Fetching proposals" />;
  if (proposals.isError) return <ErrorState error={proposals.error} what="proposals" />;

  const all = proposals.data ?? [];
  const open = all.filter((p) => p.status === 'voting');
  const raising = all.filter((p) => p.status === 'deposit');
  const decided = all.filter((p) => p.status !== 'voting' && p.status !== 'deposit');

  const govParams = params.data;
  const bonded = staking.data?.bonded ?? '0';
  const expert = mode === 'expert';

  const render = (p: Proposal) => (
    <ProposalRow
      key={p.id}
      proposal={p}
      assessment={govParams ? assessProposal(p, govParams, bonded) : null}
      params={govParams}
      expert={expert}
    />
  );

  return (
    <>
      <h1>{t('exp.governance')}</h1>
      <p className="lede">
        Changes to the chain — its settings, and who is allowed to validate, issue currency or route
        payments — are decided by a vote of everyone who has staked.
      </p>

      {govParams ? <Rules params={govParams} bonded={bonded} /> : null}

      <Card title={`Open for voting${open.length ? ` (${open.length})` : ''}`} flush>
        {open.length === 0 ? (
          <Empty
            title="Nothing is being voted on"
            hint="When somebody proposes a change, it will appear here with the deadline to vote and how close it is to passing."
          />
        ) : (
          <ul className="rows">{open.map(render)}</ul>
        )}
      </Card>

      {raising.length > 0 ? (
        <Card title={`Raising a deposit (${raising.length})`} flush>
          <ul className="rows">{raising.map(render)}</ul>
        </Card>
      ) : null}

      <Card title="Decided" flush>
        {decided.length === 0 ? <Empty title="No past proposals" /> : <ul className="rows">{decided.map(render)}</ul>}
      </Card>
    </>
  );
}

/**
 * The rules, stated once at the top rather than repeated per proposal.
 *
 * People read these once and then read outcomes; repeating "needs 33.4%" on
 * every row would crowd out the thing that actually differs between them.
 */
function Rules({ params, bonded }: { params: GovParams; bonded: string }) {
  return (
    <Card>
      <p className="small muted" style={{ margin: 0 }}>
        To pass, a proposal needs <strong>{formatPercent(params.quorum, 1)}</strong> of all staked tokens to
        vote, and more than <strong>{formatPercent(params.threshold, 0)}</strong> of those votes to be yes.
        More than <strong>{formatPercent(params.vetoThreshold, 1)}</strong> vetoes rejects it regardless.
        {params.votingPeriodSeconds > 0 ? (
          <>
            {' '}
            Voting runs for <strong>{formatDuration(params.votingPeriodSeconds)}</strong>.
          </>
        ) : null}
        {params.minDeposit.length > 0 ? (
          <>
            {' '}
            A proposal needs a <strong>{formatCoins(params.minDeposit)}</strong> deposit before it reaches a
            vote.
          </>
        ) : null}
        {bonded !== '0' ? null : ' Nothing is staked yet, so no vote can reach quorum.'}
      </p>
    </Card>
  );
}

function ProposalRow({
  proposal,
  assessment,
  params,
  expert,
}: {
  proposal: Proposal;
  assessment: ProposalAssessment | null;
  params: GovParams | undefined;
  expert: boolean;
}) {
  const badge =
    proposal.status === 'passed'
      ? 'badge badge--ok'
      : proposal.status === 'rejected' || proposal.status === 'failed'
        ? 'badge badge--bad'
        : proposal.status === 'voting'
          ? 'badge badge--warn'
          : 'badge';

  return (
    <li className="row">
      <div className="row__main">
        <div className="spread" style={{ gap: '0.6rem', marginBottom: '0.2rem' }}>
          <strong>
            <span className="faint mono small">#{proposal.id}</span> {proposal.title}
          </strong>
          <span className={badge}>{proposal.statusLabel}</span>
        </div>

        {proposal.summary ? (
          <p className="small muted" style={{ margin: '0 0 0.4rem' }}>
            {proposal.summary}
          </p>
        ) : null}

        {proposal.actions.length > 0 ? (
          <div className="small" style={{ marginBottom: '0.4rem' }}>
            <span className="muted">If it passes: </span>
            {proposal.actions.join('; ')}
          </div>
        ) : null}

        {proposal.status === 'voting' && proposal.votingEndsAt ? (
          <div className="small muted">Voting: {timeUntil(proposal.votingEndsAt)}</div>
        ) : proposal.status === 'deposit' && proposal.depositEndsAt ? (
          <div className="small muted">Deposit period: {timeUntil(proposal.depositEndsAt)}</div>
        ) : (
          <div className="small faint">Submitted {timeAgo(proposal.submittedAt)}</div>
        )}

        {assessment ? (
          <Assessment proposal={proposal} assessment={assessment} params={params} />
        ) : proposal.totalVoted !== '0' ? (
          <Tally proposal={proposal} />
        ) : (
          <div className="small faint" style={{ marginTop: '0.4rem' }}>
            No votes cast yet.
          </div>
        )}

        {proposal.status === 'voting' ? <VoteButtons proposal={proposal} /> : null}

        {expert ? <RawJson value={proposal.raw} label="Raw proposal" /> : null}
      </div>
    </li>
  );
}

/**
 * Voting, one click from the list.
 *
 * All four options are shown rather than hiding abstain and veto behind a menu:
 * they mean genuinely different things here — abstaining counts toward quorum
 * without opposing, vetoing rejects outright — and an interface that offers
 * only yes and no misrepresents the vote somebody is casting.
 */
function VoteButtons({ proposal }: { proposal: Proposal }) {
  const options: Array<{ value: VoteOption; label: string; effect: string }> = [
    { value: 'yes', label: 'Yes', effect: 'in favour' },
    { value: 'no', label: 'No', effect: 'against' },
    { value: 'abstain', label: 'Abstain', effect: 'counted toward quorum, but neither for nor against' },
    { value: 'veto', label: 'Veto', effect: 'against, and rejects the proposal outright if a third of votes agree' },
  ];
  const [choice, setChoice] = useState<VoteOption>('yes');
  const selected = options.find((o) => o.value === choice)!;

  return (
    <div style={{ marginTop: '0.6rem' }}>
      <div className="inline small" style={{ gap: '0.4rem', flexWrap: 'wrap' }}>
        <span className="muted">Vote</span>
        {options.map((o) => (
          <button
            key={o.value}
            type="button"
            className={o.value === choice ? 'badge badge--warn' : 'badge'}
            onClick={() => setChoice(o.value)}
            aria-pressed={o.value === choice}
            style={{ cursor: 'pointer' }}
          >
            {o.label}
          </button>
        ))}
      </div>

      <SignAction
        label={`Vote ${selected.label.toLowerCase()}`}
        consequence={
          <>
            Your vote is weighted by everything you have staked, and counts {selected.effect}. It can be
            changed until voting closes.
          </>
        }
        build={(address) => [voteMsg(address, proposal.id, choice)]}
      />
    </div>
  );
}

function Assessment({
  proposal,
  assessment,
  params,
}: {
  proposal: Proposal;
  assessment: ProposalAssessment;
  params: GovParams | undefined;
}) {
  const live = proposal.status === 'voting';
  const tone = assessment.wouldPass ? 'good' : 'bad';

  return (
    <div style={{ marginTop: '0.5rem' }}>
      <p className={live ? `verdict verdict--${tone}` : 'verdict'}>{assessment.verdict}</p>
      {assessment.blocker ? (
        <p className="small muted" style={{ margin: '0 0 0.4rem' }}>
          {assessment.blocker}
        </p>
      ) : null}

      {proposal.status === 'deposit' ? (
        <Meter
          value={assessment.depositProgress}
          label="Deposit raised"
          caption={
            <>
              {formatCoins(proposal.totalDeposit)} of {formatCoins(params?.minDeposit ?? [])} deposited (
              {formatPercent(assessment.depositProgress, 0)})
            </>
          }
        />
      ) : (
        <>
          {proposal.totalVoted !== '0' ? <Tally proposal={proposal} /> : null}
          <Meter
            value={assessment.quorumProgress}
            threshold={params?.quorum}
            tone={assessment.quorumMet ? 'good' : 'neutral'}
            label="Share of staked tokens that has voted"
            caption={
              <>
                {formatPercent(assessment.quorumProgress, 1)} of staked tokens have voted
                {params ? ` · ${formatPercent(params.quorum, 1)} needed` : ''}
                {assessment.quorumMet ? ' · reached' : ''}
              </>
            }
          />
        </>
      )}
    </div>
  );
}

/**
 * Votes as a single bar. Each segment carries a label as well as a colour,
 * because "is this passing?" should not depend on distinguishing green from
 * red.
 */
function Tally({ proposal }: { proposal: Proposal }) {
  const segments = [
    { key: 'yes', label: 'Yes', value: proposal.tally.yes, colour: 'var(--positive)' },
    { key: 'no', label: 'No', value: proposal.tally.no, colour: 'var(--negative)' },
    { key: 'veto', label: 'Veto', value: proposal.tally.veto, colour: 'var(--warning)' },
    { key: 'abstain', label: 'Abstain', value: proposal.tally.abstain, colour: 'var(--text-faint)' },
  ].filter((s) => s.value > 0);

  if (segments.length === 0) {
    return (
      <div className="small faint" style={{ marginTop: '0.4rem' }}>
        No votes cast yet.
      </div>
    );
  }

  return (
    <div style={{ marginTop: '0.5rem' }}>
      <div
        style={{ display: 'flex', height: 8, borderRadius: 4, overflow: 'hidden', background: 'var(--surface-2)' }}
        role="img"
        aria-label={segments.map((s) => `${s.label} ${formatPercent(s.value, 0)}`).join(', ')}
      >
        {segments.map((s) => (
          <div key={s.key} style={{ width: `${s.value * 100}%`, background: s.colour }} />
        ))}
      </div>
      <div className="small muted" style={{ marginTop: '0.25rem' }}>
        {segments.map((s) => `${s.label} ${formatPercent(s.value, 0)}`).join(' · ')}
      </div>
    </div>
  );
}
