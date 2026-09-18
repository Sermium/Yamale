/**
 * Every tokenisation on the chain, and — far more often — the fact that there
 * are none.
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
 *
 * # What the card shows, and what it no longer does
 *
 * The desktop card carried five figures at once. On a 358px column that is a
 * wall, and four of the five were things a reader checks after they have picked
 * a vehicle rather than while they are picking one. So the card is the NAME, a
 * status, and the two facts that decide whether to open it: what a token
 * carries of the asset, and how well protected the sale price is. The rest —
 * the challenge window, the attestations still needed — is one tap deeper, on
 * the vehicle's own screen where it already lived.
 */
import { Link } from 'react-router-dom';
import { t, useLocale } from '@yamale/chain';

import type { Head } from './abci.ts';
import { useAssets, useCollections } from './data.ts';
import { formatDuration } from './duration.ts';
import { ChainLine, Chip, Detail, Empty, Field, Fields, Loading, Panel, Percent, Unreachable } from './ui.tsx';
import {
  bpsToPercent,
  protectionOf,
  saleState,
  descriptorOf,
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
  const protection = collection ? protectionOf(collection) : null;
  const sale = collection ? saleState(asset, null, collection, now) : null;
  const desc = descriptorOf(asset.uri);

  return (
    <li>
      {/* The whole card is the target. A tap area made of one word in the
          middle of a card is the commonest defect on a phone. */}
      <Link to={`/v/${asset.id}`} className="y-card y-card--link">
        <div className="y-card__head">
          <span className="y-eyebrow">{asset.collectionId || t('rwa.noCollection')}</span>
          <Chip tone={statusTone(asset.status)}>{t(statusKey(asset.status))}</Chip>
        </div>

        {/* The name comes from the uri, which is the only field on the asset
            that carries one. The list used to head every card "Tokenisation 4"
            and print the raw uri under it, so three cards differed by a digit
            and the one thing telling them apart was machine text. An asset
            whose uri names nothing still falls back to the number — inventing
            a name would label a card with a fact the chain does not hold. */}
        <h3 className="card__title">
          {desc.name ?? t('rwa.vehicleN', { id: asset.id })}
        </h3>
        <p className="card__ref">
          {desc.name ? t('rwa.vehicleN', { id: asset.id }) : (asset.uri || t('rwa.noDoc'))}
        </p>

        <div className="card__foot y-row">
          <div className="y-row__main">
            <div className="y-row__sub">{t('rwa.tokensCarry')}</div>
            <div className="y-row__title">
              <Percent value={asset.fractionDenom ? bpsToPercent(asset.holderShareBps) : null} />
            </div>
          </div>
          <div className="y-row__end">
            <div className="y-row__sub">{t('rwa.protection')}</div>
            {protection
              ? <GradeChip level={protection.level} />
              : <Chip tone="mute">{t('rwa.grade.unknown')}</Chip>}
          </div>
        </div>

        {/* How many independent attestations a reported sale price will need
            before anybody can be paid out against it. A fact about the
            collection rather than about a sale, and the single most useful
            thing to know before opening a vehicle at all. Stated as a sentence
            rather than as a bar at zero — the desktop card drew an empty meter
            here, which on a vehicle with no reported sale reads as "none
            gathered yet" about a sale that does not exist. */}
        {sale && sale.needed > 0 && (
          <p className="y-card__note">{t('rwa.attestationsNeeded', { need: sale.needed })}</p>
        )}
      </Link>
    </li>
  );
}

/**
 * One collection's terms, as a card rather than a row of a six-column table.
 *
 * A collection is the frame every vehicle issued under it inherits — how a sale
 * is verified, how long it can be challenged, what a challenge costs — so an
 * investor can read what the terms would be before anything exists to be sold.
 * That is exactly the sort of thing somebody reads once and carefully, which is
 * why it sits behind a disclosure instead of above the list.
 */
function CollectionCard({ collection }: { collection: Collection }) {
  const locale = useLocale();
  const protection = protectionOf(collection);

  return (
    <li className="y-card">
      <div className="y-card__head">
        <span className="y-mono">{collection.id}</span>
        <GradeChip level={protection.level} />
      </div>
      <Fields>
        <Field label={t('rwa.verification')}>{t(`rwa.verify.${collection.verification}`)}</Field>
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
        </Field>
      </Fields>
    </li>
  );
}

export function Vehicles({ now, head }: { now: Date; head: Head }) {
  const [collections, reloadCollections] = useCollections();
  const [assets, reloadAssets] = useAssets();

  if (collections.loading || assets.loading) {
    return <main className="y-screen"><Loading what={t('rwa.reading')} /></main>;
  }

  // The chain not answering is not a finding about the chain's contents, and
  // must never be rendered as one.
  if (!collections.outcome.ok && collections.outcome.reason === 'unreachable') {
    return (
      <main className="y-screen">
        <Unreachable detail={collections.outcome.detail}
                     onRetry={() => { reloadCollections(); reloadAssets(); }} />
      </main>
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
    <main className="y-screen">
      <h1 className="y-screen__title">{t('rwa.indexTitle')}</h1>
      <p className="y-screen__lede">{t('rwa.indexLede')}</p>

      {offered.length === 0 ? (
        <Empty title={t('rwa.emptyTitle')} at={t('rwa.readAt', { height })}>
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
        <ul className="y-cards">
          {offered.map((a) => (
            <VehicleCard key={a.id} asset={a} collection={byId.get(a.collectionId)} now={now} />
          ))}
        </ul>
      )}

      <Panel eyebrow={t('rwa.collectionsEyebrow')} title={t('rwa.collectionsTitle')}>
        <p className="panel__lede">{t('rwa.collectionsLede')}</p>
        {open.length === 0 ? (
          <p className="muted">{t('rwa.noCollections')}</p>
        ) : (
          <Detail summary={t('rwa.collectionsTitle')}>
            <ul className="y-cards">
              {open.map((c) => <CollectionCard key={c.id} collection={c} />)}
            </ul>
          </Detail>
        )}
      </Panel>

      <ChainLine head={head} />
    </main>
  );
}
