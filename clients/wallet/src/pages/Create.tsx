import { countryName, countryProblem, placementRequest, t } from '@yamale/chain';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { DirectSecp256k1HdWallet } from '@cosmjs/proto-signing';

import { createVault } from '../vault.ts';

/**
 * Creating a key, in the browser, and nowhere else.
 *
 * The phrase is generated here and never sent anywhere — there is no request in
 * this file, and there is nothing on the other end to receive one. That claim
 * is the entire product of this screen, so the interface is built to make it
 * checkable rather than to be believed: no autosave, no draft, no analytics on
 * this route, and nothing written to storage.
 *
 * The flow is deliberately slow. A phrase shown and dismissed in one click is a
 * phrase somebody loses, and the loss is discovered weeks later when they try
 * to recover.
 */
export function CreatePage() {
  const [phrase, setPhrase] = useState<string | null>(null);
  const [address, setAddress] = useState<string | null>(null);
  const [written, setWritten] = useState(false);
  const [busy, setBusy] = useState(false);
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [saved, setSaved] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [country, setCountry] = useState('');
  const [institution, setInstitution] = useState('');
  const [copied, setCopied] = useState<string | null>(null);

  async function generate() {
    setBusy(true);
    try {
      // 24 words, matching the CLI. The extra entropy costs one line to write
      // down and is not a choice worth putting in front of somebody.
      const wallet = await DirectSecp256k1HdWallet.generate(24, { prefix: 'yml' });
      const [account] = await wallet.getAccounts();
      setPhrase(wallet.mnemonic);
      setAddress(account!.address);
    } finally {
      setBusy(false);
    }
  }

  if (!phrase) {
    return (
      <>
        <h1>{t('create.title')}</h1>
        <p className="lede">
          Your account is a phrase of 24 words. Whoever has it controls the money — there is no
          support desk that can restore it and nobody who can reset it for you.
        </p>

        <section className="card">
          <h2>{t('create.before')}</h2>
          <ul className="plain">
            <li>Have something to write on. The phrase is shown once.</li>
            <li>Do not photograph it, and do not put it in a password manager you sync.</li>
            <li>Nobody legitimate will ever ask you for it. Not us, not a validator, not support.</li>
          </ul>
          <button type="button" onClick={generate} disabled={busy}>
            {busy ? 'Generating…' : 'Generate my phrase'}
          </button>
        </section>
      </>
    );
  }

  const words = phrase.split(' ');

  return (
    <>
      <h1>{t('create.writeDown')}</h1>
      <p className="lede">
        Twenty-four words, in this order. This is the only time they are shown, and closing this
        page loses them.
      </p>

      {/* Numbered, because a phrase transcribed out of order recovers nothing
          and people do transcribe out of order. */}
      <section className="card phrase">
        <ol className="words">
          {words.map((word, i) => (
            // The word in its own element, not a bare text node: without it
            // the number and the word run together for a screen reader and for
            // anyone who copies the list — "1frown" is not a word anybody can
            // check against the paper they just wrote.
            <li key={i}>
              <span className="words__n">{i + 1}</span>
              <span className="words__w">{word}</span>
            </li>
          ))}
        </ol>
      </section>

      <section className="card">
        <label className="confirm">
          <input type="checkbox" checked={written} onChange={(e) => setWritten(e.target.checked)} />
          I have written the phrase down on paper and can read it back.
        </label>

        {written && address && (
          <>
            <p>
              Your address is <code className="address">{address}</code>
            </p>
            <p className="small muted">
              Share this freely — it is how people pay you. The phrase is the part that must stay
              secret.
            </p>
            <p>
              <Link to={`/a/${address}`}>Watch this account →</Link>
            </p>
          </>
        )}
      </section>

      {/* Offered only after the phrase is written down, and never instead of
          writing it down. The vault is a convenience that lives on one browser
          on one machine; clearing site data destroys it, and the paper is what
          survives that. */}
      {written && address && !saved && (
        <section className="card">
          <h2>{t('create.keepHere')}</h2>
          <p className="muted">
            Encrypt the phrase under a password so applications can ask this wallet to sign without
            you retyping it. This is a convenience, not a backup — the paper is the backup.
          </p>
          <label className="field">
            <span>Password</span>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </label>
          <label className="field">
            <span>Again</span>
            <input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
          </label>
          {password !== '' && password.length < 10 && (
            <p className="notice">
              Ten characters at least. This password is the only thing between anyone using this
              computer and your money.
            </p>
          )}
          {confirm !== '' && password !== confirm && (
            <p className="notice notice--bad">{t('msg.noMatch')}</p>
          )}
          {saveError && <div className="notice notice--bad">{saveError}</div>}
          <button
            type="button"
            disabled={password.length < 10 || password !== confirm}
            onClick={async () => {
              setSaveError(null);
              try {
                await createVault(phrase!, password);
                setSaved(true);
                setPassword('');
                setConfirm('');
              } catch (err) {
                setSaveError(err instanceof Error ? err.message : 'Could not save.');
              }
            }}
          >
            Save to this device
          </button>
        </section>
      )}

      {saved && (
        <section className="card">
          <h2>{t('create.saved')}</h2>
          <p className="muted">
            The phrase is encrypted in this browser. Applications can now open this wallet to ask
            for a signature, and each request shows you what it is before you approve it.
          </p>
          <p className="notice">
            <strong>{t('placement.pending')}</strong> {t('placement.lede')}
          </p>
        </section>
      )}

      {/* The step that makes the account usable, and the one a key generator
          cannot perform.

          An account with no country recorded gets no user ID — the chain
          refuses to issue one — and without a user ID nobody can address a
          payment to it. So the flow does not end at "saved". It ends at a
          request the holder hands to the institution that onboarded them,
          because the first recording of a country belongs to the party that
          performed the KYC and to nobody else. An account free to name its own
          perimeter would name the one with no authority watching it. */}
      {written && address && (
        <section className="card">
          <h2>{t('placement.title')}</h2>
          <p className="lede">{t('placement.lede')}</p>
          <p className="notice">{t('placement.cannotSelf')}</p>

          {/* Two letters, so the field is two letters wide. A full-width input
              for a two-character value tells somebody to type a sentence, and
              the mono face at display size makes the pair legible enough to
              read back down a telephone — which is exactly what happens next. */}
          <div className="placement__country">
            <label className="field">
              <span>{t('placement.country')}</span>
              <input
                value={country}
                onChange={(e) => setCountry(e.target.value.toUpperCase().slice(0, 2))}
                placeholder="SN"
                autoCapitalize="characters"
                autoComplete="off"
                inputMode="text"
                size={4}
                aria-describedby="placement-country-help"
              />
            </label>
            {/* The name in the reader's own language, beside the code the chain
                stores. Somebody choosing their country needs the name; somebody
                confirming it to their institution needs the two letters. */}
            {country.length === 2 && !countryProblem(country) && (
              <p className="placement__name" id="placement-country-help">
                {countryName(country)} <span className="y-mono">{country}</span>
              </p>
            )}
          </div>
          {country !== '' && countryProblem(country) !== null && (
            <p className="notice notice--bad" id="placement-country-help">
              {countryProblem(country)}
            </p>
          )}

          <label className="field">
            <span>{t('placement.institution')}</span>
            <input
              value={institution}
              onChange={(e) => setInstitution(e.target.value)}
              autoComplete="off"
            />
          </label>

          <PlacementRequest
            address={address}
            country={country}
            institution={institution}
            copied={copied}
            onCopy={setCopied}
          />
        </section>
      )}

      <p className="small muted">
        Nothing on this page was transmitted. You can confirm that: disconnect from the network and
        generate another — it works the same, because the phrase is made in your browser.
      </p>
    </>
  );
}

