/**
 * What is on this device, what is on the chain, and who is holding it.
 *
 * Three questions and they are deliberately on one screen, because on a field
 * device they are the same question asked three ways: can I still work, has
 * what I did got out, and am I still allowed to do it. An operator opens this
 * once in the morning and once in the evening.
 *
 * # The queue is the part that must not lie
 *
 * `x/herd` has no message for a biometric enrolment and none for an
 * identification claim. §8 proposes both; neither is on the chain this talks
 * to. So the queue has two lists and not one:
 *
 *   WAITING TO SEND   things the chain will accept, held only by the radio.
 *   HELD ON THIS DEVICE   things there is nowhere to send yet.
 *
 * A single list would show a capture as "waiting" forever — a progress bar with
 * no end — and an operator who has watched a counter fail to go down for a
 * fortnight stops believing the other counter too.
 */
import { useCallback, useState } from 'react';
import { t, useLocale } from '@yamale/chain';

import { useAgents, useFreshness, useHead, useOnline, type DownloadState, type LocalHerd } from './data.ts';
import { attestTransfer, recordHealthEvent } from './chain.ts';
import {
  advance,
  disagreements,
  sendable,
  tally,
  type Entry,
} from './outbox.ts';
import type { Identity, Operator } from './ports.ts';
import { LanguagePicker } from '@yamale/chain';
import {
  ActionBar,
  Address,
  Button,
  Chip,
  Empty,
  Field,
  Fields,
  Figure,
  Figures,
  Loading,
  Note,
  Panel,
  ReadAt,
} from './ui.tsx';

