/**
 * One animal's whole record: who it is, where it came from, what has been done
 * to it, and who has tried to move it.
 *
 * Five sections in a scrolling segmented control rather than in the tab bar.
 * The tab bar is the APPLICATION — identify, my herd, sync — and means the same
 * thing on every screen; these are peers of each other inside one object, and
 * putting them in the bar would make "pedigree" look like a destination rather
 * than a view of the beast currently open.
 *
 * # The pedigree is the section that justifies the module
 *
 * The certificate the Ladoum sector issues today reads "Classico / Atomique /
 * Palmarin" — names, in free text, which nobody can walk and nobody can
 * contradict. Here every ancestor is a record with its own identifier, so the
 * line is walkable: each generation is a link to that animal's own screen, and
 * a buyer can follow it to the founder. That is the whole difference, and this
 * screen exists to make it visible rather than merely possible.
 *
 * Two things the line must never do. It must not present a gap as a founder —
 * `truncated` says the walk stopped at the depth bound, and a missing parent id
 * of zero says nobody recorded one, and those are different sentences. And it
 * must not show a name without its identifier, because a name is exactly the
 * free text the module replaced.
 */
import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { formatTimestamp, t, timeAgo, useLocale } from '@yamale/chain';

import { isSilence } from './abci.ts';
import { useAnimal, useHead, usePedigree, useTransfers, type LocalHerd } from './data.ts';
import {
  ageInMonths,
  lineGaps,
  transferTone,
  type Ancestor,
  type Animal as AnimalRecord,
  type Transfer,
} from './herd.ts';
import type { StoredEnrolment } from './store.ts';
import { reEnrolmentDue } from './replica.ts';
import {
  Address,
  Chip,
  Digest,
  Empty,
  Field,
  Fields,
  Loading,
  More,
  NationalId,
  Note,
  Panel,
  ReadAt,
  Silence,
  StatusChip,
  Tag,
} from './ui.tsx';

type Section = 'identity' | 'pedigree' | 'health' | 'transfers' | 'enrolments';

const SECTIONS: Section[] = ['identity', 'pedigree', 'health', 'transfers', 'enrolments'];

export function AnimalScreen({ local }: { local: LocalHerd }) {
  const { animalId = '' } = useParams();
  const head = useHead();
  const [section, setSection] = useState<Section>('identity');
  const [record, retry] = useAnimal(animalId, local);

  if (record.loading) return <div className="y-screen"><Loading what={t('herd.loadingAnimal')} /></div>;
  if (!record.outcome.ok) {
    const o = record.outcome;
    if (isSilence(o)) {
      return (
        <div className="y-screen">
          <Silence reason={o.reason === 'offline' ? 'offline' : 'unreachable'}
                   detail={o.detail} onRetry={retry} />
        </div>
      );
    }
    return (
      <div className="y-screen">
        <Empty title={t('herd.noSuchAnimalTitle')}>
          <p>{t('herd.noSuchAnimalBody', { id: animalId })}</p>
        </Empty>
        <ReadAt head={head} />
      </div>
    );
  }

  const { animal, from, chainSilent } = record.outcome.value;

  return (
    <div className="y-screen">
      <div className="found__head">
        <NationalId value={animal.nationalId} className="found__id" />
        <StatusChip status={animal.status} />
      </div>
      <p className="found__name">{animal.name || t('herd.unnamed')}</p>

      {from === 'replica' ? (
        <Note tone={chainSilent ? 'warn' : 'mute'} title={t('herd.fromReplicaTitle')}>
          {chainSilent ? t('herd.fromReplicaSilent') : t('herd.fromReplicaBody')}
        </Note>
      ) : null}

      <div className="segs" role="tablist" aria-label={t('herd.sections')}>
        {SECTIONS.map((s) => (
          <button key={s} type="button" role="tab" aria-selected={section === s}
                  className={`seg${section === s ? ' seg--on' : ''}`}
                  onClick={() => setSection(s)}>
            {t(`herd.section.${s}`)}
          </button>
        ))}
      </div>

      {section === 'identity' ? <Identity animal={animal} /> : null}
      {section === 'pedigree' ? <Pedigree animal={animal} /> : null}
      {section === 'health' ? <Health local={local} animal={animal} /> : null}
      {section === 'transfers' ? <Transfers animal={animal} /> : null}
      {section === 'enrolments' ? <Enrolments local={local} animal={animal} /> : null}

      <ReadAt head={head} from={from} replicaHeight={local.meta?.height ?? null} />
    </div>
  );
}

