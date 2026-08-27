import {
  EMPTY_AMOUNT,
  REVERSIBILITY_NOTE,
  arriving,
  consequencesOf,
  describeFee,
  describeRequestMessage,
  formatAmount,
  formatCoins,
  formatDuration,
  leaving,
  locking,
  summariseSigningRequest,
  t,
  timeUntil,
  transactionHash,
  translateError,
  truncateAddress,
  type Coin,
  type Reversibility,
  type RequestMessage,
  type SigningConsequences,
  type SigningRequestSummary,
  type TxStatus,
} from '@yamale/chain';
import { useEffect, useMemo, useRef, useState } from 'react';
import { serveWalletRequests, fromBase64, toBase64 } from '@yamale/connect';
import type { AnyRequest, AnyResult } from '@yamale/connect';

import { listContacts } from '@yamale/chain';

import { client } from '../chain.ts';
import { Named } from '../Named.tsx';
import { Identifier } from '../Identifier.tsx';
import { getUnlocked, openVault, setUnlocked, touch, vaultSummary } from '../vault.ts';

/**
 * The approval window.
 *
 * This is the page an application opens when it wants an address or a
 * signature, and it is the only screen in the system where a person authorises
 * something. It answers three questions, in this order, because that is the
 * order they are asked:
 *
 *   1. **What will this do to my account?** Not what messages are in it — what
 *      leaves, what arrives, what stops being spendable and until when. The
 *      decode has been able to name the messages for a while; naming them is not
 *      the same as answering this.
 *   2. **Can I undo it?** Stated as a word and a colour and a sentence, never as
 *      a colour alone, and taken at the worst message in the transaction.
 *   3. **Who is asking?** From the browser's own `event.origin`, not from
 *      anything the calling page said about itself.
 *
 * And then, after signing, it answers the fourth: **did it happen?** A wallet
 * that only signs normally cannot say — it hands back a signature and the
 * application broadcasts. But the transaction's hash is computable from the
 * bytes just signed, so this one watches the chain for its own transaction and
 * reports the block. That matters here more than anywhere: broadcast success is
 * not execution success, and the only place the difference shows up is in the
 * code the block recorded.
 */
type Pending = { request: AnyRequest; origin: string; resolve: (r: AnyResult) => void };

/** What happened after Approve, once there is something to watch. */
type Signed = { hash: string; origin: string; summary: SigningRequestSummary; signer: string };