export function Sync({ local, operator, identity, download, startDownload, queue, reload }: {
  local: LocalHerd;
  operator: Operator | null;
  identity: Identity;
  download: DownloadState;
  startDownload: (herdId: string) => void;
  queue: (entries: readonly Entry[]) => Promise<void>;
  reload: () => void;
}) {
  const head = useHead();
  const locale = useLocale();
  const connected = useOnline();
  const fresh = useFreshness(local.meta, head);
  const [herdId, setHerdId] = useState(local.meta?.herdId ?? '1');
  const [sending, setSending] = useState(false);

  const counts = tally(local.outbox);
  const conflicts = disagreements(local.outbox);
  const ready = sendable(local.outbox);

  /**
   * Send what can be sent, one at a time, keeping everything.
   *
   * Sequential rather than in parallel on purpose: these are signed
   * transactions from one account, and a parallel burst would race the account
   * sequence number and produce refusals that look like the chain rejecting the
   * content when it is rejecting the ordering.
   */
  const flush = useCallback(async () => {
    if (!operator) return;
    setSending(true);
    try {
      for (const entry of ready) {
        try {
          const message = entry.kind === 'attestation'
            ? attestTransfer({
                attestor: operator.address,
                transferId: String(entry.detail.transferId ?? ''),
                evidenceHash: String(entry.detail.evidenceHash ?? ''),
              })
            : recordHealthEvent({
                agent: operator.address,
                herdId: entry.herdId,
                disease: String(entry.detail.disease ?? ''),
                batch: String(entry.detail.batch ?? ''),
                treated: Number(entry.detail.treated ?? 0),
              });
          const txHash = await identity.submit([message]);
          await queue([advance(entry, { ok: true, txHash })]);
        } catch (err) {
          // Kept, with the reason. Never dropped: a refused entry is still the
          // only record that somebody stood in front of that animal.
          await queue([advance(entry, {
            ok: false, note: err instanceof Error ? err.message : String(err),
          })]);
        }
      }
    } finally {
      setSending(false);
      reload();
    }
  }, [ready, operator, identity, queue, reload]);

  return (
    <>
      <div className="y-screen">
        <h1 className="y-screen__title">{t('herd.syncTitle')}</h1>

        {!connected ? (
          <Note tone="mute" title={t('herd.offlineTitle')}>{t('herd.offlineBody')}</Note>
        ) : null}

        {/* ---------------------------------------------------- the queue */}

        <Panel title={t('herd.queueTitle')} sub={t('herd.queueLede')}>
          <Figures>
            <Figure label={t('herd.waitingToSend')}
                    value={<span className="num">{counts.queued + counts.refused}</span>}
                    tone={counts.refused ? 'warn' : undefined}
                    hint={t('herd.waitingHint')} />
            <Figure label={t('herd.heldOnDevice')}
                    value={<span className="num">{counts.held}</span>}
                    hint={t('herd.heldHint')} />
            <Figure label={t('herd.onChain')}
                    value={<span className="num">{counts.sent}</span>}
                    hint={t('herd.onChainHint')} />
            <Figure label={t('herd.oldestUnsent')}
                    value={counts.oldestUnsent === null
                      ? null
                      : new Date(counts.oldestUnsent).toLocaleDateString(locale)}
                    hint={t('herd.oldestUnsentHint')} />
          </Figures>

          {counts.held > 0 ? (
            <Note tone="warn" title={t('herd.heldWhyTitle')}>{t('herd.heldWhyBody')}</Note>
          ) : null}

          {counts.refused > 0 ? (
            <Note tone="bad" title={t('herd.refusedTitle')}>{t('herd.refusedBody')}</Note>
          ) : null}

          {local.outbox.length === 0 ? (
            <Empty title={t('herd.queueEmptyTitle')}>
              <p>{t('herd.queueEmptyBody')}</p>
            </Empty>
          ) : (
            <ul className="y-cards">
              {[...local.outbox].sort((a, b) => b.observedAt - a.observedAt).map((e) => (
                <li key={e.id} className="y-card">
                  <div className="y-card__head">
                    <strong>{t(`herd.entryKind.${e.kind}`)}</strong>
                    <Chip tone={e.state === 'sent' ? 'ok'
                      : e.state === 'refused' ? 'bad'
                      : e.state === 'held' ? 'mute' : 'warn'}>
                      {t(`herd.entryState.${e.state}`)}
                    </Chip>
                  </div>
                  <Fields>
                    <Field label={t('herd.observedAt')}>
                      {new Date(e.observedAt).toLocaleString(locale)}
                    </Field>
                    <Field label={t('herd.animal')}>
                      {e.animalId ? t('herd.animalN', { id: e.animalId }) : t('herd.noAnimalNamed')}
                    </Field>
                    <Field label={t('herd.device')}><span className="mono">{e.deviceId}</span></Field>
                    <Field label={t('herd.agent')}><Address value={e.agent} /></Field>
                  </Fields>
                  {e.note ? <p className="entry__note">{e.note}</p> : null}
                </li>
              ))}
            </ul>
          )}
        </Panel>

        {/* ---------------------------------------- two devices disagreeing */}

        {conflicts.length ? (
          <Panel title={t('herd.conflictTitle')} sub={t('herd.conflictLede')}>
            {/*
              Shown, never resolved. §5: both are submitted, the chain holds
              both with their agents, and the contradiction is visible rather
              than silently settled by whoever synced last. There is no button
              on this panel that picks one.
            */}
            <ul className="y-cards">
              {conflicts.map((c) => (
                <li key={`${c.animalId}:${c.kind}`} className="y-card y-card--danger">
                  <div className="y-card__head">
                    <strong>{t('herd.animalN', { id: c.animalId })}</strong>
                    <Chip tone={c.differentAgents ? 'bad' : 'warn'}>
                      {c.differentAgents ? t('herd.twoAgents') : t('herd.twoCaptures')}
                    </Chip>
                  </div>
                  <ul className="atts">
                    {c.entries.map((e) => (
                      <li key={e.id} className="att">
                        <Address value={e.agent} />
                        <span className="att__when">{new Date(e.observedAt).toLocaleString(locale)}</span>
                        <span className="att__where"><span className="mono">{e.deviceId}</span></span>
                      </li>
                    ))}
                  </ul>
                  <p className="caveat">{t('herd.conflictBothSent')}</p>
                </li>
              ))}
            </ul>
          </Panel>
        ) : null}

        {/* --------------------------------------------------- the replica */}

        <Panel title={t('herd.replicaTitle')} sub={t('herd.replicaLede')}>
          {local.meta ? (
            <Fields>
              <Field label={t('herd.herdOnDevice')}>
                {local.herd?.reference ?? t('herd.herdN', { id: local.meta.herdId })}
              </Field>
              <Field label={t('herd.animalsHeld')}>
                <span className="num">{local.animals.length}</span>
              </Field>
              <Field label={t('herd.downloadedAtBlock')}>
                <span className="num">{local.meta.height.toLocaleString(locale)}</span>
              </Field>
              <Field label={t('herd.downloadedWhen')}>
                {new Date(local.meta.downloadedAt).toLocaleString(locale)}
              </Field>
              <Field label={t('herd.blocksBehind')}>
                {fresh?.blocksBehind === null || !fresh
                  ? <span className="absent">{t('herd.chainUnknown')}</span>
                  : <span className="num">{fresh.blocksBehind.toLocaleString(locale)}</span>}
              </Field>
              <Field label={t('herd.storage')}>
                <Chip tone={local.durable ? 'ok' : 'bad'}>
                  {local.durable ? t('herd.durable') : t('herd.notDurable')}
                </Chip>
              </Field>
            </Fields>
          ) : (
            <Empty title={t('herd.noReplicaTitle')}><p>{t('herd.noReplicaBody')}</p></Empty>
          )}

          <label className="bigfield">
            <span className="y-label">{t('herd.herdToDownload')}</span>
            <input className="bigfield__input" value={herdId} inputMode="numeric"
                   onChange={(e) => setHerdId(e.target.value.replace(/\D+/g, ''))}
                   autoComplete="off" />
          </label>

          <Button kind="plain" wide disabled={!connected || download.phase === 'working'}
                  onClick={() => startDownload(herdId)}>
            {download.phase === 'working' ? t('herd.downloading') : t('herd.downloadHerd')}
          </Button>

          {download.phase === 'failed'
            ? <Note tone="bad" title={t('herd.downloadFailed')}>{download.detail}</Note>
            : null}
          {download.phase === 'done'
            ? <Note tone="ok" title={t('herd.downloadDone')}>
                {t('herd.downloadDoneBody', { n: download.animals })}
              </Note>
            : null}

          <p className="caveat">{t('herd.replicaScopeCaveat')}</p>
        </Panel>

        {/* ------------------------------------------------------- the person */}

        <Who operator={operator} />

        <Panel title={t('herd.appearance')}>
          <LanguagePicker />
        </Panel>

        <ReadAt head={head} />
      </div>

      <ActionBar>
        <Button kind="primary" wide onClick={flush}
                disabled={!connected || !operator || ready.length === 0 || sending}>
          {sending
            ? t('herd.sending')
            : t('herd.sendN', { n: ready.length })}
        </Button>
      </ActionBar>
    </>
  );
}

