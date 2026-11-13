/**
 * Enrolling an account with the custodian and recovery services.
 *
 * # What this is for
 *
 * The wallet has always held a key. What it has never had is a way to get one
 * back: a vault is one encrypted blob in one browser's storage, so a lost
 * phone, a cleared site, or a reinstalled operating system was the end of the
 * account. That is recorded in the backlog as the thing that made a forgotten
 * password a lost account.
 *
 * Enrolment fixes exactly that and nothing else. It seals the key under the
 * account holder's password, splits the sealed blob three ways, keeps one part
 * here and deposits the other two with two independent services. Any two parts
 * and the password rebuild the key.
 *
 * # The address does not change
 *
 * This enrols the key the wallet ALREADY has, rather than making a new one.
 * That is the whole reason `privateKeyFromMnemonic` exists: the vault stores a
 * BIP39 mnemonic and the services need the 32-byte key it derives to, so the
 * derivation has to be done here, at the same HD path CosmJS used to create the
 * account in the first place.
 *
 * Get that path wrong and everything still appears to work — an enrolment
 * succeeds, a deposit is accepted, a recovery returns a key — and the key
 * controls a different address from the one holding the money. So there is a
 * test that derives the key and compares its address to the wallet's own.
 *
 * # What is sent, and what is not
 *
 * Sent: the email, the password, the compressed public key, one holding, and a
 * signature over all of it. The password goes because each service stores an
 * Argon2id verifier for it — it is never stored, and it is not what opens the
 * seal on the service side, because nothing there ever opens the seal.
 *
 * Not sent: the mnemonic, the private key, the device's own holding, and the
 * other service's holding. Each service is told a different one, and a service
 * that received both would be a service that could attempt a recovery alone.
 */

import { Bip39, EnglishMnemonic, Secp256k1, Slip10, Slip10Curve } from '@cosmjs/crypto';
import { rawSecp256k1PubkeyToRawAddress } from '@cosmjs/amino';
import { toBech32 } from '@cosmjs/encoding';
import { makeCosmoshubPath } from '@cosmjs/proto-signing';

import { publicKeyOf, signDeposit, type Holding } from './deposit.ts';
import { protect, ROLE_CUSTODIAN, ROLE_DEVICE, ROLE_RECOVERY } from './protect.ts';

export class EnrolError extends Error {
  // Declared and assigned rather than written as a constructor parameter
  // property: Node's --experimental-strip-types removes types without
  // transforming, and a parameter property is syntax that has to be
  // transformed. The repo's test runner uses exactly that mode.
  readonly role: string | undefined;

  constructor(message: string, role?: string) {
    super(message);
    this.name = 'EnrolError';
    this.role = role;
  }
}

/**
 * The private key behind an account, derived from its mnemonic.
 *
 * The default path is `makeCosmoshubPath(0)` because that is what
 * `DirectSecp256k1HdWallet.fromMnemonic` uses when it is given no `hdPaths`,
 * which is how ../vault.ts creates every account. Passing the path explicitly
 * rather than relying on that default would not make it safer — it would move
 * the assumption, not remove it — so the assumption is tested instead.
 */
export async function privateKeyFromMnemonic(
  mnemonic: string,
  accountIndex = 0,
): Promise<Uint8Array> {
  const seed = await Bip39.mnemonicToSeed(new EnglishMnemonic(mnemonic));
  const { privkey } = Slip10.derivePath(
    Slip10Curve.Secp256k1,
    seed,
    makeCosmoshubPath(accountIndex),
  );
  return privkey;
}

/** The bech32 address a private key controls on this chain. */
export async function addressOf(privateKey: Uint8Array, prefix = 'yml'): Promise<string> {
  const { pubkey } = await Secp256k1.makeKeypair(privateKey);
  return toBech32(prefix, rawSecp256k1PubkeyToRawAddress(Secp256k1.compressPubkey(pubkey)));
}

/** Where the two services are. Separate hosts in a real deployment. */
export interface Services {
  custodianUrl: string;
  recoveryUrl: string;
}

export interface EnrolResult {
  address: string;
  /** The holding this device keeps. Everything else went to a service. */
  deviceHolding: Holding;
  /**
   * Roles whose service answered 409: an account already exists under that
   * email. That is success for re-running a half-finished enrolment of THIS
   * key, but the service may hold another key's copies, so a caller enrolling
   * a brand-new key must not treat it as protection.
   */
  alreadyEnrolled: Array<'custodian' | 'recovery'>;
}

/**
 * What a service is told. Byte arrays go as base64, matching Go's JSON
 * encoding of `[]byte`, which is what the services unmarshal into.
 */
interface DepositBody {
  email: string;
  password: string;
  pubkey: string;
  holding: unknown;
  signature: string;
}

/**
 * Whether both services are actually there.
 *
 * Asked before the screen offers to enrol, rather than discovered when somebody
 * presses the button. The two deployments are not installed on every
 * environment — `deploy/nginx/yamale-account.conf` says so at the top and
 * `deploy/deploy.sh` does not copy it — so a wallet served from a host without
 * them would otherwise show a prominent control that always fails.
 *
 * "Press this and find out" is a reasonable design for an action that usually
 * works. This one usually does not, yet, and the difference between "your
 * account cannot be protected here" and "something went wrong" is the whole of
 * what a person needs to know.
 *
 * Both must answer. One service is not half a protection: the parts are
 * deposited with both or the account is recoverable only from the device, which
 * is the situation enrolment exists to leave.
 */
