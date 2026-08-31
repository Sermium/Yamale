/**
 * Every vehicle on the chain, and — far more often — the fact that there are
 * none.
 *
 * The empty state was designed before the populated one, because this app will
 * spend most of its life in it. An empty list that renders perfectly tells a
 * reader nothing about whether there is nothing to show or nothing working, so
 * this screen distinguishes four situations and words each of them differently:
 *
 *   - no collections at all, which only governance can change;
 *   - collections open but nothing minted into them, which is the registry's
 *     move and not governance's;
 *   - assets minted but none fractionalised, so there is title and no
 *     shareholding to buy;
 *   - the chain did not answer, which is not a finding about the chain's
 *     contents at all.
 *
 * Each of those is a different sentence about who would have to act next.
 */
import { Link } from 'react-router-dom';
import { t, useLocale } from '@yamale/chain';

import { useAssets, useCollections } from './data.ts';
import { formatDuration } from './duration.ts';
import { Chip, Empty, Loading, Meter, Panel, Percent, Unreachable } from './ui.tsx';
import {
  bpsToPercent,
  protectionOf,
  saleState,
  statusKey,
  statusTone,
  type Asset,
  type Collection,
} from './vehicle.ts';

const GRADE_TONE = {
  none: 'bad', weak: 'warn', standard: 'ok', strong: 'ok',
} as const;

export function GradeChip({ level }: { level: keyof typeof GRADE_TONE }) {
  return <Chip tone={GRADE_TONE[level]}>{t(`rwa.grade.${level}`)}</Chip>;
}

function VehicleCard({ asset, collection, now }: {
  asset: Asset;
  collection: Collection | undefined;
  now: Date;
}) {
  const locale = useLocale();
  const protection = collection ? protectionOf(collection) : null;
  const sale = collection ? saleState(asset, null, collection, now) : null;

  return (
    <li className="vcard">
      <Link to={`/v/${asset.id}`} className="vcard__link">
        <div className="vcard__head">
          <span className="y-eyebrow">{asset.collectionId || t('rwa.noCollection')}</span>
          <Chip tone={statusTone(asset.status)}>{t(statusKey(asset.status))}</Chip>
        </div>

        <h3 className="vcard__title">
          {t('rwa.vehicleN', { id: asset.id })}
        </h3>

        <p className="vcard__uri">{asset.uri || t('rwa.noDoc')}</p>

        <dl className="vcard__stats">
          <div>
            <dt className="y-label">{t('rwa.tokensCarry')}</dt>
            <dd><Percent value={asset.fractionDenom ? bpsToPercent(asset.holderShareBps) : null} /></dd>
          </div>
          <div>
            <dt className="y-label">{t('rwa.protection')}</dt>
            <dd>{protection ? <GradeChip level={protection.level} /> : <Chip tone="mute">{t('rwa.grade.unknown')}</Chip>}</dd>
          </div>
          <div>
            <dt className="y-label">{t('rwa.challengeWindow')}</dt>
            <dd className="y-num">
              {collection
                ? (collection.challengeWindowSeconds > 0
                  ? formatDuration(collection.challengeWindowSeconds, locale)
                  : t('rwa.windowNone'))
                : '–'}
            </dd>
          </div>
        </dl>

        {sale && sale.needed > 0 && (
          <div className="vcard__meter">
            <span className="y-label">
              {t('rwa.attestationsNeeded', { need: sale.needed })}
            </span>
            <Meter value={0} of={sale.needed} tone="mute"
                   label={t('rwa.attestationsNeeded', { need: sale.needed })} />
          </div>
        )}
      </Link>
    </li>
  );
}

