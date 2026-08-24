/**
 * The activity feed, rendered.
 *
 * `feed.ts` decides what each row means; this decides how much weight it gets.
 * The rule the old feed broke is the abstraction rule's plainest consequence:
 * **an interface that gives every row the same weight has made no
 * interpretation.** Ten identical lines with identical icons is a JSON array
 * with a stylesheet on it.
 *
 * So there are three weights, and they follow the tier:
 *
 *   Done         the headline size, the amount in mono at display size, a solid
 *                marker. Something is true now that was not.
 *   In progress  body size, muted, a hollow marker. A vote, a submission.
 *   Housekeeping smallest, faint, hidden entirely from the everyday view.
 *
 * Refusals are their own treatment rather than a red tint on a normal row,
 * because "the payment was made" and "the payment was refused" must not be one
 * character apart.
 */

import { Fragment, useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { formatNumber, t, timeAgo, type MessageKind } from '@yamale/chain';

import { client } from './chain.ts';
import { formatCoinList } from './amount.ts';
import { messageLabel, type FeedEntry, type FeedTier } from './feed.ts';
import { Account, TxHash } from './Identifier.tsx';
import { Empty } from './components.tsx';

/**
 * Denom metadata, once per session.
 *
 * The exponent has to come from the chain rather than from a guess: x/stablecoin
 * publishes metadata when governance approves an issuer, so a currency added
 * after launch is readable without a client release — and a guessed exponent
 * misstates an amount by a factor of a million.
 */
export function useDenomRegistry() {
  const { data } = useQuery({
    queryKey: ['denomRegistry'],
    queryFn: () => client.denomRegistry(),
    staleTime: Infinity,
  });
  return data;
}

/** What area of the chain a row belongs to. Encodes the module, not decoration. */
const KIND_GLYPH: Record<MessageKind, string> = {
  transfer: '⇄',
  payment: '⇄',
  staking: '◆',
  governance: '§',
  trade: '⇋',
  treasury: '▣',
  issuance: '＋',
  admin: '⚙',
  other: '·',
};

/**
 * A few messages whose module glyph would mislead.
 *
 * Issuing somebody an identity is `admin` in the decoder's taxonomy, and a
 * spanner beside "was issued a Yamale user ID" reads as a settings change rather
 * than as a person joining the network.
 */
const TYPE_GLYPH: Record<string, string> = {
  '/blockchain.alias.v1.MsgRegisterAlias': '◉',
  '/blockchain.alias.v1.MsgRotateAlias': '◉',
  '/blockchain.alias.v1.MsgSetJurisdiction': '⌖',
};

const TIER_WORD: Record<FeedTier, string> = {
  outcome: 'xp.feed.outcome',
  step: 'xp.feed.step',
  routine: 'xp.feed.routine',
};

export function FeedList({
  entries,
  stopped,
  expert,
}: {
  entries: FeedEntry[];
  stopped?: boolean;
  expert?: boolean;
}) {
  const registry = useDenomRegistry();

  if (entries.length === 0) {
    return (
      <Empty
        title={t('xp.feed.emptyTitle')}
        hint={stopped ? t('xp.feed.emptyStopped') : t('xp.feed.emptyHint')}
      />
    );
  }

  return (
    <ul className="feed">
      {entries.map((entry) => (
        <FeedRow key={entry.key} entry={entry} registry={registry} expert={expert} />
      ))}
    </ul>
  );
}

function FeedRow({
  entry,
  registry,
  expert,
}: {
  entry: FeedEntry;
  registry: ReturnType<typeof useDenomRegistry>;
  expert?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const amount = formatCoinList(entry.coins, registry);
  const tier = entry.failed ? 'refused' : entry.tier;

  return (
    <li className={`feed__row feed__row--${tier}`}>
      <span className={`feed__marker feed__marker--${tier}`} aria-hidden="true">
        {entry.failed ? '✕' : (TYPE_GLYPH[entry.typeUrl] ?? KIND_GLYPH[entry.kind])}
      </span>

      <div className="feed__body">
        <p className="feed__headline">{entry.headline}</p>

        <div className="feed__meta">
          {/* The loudest row needs no label — its weight, its marker and its
              amount already say it. The quiet ones need to say why they are
              quiet, and a refusal needs to say so in words rather than in a
              tint. The label is still announced on every row, because none of
              that reaches a screen reader. */}
          {entry.failed ? (
            <span className="y-chip y-chip--bad">{t('xp.feed.refused')}</span>
          ) : entry.tier === 'outcome' ? (
            <span className="visually-hidden">{t('xp.feed.outcome')}</span>
          ) : (
            <span className="y-chip y-chip--mute">{t(TIER_WORD[entry.tier])}</span>
          )}

          {/* How it was approved. The tally is the difference between "it was
              carried out" and "three of three custodians agreed and it was
              carried out", and only one of those is auditable. */}
          {entry.approval?.tally ? (
            <span className="feed__fact">
              {t('xp.feed.approved', {
                yes: String(entry.approval.tally.yes),
                total: String(
                  entry.approval.tally.yes +
                    entry.approval.tally.no +
                    entry.approval.tally.abstain +
                    entry.approval.tally.veto,
                ),
              })}
            </span>
          ) : null}

          {entry.approval?.ran === 'failure' ? (
            <span className="y-chip y-chip--warn">{t('xp.feed.ranFailed')}</span>
          ) : null}

          {entry.request && entry.typeUrl === '/cosmos.group.v1.MsgVote' ? (
            <span className="feed__fact">
              {t('xp.feed.votedOn', {
                option: entry.request.option,
                what: entry.request.title ?? t('xp.feed.request', { id: entry.request.proposalId }),
              })}
            </span>
          ) : null}

          {/* The parties as identifiers, on the rows where somebody acts on
              them. The headline already names them in words; this is where the
              swatch, the link, copy-on-click and the reveal live, and an
              auditor works from the identifier rather than from the sentence.
              Only on outcome rows: repeating them under a vote would be
              duplication with nothing behind it. */}
          {entry.tier === 'outcome' || entry.failed ? (
            <>
              {entry.actor ? <Account address={entry.actor} compact /> : null}
              {entry.counterparty && entry.counterparty !== entry.actor ? (
                <>
                  <span className="feed__arrow" aria-hidden="true">
                    →
                  </span>
                  <Account address={entry.counterparty} compact />
                </>
              ) : null}
            </>
          ) : null}

          <time className="feed__fact" dateTime={entry.timestamp} title={entry.timestamp}>
            {timeAgo(entry.timestamp)}
          </time>

          <Link className="feed__fact y-mono" to={`/block/${entry.height}`}>
            {t('xp.feed.inBlock', { height: formatNumber(entry.height) })}
          </Link>

          {expert ? (
            <>
              <TxHash hash={entry.hash} />
              <span className="y-chip y-chip--mute">{messageLabel(entry.typeUrl)}</span>
            </>
          ) : null}
        </div>

        {/* Why it was refused, in the chain's own words. Preserved rather than
            paraphrased: this is the sentence somebody quotes to support. */}
        {entry.failed && entry.failureReason ? (
          <details className="feed__raw">
            <summary>{t('xp.feed.whyRefused')}</summary>
            <pre className="raw y-scroll">{entry.failureReason}</pre>
          </details>
        ) : null}

        {/* The raw message, one disclosure away, on every row that hid
            something. Experts stop trusting an interface the moment they cannot
            audit it, and that trust is the product. */}
        <details
          className="feed__raw"
          open={open}
          onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}
        >
          <summary>{t('xp.feed.rawMessage')}</summary>
          {open ? (
            <>
              {entry.details?.length ? (
                <dl className="defs">
                  {entry.details.map((d) => (
                    <Fragment key={d.label}>
                      <dt>{d.label}</dt>
                      <dd>
                        {d.address && /^yml1/.test(d.value) ? (
                          <Account address={d.value} compact />
                        ) : (
                          d.value
                        )}
                      </dd>
                    </Fragment>
                  ))}
                </dl>
              ) : null}
              {entry.approval?.actions.length ? (
                <ul className="feed__actions">
                  {entry.approval.actions.map((action, i) => (
                    <li key={i}>{action}</li>
                  ))}
                </ul>
              ) : null}
              <pre className="raw y-scroll">{JSON.stringify(entry.raw, null, 2)}</pre>
            </>
          ) : null}
        </details>
      </div>

      {/* The amount, where a reader's eye already goes for one. Tabular so two
          rows can be compared by eye, which is the operation this audience
          performs most. */}
      {/* Nothing at all when nothing moved. An em dash in the money column is a
          glyph that encodes no fact, and on a phone it costs a whole line. */}
      <div className="feed__amount">
        {amount ? <span className="feed__figure y-mono y-num">{amount}</span> : null}
      </div>
    </li>
  );
}

