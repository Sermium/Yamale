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
import { formatAmount, t, useLocale } from '@yamale/chain';

import { ClaimDialog, DisputeDialog, RedeemDialog } from './Act.tsx';
import { GradeChip } from './Vehicles.tsx';
import { addressOf, canSign, type Account } from './address.ts';
import { useCollections, useHolding, useLand, useVehicle } from './data.ts';
import { formatDate, formatDuration } from './duration.ts';
import {
  Address,
  Amount,
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
  dilutionProtected,
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

  if (gate.kind === 'not-land') {
    return (
      <Panel eyebrow={t('rwa.underlyingEyebrow')} title={t('rwa.notLandTitle')}>
        <p className="panel__lede">{t('rwa.notLandBody')}</p>
      </Panel>
    );
  }

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

/* ------------------------------------------------------------------- page */

export function Vehicle({ assetId, account }: { assetId: string; account: Account }) {
  const locale = useLocale();
  const [record, reload] = useVehicle(assetId);
  const [collections] = useCollections();
  const [dialog, setDialog] = useState<'claim' | 'redeem' | 'dispute' | null>(null);

  const asset = record.loading || !record.outcome.ok ? null : record.outcome.value.asset;
  const parcelId = asset?.parcelId ?? '0';
  const fractionDenom = asset?.fractionDenom ?? '';

  const land = useLand(parcelId, Math.floor(Date.now() / 1000));
  const [holding, reloadHolding] = useHolding(assetId, fractionDenom, addressOf(account));

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
  const share = shareholding(held?.balance ?? '0', held?.supply ?? '0', a.holderShareBps);

  const acts = state
    ? actionsFor(a, state, held?.balance ?? '0', held?.owed?.amount ?? '0', canSign(account))
    : null;

  return (
    <div className="page page--vehicle">
      <header className="page__head">
        <p className="y-eyebrow">
          <Link to="/" className="crumb">{t('rwa.navVehicles')}</Link>
          {' / '}
          {a.collectionId || t('rwa.noCollection')}
        </p>
        <div className="page__titleRow">
          <h1 className="page__title">{t('rwa.vehicleN', { id: a.id })}</h1>
          <Chip tone={statusTone(a.status)}>{t(statusKey(a.status))}</Chip>
        </div>
        <p className="page__lede">
          {a.uri
            ? <a href={a.uri} rel="noopener noreferrer nofollow" target="_blank" className="linkish">{a.uri}</a>
            : t('rwa.noDoc')}
        </p>
      </header>

      <div className="split">
        <div className="split__main">
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

          {land.loading
            ? <Panel title={t('rwa.landTitle')}><Loading what={t('rwa.reading')} /></Panel>
            : land.outcome.ok
              ? <LandPanel gate={land.outcome.value.gate} parcel={land.outcome.value.parcel}
                           parcelUnreadable={land.outcome.value.parcelUnreadable} />
              : <Panel title={t('rwa.landTitle')}><Note tone="warn">{t('rwa.auth.unreachable')}</Note></Panel>}

          {/* --- what could go wrong. The part that matters most. --- */}
          <Panel eyebrow={t('rwa.riskEyebrow')} title={t('rwa.riskTitle')} className="panel--risk">
            <p className="panel__lede">{t('rwa.riskLede')}</p>

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

          {collection && state && (
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
