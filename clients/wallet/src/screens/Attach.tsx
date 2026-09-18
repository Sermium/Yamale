/**
 * How you are attached to an account.
 *
 * One screen, offering ways in, rather than a landing page with "create" and
 * "connect" as two buttons. That shape is deliberate and it is about where this
 * platform is going: accounts are moving to an email and a password with no
 * recovery phrase, and the device will sign on its own. When that arrives it is
 * a new row on this list — not a second front door, and not a rewrite of the
 * navigation.
 *
 * Until then the two ways in are the two that exist, and they are kept apart
 * from each other for the older reason: creating an account puts a phrase on
 * screen that must never leave the page, and blending that with the harmless
 * act of looking one up puts the dangerous action one tap from the safe one.
 */
import { useEffect, useState } from 'react';
import { DirectSecp256k1HdWallet } from '@cosmjs/proto-signing';
import { countryName, countryProblem, placementRequest, t } from '@yamale/chain';

import { client } from '../chain.ts';
import { createVault, vaultSummary } from '../vault.ts';
import { Back, Field, Note, ScreenTitle } from '../ui.tsx';
import { Identifier } from '../Identifier.tsx';

type Way = 'choose' | 'create' | 'load';

export function Attach({ onAttached, onWatch }: { onAttached: () => void; onWatch: () => void }) {
  const [way, setWay] = useState<Way>('choose');

  if (way === 'create') return <Create onBack={() => setWay('choose')} onDone={onAttached} />;
  if (way === 'load') return <Load onBack={() => setWay('choose')} onDone={onAttached} />;

  return (
    <>
      <ScreenTitle lede={t('wal.attachLede')}>{t('wal.attachTitle')}</ScreenTitle>

      <ul className="y-cards">
        <li>
          <button type="button" className="y-row" onClick={() => setWay('create')}>
            <span className="y-row__main">
              <span className="y-row__title">{t('create.title')}</span>
              <span className="y-row__sub">{t('wal.attachCreateSub')}</span>
            </span>
            <Chev />
          </button>
        </li>
        <li>
          <button type="button" className="y-row" onClick={() => setWay('load')}>
            <span className="y-row__main">
              <span className="y-row__title">{t('import.title')}</span>
              <span className="y-row__sub">{t('wal.attachLoadSub')}</span>
            </span>
            <Chev />
          </button>
        </li>
        <li>
          <button type="button" className="y-row" onClick={onWatch}>
            <span className="y-row__main">
              <span className="y-row__title">{t('wal.attachWatch')}</span>
              <span className="y-row__sub">{t('wal.attachWatchSub')}</span>
            </span>
            <Chev />
          </button>
        </li>
      </ul>

      {/* Said plainly, and without a date. The email-and-password account is
          decided but not built, and a row promising it would be a row somebody
          taps. A sentence is enough to stop this list reading as finished. */}
      <p className="wal-small wal-muted wal-attach__soon">{t('wal.attachMoreWays')}</p>
    </>
  );
}

function Chev() {
  return (
    <svg className="y-row__chev" viewBox="0 0 20 20" width="16" height="16" fill="none"
         stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"
         aria-hidden="true">
      <path d="M8 4 L14 10 L8 16" />
    </svg>
  );
}

/* ----------------------------------------------------------------- create */

/**
 * Creating a key, in the browser, and nowhere else.
 *
 * The phrase is generated here and never sent anywhere — there is no request in
 * this file and nothing on the other end to receive one. That claim is the
 * whole product of this screen, so the flow is built to make it checkable
 * rather than to be believed.
 *
 * It is deliberately slow. A phrase shown and dismissed in one tap is a phrase
 * somebody loses, and the loss is discovered weeks later on the day they need
 * to recover.
 */
