/**
 * One vehicle, answering the three questions somebody putting money in has.
 *
 *   1. What is this? The collection, the title, the underlying — and where it is
 *      land, the parcel, its restrictions, its encumbrances, and whether the
 *      registry's fractionalisation authorisation is *actually live*. A vehicle
 *      selling shares against a withdrawn or expired authorisation is selling
 *      something the chain will refuse to mint, so that finding is on this page
 *      in plain words rather than behind a disclosure.
 *   2. What do I own, and what is it earning? A percentage, not a token balance,
 *      and the entitlement queried rather than derived.
 *   3. What could go wrong? Which on a closed-end vehicle is one thing: the sale
 *      price the sponsor reports. Nobody can dilute a holder — say so, because
 *      it is unusual and it is true — but every holder is paid out against a
 *      number the sponsor states, and the only things standing between that
 *      number and a lie are the collection's attestation threshold and its
 *      challenge window.
 *
 * The order on screen is that order.
 */
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { formatAmount, t, useLocale, resolveDenom,} from '@yamale/chain';

import { ClaimDialog, DisputeDialog, RedeemDialog } from './Act.tsx';
import { GradeChip } from './Vehicles.tsx';
import { addressOf, canSign, type Account } from './address.ts';
import {
  chainNow, useCollections, useCovenants, useHead, useHolders, useHolding, useLand, useVehicle,
  type Load,
} from './data.ts';
import { formatDate, formatDuration } from './duration.ts';
import {
  covenantTone,
  deadlineOf,
  hasPeriod,
  isSuperseded,
  kindKey,
  obligationKey,
  openDeadlines,
  orderCovenants,
  verifyCommand,
  type Covenant,
} from './covenant.ts';
import { Market } from './Market.tsx';
import { costOf, pendingAccrual, returnOn } from './position.ts';
import {
  Address,
  Amount,
  Digest,
  Donut,
  Folders,
  Kpi,
  KpiBand,
  Chip,
  Empty,
  Field,
  Fields,
  Loading,
  Meter,
  Note,
  Panel,
  Percent,
  Raw,
  Symbol,
  Unreachable,
} from './ui.tsx';
import {
  actionsFor,
  bpsToPercent,
  descriptorOf,
  dilutionProtected,
  isRealId,
  parcelStatusKey,
  parcelTone,
  protectionOf,
  saleState,
  shareholding,
  statusKey,
  statusTone,
  type Collection,
  type LandGate,
  type SaleState,
  type SaleReport,
} from './vehicle.ts';

/* ------------------------------------------------- the registry's permission */

const GATE_TONE = {
  live: 'ok', 'not-land': 'mute', absent: 'bad', withdrawn: 'bad',
  expired: 'bad', restricted: 'bad', unreachable: 'warn',
} as const;

function LandPanel({ gate, parcel, parcelUnreadable }: {
  gate: LandGate;
  parcel: import('./chain.ts').ParcelRecord | null;
  parcelUnreadable: boolean;
}) {
  const locale = useLocale();

  // A tokenisation over a company is not a tokenisation over land, and a panel
  // headed "The underlying / Not registered land" tells its reader that land was
  // the expected answer and this one fell short. Most vehicles worth issuing are
  // not land: an operating business, a holding company, a receivable.
  //
  // So nothing is rendered here. The one fact that panel carried which a holder
  // actually needs — that no registry caps what may be issued and no office can
  // withdraw permission — is not lost: it moves into the risk panel, beside the
  // other protections a holder has and does not have. An absence does not
  // deserve a section of its own; it deserves a line where the reader is already
  // looking for what can go wrong.
  if (gate.kind === 'not-land') return null;

  const live = gate.kind === 'live';
  const auth = 'auth' in gate ? gate.auth : null;

  return (
    <Panel eyebrow={t('rwa.underlyingEyebrow')} title={t('rwa.landTitle')}>
      {/*
        The headline finding, above everything else on the panel. An investor
        reading this page needs to know before anything else whether the office
        that governs this land still permits shares in it to be sold.
      */}
      <div className={`gate gate--${gate.kind}`}>
        <Chip tone={GATE_TONE[gate.kind]}>
          {t(live ? 'rwa.authLiveChip' : 'rwa.authDeadChip')}
        </Chip>
        <p className="gate__line">{t(`rwa.auth.${gate.kind}`)}</p>
        {!live && gate.kind !== 'unreachable' && (
          <p className="gate__consequence">{t('rwa.authRefusesMint')}</p>
        )}
        {gate.kind === 'unreachable' && (
          <p className="gate__consequence">{t('rwa.authUnknownConsequence')}</p>
        )}
      </div>

      {auth && (
        <Fields>
          <Field label={t('rwa.authRight')}>{auth.right || '—'}</Field>
          <Field label={t('rwa.authCeiling')}>
            <Percent value={bpsToPercent(auth.maxShareBps)} />
            <span className="field__aside">{t('rwa.authCeilingNote')}</span>
          </Field>
          <Field label={t('rwa.authExpires')}>
            {formatDate(new Date(auth.expiresAt * 1000), locale)}
          </Field>
          <Field label={t('rwa.authGrantedBy')}><Address value={auth.grantedBy} /></Field>
          {auth.withdrawn && (
            <Field label={t('rwa.authWithdrawnAt')}>
              {formatDate(new Date(auth.withdrawnAt * 1000), locale)}
            </Field>
          )}
        </Fields>
      )}

      {parcel ? (
        <>
          <h4 className="panel__sub">{t('rwa.parcelTitle')}</h4>
          {parcel.frozen && (
            <Note tone="bad" title={t('rwa.parcelFrozen')}>{parcel.freezeReason}</Note>
          )}
          <Fields>
            <Field label={t('rwa.cadastral')}><span className="y-mono">{parcel.cadastralRef}</span></Field>
            <Field label={t('rwa.parcelHolder')}><Address value={parcel.holder} /></Field>
            <Field label={t('rwa.registryOffice')}><Address value={parcel.authority} /></Field>
            <Field label={t('rwa.parcelStatus')}>
              {/*
                A sentence, not the enum. STATUS_TRANSFER_PENDING is not a
                status somebody buying into a vehicle can act on; "a transfer of
                this land is under way" is. The raw value stays reachable
                through the panel's Raw disclosure.
              */}
              <Chip tone={parcelTone(parcel.status)}>{t(parcelStatusKey(parcel.status))}</Chip>
            </Field>
          </Fields>

          <h4 className="panel__sub">{t('rwa.restrictions')}</h4>
          {parcel.restrictions.filter((r) => !r.lifted).length === 0 ? (
            <p className="muted">{t('rwa.noneRecorded')}</p>
          ) : (
            <ul className="taglist">
              {parcel.restrictions.filter((r) => !r.lifted).map((r, i) => (
                <li key={`${r.kind}-${i}`}>
                  <Chip tone="warn">{r.kind}</Chip>
                  {r.value && <span className="y-mono taglist__value">{r.value}</span>}
                  {r.detail && <span className="taglist__detail">{r.detail}</span>}
                </li>
              ))}
            </ul>
          )}

          {/*
            Encumbrances are shown even when there are none, and the sentence
            says "none recorded" rather than nothing at all. A title displayed
            without its encumbrances is a lie that gets somebody's house taken,
            and an absent section reads as an absent question.
          */}
          <h4 className="panel__sub">{t('rwa.encumbrances')}</h4>
          {parcel.encumbrances.filter((e) => !e.released).length === 0 ? (
            <p className="muted">{t('rwa.noneRecorded')}</p>
          ) : (
            <ul className="taglist">
              {parcel.encumbrances.filter((e) => !e.released).map((e, i) => (
                <li key={`${e.kind}-${i}`}>
                  <Chip tone="warn">{e.kind}</Chip>
                  <Address value={e.holder} />
                  {e.detail && <span className="taglist__detail">{e.detail}</span>}
                </li>
              ))}
            </ul>
          )}
        </>
      ) : (
        <p className="muted">
          {parcelUnreadable ? t('rwa.parcelUnreadable') : t('rwa.parcelMissing')}
        </p>
      )}
    </Panel>
  );
}

