import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { describeCase, requiredPower, truncateAddress, t} from '@yamale/chain';

import { client } from '../chain.ts';

/**
 * The operations console: what a validator or the founders' group has to act on.
 *
 * Folded into the Safe rather than built as a fourth application, because a
 * validator voting on a case and a treasurer approving a spend are the same
 * interaction over different messages — the same signers, the same group
 * policies, the same "read this carefully before you sign".
 *
 * Every action here produces a **message, not a transaction**. Nothing is
 * signed by this page. The authority-gated ones are executed through an x/group
 * proposal, and a validator's vote is signed with their own key wherever they
 * keep it — which is not a browser. Showing the exact payload is therefore the
 * product, not a debugging aid: whoever approves it should be able to read what
 * they are approving.
 */
export function OperationsPage() {
  const cases = useQuery({
    queryKey: ['open-cases'],
    queryFn: () => client.enforcementCases(),
    refetchInterval: 15_000,
  });

  const open = (cases.data ?? []).filter((c) => c.status === 'voting');

  return (
    <>
      <h1>{t('safe.operations')}</h1>
      <p className="lede">
        What is waiting on a decision, and the exact messages that make one. Nothing here signs
        anything — copy the payload into your signing setup or an x/group proposal.
      </p>

      <h2 className="section">Waiting on a vote</h2>
      {cases.isPending ? (
        <p className="muted">Reading cases…</p>
      ) : open.length === 0 ? (
        <p className="muted">
          No open enforcement cases. Nothing is frozen pending a decision.
        </p>
      ) : (
        open.map((c) => {
          const needed = requiredPower(c.totalPowerAtOpen);
          return (
            <section className="card" key={c.id}>
              <div className="case-head">
                <span className="badge badge-voting">{t('safe.beingVotedOn')}</span>
                <span className="case-action">
                  {c.action === 'seize' ? 'Freeze and seize' : 'Freeze only'}
                </span>
                {c.emergency && <span className="badge badge-emergency">Founders' emergency</span>}
                <span className="case-id">Case {c.id}</span>
              </div>

              <p className="case-target">
                Against {truncateAddress(c.target)}, opened by {truncateAddress(c.opener)}
              </p>
              <blockquote className="case-reason">{c.reason}</blockquote>
              <p className="case-meaning">{describeCase(c)}</p>
              <p className="small muted">
                {c.yesPower} of the {needed} voting power it needs · {c.noPower} against · voting
                ends at block {c.votingEndsAtHeight.toLocaleString()}
              </p>

              <VoteBuilder caseId={c.id} />
            </section>
          );
        })
      )}

      <h2 className="section">Start something</h2>
      <ActionBuilder />
    </>
  );
}

/**
 * A vote, prefilled.
 *
 * Three buttons rather than a form, because the only variable is the choice and
 * a validator at three in the morning should not be assembling JSON. Abstain is
 * offered as an equal: it counts towards nothing, and exists so a validator can
 * record that they saw the case and declined to judge it.
 */
function VoteBuilder({ caseId }: { caseId: string }) {
  const [option, setOption] = useState<'yes' | 'no' | 'abstain' | null>(null);

  const message = option && {
    '@type': '/blockchain.enforcement.v1.MsgVoteCase',
    voter: '<your validator account, yml1…>',
    case_id: caseId,
    option: `VOTE_OPTION_${option.toUpperCase()}`,
  };

  return (
    <div className="actions">
      <div className="actions__row">
        {(['yes', 'no', 'abstain'] as const).map((choice) => (
          <button
            key={choice}
            type="button"
            className={option === choice ? 'chip chip--on' : 'chip'}
            onClick={() => setOption(choice)}
          >
            {choice === 'yes' ? 'Uphold' : choice === 'no' ? 'Refuse' : 'Abstain'}
          </button>
        ))}
      </div>

      {message && (
        <>
          <p className="small muted">
            Sign this with the validator's own key. `voter` is the account the validator signs with
            — the chain reads its operator address and voting power from the staking module.
          </p>
          <pre className="payload__pre">{JSON.stringify(message, null, 2)}</pre>
        </>
      )}
    </div>
  );
}

type Action = 'freeze' | 'seize' | 'emergency-freeze' | 'emergency-release';

/**
 * The prefilled proposal builders.
 *
 * Each one states who may sign it and what it does before showing the payload,
 * because the difference between these four is the difference between stopping
 * money for a day and taking it permanently — and that difference is invisible
 * in the JSON to anybody who does not already know the message names.
 */
function ActionBuilder() {
  const [action, setAction] = useState<Action>('freeze');
  const [target, setTarget] = useState('');
  const [reason, setReason] = useState('');
  const [caseId, setCaseId] = useState('');
  const [evidenceUri, setEvidenceUri] = useState('');
  const [evidenceHash, setEvidenceHash] = useState('');

  const needsEvidence = action === 'seize';
  const needsCase = action === 'emergency-release';

  const message = build(action, {
    target: target || '<address, yml1…>',
    reason,
    caseId: caseId || '<case id>',
    evidenceUri,
    evidenceHash,
  });

  return (
    <section className="card">
      <div className="actions__row">
        {(
          [
            ['freeze', 'Freeze'],
            ['seize', 'Freeze and seize'],
            ['emergency-freeze', "Founders' freeze"],
            ['emergency-release', "Founders' release"],
          ] as [Action, string][]
        ).map(([key, label]) => (
          <button
            key={key}
            type="button"
            className={action === key ? 'chip chip--on' : 'chip'}
            onClick={() => setAction(key)}
          >
            {label}
          </button>
        ))}
      </div>

      <p className="case-meaning">{explain(action)}</p>

      {needsCase ? (
        <label className="field">
          <span>Case to release</span>
          <input value={caseId} onChange={(e) => setCaseId(e.target.value)} placeholder="4" />
        </label>
      ) : (
        <label className="field">
          <span>Address</span>
          <input value={target} onChange={(e) => setTarget(e.target.value)} placeholder="yml1…" />
        </label>
      )}

      <label className="field">
        <span>Grounds — in words the accused can read</span>
        <input
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="drained the YML/NGN pool at block 812,400"
        />
      </label>

      {needsEvidence && (
        <div className="field-row">
          <label className="field">
            <span>Evidence link</span>
            <input
              value={evidenceUri}
              onChange={(e) => setEvidenceUri(e.target.value)}
              placeholder="https://…/report.pdf"
            />
          </label>
          <label className="field">
            <span>SHA-256 of it</span>
            <input
              value={evidenceHash}
              onChange={(e) => setEvidenceHash(e.target.value)}
              placeholder="9f2c0e1b…"
            />
          </label>
        </div>
      )}

      {/* Refused here rather than by the chain. A seizure proposal that reached
          the signers without evidence would collect approvals and then fail at
          execution, which reads as a chain fault rather than as the rule it is. */}
      {needsEvidence && (!evidenceUri || !evidenceHash) && (
        <div className="notice notice--bad">
          A seizure needs both an evidence link and its hash. The chain refuses one without them,
          and it is better to find that out now than after collecting signatures.
        </div>
      )}
      {!reason.trim() && (
        <div className="notice">
          Every case must state its grounds. Acting in an emergency is not a reason to leave the
          record blank — it is the reason the record matters.
        </div>
      )}

      <pre className="payload__pre">{JSON.stringify(message, null, 2)}</pre>
      <p className="small muted">{howToSign(action)}</p>
    </section>
  );
}

function explain(action: Action): string {
  switch (action) {
    case 'freeze':
      return 'Stops the account sending, in the block this lands. Nothing is taken. The freeze expires by itself unless the validators confirm it, and lifts the moment they refuse.';
    case 'seize':
      return 'Freezes now and, if two thirds of the validators agree, sends the balance to the recovery destination. Irreversible: what is taken can only be returned by whoever controls that address.';
    case 'emergency-freeze':
      return "The founders' group stopping an account without waiting for a validator to open a case. It is still provisional — it lapses unless the set confirms it, and the validators can refuse it.";
    case 'emergency-release':
      return "The founders' group lifting a freeze immediately, whoever imposed it. It does not return anything already seized — those funds are the recovery destination's to send back.";
  }
}

function howToSign(action: Action): string {
  switch (action) {
    case 'freeze':
    case 'seize':
      return 'Signed by a bonded validator with its own account key: blockchaind tx enforcement open-case …';
    default:
      return "Signed by the founders' group policy, so it goes inside an x/group proposal: tx group submit-proposal, two votes, then exec.";
  }
}

function build(
  action: Action,
  f: { target: string; reason: string; caseId: string; evidenceUri: string; evidenceHash: string },
) {
  const grounds = f.reason || '<the grounds, in plain words>';

  switch (action) {
    case 'freeze':
    case 'seize':
      return {
        '@type': '/blockchain.enforcement.v1.MsgOpenCase',
        opener: '<your validator account, yml1…>',
        target: f.target,
        action: action === 'seize' ? 'CASE_ACTION_SEIZE' : 'CASE_ACTION_FREEZE',
        reason: grounds,
        evidence_uri: f.evidenceUri,
        evidence_hash: f.evidenceHash,
      };
    case 'emergency-freeze':
      return {
        '@type': '/blockchain.enforcement.v1.MsgEmergencyFreeze',
        authority: "<the founders' group policy address>",
        target: f.target,
        reason: grounds,
        evidence_uri: f.evidenceUri,
        evidence_hash: f.evidenceHash,
      };
    case 'emergency-release':
      return {
        '@type': '/blockchain.enforcement.v1.MsgEmergencyRelease',
        authority: "<the founders' group policy address>",
        case_id: f.caseId,
        reason: grounds,
      };
  }
}