/* ------------------------------------------------------------- identity */

function Identity({ animal }: { animal: AnimalRecord }) {
  const months = ageInMonths(animal.bornOn, new Date());
  return (
    <Panel>
      <Fields>
        <Field label={t('herd.nationalId')}><NationalId value={animal.nationalId} /></Field>
        <Field label={t('herd.tag')}><Tag value={animal.tag} /></Field>
        <Field label={t('herd.level')}>{t(`herd.level.${animal.level}`)}</Field>
        <Field label={t('herd.sex')}>{t(`herd.sex.${animal.sex}`)}</Field>
        <Field label={t('herd.coat')}>{animal.coat || t('herd.notRecorded')}</Field>
        <Field label={t('herd.bornOn')}>
          {animal.bornOn || t('herd.notRecorded')}
          {/* Null, not zero. A zero here would say the animal was born today. */}
          {months === null ? null : <span className="muted"> · {t('herd.months', { n: months })}</span>}
        </Field>
        <Field label={t('herd.birthType')}>{animal.birthType || t('herd.notRecorded')}</Field>
        <Field label={t('herd.holder')}><Address value={animal.holder} /></Field>
        <Field label={t('herd.authority')}><Address value={animal.authority} /></Field>
      </Fields>

      <More summary={t('herd.rawSummary')}>
        <Fields>
          <Field label={t('herd.recordId')}><span className="num">{animal.id}</span></Field>
          <Field label={t('herd.herdId')}><span className="num">{animal.herdId}</span></Field>
          {/*
            A BLOCK, not a date. x/herd writes Animal.registered_at with
            blockHeight(ctx) while Transfer.proposed_at gets blockTime(ctx), and
            both are int64 with no comment saying which. Rendering this one as a
            time prints the 4th of January 1970 on a certificate.
          */}
          <Field label={t('herd.registeredAtBlock')}>
            <span className="num">{animal.registeredAt.toLocaleString()}</span>
          </Field>
          <Field label={t('herd.dnaHash')}><Digest hex={animal.dnaHash} /></Field>
          {/*
            photo_hash answers "is this the same file?" — it cannot answer
            "which animal is this?". §1.2. It is shown as what it is: a
            commitment to the level-3 registration photograph, and not a
            biometric anybody can match against.
          */}
          <Field label={t('herd.photoHash')}><Digest hex={animal.photoHash} /></Field>
        </Fields>
        <p className="caveat">{t('herd.photoHashCaveat')}</p>
      </More>
    </Panel>
  );
}

/* ------------------------------------------------------------- pedigree */

