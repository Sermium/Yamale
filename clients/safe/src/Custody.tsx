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
      {/* Deliberately not an SVG.
       *
       * The first version of this was one, and it was wrong in two ways that
       * only show up outside English. SVG text does not wrap, so the longest
       * caption was clipped by 41px at 1600 wide — measured, not guessed — and
       * every translation of it is longer than the English. And an SVG's
       * geometry does not mirror: with `dir="rtl"` on the document, a label
       * anchored at x extends the other way, so the Arabic build would have put
       * the refusals on the wrong side of the wall it is describing.
       *
       * As HTML the same figure wraps at any width, mirrors for Arabic through
       * logical properties, and stays selectable and searchable. Nothing about
       * it needed to be drawn — it is a wall with two columns either side, and
       * that is a layout. */}
      <div className="custody__bar" role="img" aria-label={t('safe.figureAlt')}>
        <div className="custody__side">
          <span className="custody__k">{t('safe.figAvailable')}</span>
          <span className="custody__in">{t('safe.figAvailableIn')}</span>
        </div>
        {/* Hatched as well as tinted: a reader who sees no colour at all still
            sees a different surface, which is the whole requirement. */}
        <div className="custody__side custody__side--held">
          <span className="custody__k custody__k--lock">{t('safe.figCommitted')}</span>
          <span className="custody__in">{t('safe.figCommittedIn')}</span>
        </div>
      </div>

      <div className="custody__cols">
        <div className="custody__col">
          <p className="custody__eyebrow custody__eyebrow--ok">{t('safe.figAllowed')}</p>
          <ul className="custody__list custody__list--ok">
            <li>{t('safe.figSpendPath')}</li>
          </ul>
          <p className="custody__note">{t('safe.figSpendWho')}</p>
        </div>

        <div className="custody__col custody__col--held">
          <p className="custody__eyebrow custody__eyebrow--bad">{t('safe.figRefused')}</p>
          <ul className="custody__list custody__list--bad">
            <li>{t('safe.figWho1')}</li>
            <li>{t('safe.figWho2')}</li>
            <li>{t('safe.figWho3')}</li>
          </ul>
          <ul className="custody__list custody__list--ok">
            <li>{t('safe.figClaimPath')}</li>
          </ul>
        </div>
      </div>

      <ul className="custody__rules">
        <li>{t('safe.figRule1')}</li>
        <li>{t('safe.figRule2')}</li>
      </ul>

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