/* --------------------------------------------------------- the reported sale */

function SalePanel({ sale, state, collection, onDispute, disputeWhy }: {
  sale: SaleReport | null;
  state: SaleState;
  collection: Collection;
  onDispute: (() => void) | null;
  disputeWhy: string | undefined;
}) {
  const locale = useLocale();

  if (!sale || state.phase === 'none') {
    return (
      <Panel eyebrow={t('rwa.saleEyebrow')} title={t('rwa.noSaleTitle')}>
        <p className="panel__lede">{t('rwa.noSaleBody')}</p>
      </Panel>
    );
  }

  const tone = state.phase === 'disputed' ? 'bad'
    : state.phase === 'in-window' ? 'warn'
      : state.phase === 'awaiting-attestations' ? 'warn' : 'ok';

  return (
    <Panel eyebrow={t('rwa.saleEyebrow')} title={t('rwa.saleTitle')} className="panel--sale">
      <p className="panel__lede">{t('rwa.saleLede')}</p>

      <div className="sale__figure">
        <span className="y-label">{t('rwa.reportedPrice')}</span>
        <Amount amount={sale.price.amount} denom={sale.price.denom} className="sale__price" />
        <p className="sale__by">
          {t('rwa.reportedBy')} <Address value={sale.reporter} />
          {' · '}
          {formatDate(sale.reportedAt, locale)}
        </p>
      </div>

      <Note tone={tone} title={t(`rwa.phase.${state.phase}`)}>
        {t(`rwa.phaseWhy.${state.phase}`)}
      </Note>

      {/*
        Attestation progress and the window are the two things that decide
        whether the figure above can be trusted, so they are the two meters on
        the page. A collection that requires none says so explicitly rather
        than showing an empty bar, which would read as "none gathered yet".
      */}
      <div className="sale__meters">
        <div className="sale__meter">
          <div className="sale__meterHead">
            <span className="y-label">{t('rwa.attestations')}</span>
            <span className="y-num">
              {state.needed > 0
                ? t('rwa.ofNeeded', { have: state.attestations, need: state.needed })
                : t('rwa.attestNotUsed')}
            </span>
          </div>
          {state.needed > 0 && (
            <Meter
              value={state.attestations}
              of={state.needed}
              tone={state.attestations >= state.needed ? 'ok' : 'warn'}
              label={t('rwa.ofNeeded', { have: state.attestations, need: state.needed })}
            />
          )}
          {state.needed === 0 && (
            <p className="muted">{t(`rwa.verify.${collection.verification}`)}</p>
          )}
        </div>

        <div className="sale__meter">
          <div className="sale__meterHead">
            <span className="y-label">{t('rwa.challengeWindow')}</span>
            <span className="y-num">
              {state.remainingSeconds > 0
                ? t('rwa.timeLeft', { time: formatDuration(state.remainingSeconds, locale) })
                : t('rwa.windowShut')}
            </span>
          </div>
          <Meter
            value={state.remainingSeconds}
            of={Math.max(state.totalSeconds, 1)}
            tone={state.remainingSeconds > 0 ? 'warn' : 'mute'}
            label={t('rwa.challengeWindow')}
          />
          <p className="muted">
            {t('rwa.windowCloses', { when: formatDate(sale.claimableAt, locale) })}
          </p>
        </div>
      </div>

      {sale.attestors.length > 0 && (
        <>
          <h4 className="panel__sub">{t('rwa.whoAttested')}</h4>
          <ul className="taglist">
            {sale.attestors.map((a) => <li key={a}><Address value={a} /></li>)}
          </ul>
        </>
      )}

      <div className="sale__act">
        {onDispute ? (
          <button type="button" className="btn btn--warn" onClick={onDispute}>
            {t('rwa.actDispute')}
          </button>
        ) : (
          <p className="muted">{disputeWhy ? t(disputeWhy) : ''}</p>
        )}
        {state.canDispute && (
          <p className="sale__bond">
            {t('rwa.bondWouldBe')}{' '}
            <strong>{formatAmount(state.bond, sale.price.denom)}</strong>
          </p>
        )}
      </div>

      {/*
        Finalisation. Worth stating rather than leaving as a silence: once the
        window has passed and the attestations are in, redemption opens through
        x/tokenisation's FinaliseSale — a permissionless crank, deliberately not
        an EndBlocker sweep. On the binary running yamale-devnet-2 that crank
        has no message exposed, so nothing can call it, and this is where a
        holder would otherwise sit waiting for a state that cannot arrive.
      */}
      {state.phase === 'clear' && (
        <Note tone="warn" title={t('rwa.awaitingFinalise')}>
          {t('rwa.awaitingFinaliseWhy')}
        </Note>
      )}
    </Panel>
  );
}

/**
 * Who owns this tokenisation.
 *
 * The register is public and needs nobody's permission: the shareholding is an
 * ordinary bank denomination, so the holders are bank's answer rather than
 * something the sponsor chooses to publish. That is most of what the whole
 * argument for tokenising rests on, and it had no screen until now.
 *
 * The ring answers the question a cap table is opened for -- how much of this
 * is one person -- before the reader has read a row. Everything past the first
 * eight is one arc, because a thousand holders is the SUCCESS case here and a
 * chart that shattered at that point would fail exactly when the argument was
 * won.
 */