export function ConnectPage() {
  const [pending, setPending] = useState<Pending | null>(null);
  const [signed, setSigned] = useState<Signed | null>(null);
  const [connected, setConnected] = useState<string | null>(null);
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const summary = vaultSummary();

  useEffect(() => {
    return serveWalletRequests({
      // Every app here is served by the same nginx as this wallet. On a network
      // carrying value this becomes an explicit list.
      allowedOrigins: '*',
      onRequest: (request, origin) =>
        new Promise<AnyResult>((resolve) => setPending({ request, origin, resolve })),
    });
  }, []);

  if (!summary) {
    return (
      <section className="card">
        <h1>{t('sign.noAccount')}</h1>
        <p className="muted">{t('sign.noAccountBody')}</p>
      </section>
    );
  }

  // The outcome screens come before the "waiting" one: after approving, the
  // request is gone but the transaction is not, and dropping straight back to
  // "waiting for a request" is how a person is left with no idea whether their
  // money moved.
  if (signed) return <Outcome signed={signed} onDone={() => setSigned(null)} />;
  if (connected) return <Connected origin={connected} address={summary.address} onDone={() => setConnected(null)} />;

  if (!pending) {
    return (
      <section className="card">
        <h1>{t('sign.waiting')}</h1>
        <p className="muted">{t('msg.noRequest')}</p>
      </section>
    );
  }

  const unlocked = getUnlocked() !== null;

  async function approve() {
    setBusy(true);
    setError(null);
    try {
      let wallet = getUnlocked();
      if (!wallet) {
        wallet = await openVault(password);
        setUnlocked(wallet);
      }
      touch();

      const [account] = await wallet.getAccounts();
      const info = {
        address: account!.address,
        pubkey: toBase64(account!.pubkey),
        algo: 'secp256k1' as const,
      };

      const { request, resolve, origin } = pending!;
      if (request.kind === 'connect' || request.kind === 'accounts') {
        resolve({ kind: request.kind, accounts: [info] } as AnyResult);
        setPending(null);
        setConnected(origin);
        return;
      }

      // Signed exactly as received. The wallet is entitled to alter the
      // document, but this one does not -- so it returns the same bytes it
      // put a signature over, which is what the caller must broadcast.
      const bodyBytes = fromBase64(request.bodyBytes);
      const authInfoBytes = fromBase64(request.authInfoBytes);
      const result = await wallet.signDirect(request.signerAddress, {
        bodyBytes,
        authInfoBytes,
        chainId: request.chainId,
        accountNumber: BigInt(request.accountNumber) as never,
      });
      resolve({
        kind: 'signDirect',
        signature: result.signature.signature,
        bodyBytes: toBase64(result.signed.bodyBytes),
        authInfoBytes: toBase64(result.signed.authInfoBytes),
      });

      // Computed from the bytes that were signed, before anybody broadcasts
      // them. See transactionHash() in the SDK for why this is knowable.
      setPending(null);
      setSigned({
        hash: transactionHash(
          result.signed.bodyBytes,
          result.signed.authInfoBytes,
          fromBase64(result.signature.signature),
        ),
        origin,
        summary: summariseSigningRequest(result.signed.bodyBytes, result.signed.authInfoBytes),
        signer: request.signerAddress,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not complete that.');
    } finally {
      setBusy(false);
    }
  }

  function reject() {
    pending!.resolve({ kind: 'error', code: 'rejected', message: 'You declined.' });
    setPending(null);
    window.close();
  }

  // Narrowed once here rather than compared again inside the JSX: the `kind`
  // check has to be the thing that produces the typed request, or the compiler
  // cannot know the sign-only fields exist.
  const signRequest = pending.request.kind === 'signDirect' ? pending.request : null;
  const translated = error ? translateError(error) : null;

  /**
   * Bytes that are not a transaction cannot be approved at all.
   *
   * Prevention rather than a warning: the screen already said "Do not approve"
   * in the loudest form this design has, with the Approve button live beside
   * it. There is no legitimate request that reaches this branch — a TxBody that
   * will not decode would be refused by the chain anyway, so the only realistic
   * sender is one hoping the person clicks through. Everything short of this,
   * including a message type this build does not recognise, stays approvable
   * and loudly flagged: that is a gap in the wallet, and an expert may know
   * exactly what they are signing.
   */
  const unreadable =
    signRequest !== null &&
    summariseSigningRequest(
      fromBase64(signRequest.bodyBytes),
      fromBase64(signRequest.authInfoBytes),
    ).undecodable !== undefined;

  return (
    <>
      {signRequest ? (
        <SignRequest request={signRequest} origin={pending.origin} />
      ) : (
        <ConnectRequest origin={pending.origin} address={summary.address} />
      )}

      <section className="card">
        {!unlocked && !unreadable && (
          <label className="field">
            <span>{t('sign.passwordFor', { label: summary.label })}</span>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoFocus
            />
          </label>
        )}
        {translated && (
          <div className="notice notice--bad">
            <strong>{translated.message}</strong>
            {translated.reason ? <> {translated.reason}</> : null}
            {translated.nextStep ? <p className="error__next">{translated.nextStep}</p> : null}
            {translated.raw && translated.raw !== translated.message ? (
              <details className="payload">
                <summary>{t('sign.whatTheChainSaid')}</summary>
                <pre className="payload__pre">{translated.raw}</pre>
              </details>
            ) : null}
          </div>
        )}
        {unreadable && <p className="muted">{t('sign.cannotApprove')}</p>}
        <div className="actions__row">
          {!unreadable && (
            <button type="button" onClick={approve} disabled={busy || (!unlocked && !password)}>
              {busy ? t('sign.working') : signRequest ? t('sign.approve') : t('sign.connectApprove')}
            </button>
          )}
          <button type="button" className={unreadable ? undefined : 'chip'} onClick={reject} disabled={busy}>
            {t('sign.reject')}
          </button>
        </div>
      </section>
    </>
  );
}

/**
 * Where a request came from.
 *
 * A strip rather than a card. It has to be visible on the same screenful as the
 * amount — a person who scrolls past it to read the figure has not checked it —
 * and a full card for one line of text pushes the figure below the fold on a
 * phone.
 */
function Provenance({ origin, signer, chainId }: { origin: string; signer?: string; chainId?: string }) {
  return (
    <p className="prov">
      <span className="y-label">{t('sign.requestedBy')}</span>{' '}
      <code className="prov__origin">{origin}</code>
      {signer && (
        <>
          {' · '}
          <span className="y-label">{t('sign.signingAs')}</span> <Named address={signer} />
        </>
      )}
      {chainId && (
        <>
          {' · '}
          <code>{chainId}</code>
        </>
      )}
      <br />
      <span className="small muted">{t('sign.originNote')}</span>
    </p>
  );
}

/** A request for the address alone. Nothing can move on the strength of it. */
function ConnectRequest({ origin, address }: { origin: string; address: string }) {
  return (
    <>
      <p className="eyebrow">{t('sign.eyebrowConnect')}</p>
      <h1>{t('sign.connectTitle')}</h1>
      <Provenance origin={origin} />

      <section className="card">
        <h2>{t('sign.whatItGets')}</h2>
        <p>
          <Identifier value={address} />
        </p>
        <p className="muted">{t('sign.connectBody')}</p>
        <p className="rev rev--none">
          <span className="rev__word">{t('sign.revLabel.none')}</span>
          <span className="rev__note">{t('sign.connectRefuses')}</span>
        </p>
      </section>
    </>
  );
}

/**
 * A request for a signature: the whole reason this screen exists.
 *
 * The order is deliberate and is the opposite of what a message list gives you.
 * The consequence comes first — what leaves, what arrives, what locks — then
 * whether it can be undone, then who asked, and only then the messages
 * themselves. Somebody who reads the first screenful and nothing else has still
 * been told the thing that costs them money if they get it wrong.
 */
function SignRequest({
  request,
  origin,
}: {
  request: Extract<AnyRequest, { kind: 'signDirect' }>;
  origin: string;
}) {
  // Memoised on the bytes: decoding builds a protobuf registry, and this
  // component re-renders on every keystroke in the password field below it.
  //
  // The names map is what turns "yml19csnys…0mdz sent 12.5 YML" into "You sent
  // 12.5 YML". A person checking a transfer should not have to compare their
  // own truncated address against the one in the sentence to work out which
  // side of it they are on — and comparing truncations is exactly the check
  // that fails when somebody is in a hurry.
  const names = useMemo(() => {
    const map: Record<string, string> = { [request.signerAddress]: t('sign.you') };
    for (const contact of listContacts()) map[contact.address] ??= contact.pseudonym;
    return map;
  }, [request.signerAddress]);

  const summary: SigningRequestSummary = useMemo(
    () =>
      summariseSigningRequest(fromBase64(request.bodyBytes), fromBase64(request.authInfoBytes), {
        names,
      }),
    [request.bodyBytes, request.authInfoBytes, names],
  );
  const consequences = useMemo(
    () => consequencesOf(summary, request.signerAddress),
    [summary, request.signerAddress],
  );

  if (summary.undecodable) {
    return (
      <>
        <p className="eyebrow">{t('sign.eyebrow')}</p>
        <h1>{t('sign.notATxTitle')}</h1>
        <Provenance origin={origin} signer={request.signerAddress} chainId={request.chainId} />
        <section className="card">
          <div className="notice notice--bad">
            <strong>{t('sign.notATx')}</strong> {t('sign.notATxBody')}{' '}
            <strong>{t('sign.doNotApprove')}</strong>
          </div>
          <details className="payload">
            <summary>{t('sign.whyUnreadable')}</summary>
            <pre className="payload__pre">{summary.undecodable}</pre>
          </details>
        </section>
      </>
    );
  }

  const fee = describeFee(summary.fee);

  return (
    <>
      <p className="eyebrow">{t('sign.eyebrow')}</p>

      {/* One sentence first. Somebody who reads nothing else on this screen
          should still have been told what it does. */}
      <h1 className="sign__headline">
        {summary.messages.length === 1
          ? describeRequestMessage(summary.messages[0]!)
          : t('sign.severalActions', { n: String(summary.messages.length) })}
      </h1>

      <Provenance origin={origin} signer={request.signerAddress} chainId={request.chainId} />

      <Ledger consequences={consequences} />

      <section className="card">
        <h2>{t('sign.detail')}</h2>

        {summary.incomplete && (
          <div className="notice notice--bad">
            <strong>{t('sign.incompleteTitle')}</strong> {t('sign.incompleteBody')}
          </div>
        )}

        <ol className="sign__list">
          {summary.messages.map((message, i) => (
            <MessageRow key={i} message={message} depth={0} />
          ))}
        </ol>

        {/* The fee, only when there is one. On this chain it is normally zero or
            sponsored, and printing "fee: 0 YML" on every approval is how people
            learn to skip the line where a real fee would appear. */}
        {fee && <p className="sign__fee">{fee}</p>}

        {summary.memo && (
          <p className="small">
            <span className="y-label">{t('sign.memo')}</span>
            <br />
            <em>{summary.memo}</em> <span className="muted">— {t('sign.memoNote')}</span>
          </p>
        )}
      </section>
    </>
  );
}

/**
 * The ledger: what signing this does to the account that signs it.
 *
 * Three figures at most, each with a label saying whose account it is about,
 * and an em dash where there is nothing rather than a zero — `0 YML` claims the
 * account deals in that currency and has none of it, which is a different fact.
 *
 * The reversibility line is a word, a wash and a sentence together. Never the
 * wash alone: the difference between "you can undo this" and "nobody can" is
 * the most important thing on the screen, and a reader who cannot distinguish
 * amber from red would be reading a blank.
 */
function Ledger({ consequences }: { consequences: SigningConsequences }) {
  const out = leaving(consequences);
  const inward = arriving(consequences);
  const locked = locking(consequences);
  const elsewhere = consequences.movements.filter((m) => m.side === 'elsewhere');
  const nothingAtAll =
    out.length === 0 &&
    inward.length === 0 &&
    locked.length === 0 &&
    elsewhere.length === 0 &&
    consequences.authority.length === 0;

  return (
    <section className="card card--ledger">
      <h2>{t('sign.whatChanges')}</h2>

      <Reversible kind={consequences.reversibility} />

      {consequences.incomplete && (
        <div className="notice notice--bad">
          <strong>{t('sign.ledgerIncompleteTitle')}</strong> {t('sign.ledgerIncompleteBody')}
        </div>
      )}

      {nothingAtAll && !consequences.incomplete && (
        <p className="muted">{t('sign.nothingMoves')}</p>
      )}

      {(out.length > 0 || inward.length > 0 || locked.length > 0) && (
        <dl className="ledger">
          {out.length > 0 && <Figure label={t('sign.leaves')} coins={out} tone="out" />}
          {inward.length > 0 && <Figure label={t('sign.arrives')} coins={inward} tone="in" />}
          {locked.length > 0 && <Figure label={t('sign.becomesLocked')} coins={locked} tone="lock" />}
        </dl>
      )}

      {consequences.locks.map((lock, i) => (
        <p className="ledger__term" key={i}>
          <span className="y-label">{t('sign.onTheseTerms')}</span>{' '}
          {formatCoins(lock.coins)} — {lock.note}
          {lock.beneficiary && (
            <>
              , {t('sign.committedTo')} <Named address={lock.beneficiary} />
            </>
          )}
          {'. '}
          {lock.releaseUnknown ? (
            <em>{t('sign.releaseUnknown')}</em>
          ) : lock.releasesAt ? (
            <>
              {/* A duration, not `timeUntil`, which returns a whole phrase
                  ("3 months left") and reads as "Released 3 months left"
                  inside this sentence. The exact date follows, because a
                  contract is reconciled against a date and not against
                  "3 months". */}
              {t('sign.releasesIn', {
                when: formatDuration(
                  Math.max(0, Math.round(lock.releasesAt - Date.now() / 1000)),
                ),
              })}{' '}
              <span className="muted">
                ({new Date(lock.releasesAt * 1000).toLocaleDateString()})
              </span>
            </>
          ) : null}{' '}
          {lock.revocable === true && <em>{t('sign.canCancel')}</em>}
          {lock.revocable === false && <em>{t('sign.cannotCancel')}</em>}
        </p>
      ))}

      {/* Money that moves without touching the signer's balance. Naming the
          source is the whole point: a treasurer signing a payment out of
          treasury 3 must not be shown "250 YML leaves this account". */}
      {elsewhere.length > 0 && (
        <dl className="ledger">
          {elsewhere.map((movement, i) => (
            <div key={i}>
              <dt>{t('sign.movesElsewhere')}</dt>
              <dd>
                <span className="ledger__figure">{formatCoins(movement.coins)}</span>
                <span className="small muted">
                  {movement.from && (
                    <>
                      {' '}
                      {t('sign.outOf')} <SourceName value={movement.from} />
                    </>
                  )}
                  {movement.to && (
                    <>
                      {' '}
                      {t('sign.into')} <SourceName value={movement.to} />
                    </>
                  )}
                </span>
              </dd>
            </div>
          ))}
        </dl>
      )}

      {consequences.authority.length > 0 && (
        <ul className="ledger__grants">
          {consequences.authority.map((grant, i) => (
            <li key={i}>
              <span className="y-label">{t('sign.grants')}</span>{' '}
              {grant.grantee ? <Named address={grant.grantee} /> : t('sign.someone')} — {grant.power}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** A figure with its label. Mono, tabular, large — it is the number on trial. */
function Figure({ label, coins, tone }: { label: string; coins: Coin[]; tone: string }) {
  return (
    <div className={`ledger__row ledger__row--${tone}`}>
      <dt>{label}</dt>
      <dd className="ledger__figure">{coins.length === 0 ? EMPTY_AMOUNT : formatCoins(coins)}</dd>
    </div>
  );
}

/**
 * `treasury 3` and `pool 7` are labels, not addresses; an address is truncated
 * and copyable. Passing a label through the address renderer produced
 * `trea…y 3`, which reads as a corrupted identifier.
 */
function SourceName({ value }: { value: string }) {
  if (value.startsWith('yml1')) return <Named address={value} />;
  return <>{value}</>;
}

/** The reversibility, as a word, a wash and a sentence — never as a wash alone. */
function Reversible({ kind }: { kind: Reversibility }) {
  const LABEL: Record<Reversibility, string> = {
    irreversible: t('sign.revLabel.irreversible'),
    delayed: t('sign.revLabel.delayed'),
    revocable: t('sign.revLabel.revocable'),
    proposal: t('sign.revLabel.proposal'),
    none: t('sign.revLabel.none'),
    unknown: t('sign.revLabel.unknown'),
  };
  const NOTE: Record<Reversibility, string> = {
    irreversible: t('sign.rev.irreversible'),
    delayed: t('sign.rev.delayed'),
    revocable: t('sign.rev.revocable'),
    proposal: t('sign.rev.proposal'),
    none: t('sign.rev.none'),
    unknown: t('sign.rev.unknown'),
  };

  return (
    <p className={`rev rev--${kind}`}>
      <span className="rev__word">{LABEL[kind]}</span>
      <span className="rev__note">{NOTE[kind] || REVERSIBILITY_NOTE[kind]}</span>
    </p>
  );
}

/**
 * One message, its amounts and its nesting.
 *
 * A nested message is indented rather than flattened. The difference between
 * "this account pays 250 YML" and "this account proposes that a shared account
 * pays 250 YML" is the whole of what a signer is deciding, and flattening the
 * two loses it.
 */
function MessageRow({ message, depth }: { message: RequestMessage; depth: number }) {
  if (message.problem) {
    return (
      <li className="sign__row sign__row--bad">
        <strong>{describeRequestMessage(message)}</strong>
        <p className="small muted">
          {message.problem === 'unregistered' ? t('sign.noDecoder') : t('sign.mismatch')}
        </p>
        <code className="y-addr">{message.typeUrl}</code>
      </li>
    );
  }

  const decoded = message.decoded!;

  return (
    <li className="sign__row">
      <div className="sign__title">{decoded.title}</div>
      <p className="sign__summary">{decoded.summary}</p>

      {/* The amount again, on its own, in mono. It is in the sentence above,
          but a figure inside a sentence is not a figure anybody double-checks. */}
      {decoded.coins && decoded.coins.length > 0 && (
        <p className="sign__amount">
          {decoded.coins.map((c) => formatAmount(c.amount, c.denom)).join(' + ')}
        </p>
      )}

      {/* One rendering of the address, not two. This printed the resolved name
          and then the truncation beside it, so an account with no name showed
          "yml1ywwjfd…z7j0 yml1ywwjfd…z7j0" — which reads as a rendering fault
          on the screen that most needs to look trustworthy. */}
      {decoded.counterparty && (
        <p className="small muted">
          {t('sign.to')} <Identifier value={decoded.counterparty} />
        </p>
      )}

      {decoded.details && decoded.details.length > 0 && (
        <dl className="sign__details">
          {decoded.details.map((d, i) => (
            <div key={i}>
              <dt>{d.label}</dt>
              <dd className={d.address ? 'y-addr' : undefined}>
                {d.address && d.value.startsWith('yml1') ? truncateAddress(d.value) : d.value}
              </dd>
            </div>
          ))}
        </dl>
      )}

      {message.contains.length > 0 && (
        <>
          <p className="y-label sign__contains">
            {message.contains.length === 1 ? t('sign.carriesOne') : t('sign.carriesMany')}
          </p>
          <ol className="sign__list">
            {message.contains.map((inner, i) => (
              <MessageRow key={i} message={inner} depth={depth + 1} />
            ))}
          </ol>
        </>
      )}

      {/* The raw message, always reachable. An expert who cannot audit what
          they are signing stops trusting the wallet, and that trust is the
          product. */}
      <details className="payload">
        <summary>{t('sign.raw')}</summary>
        <pre className="payload__pre">{JSON.stringify(decoded.raw, null, 2)}</pre>
      </details>
    </li>
  );
}

/** Nothing was signed, so there is nothing to watch. */
function Connected({ origin, address, onDone }: { origin: string; address: string; onDone: () => void }) {
  return (
    <>
      <p className="eyebrow">{t('sign.eyebrowConnect')}</p>
      <h1>{t('sign.connected')}</h1>
      <section className="card">
        <p>{t('sign.connectedBody', { origin })}</p>
        <p>
          <Identifier value={address} />
        </p>
        <div className="actions__row">
          <button type="button" className="chip" onClick={() => window.close()}>
            {t('sign.close')}
          </button>
          <button type="button" className="chip" onClick={onDone}>
            {t('sign.backToWaiting')}
          </button>
        </div>
      </section>
    </>
  );
}

/**
 * How long before this stops reading as "any moment now".
 *
 * Blocks on this chain are a few seconds apart, so ninety seconds is many
 * blocks. Past that the honest answer is not "still waiting" — it is that the
 * transaction has not appeared, which usually means the application never
 * broadcast it, and a spinner that turns forever says neither.
 *
 * What it does *not* do past that point is stop looking. The first version
 * gave up, and said "Nothing has left your account" — which was true when it
 * printed and became false four minutes later when the application finally
 * broadcast, with the window still open and still asserting it. Observed
 * exactly that way on the devnet. A screen that stops watching must not make a
 * claim that only holds while it is watching, so this one slows down instead.
 */
const WATCH_SECONDS = 90;
const POLL_FAST_MS = 2500;
const POLL_SLOW_MS = 15000;

/**
 * What happened to the transaction that was just signed.
 *
 * Four states, and none of them is a spinner with no anchor:
 *
 *   **pending** — signed, handed back, not in a block yet. The hash is already
 *   shown, so it can be looked up elsewhere from the first second.
 *   **included, code 0** — settled, with the block. This is the only success.
 *   **included, code ≠ 0** — the chain took it and refused it, which is exactly
 *   the case a broadcast-level "accepted" hides. Translated, with the raw log.
 *   **unreachable** — this wallet cannot see the chain. Said plainly rather than
 *   rendered as "still pending", because that would be a guess presented as an
 *   observation.
 */
function Outcome({ signed, onDone }: { signed: Signed; onDone: () => void }) {
  const [status, setStatus] = useState<TxStatus>({ state: 'pending' });
  const [elapsed, setElapsed] = useState(0);
  const started = useRef(Date.now());

  useEffect(() => {
    let live = true;
    let timer: ReturnType<typeof setTimeout>;

    async function look() {
      const next = await client.txStatus(signed.hash);
      if (!live) return;
      const since = Date.now() - started.current;
      setStatus(next);
      setElapsed(Math.round(since / 1000));
      // Stop on a settled answer, and only on that. Polling a chain that has
      // already answered is how a wallet keeps a node busy for as long as
      // somebody leaves a tab open; giving up on one that has not is how a
      // wallet ends up displaying a stale conclusion.
      if (next.state === 'included') return;
      timer = setTimeout(look, since > WATCH_SECONDS * 1000 ? POLL_SLOW_MS : POLL_FAST_MS);
    }

    void look();
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [signed.hash]);

  const consequences = consequencesOf(signed.summary, signed.signer);
  const out = leaving(consequences);
  const gaveUp = status.state === 'pending' && elapsed >= WATCH_SECONDS;

  return (
    <>
      <p className="eyebrow">{t('sign.eyebrowSigned')}</p>
      <h1>
        {status.state === 'included' && status.code === 0
          ? t('sign.settled')
          : status.state === 'included'
            ? t('sign.refused')
            : status.state === 'unreachable'
              ? t('sign.cannotSee')
              : gaveUp
                ? t('sign.notSeen')
                : t('sign.signedTitle')}
      </h1>

      <section className="card">
        {/* The amount, in the past tense once it is in a block and in the
            conditional until then. "250 YML has left your account" printed
            while the transaction is still in a mempool is the exact lie this
            whole screen exists to stop. */}
        {out.length > 0 && (
          <p className="sign__amount">{formatCoins(out)}</p>
        )}

        <ol className="track" aria-label={t('sign.progress')}>
          <li className="track__step track__step--done">
            <span className="track__dot" aria-hidden="true" />
            <span>{t('sign.stepSigned')}</span>
          </li>
          <li
            className={
              status.state === 'included'
                ? 'track__step track__step--done'
                : status.state === 'unreachable' || gaveUp
                  ? 'track__step track__step--stalled'
                  : 'track__step track__step--live'
            }
          >
            <span className="track__dot" aria-hidden="true" />
            <span>
              {status.state === 'included'
                ? t('sign.stepInBlock', { height: status.height.toLocaleString() })
                : status.state === 'unreachable'
                  ? t('sign.stepUnknown')
                  : gaveUp
                    ? t('sign.stepNotSeen')
                    : t('sign.stepWaiting', { seconds: String(elapsed) })}
            </span>
          </li>
          <li
            className={
              status.state === 'included' && status.code === 0
                ? 'track__step track__step--done'
                : status.state === 'included'
                  ? 'track__step track__step--bad'
                  : 'track__step'
            }
          >
            <span className="track__dot" aria-hidden="true" />
            <span>
              {status.state === 'included'
                ? status.code === 0
                  ? t('sign.stepExecuted')
                  : t('sign.stepRefused', { code: String(status.code) })
                : t('sign.stepPendingExecution')}
            </span>
          </li>
        </ol>

        {status.state === 'included' && status.code !== 0 && (
          <Refusal rawLog={status.rawLog} />
        )}

        {status.state === 'unreachable' && (
          <div className="notice">
            <strong>{t('sign.cannotSeeTitle')}</strong> {t('sign.cannotSeeBody')}
            <details className="payload">
              <summary>{t('sign.whatTheChainSaid')}</summary>
              <pre className="payload__pre">{status.error}</pre>
            </details>
          </div>
        )}

        {gaveUp && (
          <div className="notice">
            <strong>{t('sign.notSeenTitle')}</strong>{' '}
            {t('sign.notSeenBody', {
              origin: signed.origin,
              duration: formatDuration(elapsed),
            })}
          </div>
        )}

        <p className="small muted">
          <span className="y-label">{t('sign.txId')}</span>
          <br />
          <Identifier value={signed.hash} />
        </p>

        <div className="actions__row">
          <button type="button" className="chip" onClick={() => window.close()}>
            {t('sign.close')}
          </button>
          <button type="button" className="chip" onClick={onDone}>
            {t('sign.backToWaiting')}
          </button>
        </div>
      </section>
    </>
  );
}

/** A refusal recorded in a block: what happened, why, and what to do. */
function Refusal({ rawLog }: { rawLog: string }) {
  const error = translateError(rawLog);
  return (
    <div className="notice notice--bad">
      <strong>{error.message}</strong>
      {error.reason ? <> {error.reason}</> : null}
      {error.nextStep ? <p className="error__next">{error.nextStep}</p> : null}
      {error.raw && error.raw !== error.message ? (
        <details className="payload">
          <summary>{t('sign.whatTheChainSaid')}</summary>
          <pre className="payload__pre">{error.raw}</pre>
        </details>
      ) : null}
    </div>
  );
}
