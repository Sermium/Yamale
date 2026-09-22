/**
 * Everything this application asks the registry, and the two things it can ask
 * it to do.
 *
 * Reads and writes in one file, the way Yamale Pay and the tokenisation app do
 * it: the list of things a field device can alter in a national livestock
 * register should be readable in under a minute. Here it is two items —
 * attest a transfer, and record a health event — and nothing else. This
 * application cannot enrol a herd, cannot identify a new animal, cannot certify
 * a pedigree and cannot move title, because a device in a pen does none of
 * those; an interface that could would be a registry office wearing a
 * herder's name.
 *
 * Every read goes over ABCI. `/api/rest/yamale/blockchain/herd/` is outside the
 * proxy's allowlist and answers 401 — verified against devnet-2 — which a
 * browser renders as a login box nobody has the password for. See `abci.ts`.
 */

import {
  QueryAgentsRequest,
  QueryAgentsResponse,
  QueryAnimalByNationalIDRequest,
  QueryAnimalByTagRequest,
  QueryAnimalRequest,
  QueryAnimalResponse,
  QueryAnimalsRequest,
  QueryAnimalsResponse,
  QueryAuthoritiesRequest,
  QueryAuthoritiesResponse,
  QueryHealthEventsRequest,
  QueryHealthEventsResponse,
  QueryHerdByRefRequest,
  QueryHerdRequest,
  QueryHerdResponse,
  QueryPedigreeRequest,
  QueryPedigreeResponse,
  QueryTransfersByAnimalRequest,
  QueryTransfersResponse,
} from '../../sdk/src/generated/blockchain/herd/v1/query.ts';
import { MsgAttestTransfer, MsgRecordHealthEvent } from '../../sdk/src/generated/blockchain/herd/v1/tx.ts';

import { decoded, query, type Outcome } from './abci.ts';
import {
  toAgent,
  toAncestor,
  toAnimal,
  toAuthority,
  toHealthEvent,
  toHerd,
  toTransfer,
  type Agent,
  type Animal,
  type Authority,
  type HealthEvent,
  type Herd,
  type Line,
  type Transfer,
} from './herd.ts';

const SVC = '/blockchain.herd.v1.Query';

/* ------------------------------------------------------------------ reads */

export async function animal(id: string): Promise<Outcome<Animal>> {
  const req = QueryAnimalRequest.encode({ id }).finish();
  return decoded(await query(`${SVC}/Animal`, req), (b) => toAnimal(QueryAnimalResponse.decode(b).animal!));
}

/** The certificate lookup: the identifier printed on the document. */
export async function animalByNationalId(nationalId: string): Promise<Outcome<Animal>> {
  const req = QueryAnimalByNationalIDRequest.encode({ nationalId }).finish();
  return decoded(
    await query(`${SVC}/AnimalByNationalID`, req),
    (b) => toAnimal(QueryAnimalResponse.decode(b).animal!),
  );
}

/** The query an agent with a stick reader makes: whose animal is this. */
export async function animalByTag(tag: string): Promise<Outcome<Animal>> {
  const req = QueryAnimalByTagRequest.encode({ tag }).finish();
  return decoded(
    await query(`${SVC}/AnimalByTag`, req),
    (b) => toAnimal(QueryAnimalResponse.decode(b).animal!),
  );
}

export async function herd(id: string): Promise<Outcome<Herd>> {
  const req = QueryHerdRequest.encode({ id }).finish();
  return decoded(await query(`${SVC}/Herd`, req), (b) => toHerd(QueryHerdResponse.decode(b).herd!));
}

/** By the registry's own reference — what an officer holding a paper file has. */
export async function herdByRef(reference: string): Promise<Outcome<Herd>> {
  const req = QueryHerdByRefRequest.encode({ reference }).finish();
  return decoded(await query(`${SVC}/HerdByRef`, req), (b) => toHerd(QueryHerdResponse.decode(b).herd!));
}

/**
 * Every animal in one herd.
 *
 * Paged to exhaustion rather than to the first page, because this is the
 * download that the offline replica is made of and a replica that quietly
 * contained the first hundred of two hundred animals would answer "not one of
 * yours" about the second hundred. The page loop stops on an empty next key,
 * and also on a page that returned nothing — a malformed key that never
 * advances would otherwise spin forever on a device with one bar of signal.
 */
export async function animals(herdId: string): Promise<Outcome<Animal[]>> {
  const out: Animal[] = [];
  let key: Uint8Array = new Uint8Array(0);
  let height = 0;

  for (let page = 0; page < 64; page++) {
    const req = QueryAnimalsRequest.encode({
      herdId,
      pagination: { key, offset: '0', limit: '200', countTotal: false, reverse: false },
    }).finish();

    const got = decoded(await query(`${SVC}/Animals`, req), (b) => QueryAnimalsResponse.decode(b));
    if (!got.ok) return got;

    height = got.height;
    out.push(...got.value.animals.map(toAnimal));

    const next = got.value.pagination?.nextKey ?? new Uint8Array(0);
    if (next.length === 0 || got.value.animals.length === 0) break;
    key = next;
  }

  return { ok: true, value: out, height };
}

