/**
 * A password field with an eye to show what was typed: on a phone keyboard a
 * long password is typed blind, and a typo found only after the account is
 * sealed under it is a password nobody can reproduce.
 *
 * Every app passes its own input class, so the field keeps the app's look; the
 * wrapper is a div because some apps style a span inside a label as a caption.
 */

import { useState, type InputHTMLAttributes } from 'react';
import { t } from '@yamale/chain';

import './password-field.css';

export function PasswordField(props: Omit<InputHTMLAttributes<HTMLInputElement>, 'type'>) {
  const [shown, setShown] = useState(false);
  return (
    <div className="pwfield">
      <input {...props} type={shown ? 'text' : 'password'} autoCapitalize="off" autoCorrect="off" spellCheck={false} />
      <button type="button" className="pwfield__eye" aria-pressed={shown}
              aria-label={t(shown ? 'account.hidePassword' : 'account.showPassword')}
              title={t(shown ? 'account.hidePassword' : 'account.showPassword')}
              onClick={() => setShown((s) => !s)}>
        <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.75"
             strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12Z" />
          <circle cx="12" cy="12" r="3" />
          {shown && <path d="m4 4 16 16" />}
        </svg>
      </button>
    </div>
  );
}
