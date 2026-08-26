import { EMPTY_AMOUNT, formatAmount, resolveDenom, t, type TreasuryBalance } from '@yamale/chain';

/**
 * The one idea this application exists to convey, drawn rather than asserted.
 *
 * A treasury's balance is not one number. Part of it has been promised to
 * somebody, and that part is not merely earmarked by convention — x/treasury
 * holds custody in a module account and refuses a spend against locked balance
 * outright. No administrator, no vote and no quorum of signers can reach it.
 *
 * That is a strong claim and it was invisible: the interface stated it in a
 * sentence at the bottom of a card, which is where nobody reads and where
 * every product makes promises it does not keep. A diagram is the honest form
 * for it, because the thing being described is a *shape* — a wall with money on
 * both sides of it and different rules either side — and a paragraph describing
 * a shape asks the reader to draw it themselves.
 *
 * Every colour here comes from the shared tokens through CSS classes rather
 * than from attributes, so the figure follows the theme. The text inside it is
 * real text: it is selectable, it is searchable, and it renders in whatever
 * font the reader's device actually has.
 */
export function CustodyFigure() {
  return (
    <figure className="custody" aria-labelledby="custody-cap">
      <svg viewBox="0 0 980 330" role="img" aria-label={t('safe.figureAlt')}>
        <defs>
          <marker
            id="cus-arw"
            viewBox="0 0 10 10"
            refX="9"
            refY="5"
            markerWidth="7"
            markerHeight="7"
            orient="auto-start-reverse"
          >
            <path d="M 0 0 L 10 5 L 0 10 z" fill="currentColor" />
          </marker>
          {/* The committed block is hatched as well as tinted. A reader who
              sees no colour at all still sees a different surface, which is the
              whole requirement. */}
          <pattern id="cus-hatch" width="7" height="7" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
            <line className="cus__hatch" x1="0" y1="0" x2="0" y2="7" />
          </pattern>
        </defs>

        {/* What the treasury holds, as one bar cut in two. */}
        <g className="cus__bar">
          <rect className="cus__avail" x="1" y="34" width="558" height="56" rx="4" />
          <rect className="cus__locked" x="561" y="34" width="418" height="56" rx="4" />
          <rect className="cus__hatchfill" x="561" y="34" width="418" height="56" rx="4" fill="url(#cus-hatch)" />
        </g>
        <text className="cus__k" x="16" y="24">{t('safe.figAvailable')}</text>
        <text className="cus__k cus__k--lock" x="576" y="24">{t('safe.figCommitted')}</text>
        <text className="cus__in" x="16" y="68">{t('safe.figAvailableIn')}</text>
        <text className="cus__in cus__in--lock" x="576" y="68">{t('safe.figCommittedIn')}</text>

        {/* The wall. Drawn full height on purpose: it is not a divider between
            two columns of a table, it is the boundary of what anybody can
            reach.

            The two sides are stacked lists rather than a fan of arrows. Arrows
            in a row need short labels, and a figure whose legibility depends on
            the label staying short is a figure that breaks in the second
            language it is translated into — which on this product is four out
            of five readers. */}
        <line className="cus__wall" x1="560" y1="18" x2="560" y2="270" />

        {/* Out of available: the ordinary path, and the only one. */}
        <text className="cus__eyebrow cus__eyebrow--ok" x="1" y="122">{t('safe.figAllowed')}</text>
        <g>
          <line className="cus__ok" x1="6" y1="146" x2="30" y2="146" markerEnd="url(#cus-arw)" />
          <text className="cus__lab" x="42" y="151">{t('safe.figSpendPath')}</text>
        </g>
        <text className="cus__sub" x="42" y="174">{t('safe.figSpendWho')}</text>

        {/* Into committed: every path, refused — by the chain, not by a rule
            this page is enforcing. */}
        <text className="cus__refused" x="576" y="122">{t('safe.figRefused')}</text>
        {[146, 178, 210].map((y, i) => (
          <g key={y}>
            <line className="cus__bad" x1="580" y1={y - 5} x2="592" y2={y + 5} />
            <line className="cus__bad" x1="592" y1={y - 5} x2="580" y2={y + 5} />
            <text className="cus__lab" x="606" y={y + 4}>
              {[t('safe.figWho1'), t('safe.figWho2'), t('safe.figWho3')][i]}
            </text>
          </g>
        ))}

        {/* The one way out of the committed side, and it is not a spend. */}
        <g>
          <line className="cus__ok" x1="580" y1="244" x2="598" y2="244" markerEnd="url(#cus-arw)" />
          <text className="cus__sub" x="606" y="249">{t('safe.figClaimPath')}</text>
        </g>

        <text className="cus__rule" x="1" y="292">{t('safe.figRule1')}</text>
        <text className="cus__rule" x="1" y="312">{t('safe.figRule2')}</text>
      </svg>
      <figcaption id="custody-cap">{t('safe.figureCaption')}</figcaption>
    </figure>
  );
}