export async function healthEvents(herdId: string): Promise<Outcome<HealthEvent[]>> {
  const req = QueryHealthEventsRequest.encode({
    herdId,
    pagination: { key: new Uint8Array(0), offset: '0', limit: '100', countTotal: false, reverse: true },
  }).finish();
  return decoded(
    await query(`${SVC}/HealthEvents`, req),
    (b) => QueryHealthEventsResponse.decode(b).healthEvents.map(toHealthEvent),
  );
}

/**
 * The line, upward, bounded.
 *
 * `truncated` is carried to the screen untouched. The module sets it when the
 * walk stopped at the depth bound rather than at a founder, and without it a
 * buyer reads "no further ancestors" where the truth is "we stopped looking".
 */
export async function pedigree(animalId: string, depth = 4): Promise<Outcome<Line>> {
  const req = QueryPedigreeRequest.encode({ animalId, depth }).finish();
  return decoded(await query(`${SVC}/Pedigree`, req), (b) => {
    const res = QueryPedigreeResponse.decode(b);
    return { nodes: res.nodes.map(toAncestor), truncated: Boolean(res.truncated) };
  });
}

/** One animal's provenance, objected transfers included. */
export async function transfersByAnimal(animalId: string): Promise<Outcome<Transfer[]>> {
  const req = QueryTransfersByAnimalRequest.encode({
    animalId,
    pagination: { key: new Uint8Array(0), offset: '0', limit: '100', countTotal: false, reverse: true },
  }).finish();
  return decoded(
    await query(`${SVC}/TransfersByAnimal`, req),
    (b) => QueryTransfersResponse.decode(b).transfers.map(toTransfer),
  );
}

/**
 * The accredited people, struck-off ones included.
 *
 * The module returns those on purpose — "was this person accredited when they
 * signed" is a question about the past — so this does not filter them out
 * either. The Account screen shows the operator's own record with its `active`
 * flag, because an agent who has been struck off and does not know it will
 * spend a day collecting attestations the chain will refuse.
 */
export async function agents(authority = ''): Promise<Outcome<Agent[]>> {
  const req = QueryAgentsRequest.encode({
    authority,
    pagination: { key: new Uint8Array(0), offset: '0', limit: '200', countTotal: false, reverse: false },
  }).finish();
  return decoded(
    await query(`${SVC}/Agents`, req),
    (b) => QueryAgentsResponse.decode(b).agents.map(toAgent),
  );
}

export async function authorities(): Promise<Outcome<Authority[]>> {
  const req = QueryAuthoritiesRequest.encode({
    pagination: { key: new Uint8Array(0), offset: '0', limit: '100', countTotal: false, reverse: false },
  }).finish();
  return decoded(
    await query(`${SVC}/Authorities`, req),
    (b) => QueryAuthoritiesResponse.decode(b).authorities.map(toAuthority),
  );
}

/* ----------------------------------------------------------------- writes */

/**
 * The two messages this device can send, as `{ typeUrl, value }` pairs.
 *
 * Built here and signed elsewhere: the signer is `identity.ts`, which is
 * another agent's, and this application deliberately never holds a key. See
 * `ports.ts`.
 *
 * NOTHING ELSE IS HERE, and the absences are the design:
 *
 *   - There is no enrolment message, because `x/herd` has none. §8 proposes
 *     `Enrolment` and `IdentificationClaim` and neither exists on the chain
 *     this talks to. `outbox.ts` holds those entries as `held` rather than
 *     pretending they are on their way.
 *   - There is no `MsgIdentifyAnimal`, no `MsgCertifyAnimal` and no transfer
 *     proposal. Creating a record and moving title are a registry office's
 *     acts, and the four-party shape exists precisely so that one person with
 *     one device cannot perform them.
 */
export const ATTEST_TRANSFER = '/blockchain.herd.v1.MsgAttestTransfer';
export const RECORD_HEALTH_EVENT = '/blockchain.herd.v1.MsgRecordHealthEvent';

/**
 * An accredited agent swearing the animal presented is the animal on the record.
 *
 * `evidenceHash` is "what the attestor actually checked" — the module's own
 * words — so a field capture reaches the chain here or nowhere. A false
 * attestation becomes a specific claim by a named person rather than a shrug,
 * which is the whole reason the field exists.
 */
export function attestTransfer(input: {
  attestor: string;
  transferId: string;
  evidenceHash: string;
}): { typeUrl: string; value: MsgAttestTransfer } {
  return {
    typeUrl: ATTEST_TRANSFER,
    value: MsgAttestTransfer.fromPartial({
      attestor: input.attestor,
      transferId: input.transferId,
      evidenceHash: input.evidenceHash,
    }),
  };
}

/**
 * A treatment administered, signed by the person who administered it.
 *
 * `treated` may differ from the herd's declared size, and the module says
 * forcing them equal "would destroy the only signal that a declaration is
 * drifting". So this passes the number the operator entered, unaltered, and
 * the screen does not offer to fill it in from the declaration.
 */
export function recordHealthEvent(input: {
  agent: string;
  herdId: string;
  disease: string;
  batch: string;
  treated: number;
}): { typeUrl: string; value: MsgRecordHealthEvent } {
  return {
    typeUrl: RECORD_HEALTH_EVENT,
    value: MsgRecordHealthEvent.fromPartial({
      agent: input.agent,
      herdId: input.herdId,
      disease: input.disease,
      batch: input.batch,
      treated: input.treated,
    }),
  };
}