function CollectionRow({ collection }: { collection: Collection }) {
  const locale = useLocale();
  const protection = protectionOf(collection);

  return (
    <tr>
      <th scope="row" className="y-mono">{collection.id}</th>
      <td>{t(`rwa.verify.${collection.verification}`)}</td>
      <td className="y-num">
        {collection.verification === 'VERIFY_ATTESTORS' ? collection.attestationThreshold : '–'}
      </td>
      <td className="y-num">
        {collection.challengeWindowSeconds > 0
          ? formatDuration(collection.challengeWindowSeconds, locale)
          : t('rwa.windowNone')}
      </td>
      <td className="y-num">{bpsToPercent(collection.disputeBondBps)}%</td>
      <td><GradeChip level={protection.level} /></td>
    </tr>
  );
}

export function Vehicles({ now }: { now: Date }) {
  const [collections, reloadCollections] = useCollections();
  const [assets, reloadAssets] = useAssets();

  if (collections.loading || assets.loading) {
    return <div className="page"><Loading what={t('rwa.reading')} /></div>;
  }

  // The chain not answering is not a finding about the chain's contents, and
  // must never be rendered as one.
  if (!collections.outcome.ok && collections.outcome.reason === 'unreachable') {
    return (
      <div className="page">
        <Unreachable detail={collections.outcome.detail}
                     onRetry={() => { reloadCollections(); reloadAssets(); }} />
      </div>
    );
  }

  const open = collections.outcome.ok ? collections.outcome.value : [];
  const minted = assets.outcome.ok ? assets.outcome.value : [];
  const height = collections.outcome.ok ? collections.outcome.height : 0;
  const byId = new Map(open.map((c) => [c.id, c]));

  // Only fractionalised assets are things anybody can hold a share of. Title
  // that has never been fractionalised is a record, not an offering, and
  // listing it as one would be the app's first lie.
  const offered = minted.filter((a) => a.fractionDenom !== '');
  const titleOnly = minted.length - offered.length;

  return (
    <div className="page">
      <header className="page__head">
        <p className="y-eyebrow">{t('rwa.eyebrow')}</p>
        <h1 className="page__title">{t('rwa.indexTitle')}</h1>
        <p className="page__lede">{t('rwa.indexLede')}</p>
      </header>

      {offered.length === 0 ? (
        <Empty
          title={t('rwa.emptyTitle')}
          at={t('rwa.readAt', { height })}
        >
          <p>{t('rwa.emptyBody')}</p>
          {open.length === 0 ? (
            <p>{t('rwa.emptyNoCollections')}</p>
          ) : titleOnly > 0 ? (
            <p>{t('rwa.emptyTitleOnly', { collections: open.length, assets: titleOnly })}</p>
          ) : (
            <p>{t('rwa.emptyNoAssets', { collections: open.length })}</p>
          )}
        </Empty>
      ) : (
        <ul className="vgrid">
          {offered.map((a) => (
            <VehicleCard key={a.id} asset={a} collection={byId.get(a.collectionId)} now={now} />
          ))}
        </ul>
      )}

      {/*
        The collections are shown even when no vehicle has been minted into
        them, and especially then. A collection is the frame every vehicle
        issued under it inherits — how a sale is verified, how long it can be
        challenged, what a challenge costs — so an investor can read what the
        terms would be before anything exists to be sold.
      */}
      <Panel eyebrow={t('rwa.collectionsEyebrow')} title={t('rwa.collectionsTitle')}
             className="page__panel">
        <p className="panel__lede">{t('rwa.collectionsLede')}</p>

        {open.length === 0 ? (
          <p className="muted">{t('rwa.noCollections')}</p>
        ) : (
          <div className="y-scroll">
            <table className="table">
              <thead>
                <tr>
                  <th scope="col">{t('rwa.collection')}</th>
                  <th scope="col">{t('rwa.verification')}</th>
                  <th scope="col">{t('rwa.threshold')}</th>
                  <th scope="col">{t('rwa.challengeWindow')}</th>
                  <th scope="col">{t('rwa.bond')}</th>
                  <th scope="col">{t('rwa.protection')}</th>
                </tr>
              </thead>
              <tbody>
                {open.map((c) => <CollectionRow key={c.id} collection={c} />)}
              </tbody>
            </table>
          </div>
        )}
      </Panel>
    </div>
  );
}

