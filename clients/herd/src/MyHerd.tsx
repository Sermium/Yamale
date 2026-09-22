/**
 * The herd on the device, which is the screen that must work with the radio off.
 *
 * Nothing here touches the network. Not "falls back to the local copy when the
 * network is slow" — never asks. §5 scopes the replica to one keeper's animals
 * precisely so that a day's work happens against it, and a list that quietly
 * re-queried the chain every time it rendered would be a list that goes blank
 * in the one place it is most needed.
 *
 * The chain's head still appears at the foot of the screen, because it is what
 * dates the copy: "downloaded at block 320,950, and the chain is now at
 * 462,701" is a fact an operator can act on, and "downloaded three days ago" is
 * one they can feel.
 *
 * # Why there is a search box and not a filter bar
 *
 * A herd is hundreds of animals and the question is always "where is this one",
 * never "show me all the fauve ones". The box matches an identifier as typed —
 * the same normalisation the Identify screen uses, so a tag with spaces in it
 * finds the same beast here as there — and falls back to a name match, because
 * in the field people say "Classico" and not "221/682/021029/L".
 */
import { useMemo, useState } from 'react';
import { t } from '@yamale/chain';

import { useFreshness, useHead, type LocalHerd } from './data.ts';
import { classify, normaliseNationalId, normaliseTag } from './herd.ts';
import {
  AnimalRow,
  Button,
  Chip,
  Empty,
  Field,
  Fields,
  Figure,
  Figures,
  Note,
  Panel,
  ReadAt,
} from './ui.tsx';

export function MyHerd({ local, onDownload }: {
  local: LocalHerd;
  onDownload: () => void;
}) {
  const head = useHead();
  const fresh = useFreshness(local.meta, head);
  const [q, setQ] = useState('');

  const matching = useMemo(() => {
    const query = q.trim();
    if (!query) return local.animals;

    // Identifier first, using the same normalisation as the lookup — a tag
    // typed with spaces must find the same animal on both screens or the two
    // will disagree in front of somebody.
    const entry = classify(query);
    if (entry.kind === 'tag') {
      return local.animals.filter((a) => normaliseTag(a.tag).includes(entry.value));
    }
    if (entry.kind === 'national-id') {
      return local.animals.filter((a) => normaliseNationalId(a.nationalId).includes(entry.value));
    }

    const lower = query.toLowerCase();
    return local.animals.filter(
      (a) => a.name.toLowerCase().includes(lower)
        || a.nationalId.toLowerCase().includes(lower)
        || a.tag.includes(query.replace(/\D+/g, '')),
    );
  }, [q, local.animals]);

  if (!local.meta || !local.herd) {
    return (
      <div className="y-screen">
        <h1 className="y-screen__title">{t('herd.myHerdTitle')}</h1>
        <Empty title={t('herd.noReplicaTitle')}>
          <p>{t('herd.noReplicaBody')}</p>
          <Button kind="primary" wide onClick={onDownload}>{t('herd.goToSync')}</Button>
        </Empty>
        <ReadAt head={head} />
      </div>
    );
  }

  const herd = local.herd;

  return (
    <div className="y-screen">
      <h1 className="y-screen__title">{herd.reference}</h1>
      <p className="y-screen__lede">
        {herd.locality || t('herd.noLocality')} · {t(`herd.species.${herd.species}`)}
      </p>

      <Figures>
        <Figure label={t('herd.onDevice')} value={<span className="num">{local.animals.length}</span>}
                hint={t('herd.onDeviceHint')} />
        {/*
          Two counts that differ, shown together on purpose. `declared_size` is
          what the keeper said; the number of records is what the registry
          holds. The module's own comment says forcing them equal "would destroy
          the only signal that a declaration is drifting", so the screen does
          not reconcile them either.
        */}
        <Figure label={t('herd.declared')} value={<span className="num">{herd.declaredSize}</span>}
                hint={t('herd.declaredHint')} />
        <Figure label={t('herd.enrolledBiometrics')}
                value={
                  <span className="num pair">
                    {local.scope.withTemplate} / {local.animals.length}
                  </span>
                }
                tone={local.scope.withTemplate === 0 ? 'warn' : undefined}
                hint={t('herd.enrolledHint')} />
        <Figure label={t('herd.healthEvents')}
                value={<span className="num">{local.healthEvents.length}</span>}
                hint={t('herd.healthEventsHint')} />
      </Figures>

      {!local.durable ? (
        <Note tone="bad" title={t('herd.notDurableTitle')}>
          {t('herd.notDurableBody', { why: local.why })}
        </Note>
      ) : null}

      {local.index?.collisions.length ? (
        <Note tone="bad" title={t('herd.collisionTitle')}>{t('herd.collisionBody')}</Note>
      ) : null}

      <label className="bigfield bigfield--search">
        <span className="y-label">{t('herd.findInHerd')}</span>
        <input className="bigfield__input" value={q} onChange={(e) => setQ(e.target.value)}
               placeholder={t('herd.findPlaceholder')} inputMode="text"
               autoComplete="off" spellCheck={false} />
      </label>

      {matching.length === 0 ? (
        <Empty title={t('herd.noneMatchTitle')}>
          {/* Scoped, always. A miss here is a statement about this herd, and
              the animal may be perfectly well registered to somebody else. */}
          <p>{t('herd.noneMatchBody', { n: local.animals.length })}</p>
        </Empty>
      ) : (
        <ul className="y-cards">
          {matching.map((a) => (
            <li key={a.id}>
              <AnimalRow animal={a} to={`/a/${a.id}`} />
            </li>
          ))}
        </ul>
      )}

      <Panel title={t('herd.aboutThisCopy')}>
        <Fields>
          <Field label={t('herd.keeper')}>{herd.keeper ? <code className="y-addr">{herd.keeper}</code> : '—'}</Field>
          <Field label={t('herd.downloadedAtBlock')}>
            <span className="num">{local.meta.height.toLocaleString()}</span>
          </Field>
          <Field label={t('herd.blocksBehind')}>
            {fresh?.blocksBehind === null || fresh === null
              ? <span className="absent">{t('herd.chainUnknown')}</span>
              : <span className="num">{fresh.blocksBehind.toLocaleString()}</span>}
          </Field>
          <Field label={t('herd.jurisdiction')}>{herd.jurisdiction || '—'}</Field>
        </Fields>
        <Chip tone={local.durable ? 'ok' : 'bad'}>
          {local.durable ? t('herd.durable') : t('herd.notDurable')}
        </Chip>
      </Panel>

      <ReadAt head={head} from="replica" replicaHeight={local.meta.height} />
    </div>
  );
}
