import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { displayName, formatNumber, t, timeAgo, type Transaction } from '@yamale/chain';

import { client, totalTransactions, useViewMode } from '../chain.ts';
import { buildFeed, everydayFeed, type FeedEntry } from '../feed.ts';
import { FeedList, useDenomRegistry } from '../Feed.tsx';
import { Card, ErrorState, Loading } from '../components.tsx';
import { Hash } from '../Identifier.tsx';
import { Kpi, KpiBand, Folders } from '../ui.tsx';

/** How many transactions the front page reads in one poll. */
const WINDOW = 40;

type Folder = 'everyday' | 'all' | 'blocks';

/**
 * The front page.
 *
 * Liveness is not here — it is the strip above every route, which is where "is
 * the chain healthy" belongs. What is left is the other question this audience
 * opens an explorer to answer: what happened, and to whose money.
 *
 * Three things changed in the 2026-09-16 pass.
 *
 * **A band of figures, above the feed.** The page opened with a heading, a
 * sentence and forty rows, and stated no quantity at all. A reader could not
 * learn how much this chain has ever done without counting rows.
 *
 * **Folders instead of a mode-gated column.** Recent blocks used to render
 * below the feed and only in expert mode, so reading them meant flipping a
 * global preference and scrolling past everything else. They are a peer of the
 * activity list, so they are a folder, and the choice is local to this page.
 *
 * **An empty feed now says which kind of empty it is.** This is the defect that
 * mattered. On the live chain the last hundred messages are all oracle price
 * submissions, so the everyday filter removed all of them and the page rendered
 * "Nothing has happened yet" — directly underneath its own control offering to
 * show the hundred things that had happened. An absence, a filter and a failed
 * read are three different facts and this page was rendering two of them as the
 * third.
 */
export function HomePage() {
  const { mode } = useViewMode();
  const expert = mode === 'expert';

  // Expert readers want every message by default; everyone else wants the
  // everyday view. The folder is still theirs to change afterwards — this only
  // decides where they land.
  const [folder, setFolder] = useState<Folder>(() => (expert ? 'all' : 'everyday'));

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
    queryFn: () => client.searchTransactions('tx.height>0', WINDOW, { names: names.data }),
  });

  // The chain's own count, which is not derivable from the window above. Slow
  // to change relative to a poll, and it is a separate request, so it refreshes
  // on its own schedule rather than with the feed.
  const total = useQuery({
    queryKey: ['totalTransactions'],
    queryFn: () => totalTransactions(),
    refetchInterval: 30_000,
  });

  // Blocks are no longer expert-only, so they load whenever their folder is
  // open. Gated on the folder rather than fetched always: an unopened folder
  // should not poll a node that may be somebody's laptop.
  const blocks = useQuery({
    queryKey: ['recentBlocks'],
    queryFn: () => client.recentBlocks(12),
    enabled: folder === 'blocks',
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
  // its outcome. The "all" folder keeps every message, in order.
  const everyday = useMemo(() => everydayFeed(feed), [feed]);
  const routine = feed.length - everyday.length;
  const rows = folder === 'everyday' ? everyday : feed;

  const read = activity.isSuccess;

  return (
    <>
      <h1>{t('xp.feed.title')}</h1>
      <p className="lede">{expert ? t('xp.feed.ledeExpert') : t('xp.feed.ledeSimple')}</p>

      {/* The figures before the list, at a size that says they are figures.
          Each one names where it came from, because they do not share a
          provenance: the first is the node's own count over all history, and
          the other three are derived from a window of forty transactions. A
          band that presented both in one voice would be claiming a precision
          the chain has not been asked for. */}
      <KpiBand>
        <Kpi
          label={t('xp.kpi.transactions')}
          // null is "we could not ask", and it must not render as a number.
          value={total.data === null || total.data === undefined ? '—' : formatNumber(total.data)}
          hint={total.data === null ? t('xp.kpi.unread') : t('xp.kpi.transactionsHint')}
          tone={total.data === null ? 'bad' : undefined}
        />
        <Kpi
          label={t('xp.kpi.outcomes')}
          value={read ? formatNumber(everyday.length) : '—'}
          hint={read ? t('xp.kpi.outcomesHint') : t('xp.kpi.unread')}
          tone={!read && activity.isError ? 'bad' : undefined}
        />
        <Kpi
          label={t('xp.kpi.routine')}
          value={read ? formatNumber(routine) : '—'}
          hint={read ? t('xp.kpi.routineHint') : t('xp.kpi.unread')}
          tone={!read && activity.isError ? 'bad' : undefined}
        />
        <Kpi
          label={t('xp.kpi.parties')}
          value={read ? formatNumber(parties.length) : '—'}
          hint={read ? t('xp.kpi.window', { count: String(feed.length) }) : t('xp.kpi.unread')}
          tone={!read && activity.isError ? 'bad' : undefined}
        />
      </KpiBand>

      <Folders
        label={t('xp.feed.title')}
        active={folder}
        onSelect={(id) => setFolder(id as Folder)}
        tabs={[
          // Counts only once the read succeeded. A "0" on a folder while the
          // node is not answering is a claim, and it is the wrong one.
          { id: 'everyday', label: t('xp.fold.everyday'), count: read ? everyday.length : null },
          { id: 'all', label: t('xp.fold.everything'), count: read ? feed.length : null },
          { id: 'blocks', label: t('xp.fold.blocks'), count: blocks.data?.length ?? null },
        ]}
      />

      {folder === 'blocks' ? (
        <Card flush>
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
      ) : (
        <Card flush>
          {activity.isPending ? (
            <Loading label={t('xp.feed.loadingActivity')} />
          ) : activity.isError ? (
            <ErrorState error={activity.error} what="recent activity" />
          ) : (
            <FeedList
              entries={rows}
              expert={expert}
              // The difference between "nothing has ever happened" and "a
              // hundred things happened and this folder hides all of them".
              filteredOut={folder === 'everyday' ? routine : 0}
              onShowAll={() => setFolder('all')}
            />
          )}
        </Card>
      )}
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

// Re-exported for the tests, which assert on the shape the page derives rather
// than on the JSX.
export type { FeedEntry };
