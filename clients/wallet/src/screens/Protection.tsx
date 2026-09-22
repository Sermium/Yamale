/**
 * "What happens if I lose this phone?"
 *
 * Until now the honest answer was: you lose the account. A vault is one
 * encrypted blob in one browser's storage, so a lost handset, a cleared site or
 * a reinstalled operating system ended it, and nobody — including the people
 * who run this chain — could do anything about it.
 *
 * This panel is where that stops being true. It seals the key this account
 * already has under the account holder's password, splits the sealed blob three
 * ways, keeps one part here and deposits the other two with two independent
 * services. Any two parts and the password rebuild the key.
 *
 * # Why it is phrased as a question about losing a phone
 *
 * Because that is the event people can picture. "Enrol with the custodian and
 * recovery services" describes the mechanism to somebody who already
 * understands it; it tells a market trader in Dakar nothing about whether they
 * should press the button.
 *
 * The mechanism is still written down, one level below the answer, because the
 * people who do want it are exactly the people who will not trust a button that
 * refuses to explain itself.
 *
 * # The address does not change, and it is said out loud
 *
 * This protects the account that already exists. Somebody who has been paid
 * into this address, or who has given it to a customer, needs to know that
 * before they act, not after — and "your key is split three ways" gives them no
 * way to work it out.
 */

import { useState, type FormEvent } from 'react';
import { t } from '@yamale/chain';

import { addressOf, enrol, EnrolError, privateKeyFromMnemonic } from '../account/enrol.ts';
import { enrolmentFor, rememberEnrolment } from '../account/stored.ts';
import { openVault, WrongPassword } from '../vault.ts';
import { Field, Note } from '../ui.tsx';

/**
 * Where the two services are.
 *
 * Same origin, proxied — see deploy/nginx/yamale-account.conf, which maps these
 * onto the two deployments and rate-limits them. Relative rather than absolute
 * so the wallet works unchanged on the devnet, on a laptop, and behind the
 * funnel, none of which agree about hostnames.
 */
const SERVICES = {
  custodianUrl: '/api/custodian',
  recoveryUrl: '/api/recovery',
};

export function Protection({ address }: { address: string }) {
  const [enrolment, setEnrolment] = useState(() => enrolmentFor(address));
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [working, setWorking] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);

  if (enrolment) {
    const when = new Date(enrolment.enrolledAt);
    return (
      <section className="y-card">
        <h2 className="wal-cardTitle">{t('wal.protTitle')}</h2>
        <Note tone="ok">{t('wal.protSafe')}</Note>
        <Field label={t('wal.protSince', { date: when.toLocaleDateString() })}>
          {enrolment.email}
        </Field>
        <p className="wal-small wal-muted">{t('wal.protHow')}</p>
      </section>
    );
  }

  async function protect(event: FormEvent) {
    event.preventDefault();
    setWorking(true);
    setProblem(null);
    try {
      // The vault is opened here rather than reusing an unlocked session,
      // because this needs the MNEMONIC and an unlocked wallet is held as a
      // signer. Asking for the password again is also the right prompt: this is
      // the password the account will be recovered with, so somebody who cannot
      // produce it today should find that out now rather than in a year.
      const wallet = await openVault(password);
      const privateKey = await privateKeyFromMnemonic(wallet.mnemonic);

      // Checked before anything leaves this device. If the derivation ever
      // disagreed with the wallet's own address, enrolling would file the wrong
      // key against a real account — and every visible signal would still say
      // it worked.
      if ((await addressOf(privateKey)) !== address) {
        throw new Error('this device derived a different address for its own account');
      }

      const result = await enrol(privateKey, email, password, SERVICES);
      const record = {
        address: result.address,
        email,
        holding: result.deviceHolding,
        enrolledAt: new Date().toISOString(),
      };
      rememberEnrolment(record);
      setEnrolment(record);
      setPassword('');
    } catch (e) {
      // The wrong password is by far the most common failure and is not worth
      // dressing up as a fault in the service.
      const reason =
        e instanceof WrongPassword
          ? e.message
          : e instanceof EnrolError
            ? e.message
            : e instanceof Error
              ? e.message
              : String(e);
      setProblem(t('wal.protFailed', { reason }));
    } finally {
      setWorking(false);
    }
  }

  return (
    <section className="y-card">
      <h2 className="wal-cardTitle">{t('wal.protTitle')}</h2>
      <Note tone="warn">{t('wal.protUnsafe')}</Note>

      <form className="wal-form" onSubmit={(e) => void protect(e)}>
        <label>
          <span className="wal-field__label">{t('wal.protEmail')}</span>
          <input
            className="wal-input"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </label>
        <label>
          <span className="wal-field__label">{t('wal.protPassword')}</span>
          <input
            className="wal-input"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>

        <p className="wal-small wal-muted">{t('wal.protSame')}</p>

        {/* A submit, not a button with a click handler: that is what makes the
            browser enforce `required` and the email format before anything is
            derived, and what lets somebody press Enter in a field. */}
        <button type="submit" className="wal-btn" disabled={working}>
          {working ? t('wal.protWorking') : t('wal.protOffer')}
        </button>
      </form>

      {problem ? <Note tone="bad">{problem}</Note> : null}
      <p className="wal-small wal-muted">{t('wal.protHow')}</p>
    </section>
  );
}