/**
 * The document a holder gives their institution, and the command that
 * institution runs.
 *
 * Neither is a transaction. This wallet holds no key that could sign one, and
 * the key that must sign belongs to the participant — so what is produced is
 * the exact thing they need in order to act, in a form that survives being sent
 * over any channel and read before it is run.
 */
function PlacementRequest({
  address,
  country,
  institution,
  copied,
  onCopy,
}: {
  address: string;
  country: string;
  institution: string;
  copied: string | null;
  onCopy: (id: string | null) => void;
}) {
  const request = placementRequest({ address, country, institution });
  if ('problem' in request) return null;

  const blocks: { id: string; label: string; text: string; command?: boolean }[] = [
    { id: 'doc', label: t('placement.give'), text: request.document },
    { id: 'cmd', label: t('placement.theyRun'), text: request.command, command: true },
  ];

  return (
    <>
      {blocks.map(({ id, label, text, command }) => (
        <div className="handoff" key={id}>
          <div className="handoff__head">
            <span className="y-label">{label}</span>
            <button
              type="button"
              className="chip"
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(text);
                  onCopy(id);
                } catch {
                  // Refused on an insecure origin or in an in-app browser. The
                  // text is on screen and selectable, so saying nothing is
                  // better than a false confirmation.
                  onCopy(null);
                }
              }}
            >
              {copied === id ? t('placement.copied') : t('placement.copy')}
            </button>
          </div>
          {/* The document wraps — it is prose somebody reads. The command does
              not — it is a line somebody pastes, and a soft wrap in the middle
              of a bech32 address is how a copy-paste goes wrong. */}
          <pre className={command ? 'handoff__cmd' : undefined}>{text}</pre>
        </div>
      ))}
    </>
  );
}
