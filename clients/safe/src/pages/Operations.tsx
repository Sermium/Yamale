import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { describeCase, formatNumber, requiredPower, t } from '@yamale/chain';

import { client } from '../chain.ts';
import { Address } from '../Address.tsx';
import { Kpi, KpiBand } from '../ui.tsx';

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
  const seizures = open.filter((c) => c.action === 'seize').length;
  const read = cases.isSuccess;

  return (
    <>
      <h1>{t('safe.operations')}</h1>
      <p className="lede">{t('sd.ops.lede')}</p>

      {/* The count before the list, because "is anything waiting on me" is the
          question this page is opened with and it used to be answerable only by
          scrolling to the bottom of however many cards there were. A seizure is
          split out from a freeze: one stops money for a day and the other takes
          it permanently, and a single count of "cases" hides that difference. */}
      <KpiBand>
        <Kpi
          label={t('sd.kpi.openCases')}
          // Not zero when the node is silent: "nothing is waiting" and "we could
          // not ask" are opposite instructions to a signer.
          value={read ? open.length : '—'}
          hint={read ? t('sd.kpi.openCasesHint') : t('sd.kpi.unread')}
          tone={cases.isError ? 'bad' : open.length > 0 ? 'warn' : undefined}
        />
        <Kpi
          label={t('sd.kpi.seizures')}
          value={read ? seizures : '—'}
          hint={read ? t('sd.kpi.seizuresHint') : t('sd.kpi.unread')}
          tone={cases.isError ? 'bad' : seizures > 0 ? 'bad' : undefined}
        />
      </KpiBand>

      <h2 className="section">{t('sd.ops.waiting')}</h2>
      {cases.isPending ? (
        <section className="card" aria-busy="true">
          <div className="skeleton"><i /><i /></div>
          <p className="small muted" role="status">{t('sd.ops.reading')}…</p>
        </section>
      ) : cases.isError ? (
        <section className="card">
          <h2>{t('safe.cannotReach')}</h2>
          <p className="muted">{t('sd.ops.unknownBody')}</p>
          <p>
            <button type="button" className="chip" onClick={() => cases.refetch()}>
              {t('sd.ops.retry')}
            </button>
          </p>
        </section>
      ) : open.length === 0 ? (
        <section className="empty">
          <h2>{t('sd.ops.nothingTitle')}</h2>
          <p>{t('sd.ops.nothingBody')}</p>
        </section>
      ) : (
        open.map((c) => {
          const needed = requiredPower(c.totalPowerAtOpen);
          return (
            <section className="card" key={c.id}>
              <div className="case-head">
                <span className="badge badge-voting">{t('safe.beingVotedOn')}</span>
                <span className="case-action">
                  {c.action === 'seize' ? t('sd.ops.seizeAction') : t('sd.ops.freezeOnly')}
                </span>
                {c.emergency && (
                  <span className="badge badge-emergency">{t('sd.ops.emergency')}</span>
                )}
                <span className="case-id">{t('sd.ops.caseN', { id: c.id })}</span>
              </div>

              {/* Labelled rows rather than "Against X, opened by Y" assembled
                  from three fragments around two components. A sentence built
                  that way cannot be reordered by a translator, and the word
                  order is not the same in French or in Arabic. It also puts
                  the accused and the accuser at the same weight inside one
                  sentence, when the first is the subject of the case and the
                  second is provenance. The full sentence is still what a
                  screen reader is given. */}
              <dl className="case-parties">
                <span className="visually-hidden">
                  {t('sd.ops.against', { target: c.target, opener: c.opener })}
                </span>
                <div aria-hidden="true">
                  <dt>{t('sd.ops.target')}</dt>
                  <dd><Address address={c.target} /></dd>
                </div>
                <div aria-hidden="true">
                  <dt>{t('sd.ops.openedBy')}</dt>
                  <dd><Address address={c.opener} /></dd>
                </div>
              </dl>
              <blockquote className="case-reason">{c.reason}</blockquote>
              <p className="case-meaning">{describeCase(c)}</p>
              {/* Progress as a bar as well as a pair of figures. Whether a
                  vote is close is a shape, not a subtraction, and two
                  six-figure integers side by side are not a comparison anybody
                  performs by eye.

                  These are consensus voting power, not an amount of YML —
                  x/staking has already divided the stake by the power
                  reduction — so they are grouped as plain integers rather than
                  run through formatAmount, which would put a currency symbol
                  on a number that is not money. */}
              <div
                className="tally"
                role="img"
                aria-label={t('sd.ops.tallyAlt', {
                  yes: formatNumber(c.yesPower),
                  needed: formatNumber(needed),
                })}
              >
                <i style={{ inlineSize: `${share(c.yesPower, needed)}%` }} />
              </div>
              {/* One sentence from the catalogue rather than five fragments
                  around four <span>s. The fragment form pinned the English word
                  order into the JSX, so no translation of it could ever be
                  right — and the figures inside it were formatted with
                  toLocaleString against the browser's locale rather than the
                  one the reader chose in this app. */}
              <p className="small muted y-num">
                {t('sd.ops.tally', {
                  yes: formatNumber(c.yesPower),
                  needed: formatNumber(needed),
                  no: formatNumber(c.noPower),
                  height: formatNumber(c.votingEndsAtHeight),
                })}
              </p>

              <VoteBuilder caseId={c.id} />
            </section>
          );
        })
      )}

      {/* Below this line the copy is deliberately still English.
          ActionBuilder's content is a proto payload, its field names, and the
          `blockchaind tx …` command that carries it — all of which are English
          identifiers a validator types verbatim. Translating the prose around
          them while the nouns it refers to stay in English produces a sentence
          that names nothing on the screen. The reader-facing part of this page
          above is translated; the operator's payload builder is a CLI surface
          and stays in the CLI's language. */}
      <h2 className="section">{t('sd.ops.start')}</h2>
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

/**
 * How far a tally has come, as a percentage of what it needs.
 *
 * Capped at 100: a bar that overflows its track reads as a rendering fault
 * rather than as a threshold already met, and a case can pass its threshold
 * before the last vote is counted.
 */
function share(have: number, needed: number): number {
  if (!Number.isFinite(have) || !Number.isFinite(needed) || needed <= 0) return 0;
  return Math.max(0, Math.min(100, (have / needed) * 100));
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
