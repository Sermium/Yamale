/**
 * The ISO 20022 instruction, drawn.
 *
 * One component, two homes: inside the phone behind a disclosure on the pay
 * screen and on the receipt, and open on the desk beside the phone at desktop
 * width. It is the same table in both, because two versions of this would
 * drift and the one that drifted would be the one making a claim about
 * somebody's money.
 *
 * The claim it makes is narrow and checkable: here is every field of
 * `blockchain.paymsg.v1.MsgSendPayment`, here is the value this payment would
 * put in it, and here is where that value actually ends up on the bank transfer
 * the app really sends. Three fields are dropped and the table says so with the
 * reason, rather than leaving somebody to notice they are missing.
 */
import { t, formatUserId } from '@yamale/chain';

import { CopyValue } from './copy.tsx';
import { blocker, fieldPlan, type Carrier, type Instruction as Msg, type Standing } from './iso.ts';
import { currencyOf, display } from './money.ts';

/**
 * How a value should read, given which field it is.
 *
 * An amount in base units is data an engineer wants and a figure nobody else
 * can parse, so the amount row shows the human figure and keeps the base units
 * behind the reveal — which is the same layering rule the rest of the app
 * follows, applied to the one screen that exists to show mechanism.
 */
function Value({ field, value, msg, names }: {
  field: string;
  value: string;
  msg: Msg;
  names: Record<string, string>;
}) {
  if (value === '') return <span className="iso__none">{t('iso.notSet')}</span>;

  if (field === 'amount') {
    return (
      <span className="iso__amount">
        <span className="y-mono">{display(value, msg.denom)}</span>
        <span className="iso__base y-mono">{value}</span>
      </span>
    );
  }
  if (field === 'denom') {
    const c = currencyOf(value);
    return <span className="y-mono">{c ? `${c.code} · ${value}` : value}</span>;
  }
  if (field === 'debtor' || field === 'creditor') {
    const id = names[value];
    return <CopyValue value={value} label={id ? formatUserId(id) : undefined} />;
  }
  return <span className="y-mono iso__plain">{value}</span>;
}

function Where({ carried }: { carried: Carrier }) {
  // Never colour alone. The word is the signal; the dot and the border are
  // reinforcement, and the shape differs per state so a monochrome screenshot
  // still reads correctly.
  const label = carried === 'ledger' ? t('iso.onLedger')
    : carried === 'memo' ? t('iso.inMemo')
    : t('iso.notSent');

  return (
    <span className={`iso__where iso__where--${carried}`}>
      <span className="iso__dot" aria-hidden="true" />
      {label}
    </span>
  );
}

export function InstructionTable({ msg, names = {} }: { msg: Msg; names?: Record<string, string> }) {
  const plan = fieldPlan(msg);

  return (
    <ol className="iso__fields">
      {plan.map((f) => (
        <li key={f.chainField} className={`iso__field iso__field--${f.carried}`}>
          <div className="iso__names">
            <span className="iso__iso">{f.iso}</span>
            <span className="iso__proto y-mono">{f.chainField}</span>
          </div>
          <div className="iso__val">
            <Value field={f.chainField} value={f.value} msg={msg} names={names} />
            {f.whyKey && <span className="iso__why">{t(f.whyKey)}</span>}
          </div>
          <Where carried={f.carried} />
        </li>
      ))}
    </ol>
  );
}

/**
 * What the chain says about the path, at a stated height.
 *
 * The height is not decoration. "No approved participants" is a claim about a
 * moment, and a number shown without the block it was read at is a number
 * nobody can check or date.
 */
export function StandingNote({ standing }: { standing: Standing }) {
  const b = blocker(standing);

  if (b.kind === 'unknown') {
    return (
      <p className="iso__standing iso__standing--unknown">
        <strong>{t('iso.standingTitle')}: {t('iso.unknown')}</strong>
        {' '}{t('iso.standingUnknown')}
      </p>
    );
  }

  if (b.kind === 'no-participants') {
    return (
      <p className="iso__standing iso__standing--blocked">
        <strong>{t('iso.standingTitle')}: 0</strong>
        {' '}{t('iso.standingNone')}
        {' '}<span className="iso__at">{t('iso.standingAt', { height: standing.known ? standing.height : 0 })}</span>
      </p>
    );
  }

  return (
    <p className="iso__standing iso__standing--blocked">
      <strong>{t('iso.standingTitle')}: {b.participants.length}</strong>
      {' '}{t('iso.standingSome')}
      {' '}<span className="iso__at">{t('iso.standingAt', { height: standing.known ? standing.height : 0 })}</span>
    </p>
  );
}

/** The whole panel: lede, table, memo, standing. */
export function InstructionPanel({ msg, memo, standing, names }: {
  msg: Msg;
  memo: string;
  standing: Standing;
  names?: Record<string, string>;
}) {
  return (
    <section className="iso">
      <h3 className="iso__title">{t('iso.title')}</h3>
      <p className="iso__lede">{t('iso.lede')}</p>

      <InstructionTable msg={msg} names={names} />

      <h4 className="iso__sub">{t('iso.memoTitle')}</h4>
      {memo
        ? <p className="ref-shown y-mono">{memo}</p>
        : <p className="iso__none">{t('iso.memoEmpty')}</p>}

      <StandingNote standing={standing} />
    </section>
  );
}