export async function servicesAvailable(
  services: Services,
  fetchImpl: typeof fetch = fetch,
  timeoutMs = 4000,
): Promise<boolean> {
  const reachable = async (url: string) => {
    // Aborted rather than left hanging. A wallet opened on a network that
    // blackholes the request would sit on "checking" for as long as the browser
    // allows, which reads as broken rather than as absent.
    const abort = new AbortController();
    const timer = setTimeout(() => abort.abort(), timeoutMs);
    try {
      const res = await fetchImpl(`${url}/v1/health`, { signal: abort.signal });
      return res.ok;
    } catch {
      return false;
    } finally {
      clearTimeout(timer);
    }
  };
  const [custodian, recovery] = await Promise.all([
    reachable(services.custodianUrl),
    reachable(services.recoveryUrl),
  ]);
  return custodian && recovery;
}

/**
 * Enrol a key with both services.
 *
 * Deliberately NOT transactional, because it cannot be: two independent
 * services with no coordinator between them means the second can fail after the
 * first has committed. What that leaves is an account enrolled with one service
 * and not the other — recoverable only with the device holding, which is the
 * situation the person was already in before they started.
 *
 * So a partial failure is reported as one, by name, and re-running is safe: the
 * service that already has the deposit answers 409 and that is treated as
 * success, because an account already enrolled with this key is the outcome
 * being asked for.
 */
export async function enrol(
  privateKey: Uint8Array,
  email: string,
  password: string,
  services: Services,
  fetchImpl: typeof fetch = fetch,
): Promise<EnrolResult> {
  const holdings = await protect(privateKey, password);
  const byRole = new Map(holdings.map((h) => [h.role, h]));

  const deviceHolding = byRole.get(ROLE_DEVICE);
  if (!deviceHolding) throw new EnrolError('no holding was produced for this device');

  const pubkey = await publicKeyOf(privateKey);

  let address = '';
  const alreadyEnrolled: Array<'custodian' | 'recovery'> = [];
  for (const [role, url] of [
    [ROLE_CUSTODIAN, services.custodianUrl],
    [ROLE_RECOVERY, services.recoveryUrl],
  ] as const) {
    const holding = byRole.get(role);
    if (!holding) throw new EnrolError(`no holding was produced for the ${role}`, role);

    // Signed over the EMAIL. The blind index each service files this under is
    // derived from a pepper that never leaves it, so the device could not sign
    // over the index even if it wanted to.
    const signature = await signDeposit(privateKey, email, role, holding);

    const body: DepositBody = {
      email,
      password,
      pubkey: base64(pubkey),
      holding: encodeHolding(holding),
      signature: base64(signature),
    };

    const { address: reported, existed } = await post(fetchImpl, `${url}/v1/enrol`, body, role);
    if (existed) alreadyEnrolled.push(role === ROLE_CUSTODIAN ? 'custodian' : 'recovery');
    // Each service derives the address from the public key it verified, rather
    // than being told it. They must agree, and with each other: a disagreement
    // means one of them verified something different.
    if (address && reported && reported !== address) {
      throw new EnrolError(
        'the two services derived different addresses from the same key, so this enrolment ' +
          'was not carried intact; nothing more was sent',
        role,
      );
    }
    if (reported) address = reported;
  }

  if (!address) {
    address = await addressOf(privateKey);
  }
  return { address, deviceHolding, alreadyEnrolled };
}

/**
 * One deposit.
 *
 * A 409 is success. The service is saying this email already has an account,
 * and re-running an enrolment that half-completed is the normal way to finish
 * it — treating that as a failure would leave somebody permanently unable to
 * complete the half that is missing.
 */
async function post(
  fetchImpl: typeof fetch,
  url: string,
  body: DepositBody,
  role: string,
): Promise<{ address: string; existed: boolean }> {
  let response: Response;
  try {
    response = await fetchImpl(url, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    });
  } catch (cause) {
    throw new EnrolError(`the ${role} service could not be reached`, role);
  }

  if (response.status === 409) return { address: '', existed: true };
  if (!response.ok) {
    // The services answer {"error": "..."} and those messages are written to be
    // read by a person, so it is passed through rather than replaced.
    let detail = `${response.status}`;
    try {
      const parsed = (await response.json()) as { error?: string };
      if (parsed?.error) detail = parsed.error;
    } catch {
      // A non-JSON body from a proxy rather than the service. The status is
      // what there is to report.
    }
    throw new EnrolError(`the ${role} service refused the deposit: ${detail}`, role);
  }

  const parsed = (await response.json()) as { address?: string };
  return { address: parsed?.address ?? '', existed: false };
}

/** Byte arrays as base64, which is how Go marshals `[]byte` into JSON. */
function encodeHolding(h: Holding) {
  return {
    role: h.role,
    salt: base64(h.salt),
    nonce: base64(h.nonce),
    time: h.time,
    memory: h.memory,
    threads: h.threads,
    share: { x: h.share.x, y: base64(h.share.y) },
  };
}

function base64(bytes: Uint8Array): string {
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary);
}