function Create({ onBack, onDone }: { onBack: () => void; onDone: () => void }) {
  const [step, setStep] = useState<'brief' | 'words' | 'password' | 'placed'>('brief');
  const [phrase, setPhrase] = useState('');
  const [address, setAddress] = useState('');
  const [busy, setBusy] = useState(false);
  const [written, setWritten] = useState(false);
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);

  async function generate() {
    setBusy(true);
    try {
      // 24 words, matching the CLI. The extra entropy costs one line to write
      // down and is not a choice worth putting in front of anybody.
      const wallet = await DirectSecp256k1HdWallet.generate(24, { prefix: 'yml' });
      const [account] = await wallet.getAccounts();
      setPhrase(wallet.mnemonic);
      setAddress(account!.address);
      setStep('words');
    } finally {
      setBusy(false);
    }
  }

  if (step === 'brief') {
    return (
      <>
        <Back onClick={onBack}>{t('action.cancel')}</Back>
        <ScreenTitle lede={t('wal.createLede')}>{t('create.title')}</ScreenTitle>
        <div className="y-card">
          <h2 className="wal-cardTitle">{t('create.before')}</h2>
          <ul className="wal-bullets">
            <li>{t('wal.createHave')}</li>
            <li>{t('wal.createNoPhoto')}</li>
            <li>{t('wal.createNobodyAsks')}</li>
          </ul>
        </div>
        <button type="button" className="wal-btn" onClick={generate} disabled={busy}>
          {busy ? t('wal.generating') : t('wal.generate')}
        </button>
      </>
    );
  }

  if (step === 'words') {
    const words = phrase.split(' ');
    return (
      <>
        <ScreenTitle lede={t('wal.wordsLede')}>{t('create.writeDown')}</ScreenTitle>

        {/* Numbered, because a phrase transcribed out of order recovers nothing
            and people do transcribe out of order. Two columns on a phone, with
            the number in its own element so a screen reader does not read
            "1frown" — a word nobody can check against their paper. */}
        <ol className="wal-words">
          {words.map((word, i) => (
            <li key={i}>
              <span className="wal-words__n">{i + 1}</span>
              <span className="wal-words__w">{word}</span>
            </li>
          ))}
        </ol>

        <label className="wal-check">
          <input type="checkbox" checked={written} onChange={(e) => setWritten(e.target.checked)} />
          <span>{t('wal.wroteItDown')}</span>
        </label>

        <button type="button" className="wal-btn" disabled={!written} onClick={() => setStep('password')}>
          {t('wal.continue')}
        </button>
      </>
    );
  }

  if (step === 'password') {
    return (
      <>
        <ScreenTitle lede={t('wal.keepHereLede')}>{t('create.keepHere')}</ScreenTitle>

        <Field label={t('wal.password')}>
          <input className="wal-input" type="password" value={password}
                 onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
        </Field>
        <Field label={t('wal.passwordAgain')}>
          <input className="wal-input" type="password" value={confirm}
                 onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" />
        </Field>

        {password !== '' && password.length < 10 && <Note tone="warn">{t('wal.tenChars')}</Note>}
        {confirm !== '' && password !== confirm && <Note tone="bad">{t('msg.noMatch')}</Note>}
        {error && <Note tone="bad">{error}</Note>}

        <button
          type="button"
          className="wal-btn"
          disabled={busy || password.length < 10 || password !== confirm}
          onClick={async () => {
            setError(null);
            setBusy(true);
            try {
              await createVault(phrase, password);
              setPassword('');
              setConfirm('');
              setStep('placed');
            } catch (err) {
              setError(err instanceof Error ? err.message : t('error.unknown'));
            } finally {
              setBusy(false);
            }
          }}
        >
          {busy ? t('wal.saving') : t('wal.saveToDevice')}
        </button>
      </>
    );
  }

  return (
    <>
      <ScreenTitle>{t('create.saved')}</ScreenTitle>
      <div className="y-card y-card--money">
        <span className="wal-fact__label">{t('watch.address')}</span>
        <Identifier value={address} />
      </div>
      <Note tone="ok">{t('wal.savedBody')}</Note>
      <Placement address={address} />
      <button type="button" className="wal-btn" onClick={onDone}>{t('wal.openMyMoney')}</button>
    </>
  );
}

