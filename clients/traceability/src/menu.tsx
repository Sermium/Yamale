/**
 * Grouped rows, the way a phone's settings read: a short group title, then one
 * rounded list where each row is an icon, a name, what it is for, and where it
 * leads. The Atelier's tools and the account's settings both use it, so the
 * two screens speak one vocabulary.
 */

import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';

import { Icon, type IconName } from './icons.tsx';

export function MenuGroup({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <section className="menu-group">
      {title && <h2 className="menu-group__title">{title}</h2>}
      <ul className="menu">{children}</ul>
    </section>
  );
}

type Tone = 'accent' | 'brass' | 'bad';

function Body({ icon, tone, title, sub, end, chevron }: {
  icon: IconName; tone?: Tone; title: string; sub?: ReactNode; end?: ReactNode; chevron: boolean;
}) {
  return (
    <>
      <span className={`menu__icon menu__icon--${tone ?? 'accent'}`}><Icon name={icon} /></span>
      <span className="menu__text">
        <span className="menu__title">{title}</span>
        {sub && <span className="menu__sub">{sub}</span>}
      </span>
      {(end || chevron) && (
        <span className="menu__end">
          {end}
          {chevron && <Icon name="chevron" size={18} />}
        </span>
      )}
    </>
  );
}

/** A row that opens another screen. */
export function MenuLink({ to, ...rest }: { to: string; icon: IconName; tone?: Tone; title: string; sub?: ReactNode; end?: ReactNode }) {
  return <li><Link className="menu__row" to={to}><Body {...rest} chevron /></Link></li>;
}

/** A row that does something here. */
export function MenuButton({ onClick, danger, ...rest }: {
  onClick: () => void; danger?: boolean; icon: IconName; tone?: Tone; title: string; sub?: ReactNode; end?: ReactNode;
}) {
  return (
    <li>
      <button type="button" className={`menu__row${danger ? ' menu__row--danger' : ''}`} onClick={onClick}>
        <Body {...rest} chevron={false} />
      </button>
    </li>
  );
}

/** A row that opens in place, for something short: a code, a confirmation. */
export function MenuDisclosure({ children, ...rest }: {
  children: ReactNode; icon: IconName; tone?: Tone; title: string; sub?: ReactNode;
}) {
  return (
    <li>
      <details className="menu__details">
        <summary className="menu__row"><Body {...rest} chevron /></summary>
        <div className="menu__panel">{children}</div>
      </details>
    </li>
  );
}