function Pedigree({ animal }: { animal: AnimalRecord }) {
  const [line, retry] = usePedigree(animal.id, 4);

  if (line.loading) return <Loading what={t('herd.loadingLine')} />;
  if (!line.outcome.ok) {
    const o = line.outcome;
    return isSilence(o)
      ? <Silence reason={o.reason === 'offline' ? 'offline' : 'unreachable'} detail={o.detail} onRetry={retry} />
      : <Empty title={t('herd.noLineTitle')}><p>{t('herd.noLineBody')}</p></Empty>;
  }

  const value = line.outcome.value;
  const gaps = lineGaps(value);
  const byGeneration = new Map<number, Ancestor[]>();
  for (const node of value.nodes) {
    const got = byGeneration.get(node.generation);
    if (got) got.push(node); else byGeneration.set(node.generation, [node]);
  }
  const generations = [...byGeneration.keys()].sort((a, b) => a - b);

  return (
    <Panel sub={t('herd.pedigreeLede')}>
      {generations.map((g) => (
        <div key={g} className="gen">
          <p className="gen__label">
            {g === 0 ? t('herd.genSelf') : t('herd.genN', { n: g })}
          </p>
          <ul className="y-cards">
            {(byGeneration.get(g) ?? []).map((node) => (
              <li key={node.animal.id}>
                <Link className="y-row row" to={`/a/${node.animal.id}`}>
                  <span className="y-row__main">
                    <span className="y-row__title">
                      <NationalId value={node.animal.nationalId} />
                    </span>
                    <span className="y-row__sub">
                      {node.path ? t(`herd.path.${node.path.split('.').pop()}`) : t('herd.genSelf')}
                      {node.animal.name ? ` · ${node.animal.name}` : ''}
                    </span>
                  </span>
                  <span className="y-row__end">
                    <Chip tone="mute">{t(`herd.level.${node.animal.level}`)}</Chip>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      ))}

      {/* Truncation and absence are different sentences. Without this a buyer
          reads "no further ancestors" where the truth is "we stopped looking". */}
      {value.truncated ? (
        <Note tone="warn" title={t('herd.truncatedTitle')}>{t('herd.truncatedBody')}</Note>
      ) : (
        <Note tone="mute" title={t('herd.lineEndTitle')}>
          {t('herd.lineEndBody', { n: gaps.unrecorded })}
        </Note>
      )}

      {gaps.unreached.length ? (
        <Note tone="warn" title={t('herd.unreachedTitle')}>
          {t('herd.unreachedBody', { ids: gaps.unreached.join(', ') })}
        </Note>
      ) : null}
    </Panel>
  );
}

/* --------------------------------------------------------------- health */

/**
 * Health events are recorded against the HERD, not the animal.
 *
 * `x/herd` costs level 1 per herd rather than per head, which is the only
 * reason it can cover a national population at all — so a campaign reaches a
 * herd and the count of animals treated is a number on that event. This section
 * says so rather than implying that every event on the list was administered to
 * the beast currently open.
 */
function Health({ local, animal }: { local: LocalHerd; animal: AnimalRecord }) {
  const locale = useLocale();
  const events = local.healthEvents.filter((e) => e.herdId === animal.herdId);

  if (!local.meta) {
    return <Empty title={t('herd.noReplicaTitle')}><p>{t('herd.noReplicaBody')}</p></Empty>;
  }

  return (
    <Panel sub={t('herd.healthLede')}>
      {events.length === 0 ? (
        <Empty title={t('herd.noHealthTitle')}><p>{t('herd.noHealthBody')}</p></Empty>
      ) : (
        <ul className="y-cards">
          {events.map((e) => (
            <li key={e.id} className="y-card">
              <div className="y-card__head">
                <strong>{e.disease}</strong>
                <span className="num">{e.treated.toLocaleString(locale)}</span>
              </div>
              <Fields>
                <Field label={t('herd.batch')}><span className="mono">{e.batch}</span></Field>
                <Field label={t('herd.recordedAtBlock')}>
                  <span className="num">{e.recordedAt.toLocaleString(locale)}</span>
                </Field>
                <Field label={t('herd.agent')}><Address value={e.agent} /></Field>
              </Fields>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

/* ------------------------------------------------------------ transfers */

function Transfers({ animal }: { animal: AnimalRecord }) {
  const [list, retry] = useTransfers(animal.id);
  const locale = useLocale();

  if (list.loading) return <Loading what={t('herd.loadingTransfers')} />;
  if (!list.outcome.ok) {
    const o = list.outcome;
    return isSilence(o)
      ? <Silence reason={o.reason === 'offline' ? 'offline' : 'unreachable'} detail={o.detail} onRetry={retry} />
      : <Empty title={t('herd.noTransfersTitle')}><p>{t('herd.noTransfersBody')}</p></Empty>;
  }

  const transfers = list.outcome.value;
  if (transfers.length === 0) {
    return (
      <Panel>
        <Empty title={t('herd.noTransfersTitle')}><p>{t('herd.noTransfersBody')}</p></Empty>
      </Panel>
    );
  }

  return (
    <Panel sub={t('herd.transfersLede')}>
      <ul className="y-cards">
        {transfers.map((tr) => <TransferCard key={tr.id} transfer={tr} locale={locale} />)}
      </ul>
    </Panel>
  );
}

function TransferCard({ transfer: tr, locale }: { transfer: Transfer; locale: string }) {
  return (
    <li className="y-card">
      <div className="y-card__head">
        <Chip tone={transferTone(tr.state)}>{t(`herd.transferState.${tr.state}`)}</Chip>
        {/*
          Free text, and the chain never witnessed the money. Rendered as the
          string the parties recorded, in a mono face so nobody mistakes it for
          a figure this registry can vouch for — the module says it "does not
          hold the money and does not pretend to have witnessed the payment".
        */}
        <span className="price">{tr.price || '—'}</span>
      </div>
      <Fields>
        <Field label={t('herd.from')}><Address value={tr.from} /></Field>
        <Field label={t('herd.to')}><Address value={tr.to} /></Field>
        <Field label={t('herd.proposedAt')}>
          {tr.proposedAt ? formatTimestamp(new Date(tr.proposedAt * 1000), locale) : t('herd.notRecorded')}
        </Field>
        <Field label={t('herd.attestations')}>
          <span className="num">{tr.attestations.length}</span>
        </Field>
      </Fields>

      {tr.attestations.length ? (
        <More summary={t('herd.whoAttested')}>
          <ul className="atts">
            {tr.attestations.map((a, i) => (
              <li key={`${a.attestor}:${i}`} className="att">
                <Address value={a.attestor} />
                <span className="att__where">
                  {t('herd.accreditedBy')} <Address value={a.authority} />
                </span>
                <span className="att__when">{timeAgo(new Date(a.attestedAt * 1000))}</span>
                {/* What they actually checked. A false attestation is meant to
                    be a specific claim by a named person, not a shrug. */}
                <span className="att__ev">
                  {t('herd.evidence')} <Digest hex={a.evidenceHash} />
                </span>
              </li>
            ))}
          </ul>
        </More>
      ) : null}

      {tr.objectedBy ? (
        <Note tone="bad" title={t('herd.objectedTitle')}>
          <p>{tr.objectionReason || t('herd.noReasonGiven')}</p>
          <p><Address value={tr.objectedBy} /></p>
          {tr.resolvedBy ? (
            <p>
              {tr.objectionUpheld ? t('herd.objectionUpheld') : t('herd.objectionDismissed')}
              {' — '}
              <Address value={tr.resolvedBy} />
            </p>
          ) : <p>{t('herd.objectionOpen')}</p>}
        </Note>
      ) : null}
    </li>
  );
}

/* ----------------------------------------------------------- enrolments */

/**
 * The biometric captures this device holds for this animal.
 *
 * On the device and nowhere else. §2 is unambiguous that a template must never
 * reach the chain — "if it leaks, it leaks for the life of the animal, and for
 * a breeding programme it is commercially valuable in itself" — so this section
 * is honest about where these live and about the fact that another device holds
 * its own set.
 *
 * §4 is why the dates are the content: an enrolment is a series, a match is
 * against the template nearest in time, and an animal whose last capture is old
 * enough to have stopped matching is the failure that "would look like the
 * system rejecting a legitimate beast".
 */
function Enrolments({ local, animal }: { local: LocalHerd; animal: AnimalRecord }) {
  const locale = useLocale();
  const mine: StoredEnrolment[] = local.enrolments.filter((e) => e.animalId === animal.id);
  const last = mine.length ? Math.max(...mine.map((e) => e.capturedAt)) : null;
  const due = reEnrolmentDue(ageInMonths(animal.bornOn, new Date()), last, Date.now());

  return (
    <Panel sub={t('herd.enrolmentsLede')}>
      {mine.length === 0 ? (
        <Empty title={t('herd.noEnrolmentsTitle')}>
          <p>{t('herd.noEnrolmentsBody')}</p>
        </Empty>
      ) : (
        <ul className="y-cards">
          {mine.sort((a, b) => b.capturedAt - a.capturedAt).map((e) => (
            <li key={e.enrolmentId} className="y-card">
              <div className="y-card__head">
                <strong>{t(`herd.modality.${e.modality}`)}</strong>
                <span className="muted">{timeAgo(new Date(e.capturedAt))}</span>
              </div>
              <Fields>
                {/*
                  Null is "this device does not score", not zero. A quality bar
                  pinned at the left-hand end would tell an operator their
                  capture was terrible when nobody measured it.
                */}
                <Field label={t('herd.quality')}>
                  {e.quality === null
                    ? <span className="absent">{t('herd.notScored')}</span>
                    : <span className="num">{e.quality.toFixed(2)}</span>}
                </Field>
                <Field label={t('herd.capturedBy')}><Address value={e.agent} /></Field>
                <Field label={t('herd.device')}><span className="mono">{e.deviceId}</span></Field>
                <Field label={t('herd.bits')}><span className="num">{e.template.bits}</span></Field>
              </Fields>
            </li>
          ))}
        </ul>
      )}

      {due.due ? (
        <Note tone="warn" title={t('herd.reEnrolTitle')}>
          {last === null ? t('herd.reEnrolNever') : t('herd.reEnrolDue')}
        </Note>
      ) : null}

      <Note tone="mute" title={t('herd.templatesStayHereTitle')}>
        {t('herd.templatesStayHereBody')}
      </Note>
    </Panel>
  );
}