/* ------------------------------------------------------------------- load */

/**
 * Loading an existing account from its recovery phrase.
 *
 * The screen people reach on their worst day — new phone, cleared browser. So
 * it checks *before* asking for a password: deriving the address first lets
 * somebody confirm they typed the right phrase by recognising the address,
 * rather than finding out after they have committed a password to it.
 *
 * The phrase is validated by actually deriving from it. BIP-39 carries a
 * checksum, so a single transposed word fails here rather than silently
 * producing a valid-but-different empty account — which is the failure that
 * makes people think their money is gone.
 */
function Load({ onBack, onDone }: { onBack: () => void; onDone: () => void }) {
  const [phrase, setPhrase] = useState('');
  const [derived, setDerived] = useState<string | null>(null);
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const existing = vaultSummary();
  const words = phrase.trim().split(/\s+/).filter(Boolean);
  const countLooksRight = words.length === 12 || words.length === 24;

  async function check() {
    setBusy(true);
    setError(null);
    setDerived(null);
    try {
      const wallet = await DirectSecp256k1HdWallet.fromMnemonic(words.join(' '), { prefix: 'yml' });
      const [account] = await wallet.getAccounts();
      setDerived(account!.address);
    } catch {
      setError(t('wal.phraseInvalid'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Back onClick={onBack}>{t('action.cancel')}</Back>
      <ScreenTitle lede={t('wal.loadLede')}>{t('import.title')}</ScreenTitle>

      {/* Said before the phrase is typed, not after. A vault is overwritten in
          place, so somebody restoring a second account onto a device that
          already holds one would otherwise destroy the first with no warning
          and no undo. */}
      {existing && (
        <Note tone="bad" title={t('wal.alreadyHolds', { label: existing.label })}>
          {t('wal.alreadyHoldsBody')}
        </Note>
      )}

      <Field label={t('wal.theWords')}
             hint={words.length === 0 ? t('wal.nobodyAsks')
                   : t('wal.wordCount', { count: String(words.length) })
                     + (countLooksRight ? '' : ` — ${t('wal.expected12or24')}`)}>
        <textarea
          className="wal-input wal-input--area"
          value={phrase}
          onChange={(e) => { setPhrase(e.target.value); setDerived(null); }}
          rows={4}
          spellCheck={false}
          autoComplete="off"
          placeholder="nuclear thrive identify time …"
        />
      </Field>

      {error && <Note tone="bad">{error}</Note>}

      {!derived ? (
        <button type="button" className="wal-btn" onClick={check} disabled={!countLooksRight || busy}>
          {busy ? t('wal.checking') : t('wal.checkPhrase')}
        </button>
      ) : (
        <>
          <h2 className="y-screen__sub">{t('import.rightAccount')}</h2>
          <div className="y-card">
            <Identifier value={derived} />
            <p className="wal-small wal-muted">{t('wal.notTheAddress')}</p>
          </div>

          <Field label={t('wal.choosePassword')}>
            <input className="wal-input" type="password" value={password}
                   onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
          </Field>
          <Field label={t('wal.passwordAgain')}>
            <input className="wal-input" type="password" value={confirm}
                   onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" />
          </Field>
          {password !== '' && password.length < 10 && <Note tone="warn">{t('wal.tenChars')}</Note>}
          {confirm !== '' && password !== confirm && <Note tone="bad">{t('msg.noMatch')}</Note>}

          <button
            type="button"
            className="wal-btn"
            disabled={busy || password.length < 10 || password !== confirm}
            onClick={async () => {
              setBusy(true);
              setError(null);
              try {
                await createVault(words.join(' '), password);
                onDone();
              } catch (err) {
                setError(err instanceof Error ? err.message : t('error.unknown'));
              } finally {
                setBusy(false);
              }
            }}
          >
            {busy ? t('wal.saving') : t('wal.loadThisAccount')}
          </button>
        </>
      )}
    </>
  );
}

/* -------------------------------------------------------------- placement */

/**
 * The step that makes a new account usable, and the one a key generator cannot
 * perform.
 *
 * An account with no country recorded gets no user ID — the chain refuses to
 * issue one — and without a user ID nobody can address a payment to it. So the
 * flow does not end at "saved". It ends at a request the holder hands to the
 * institution that onboarded them, because the first recording of a country
 * belongs to the party that performed the KYC and to nobody else.
 */
export function Placement({ address }: { address: string }) {
  const [country, setCountry] = useState('');
  const [institution, setInstitution] = useState('');
  const [copied, setCopied] = useState<string | null>(null);
  // null is "not asked yet", which is NOT the same as []. An empty register is
  // a fact worth a sentence; a pending read is a wait. Collapsing the two
  // renders the live state of devnet-2 as a loading failure.
  const [participants, setParticipants] = useState<
    { participant: string; code: string; name: string }[] | null
  >(null);

  useEffect(() => {
    let cancelled = false;
    void client
      .approvedParticipants()
      .then((rows) => !cancelled && setParticipants(rows))
      // A failed read degrades to the empty-register sentence rather than to a
      // wait that never ends. Both say "you cannot pick one here", and only one
      // of them stops.
      .catch(() => !cancelled && setParticipants([]));
    return () => { cancelled = true; };
  }, []);

  const request = placementRequest({ address, country, institution });
  const ready = !('problem' in request);

  return (
    <>
      <h2 className="y-screen__sub">{t('placement.title')}</h2>
      <p className="wal-lede">{t('placement.lede')}</p>
      <Note tone="warn">{t('placement.cannotSelf')}</Note>

      <Field label={t('placement.country')}
             hint={country.length === 2 && !countryProblem(country)
               ? countryName(country)
               : countryProblem(country) ?? undefined}>
        {/* Two letters, so the field is two letters wide. A full-width input
            for a two-character value tells somebody to type a sentence. */}
        <input
          className="wal-input wal-input--short y-mono"
          value={country}
          onChange={(e) => setCountry(e.target.value.toUpperCase().slice(0, 2))}
          placeholder="SN"
          autoCapitalize="characters"
          autoComplete="off"
          size={4}
        />
      </Field>

      <Field label={t('placement.institution')}>
        {/* A select and not free text. MsgSetJurisdiction accepts the first
            recording only from an approved participant, so a name typed into a
            box is a name the chain will never honour. */}
        {participants === null ? (
          <span className="wal-muted wal-small">{t('acct.reading')}</span>
        ) : participants.length === 0 ? (
          <Note tone="warn">{t('wal.noParticipants')}</Note>
        ) : (
          <select className="wal-input" value={institution} onChange={(e) => setInstitution(e.target.value)}>
            <option value="">—</option>
            {participants.map((p) => (
              <option key={p.participant} value={p.name}>{p.name} ({p.code})</option>
            ))}
          </select>
        )}
      </Field>

      {ready &&
        ([
          { id: 'doc', label: t('placement.give'), text: request.document, cmd: false },
          { id: 'cmd', label: t('placement.theyRun'), text: request.command, cmd: true },
        ] as const).map(({ id, label, text, cmd }) => (
          <div className="wal-handoff" key={id}>
            <div className="wal-handoff__head">
              <span className="wal-fact__label">{label}</span>
              <button
                type="button"
                className="wal-chip"
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(text);
                    setCopied(id);
                  } catch {
                    // Refused on an insecure origin or in an in-app browser. The
                    // text is on screen and selectable, so saying nothing beats
                    // a false confirmation.
                    setCopied(null);
                  }
                }}
              >
                {copied === id ? t('placement.copied') : t('placement.copy')}
              </button>
            </div>
            {/* The document wraps — it is prose somebody reads. The command does
                not — it is a line somebody pastes, and a soft wrap in the middle
                of a bech32 address is how a copy-paste goes wrong. */}
            <pre className={cmd ? 'wal-pre wal-pre--cmd y-scroll' : 'wal-pre'}>{text}</pre>
          </div>
        ))}
    </>
  );
}