function CapTable({ cap, supply, sponsor, denom }: {
  cap: ReturnType<typeof useHolders>;
  supply: string | null;
  sponsor: string;
  denom: string;
}) {
  if (cap.loading) return <Panel title={t('rwa.holdersTitle')}><Loading what={t('rwa.reading')} /></Panel>;
  if (!cap.outcome.ok) {
    return (
      <Panel title={t('rwa.holdersTitle')}>
        <Note tone="warn">{t('rwa.unreachableWhy')}</Note>
      </Panel>
    );
  }

  const rows = cap.outcome.value.rows;
  const total = BigInt(supply || '0');
  if (rows.length === 0 || total === 0n) {
    return (
      <Panel title={t('rwa.holdersTitle')}>
        <p className="panel__lede">{t('rwa.holdersEmpty')}</p>
      </Panel>
    );
  }

  const fraction = (amount: string) => Number((BigInt(amount) * 1000000n) / total) / 1000000;

  // Eight arcs and a remainder. The palette is the shared accent walked through
  // decreasing opacity rather than eight invented hues: a cap table is one
  // quantity split up, not eight unrelated categories, and eight hues would
  // imply a difference in kind that is not there.
  const TOP = 8;
  const shown = rows.slice(0, TOP);
  const rest = rows.slice(TOP);
  const restFraction = rest.reduce((sum, r) => sum + fraction(r.amount), 0);

  const tone = (i: number) => `color-mix(in srgb, var(--accent) ${100 - i * 9}%, var(--surface-2))`;
  const slices = shown.map((r, i) => ({
    label: r.address, fraction: fraction(r.amount), tone: tone(i),
  }));
  if (restFraction > 0) {
    slices.push({ label: t('rwa.holdersOthers'), fraction: restFraction, tone: 'var(--text-faint)' });
  }

  return (
    <Panel eyebrow={t('rwa.tabHolders')} title={t('rwa.holdersTitle')}>
      <p className="panel__lede">{t('rwa.holdersLede')}</p>
      <div className="captable">
        <Donut slices={slices} />
        <table className="holders">
          <thead>
            <tr>
              <th>{t('rwa.colHolder')}</th>
              <th>{t('rwa.colTokens')}</th>
              <th>{t('rwa.colShare')}</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((r, i) => (
              <tr key={r.address} className={r.address === sponsor ? 'holders__row--self' : ''}>
                <td>
                  <span className="holders__swatch" style={{ background: tone(i) }} />
                  <Address value={r.address} />
                  {r.address === sponsor ? <span className="muted"> ({t('rwa.holdersSponsor')})</span> : null}
                </td>
                <td><Amount amount={r.amount} denom={denom} /></td>
                <td><Percent value={fraction(r.amount) * 100} places={2} /></td>
              </tr>
            ))}
            {rest.length > 0 && (
              <tr>
                <td>
                  <span className="holders__swatch" style={{ background: 'var(--text-faint)' }} />
                  {t('rwa.holdersOthers')} <span className="muted">({rest.length})</span>
                </td>
                <td><Amount amount={rest.reduce((n, r) => n + BigInt(r.amount), 0n).toString()} denom={denom} /></td>
                <td><Percent value={restFraction * 100} places={2} /></td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      {cap.outcome.value.more && <Note tone="mute">{t('rwa.kpiHoldersMore')}</Note>}
    </Panel>
  );
}

/* ------------------------------------------------------ what it cost me */

/**
 * What this holding cost and what it has returned on that cost.
 *
 * The three refusals here are the content. A holding with no recorded basis
 * says so and shows no percentage; a holding recorded at a price of nothing
 * says THAT, which is a different fact; and a partly covered holding reports
 * its return on the covered part with the uncovered part stated beside it,
 * never folded in.
 *
 * The return is also named for what it is. MsgClaim zeroes `accrued` and keeps
 * no running total of what an account has withdrawn, so income already taken is
 * not on the chain and this figure is a FLOOR for anybody who has ever claimed.
 * Calling it "return" flat would be a lie in the direction that looks like
 * modesty.
 */
function CostBlock({ cost, ret, unknown, incomeDenom, shareDenom }: {
  cost: ReturnType<typeof costOf>;
  ret: ReturnType<typeof returnOn>;
  unknown: boolean;
  incomeDenom: string;
  shareDenom: string;
}) {
  if (unknown) {
    // Not the same as "no cost recorded". Saying the chain has forgotten what
    // somebody paid, when it simply was not asked, sends them looking for a
    // fault that is not there.
    return (
      <div className="cost">
        <h4 className="panel__sub">{t('rwa.cost.title')}</h4>
        <Note tone="warn">{t('rwa.cost.unreadable')}</Note>
      </div>
    );
  }

  if (cost.state === 'none') {
    return (
      <div className="cost">
        <h4 className="panel__sub">{t('rwa.cost.title')}</h4>
        <Note tone="mute" title={t('rwa.cost.notRecordedTitle')}>
          {t('rwa.cost.notRecordedBody')}
        </Note>
      </div>
    );
  }

  return (
    <div className="cost">
      <h4 className="panel__sub">{t('rwa.cost.title')}</h4>
      <Fields>
        <Field label={t('rwa.cost.paid')}>
          <Amount amount={cost.basis} denom={incomeDenom} />
        </Field>
        <Field label={t('rwa.cost.forTokens')}>
          <Amount amount={cost.covered} denom={shareDenom} />
        </Field>
        <Field label={t('rwa.cost.income')}>
          {ret.income === null
            ? <span className="muted">{ret.whyKey ? t(ret.whyKey) : '—'}</span>
            : <Amount amount={ret.income} denom={incomeDenom} />}
        </Field>
        <Field label={t('rwa.cost.return')}>
          {ret.percent === null
            ? <span className="muted">{ret.whyKey ? t(ret.whyKey) : '—'}</span>
            : (
              <>
                <Percent value={ret.percent} places={2} />
                <span className="field__aside">{t('rwa.cost.returnFloor')}</span>
              </>
            )}
        </Field>
      </Fields>

      {/*
        The uncovered part, stated rather than averaged away. Nine million
        tokens of which five were subscribed and four arrived as a plain
        transfer is the ORDINARY case, and an interface that divided the known
        cost across all nine would report a price nobody paid.
      */}
      {cost.state === 'partial' && (
        <Note tone="warn" title={t('rwa.cost.partialTitle')}>
          {t('rwa.cost.partialBody', { tokens: formatAmount(cost.uncovered, shareDenom) })}
        </Note>
      )}

      {cost.zeroCost && (
        <Note tone="mute" title={t('rwa.cost.zeroTitle')}>{t('rwa.cost.zeroBody')}</Note>
      )}
    </div>
  );
}

/* -------------------------------------------- what is accruing right now */

/**
 * Income accruing in the current period, which is not money yet.
 *
 * Shown in its own units and never converted. The keeper's comment is the
 * specification and it is worth repeating where somebody might be tempted:
 * "An interface that converted them at the last period's rate would be
 * publishing a forecast beside a settled figure in the same units, and a reader
 * cannot tell those apart once they are both in dollars."
 *
 * So there are two figures and neither is a currency: how long this balance has
 * been accruing in the open period, which is the one a person has intuitions
 * about, and the exact token-seconds behind it, which is the quantity the
 * keeper will actually divide the distribution over.
 */
function AccrualBlock({ pending, now, incomeDenom }: {
  pending: ReturnType<typeof pendingAccrual>;
  now: Date;
  incomeDenom: string;
}) {
  const locale = useLocale();
  const seconds = BigInt(pending.tokenSeconds);

  return (
    <div className="accrual">
      <h4 className="panel__sub">{t('rwa.accrual.title')}</h4>

      <p className="accrual__period">
        <span className="accrual__periodName">
          {pending.first
            ? t('rwa.accrual.firstPeriod')
            : t('rwa.accrual.period', { n: pending.periodIndex })}
        </span>
        {pending.periodStart > 0 && (
          <span className="accrual__periodSince">
            {t('rwa.accrual.since', {
              when: formatDate(new Date(pending.periodStart * 1000), locale),
            })}
          </span>
        )}
      </p>

      {seconds <= 0n ? (
        <p className="muted">{t('rwa.accrual.none')}</p>
      ) : (
        <Fields>
          <Field label={t('rwa.accrual.timeHeld')}>
            <span className="y-num">
              {pending.heldSeconds === null
                ? '—'
                : formatDuration(pending.heldSeconds, locale)}
            </span>
          </Field>
          <Field label={t('rwa.accrual.tokenSeconds')}>
            {/*
              A raw integer with its own unit attached, deliberately NOT run
              through Amount: Amount resolves a denom and applies an exponent,
              and a token-second is neither a token nor a currency. Rendering it
              through the money formatter is how it would silently acquire a
              currency symbol.
            */}
            <span className="y-num y-mono accrual__figure">
              {seconds.toLocaleString(locale)}
            </span>
            <span className="field__aside">{t('rwa.accrual.tokenSecondsUnit')}</span>
          </Field>
        </Fields>
      )}

      <Note tone="mute" title={t('rwa.accrual.notMoneyTitle')}>
        {t('rwa.accrual.notMoneyBody', { denom: resolveDenom(incomeDenom).symbol })}
      </Note>

      {/* The clock this was measured against. A pending figure is an integral
          up to "now", so which "now" was used is part of the figure. */}
      <p className="accrual__at y-mono">
        {t('rwa.accrual.measuredAt', { when: formatDate(now, locale) })}
      </p>
    </div>
  );
}

/* ------------------------------------------------- what has been published */

/**
 * One filing.
 *
 * The layout carries the distinction the whole panel exists for. A filing that
 * asks something of the reader leads with a DATE and a countdown, set as a
 * block they cannot scroll past; one that asks nothing leads with what it is
 * and what period it covers. They are not the same shape, because they are not
 * the same kind of fact — "an assembly notice is something a holder can MISS;
 * accounts are something they read".
 *
 * A superseded filing keeps every one of its fields and loses its emphasis. It
 * is never hidden: a dispute is usually about what was said at a date, and a
 * register that showed only the current version could not answer that.
 */
function CovenantCard({ covenant, now }: { covenant: Covenant; now: Date }) {
  const locale = useLocale();
  const nowSeconds = Math.floor(now.getTime() / 1000);
  const deadline = deadlineOf(covenant, nowSeconds);
  const superseded = isSuperseded(covenant);
  const tone = covenantTone(covenant, deadline);

  return (
    <article className={`filing${superseded ? ' filing--superseded' : ''}`}>
      <header className="filing__head">
        <Chip tone={tone}>{t(kindKey(covenant.kind))}</Chip>
        <h4 className="filing__title">{covenant.title}</h4>
        {superseded && (
          <Chip tone="mute">
            {t('rwa.cov.supersededBy', { id: covenant.supersededBy })}
          </Chip>
        )}
      </header>

      {/*
        The deadline, when there is one. Set as its own block above everything
        else on the card rather than as one more label/value row, because a row
        reading "Acts by: 4 October" among six other rows is exactly as easy to
        skip as the row above it — which is the failure the chain added the
        field to prevent.
      */}
      {deadline.state !== 'none' && (
        <div className={`filing__deadline filing__deadline--${superseded ? 'mute' : deadline.state}`}>
          <span className="y-label">{t(obligationKey(deadline.state))}</span>
          <span className="filing__by">{formatDate(new Date(deadline.at * 1000), locale)}</span>
          <span className="filing__left">
            {deadline.state === 'open'
              ? t('rwa.cov.timeLeft', { time: formatDuration(deadline.remainingSeconds, locale) })
              : t('rwa.cov.deadlineGone')}
          </span>
          {superseded && <span className="filing__left">{t('rwa.cov.deadlineCorrected')}</span>}
        </div>
      )}

      <Fields>
        {hasPeriod(covenant) ? (
          <Field label={t('rwa.cov.period')}>
            {t('rwa.cov.periodSpan', {
              from: formatDate(new Date(covenant.periodStart * 1000), locale),
              to: formatDate(new Date(covenant.periodEnd * 1000), locale),
            })}
          </Field>
        ) : (
          <Field label={t('rwa.cov.period')}>
            <span className="muted">{t('rwa.cov.aMoment')}</span>
          </Field>
        )}
        <Field label={t('rwa.cov.publishedBy')}><Address value={covenant.publishedBy} /></Field>
        <Field label={t('rwa.cov.publishedAt')}>
          {formatDate(new Date(covenant.publishedAt * 1000), locale)}
        </Field>
        {isRealId(covenant.supersedes) && (
          <Field label={t('rwa.cov.corrects')}>
            <span className="y-mono">#{covenant.supersedes}</span>
          </Field>
        )}
        <Field label={t('rwa.cov.digest')} wide>
          <Digest hex={covenant.digest} command={verifyCommand(covenant.digest, covenant.uri)} />
        </Field>
        <Field label={t('rwa.cov.where')} wide>
          {covenant.uri
            ? <span className="y-mono filing__uri">{covenant.uri}</span>
            : <span className="muted">{t('rwa.cov.noUri')}</span>}
        </Field>
      </Fields>
    </article>
  );
}

/**
 * The register of undertakings.
 *
 * Lives in the Documents folder beside the asset's manifest digest, which is
 * the thing it replaces in practice: the manifest is one string fixed at mint,
 * and "an operating business publishes for as long as it exists".
 *
 * The three outcomes of the read stay distinct. "Nothing has been filed" is an
 * accusation against an operator, and printing it because a node did not answer
 * would make that accusation against one who may have filed everything.
 */
function CovenantsPanel({ load, now }: { load: Load<Covenant[]>; now: Date }) {
  const locale = useLocale();
  const nowSeconds = Math.floor(now.getTime() / 1000);

  if (load.loading) {
    return <Panel title={t('rwa.cov.title')}><Loading what={t('rwa.reading')} /></Panel>;
  }

  if (!load.outcome.ok) {
    return (
      <Panel eyebrow={t('rwa.tabDocuments')} title={t('rwa.cov.title')}>
        {/*
          A refusal and a timeout need different sentences. The Covenants query
          did not exist before this upgrade, so a node running the older binary
          answers "unknown query path" — which is the chain saying it cannot
          answer, not the network failing to carry the question, and a reader
          owed an explanation deserves the right one.
        */}
        <Note tone="warn" title={t('rwa.cov.unreadableTitle')}>
          {load.outcome.reason === 'unreachable'
            ? t('rwa.cov.unreadableNetwork')
            : t('rwa.cov.unreadableModule')}
        </Note>
      </Panel>
    );
  }

  const all = load.outcome.value;
  if (all.length === 0) {
    return (
      <Panel eyebrow={t('rwa.tabDocuments')} title={t('rwa.cov.title')}>
        <p className="panel__lede">{t('rwa.cov.empty')}</p>
        <Note tone="mute">{t('rwa.cov.emptyWhy')}</Note>
      </Panel>
    );
  }

  const ordered = orderCovenants(all, nowSeconds);
  const open = openDeadlines(all, nowSeconds);

  return (
    <Panel eyebrow={t('rwa.tabDocuments')} title={t('rwa.cov.title')}>
      <p className="panel__lede">{t('rwa.cov.lede')}</p>

      {/*
        The banner exists because a holder who opens this folder at all is
        usually looking for the accounts, and the thing that can cost them
        something is the item they were not looking for.
      */}
      {open.length > 0 && (
        <Note tone="warn" title={t('rwa.cov.actionTitle')}>
          {t('rwa.cov.actionBody', {
            title: open[0]!.title,
            // The banner is the one line most likely to be read, so it says how
            // long rather than a date somebody has to subtract from.
            when: formatDuration(deadlineOf(open[0]!, nowSeconds).remainingSeconds, locale),
          })}
        </Note>
      )}

      <div className="filings">
        {ordered.map((c) => <CovenantCard key={c.id} covenant={c} now={now} />)}
      </div>

      <Note tone="mute" title={t('rwa.cov.whatItProvesTitle')}>
        {t('rwa.cov.whatItProves')}
      </Note>
    </Panel>
  );
}

/* ------------------------------------------------------------------- page */

export function Vehicle({ assetId, account }: { assetId: string; account: Account }) {
  const locale = useLocale();
  const [record, reload] = useVehicle(assetId);
  const [collections] = useCollections();
  const [dialog, setDialog] = useState<'claim' | 'redeem' | 'dispute' | null>(null);
  // Which folder is open. In state rather than the URL for now: the route is
  // /v/:assetId and adding a second segment would change every link to a
  // tokenisation that exists. A folder worth linking to gets its own segment
  // when the creation surface lands and the routes are revisited anyway.
  const [folder, setFolder] = useState('overview');

  const asset = record.loading || !record.outcome.ok ? null : record.outcome.value.asset;
  const parcelId = asset?.parcelId ?? '0';
  const fractionDenom = asset?.fractionDenom ?? '';

  const land = useLand(parcelId, Math.floor(Date.now() / 1000));
  const [holding, reloadHolding] = useHolding(assetId, fractionDenom, addressOf(account));
  const cap = useHolders(fractionDenom);
  const [covenants] = useCovenants(assetId);

  // Every deadline on this page is decided by a keeper against BlockTime(), so
  // the countdown beside one has to run on the same clock. A reader whose
  // laptop is four minutes fast must not be told an assembly has closed when
  // the chain still accepts it.
  const head = useHead();
  const now = chainNow(head).at;

  if (record.loading) return <div className="page"><Loading what={t('rwa.reading')} /></div>;

  if (!record.outcome.ok) {
    if (record.outcome.reason === 'unreachable') {
      return <div className="page"><Unreachable detail={record.outcome.detail} onRetry={reload} /></div>;
    }
    return (
      <div className="page">
        <Empty title={t('rwa.noVehicleTitle')}>
          <p>{t('rwa.noVehicleBody', { id: assetId })}</p>
          <p><Link to="/" className="linkish">{t('rwa.backToList')}</Link></p>
        </Empty>
      </div>
    );
  }

  const { asset: a, vault, sale } = record.outcome.value;
  const collection = collections.loading || !collections.outcome.ok
    ? undefined
    : collections.outcome.value.find((c) => c.id === a.collectionId);

  // Without the collection there is no threshold, no window and no bond — so
  // there is no honest way to grade the protection or run the sale clock. The
  // page says so rather than defaulting to zeroes, which would grade a sound
  // collection as offering nothing.
  const state: SaleState | null = collection
    ? saleState(a, sale, collection, new Date())
    : null;

  const protection = collection ? protectionOf(collection) : null;

  const held = holding.loading || !holding.outcome.ok ? null : holding.outcome.value;

  // What this tokenisation is, read out of its uri: the only field on the asset
  // that carries a name. See descriptorOf.
  const desc = descriptorOf(a.uri);

  // Everything paid into the vault over the tokenisation's life, across every
  // denomination it has been funded in. Summed rather than assumed to be one
  // coin: `funded` is a repeated Coin and a vehicle refunded in a second
  // currency would otherwise show only the first.
  const fundedTotal = vault
    ? vault.funded.reduce((sum, c) => sum + BigInt(c.amount || '0'), 0n).toString()
    : '0';

  // Concentration: the share of the whole held by the largest single account.
  // The question a cap table is opened to answer.
  const capRows = cap.loading || !cap.outcome.ok ? null : cap.outcome.value.rows;
  const supplyInt = BigInt(held?.supply || '0');
  const largest = capRows && capRows.length > 0 && supplyInt > 0n
    ? Number((BigInt(capRows[0]!.amount) * 10000n) / supplyInt) / 100
    : null;
  const share = shareholding(held?.balance ?? '0', held?.supply ?? '0', a.holderShareBps);

  const acts = state
    ? actionsFor(a, state, held?.balance ?? '0', held?.owed?.amount ?? '0', canSign(account))
    : null;

  // What the holding cost, and what it has returned on that cost.
  //
  // `owed` is passed as null when the entitlement query FAILED, so the return
  // reports "not known" rather than a return of zero. A zero here would be the
  // most damaging possible rounding of a network error: it tells somebody their
  // investment has earned nothing.
  const cost = costOf(held?.position ?? null, held?.balance ?? '0');
  const ret = returnOn(
    cost,
    held?.owedUnknown ? null : (held?.owed?.amount ?? null),
    held?.balance ?? '0',
  );

  // Token-seconds accruing in the open period. Measured against the chain's
  // clock, because that is the clock the keeper will integrate against when the
  // period closes.
  const pending = vault
    ? pendingAccrual(
      held?.position ?? null,
      vault,
      held?.balance ?? '0',
      Math.floor(now.getTime() / 1000),
    )
    : null;

  return (
    <div className="page page--vehicle">
      <header className="page__head">
        <p className="y-eyebrow">
          <Link to="/" className="crumb">{t('rwa.navVehicles')}</Link>
          {' / '}
          {a.collectionId || t('rwa.noCollection')}
        </p>
        {/* The name comes out of the uri, which is the only field on the asset
            that carries one. Falling back to "Tokenisation 4" rather than
            inventing something: an asset whose uri names nothing has no name,
            and a page that made one up would label it with a fact the chain
            does not hold. See descriptorOf. */}
        <div className="page__titleRow">
          <h1 className="page__title">
            {desc.name ?? t('rwa.vehicleN', { id: a.id })}
          </h1>
          <Chip tone={statusTone(a.status)}>{t(statusKey(a.status))}</Chip>
        </div>
        {/* The raw uri was the page's opening line: machine text, presented as
            the headline. It is a reference, so it goes where references go. */}
        <p className="page__lede">
          {desc.name
            ? t('rwa.vehicleN', { id: a.id })
            : (a.uri || t('rwa.noDoc'))}
        </p>
      </header>

      {/* The figures a reader came for, before the prose explaining them.
          Every one is read from the chain.

          What is deliberately absent is a PRICE, and therefore a yield.
          Nothing on this chain knows what a token was sold for until a sale is
          reported or a market exists. A headline yield computed from an assumed
          price would be the most quoted number on this page and the only
          invented one on it. */}
      <KpiBand>
        <Kpi
          label={t('rwa.kpiIssued')}
          value={a.fractionDenom
            ? <Amount amount={held?.supply ?? null} denom={a.fractionDenom} />
            : t('rwa.kpiNotIssued')}
          hint={t('rwa.kpiIssuedHint')}
        />
        <Kpi
          label={t('rwa.kpiHolders')}
          value={cap.loading ? '…' : cap.outcome.ok ? String(cap.outcome.value.rows.length) : '—'}
          hint={!cap.loading && cap.outcome.ok && cap.outcome.value.more
            ? t('rwa.kpiHoldersMore')
            : t('rwa.kpiHoldersHint')}
        />
        <Kpi
          label={t('rwa.kpiLargest')}
          value={<Percent value={largest} places={1} />}
          tone={largest !== null && largest >= 50 ? 'warn' : undefined}
          hint={t('rwa.kpiLargestHint')}
        />
        <Kpi
          label={t('rwa.kpiDistributed')}
          value={vault
            ? <Amount amount={fundedTotal} denom={vault.denom} />
            : '—'}
          hint={t('rwa.kpiDistributedHint')}
        />
        <Kpi
          label={t('rwa.kpiHolderShare')}
          value={<Percent value={bpsToPercent(a.holderShareBps)} places={2} />}
          hint={t('rwa.kpiHolderShareHint')}
        />
        <Kpi
          label={t('rwa.kpiStatus')}
          value={t(statusKey(a.status))}
          tone={statusTone(a.status)}
          hint={t('rwa.kpiStatusHint')}
        />
      </KpiBand>

      {/* Folders, because one scrolling column is not a document. Finding the
          register of holders previously meant scrolling past a risk
          disclosure. */}
      <Folders
        active={folder}
        onSelect={setFolder}
        tabs={[
          { id: 'overview', label: t('rwa.tabOverview') },
          {
            id: 'holders',
            label: t('rwa.tabHolders'),
            count: cap.loading || !cap.outcome.ok ? null : cap.outcome.value.rows.length,
          },
          {
            id: 'documents',
            label: t('rwa.tabDocuments'),
            // The count is the number of FILINGS, and a live deadline shows as
            // a mark beside it: a reader who never opens this folder should
            // still be able to tell that something in it has a clock on it.
            count: covenants.loading || !covenants.outcome.ok
              ? null
              : covenants.outcome.value.length,
            flag: !covenants.loading && covenants.outcome.ok
              && openDeadlines(covenants.outcome.value, Math.floor(now.getTime() / 1000)).length > 0,
          },
          { id: 'distributions', label: t('rwa.tabDistributions') },
          // Last, deliberately: somebody arrives to understand the
          // vehicle and leaves to trade it. A market tab first would put
          // a price in front of the terms that decide whether it is one.
          { id: 'market', label: t('mkt.tab') },
        ]}
      />

      <div className="split">
        <div className="split__main">
          {folder === 'market' && (
            <Market assetId={a.id} denom={a.fractionDenom}
                    account={account} balance={held?.balance ?? '0'} />
          )}

          {folder === 'holders' && (
            <CapTable cap={cap} supply={held?.supply ?? null} sponsor={a.owner}
                      denom={a.fractionDenom} />
          )}

          {/* The register of undertakings comes FIRST in this folder, above the
              mint-time manifest. The manifest is one string fixed when title
              was created; the covenants are what the operator has published
              since, and they are what a holder opened this folder to find. */}
          {folder === 'documents' && <CovenantsPanel load={covenants} now={now} />}

          {folder === 'documents' && (
            <Panel eyebrow={t('rwa.tabDocuments')} title={t('rwa.docsTitle')}>
              {desc.docsDigest ? (
                <>
                  <p className="panel__lede">{t('rwa.docsLede')}</p>
                  <Fields>
                    <Field label={t('rwa.docsManifest')} wide>
                      <span className="y-mono">{desc.docsDigest}</span>
                    </Field>
                  </Fields>
                  <Note tone="mute" title={t('rwa.docsOneFileTitle')}>{t('rwa.docsOneFileBody')}</Note>
                </>
              ) : (
                <p className="panel__lede">{t('rwa.docsNone')}</p>
              )}
            </Panel>
          )}

          {folder === 'overview' && (
          <Panel eyebrow={t('rwa.whatEyebrow')} title={t('rwa.whatTitle')}>
            <Fields>
              <Field label={t('rwa.collection')}>
                <span className="y-mono">{a.collectionId || '—'}</span>
              </Field>
              <Field label={t('rwa.sponsor')}><Address value={a.owner} /></Field>
              <Field label={t('rwa.shareDenom')}>
                {a.fractionDenom
                  ? <span className="y-mono">{a.fractionDenom}</span>
                  : <span className="muted">{t('rwa.notIssued')}</span>}
              </Field>
              <Field label={t('rwa.tokensCarry')}>
                {a.fractionDenom ? (
                  <>
                    <Percent value={bpsToPercent(a.holderShareBps)} />
                    <span className="field__aside">
                      {t('rwa.sponsorKeeps', { percent: bpsToPercent(10_000 - a.holderShareBps) })}
                    </span>
                  </>
                ) : <span className="muted">{t('rwa.notIssued')}</span>}
              </Field>
              <Field label={t('rwa.sharesInIssue')}>
                {held?.supply
                  ? <Amount amount={held.supply} denom={a.fractionDenom} />
                  : <span className="muted">{t('rwa.notIssued')}</span>}
              </Field>
              <Field label={t('rwa.incomeDenom')}>
                {/*
                  The currency people call it, not the base denom the chain
                  stores. `ucdf` is a unit nobody outside this repository reads,
                  and it is the currency a holder is being promised income in.
                */}
                {vault ? <Symbol denom={vault.denom} /> : <span className="muted">—</span>}
              </Field>
            </Fields>

            <Raw>
              <pre className="y-mono">{JSON.stringify({ asset: a, vault, sale }, null, 2)}</pre>
            </Raw>
          </Panel>

          )}

          {/* Only shown when this tokenisation IS over registered land, and only
              in the overview. A tokenisation over a company renders nothing
              here; see LandPanel. */}
          {folder === 'overview' && (land.loading
            ? <Panel title={t('rwa.landTitle')}><Loading what={t('rwa.reading')} /></Panel>
            : land.outcome.ok
              ? <LandPanel gate={land.outcome.value.gate} parcel={land.outcome.value.parcel}
                           parcelUnreadable={land.outcome.value.parcelUnreadable} />
              : <Panel title={t('rwa.landTitle')}><Note tone="warn">{t('rwa.auth.unreachable')}</Note></Panel>)}

          {/* --- what could go wrong. The part that matters most, so it stays
                  in the overview rather than behind a folder nobody opens. --- */}
          {folder === 'overview' && (
          <Panel eyebrow={t('rwa.riskEyebrow')} title={t('rwa.riskTitle')} className="panel--risk">
            <p className="panel__lede">{t('rwa.riskLede')}</p>

            {/* Moved out of the old "Not registered land" panel. The fact is
                worth keeping and the panel was not: a holder needs to know
                nothing outside this chain caps issuance, and they need to know
                it here, among the other things that can go wrong. */}
            {!land.loading && land.outcome.ok && land.outcome.value.gate.kind === 'not-land' && (
              <Note tone="mute" title={t('rwa.notLandTitle')}>{t('rwa.notLandBody')}</Note>
            )}

            {dilutionProtected(a) ? (
              <Note tone="ok" title={t('rwa.noDilutionTitle')}>{t('rwa.noDilutionBody')}</Note>
            ) : (
              <Note tone="mute">{t('rwa.notIssuedYet')}</Note>
            )}

            {protection ? (
              <>
                <div className="grade">
                  <span className="y-label">{t('rwa.protection')}</span>
                  <GradeChip level={protection.level} />
                </div>
                <ul className="findings">
                  {protection.findings.map((f) => (
                    <li key={f.key} className={`finding finding--${f.tone}`}>
                      <span className="finding__dot" aria-hidden="true" />
                      <span>
                        {t(f.key, {
                          ...f.values,
                          ...(f.values?.seconds !== undefined
                            ? { seconds: formatDuration(Number(f.values.seconds), locale) }
                            : {}),
                        })}
                      </span>
                    </li>
                  ))}
                </ul>
              </>
            ) : (
              <Note tone="warn">{t('rwa.noCollectionRead')}</Note>
            )}
          </Panel>
          )}

          {/* A reported sale is what every payout is computed from, so it lives
              with the distributions rather than in the overview. */}
          {folder === 'distributions' && collection && state && (
            <SalePanel
              sale={sale}
              state={state}
              collection={collection}
              disputeWhy={acts?.dispute.whyKey}
              onDispute={acts?.dispute.enabled && canSign(account)
                ? () => setDialog('dispute')
                : null}
            />
          )}

          {/* The vault had no panel anywhere: what has been paid in, what is
              still held against it, and the per-token index every entitlement
              on this chain is computed from. */}
          {folder === 'distributions' && (
            <Panel eyebrow={t('rwa.tabDistributions')} title={t('rwa.vaultTitle')}>
              {vault ? (
                <Fields>
                  {/* No <Symbol> after this: Amount already ends in the unit,
                      and the pair printed "15,000,000 USD USD". The same
                      mistake was made once before with SRMT. */}
                  <Field label={t('rwa.kpiDistributed')}>
                    <Amount amount={fundedTotal} denom={vault.denom} />
                  </Field>
                  {/* What the module still holds against THIS tokenisation:
                      funded and not yet claimed. One module account serves
                      every vehicle, so this ledger rather than a bank balance
                      is the only thing that says what belongs to whom -- and
                      the gap between it and what was funded is what holders
                      have already taken out. */}
                  <Field label={t('rwa.vaultHeld')}>
                    {vault.held.length > 0
                      ? vault.held.map((c) => (
                        <span key={c.denom} className="mine__coin">
                          <Amount amount={c.amount} denom={c.denom} />
                        </span>
                      ))
                      : <span className="muted">{t('rwa.vaultHoldsNothing')}</span>}
                  </Field>
                  {/* The period is what pro-rata temporis apportions, so the
                      open one is the span every holder's token-seconds are
                      being counted over right now. */}
                  <Field label={t('rwa.vaultPeriod')}>
                    <span className="y-num">
                      {vault.periodIndex === '0'
                        ? t('rwa.accrual.firstPeriod')
                        : t('rwa.accrual.period', { n: vault.periodIndex })}
                    </span>
                  </Field>
                  <Field label={t('rwa.vaultPeriodStart')}>
                    {vault.periodStart > 0
                      ? formatDate(new Date(vault.periodStart * 1000), locale)
                      : <span className="muted">—</span>}
                    {/* Not "held since". For a vehicle that predates pro-rata
                        temporis this is the UPGRADE HEIGHT's time, because the
                        chain has no record of when balances moved before it.
                        Presenting it as issuance would be wrong by the whole
                        life of the vehicle. */}
                    <span className="field__aside">{t('rwa.vaultPeriodStartNote')}</span>
                  </Field>
                  <Field label={t('rwa.vaultPerToken')} wide>
                    {/* The index is carried at 1e18 and the denomination has its
                        own exponent, so a per-token figure is the index divided
                        by both. Shown because it is the number every
                        entitlement is derived from, and a holder who cannot see
                        it cannot check their own. */}
                    <span className="y-mono">
                      {(Number(BigInt(vault.cumulativePerToken || '0') / 1000000000000n) / 1e6
                        / Math.pow(10, resolveDenom(vault.denom).exponent ?? 0)).toFixed(6)}
                    </span>{' '}
                    <Symbol denom={vault.denom} /> {t('rwa.vaultPerTokenUnit')}
                  </Field>
                </Fields>
              ) : (
                <p className="panel__lede">{t('rwa.vaultNone')}</p>
              )}
            </Panel>
          )}
        </div>

        {/* --- what I own, and what I can do about it --- */}
        <aside className="split__side">
          <Panel eyebrow={t('rwa.mineEyebrow')} title={t('rwa.mineTitle')} className="panel--mine">
            {account.mode === 'none' ? (
              <p className="muted">{t('rwa.attachToSee')}</p>
            ) : !a.fractionDenom ? (
              <p className="muted">{t('rwa.notIssued')}</p>
            ) : holding.loading ? (
              <Loading what={t('rwa.reading')} />
            ) : (
              <>
                <div className="mine__headline">
                  <span className="y-label">{t('rwa.ofAsset')}</span>
                  <span className="mine__figure"><Percent value={share.ofAsset} places={3} /></span>
                  <span className="mine__sub">
                    <Percent value={share.ofSupply} places={3} /> {t('rwa.ofShares')}
                  </span>
                </div>

                <Fields>
                  <Field label={t('rwa.youHold')}>
                    <Amount amount={held?.balance ?? null} denom={a.fractionDenom} />
                  </Field>
                  <Field label={t('rwa.claimableNow')}>
                    {held?.owedUnknown
                      ? <span className="muted">{t('rwa.owedUnknown')}</span>
                      : <Amount amount={held?.owed?.amount ?? null}
                                denom={held?.owed?.denom ?? vault?.denom ?? ''} />}
                  </Field>
                  <Field label={t('rwa.vaultPaid')}>
                    {vault && vault.funded.length > 0
                      ? vault.funded.map((c) => (
                        <span key={c.denom} className="mine__coin">
                          <Amount amount={c.amount} denom={c.denom} />
                        </span>
                      ))
                      : <span className="muted">{t('rwa.nothingPaidIn')}</span>}
                  </Field>
                </Fields>

                {/* What it cost, and what that cost has returned. Beneath the
                    balance rather than above it: the balance is the fact, the
                    return is the judgement about it. */}
                <CostBlock
                  cost={cost}
                  ret={ret}
                  unknown={held?.positionUnknown ?? false}
                  incomeDenom={held?.owed?.denom ?? vault?.denom ?? ''}
                  shareDenom={a.fractionDenom}
                />

                {/* Only where there is a vault to accrue against. Without one
                    there is no period, and a panel about an open period on a
                    vehicle that has never distributed would be inventing one. */}
                {vault && pending && (
                  <AccrualBlock pending={pending} now={now} incomeDenom={vault.denom} />
                )}

                <p className="mine__note">{t('rwa.entitlementWhy')}</p>

                {acts && (
                  <div className="mine__acts">
                    <button
                      type="button"
                      className="btn"
                      disabled={!acts.claim.enabled}
                      onClick={() => setDialog('claim')}
                    >
                      {t('rwa.actClaim')}
                    </button>
                    {!acts.claim.enabled && acts.claim.whyKey && (
                      <p className="mine__why">{t(acts.claim.whyKey)}</p>
                    )}

                    <button
                      type="button"
                      className="btn btn--ghost btn--bad"
                      disabled={!acts.redeem.enabled}
                      onClick={() => setDialog('redeem')}
                    >
                      {t('rwa.actRedeem')}
                    </button>
                    {!acts.redeem.enabled && acts.redeem.whyKey && (
                      <p className="mine__why">{t(acts.redeem.whyKey)}</p>
                    )}
                  </div>
                )}
              </>
            )}
          </Panel>

          <Panel eyebrow={t('rwa.termsEyebrow')} title={t('rwa.termsTitle')} className="panel--terms">
            {collection ? (
              <Fields>
                <Field label={t('rwa.verification')}>
                  {t(`rwa.verify.${collection.verification}`)}
                </Field>
                <Field label={t('rwa.threshold')}>
                  {collection.verification === 'VERIFY_ATTESTORS'
                    ? <span className="y-num">{collection.attestationThreshold}</span>
                    : <span className="muted">{t('rwa.attestNotUsed')}</span>}
                </Field>
                <Field label={t('rwa.challengeWindow')}>
                  <span className="y-num">
                    {collection.challengeWindowSeconds > 0
                      ? formatDuration(collection.challengeWindowSeconds, locale)
                      : t('rwa.windowNone')}
                  </span>
                </Field>
                <Field label={t('rwa.bond')}>
                  <Percent value={bpsToPercent(collection.disputeBondBps)} />
                  <span className="field__aside">{t('rwa.bondNote')}</span>
                </Field>
                <Field label={t('rwa.collectionAuthority')}>
                  <Address value={collection.authority} />
                </Field>
              </Fields>
            ) : (
              <p className="muted">{t('rwa.noCollectionRead')}</p>
            )}
          </Panel>
        </aside>
      </div>

      {/* --- the dialogs --- */}
      {dialog === 'claim' && canSign(account) && (
        <ClaimDialog
          account={account}
          assetId={a.id}
          owed={held?.owed?.amount ?? '0'}
          owedDenom={held?.owed?.denom ?? vault?.denom ?? ''}
          onClose={() => setDialog(null)}
          onDone={() => { reloadHolding(); reload(); }}
        />
      )}

      {dialog === 'redeem' && canSign(account) && (
        <RedeemDialog
          account={account}
          assetId={a.id}
          balance={held?.balance ?? '0'}
          shareDenom={a.fractionDenom}
          accrued={held?.owed?.amount ?? '0'}
          payoutDenom={held?.owed?.denom ?? vault?.denom ?? ''}
          onClose={() => setDialog(null)}
          onDone={() => { reloadHolding(); reload(); }}
        />
      )}

      {dialog === 'dispute' && canSign(account) && sale && state && (
        <DisputeDialog
          account={account}
          assetId={a.id}
          bond={state.bond}
          bondDenom={sale.price.denom}
          price={sale.price}
          onClose={() => setDialog(null)}
          onDone={() => reload()}
        />
      )}
    </div>
  );
}