/**
 * Who is signed in, and whether the register still says they may act.
 *
 * The accreditation is read from the chain rather than from the device. An
 * agent who has been struck off and does not know it will spend a day
 * collecting attestations the chain will refuse, and `x/herd` returns
 * struck-off agents on purpose so that this question stays answerable.
 */
function Who({ operator }: { operator: Operator | null }) {
  const [list] = useAgents();

  if (!operator) {
    return (
      <Panel title={t('herd.whoTitle')}>
        <Empty title={t('herd.noOperatorTitle')}>
          <p>{t('herd.noOperatorBody')}</p>
        </Empty>
      </Panel>
    );
  }

  const record = !list.loading && list.outcome.ok
    ? list.outcome.value.find((a) => a.address === operator.address) ?? null
    : null;

  return (
    <Panel title={t('herd.whoTitle')}>
      <Fields>
        <Field label={t('herd.signedInAs')}>
          {record?.name || operator.label || t('herd.unnamed')}
        </Field>
        <Field label={t('herd.address')}><Address value={operator.address} /></Field>
        <Field label={t('herd.accreditation')}>
          {list.loading ? <Loading what={t('herd.checking')} />
            : !list.outcome.ok ? <span className="absent">{t('herd.accreditationUnknown')}</span>
            : record ? (
              <Chip tone={record.active ? 'ok' : 'bad'}>
                {record.active ? t('herd.accredited') : t('herd.struckOff')}
              </Chip>
            ) : <Chip tone="warn">{t('herd.notOnRegister')}</Chip>}
        </Field>
        {record ? (
          <Field label={t('herd.accreditedBy')}><Address value={record.authority} /></Field>
        ) : null}
      </Fields>

      {record && !record.active ? (
        <Note tone="bad" title={t('herd.struckOffTitle')}>{t('herd.struckOffBody')}</Note>
      ) : null}
      {!list.loading && !list.outcome.ok ? (
        <Note tone="mute" title={t('herd.accreditationUnknownTitle')}>
          {t('herd.accreditationUnknownBody')}
        </Note>
      ) : null}
    </Panel>
  );
}
