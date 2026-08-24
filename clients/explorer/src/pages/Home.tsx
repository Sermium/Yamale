import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { displayName, formatNumber, t, timeAgo, type Transaction } from '@yamale/chain';

import { client, useViewMode } from '../chain.ts';
import { buildFeed, everydayFeed } from '../feed.ts';
import { FeedList, useDenomRegistry } from '../Feed.tsx';
import { Card, ErrorState, Loading } from '../components.tsx';
import { Hash } from '../Identifier.tsx';

/**
 * The front page.
 *
 * Liveness is no longer here — it is the strip above every route, which is where
 * "is the chain healthy" belongs. What is left is the other question this
 * audience opens an explorer to answer: what happened, and to whose money.
 *
 * One feed, not two. The old page showed a different card in each view mode with
 * different copy and a different filter, which meant the simple and detailed
 * explorers could disagree about what had happened. There is now one list, and
 * the mode decides how much of it is shown and how much of each row is
 * unfolded — so a link shared from one lands somebody on the same events in the
 * other, which is the hand-off where an explorer earns its keep.
 */
export function HomePage() {
  const { mode } = useViewMode();
  const expert = mode === 'expert';
  const [showRoutine, setShowRoutine] = useState(false);

  const registry = useDenomRegistry();

  // Validator monikers, so a sentence can say "pi-2" instead of a bech32
  // operator address. Slow-moving, so it is fetched once a minute rather than
  // once a block.
  const names = useQuery({
    queryKey: ['validatorNames'],
    queryFn: () => client.validatorNames(),
    refetchInterval: 60_000,
  });

  const activity = useQuery({
    queryKey: ['recentActivity', names.data],
    queryFn: () => client.searchTransactions('tx.height>0', 40, { names: names.data }),
  });

  const blocks = useQuery({
    queryKey: ['recentBlocks'],
    queryFn: () => client.recentBlocks(12),
    enabled: expert,
  });

  // Every account named in the window, so the sentences can use the chain's own
  // identifier for a person instead of a bech32 prefix. `x/alias` issues one per
  // placed account and the binding is permanent, so this is cached for the
  // session and costs nothing on the poll.
  const parties = useMemo(() => addressesIn(activity.data ?? []), [activity.data]);

  const ids = useQuery({
    queryKey: ['partyNames', parties.join(',')],
    queryFn: async () => {
      const resolved: Record<string, string> = {};
      await Promise.all(
        parties.map(async (address) => {
          const id = await client.userIdOf(address).catch(() => null);
          // displayName ranks the four sources: the reader's own address-book
          // name first, then the chain's user ID, then the bare address. Only a
          // real answer goes in the map — a truncation is what the decoder falls
          // back to anyway.
          const name = displayName(address, () => id ?? undefined);
          if (name.kind !== 'address') resolved[address] = name.label;
        }),
      );
      return resolved;
    },
    enabled: parties.length > 0,
    staleTime: Infinity,
  });

  const feed = useMemo(
    () =>
      buildFeed(activity.data ?? [], {
        names: { ...names.data, ...ids.data },
        registry,
      }),
    [activity.data, names.data, ids.data, registry],
  );

  // The everyday view drops housekeeping and collapses a finished procedure to
  // its outcome. The detailed view keeps every message, in order.
  const rows = expert || showRoutine ? feed : everydayFeed(feed);
  const hidden = feed.length - rows.length;

  return (
    <>
      <h1>{t('xp.feed.title')}</h1>
      <p className="lede">{expert ? t('xp.feed.ledeExpert') : t('xp.feed.ledeSimple')}</p>

      {/* No card title: it would repeat the h1 immediately above it. The header
          carries only the control. */}
      <Card
        actions={
          hidden > 0 || showRoutine ? (
            <button
              type="button"
              className="linkbutton"
              onClick={() => setShowRoutine((s) => !s)}
              aria-pressed={showRoutine}
            >
              {showRoutine ? t('xp.feed.showEveryday') : `${t('xp.feed.showAll')} (${hidden})`}
            </button>
          ) : null
        }
        flush
      >
        {activity.isPending ? (
          <Loading label={t('xp.feed.loadingActivity')} />
        ) : activity.isError ? (
          <ErrorState error={activity.error} what="recent activity" />
        ) : (
          <FeedList entries={rows} expert={expert} />
        )}
      </Card>

      {expert ? (
        <Card title={t('xp.feed.blocks')} flush>
          {blocks.isPending ? (
            <Loading label={t('xp.feed.loadingBlocks')} />
          ) : blocks.isError ? (
            <ErrorState error={blocks.error} what="blocks" />
          ) : (
            <div className="y-scroll">
              <table className="grid">
                <thead>
                  <tr>
                    <th className="y-num">{t('col.height')}</th>
                    <th>{t('col.time')}</th>
                    <th className="y-num">{t('col.transactions')}</th>
                    <th>{t('col.hash')}</th>
                  </tr>
                </thead>
                <tbody>
                  {(blocks.data ?? []).map((b) => (
                    <tr key={b.height}>
                      <td className="y-num">
                        <Link to={`/block/${b.height}`} className="y-mono">
                          {formatNumber(b.height)}
                        </Link>
                      </td>
                      <td className="muted">{timeAgo(b.timestamp)}</td>
                      <td className="y-num">
                        {b.txCount === 0 ? (
                          <span className="faint">{t('xp.feed.emptyBlock')}</span>
                        ) : (
                          b.txCount
                        )}
                      </td>
                      <td>
                        <Hash value={b.hash} label="Block hash" />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      ) : null}
    </>
  );
}

/**
 * Distinct accounts named anywhere in a window of transactions.
 *
 * Sorted so the query key is stable across polls: the transaction list is a new
 * array every five seconds, and an unsorted derivation would re-key the name
 * lookup on every tick.
 */
function addressesIn(transactions: Transaction[]): string[] {
  const found = new Set<string>();
  for (const tx of transactions) {
    for (const message of tx.messages) {
      if (message.actor) found.add(message.actor);
      if (message.counterparty) found.add(message.counterparty);
    }
  }
  return [...found].sort();
}