/**
 * Available against committed, as a proportion.
 *
 * Two figures side by side answer "how much of each"; they do not answer "how
 * much of this treasury can I actually touch", which is a ratio and therefore a
 * length. A treasurer scanning a list of six treasuries reads the bars and
 * never subtracts anything.
 *
 * Widths are computed in BigInt. A treasury balance routinely exceeds 2^53 in
 * base units, and `Number(locked) / Number(total)` is exact only below that —
 * so the obvious version renders a wrong proportion at exactly the sizes where
 * an institution is using this.
 */
export function Split({
  total,
  locked,
  label,
}: {
  total: string;
  locked: string;
  /** Accessible description; the figures beside the bar carry the same facts. */
  label: string;
}) {
  let lockedPercent = 0;
  try {
    const t0 = BigInt(total);
    const l0 = BigInt(locked);
    // Per ten-thousand, then divided down: integer arithmetic all the way, so a
    // 1-in-a-million sliver still renders as a sliver rather than as zero.
    if (t0 > 0n && l0 > 0n) lockedPercent = Number((l0 * 10000n) / t0) / 100;
  } catch {
    lockedPercent = 0;
  }
  const clamped = Math.max(0, Math.min(100, lockedPercent));

  return (
    <div className="split" role="img" aria-label={label}>
      <span className="split__avail" style={{ inlineSize: `${100 - clamped}%` }} />
      <span className="split__locked" style={{ inlineSize: `${clamped}%` }} />
    </div>
  );
}

/**
 * One currency's row: symbol, the bar, and the three figures behind it.
 *
 * Available is set in the strong weight because it is the only one a treasurer
 * can act on. Total is the number that misleads, so it is the quietest thing in
 * the row rather than the first.
 */
export function DenomRow({ balance }: { balance: TreasuryBalance }) {
  const info = resolveDenom(balance.denom);
  const hasLocked = balance.locked !== '0' && balance.locked !== '';

  return (
    <tr>
      <td>
        {/* The symbol and the name, not the base denom. `uxof` is what the
            chain stores; "XOF — West African CFA franc" is what a treasurer
            reconciles against. */}
        <strong>{info.symbol}</strong>
        <span className="small muted"> {info.name}</span>
        <Split
          total={balance.total}
          locked={balance.locked}
          label={t('safe.splitAlt', {
            available: formatAmount(balance.available, balance.denom),
            committed: formatAmount(balance.locked, balance.denom),
          })}
        />
      </td>
      <td className="num strong">{formatAmount(balance.available, balance.denom, { withSymbol: false })}</td>
      <td className="num">
        {hasLocked ? (
          <span className="num--lock">
            {formatAmount(balance.locked, balance.denom, { withSymbol: false })}
          </span>
        ) : (
          <span className="muted">{EMPTY_AMOUNT}</span>
        )}
      </td>
      <td className="num muted">{formatAmount(balance.total, balance.denom, { withSymbol: false })}</td>
    </tr>
  );
}
