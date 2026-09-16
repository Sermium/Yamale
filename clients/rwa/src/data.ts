/**
 * Reading the chain into the screens, and keeping the "we could not ask" case
 * intact all the way to the render.
 *
 * Every hook here returns an `Outcome`, never a value-or-null. The temptation
 * on a surface with no data yet is to collapse "no vehicles have been issued"
 * and "the node did not answer" into an empty array, and an interface that does
 * that renders a perfectly correct empty page while entirely broken behind it.
 * That has happened once on this project already, on the explorer.
 *
 * There is also a clock here, and it is the chain's rather than the reader's.
 * Every deadline these screens render is decided by a keeper against
 * `BlockTime()`. A countdown driven by a laptop clock that is four minutes fast
 * tells somebody the challenge window has closed when it has not.
 */
import { useCallback, useEffect, useState } from 'react';

import { head, type Head, type Outcome } from './abci.ts';
import {
  assets as fetchAssets,
  balances as fetchBalances,
  collections as fetchCollections,
  covenants as fetchCovenants,
  entitlement as fetchEntitlement,
  landAuthorisation,
  parcel as fetchParcel,
  position as fetchPosition,
  supplyOf,
  balanceOf,
  vehicle as fetchVehicle,
  type AuthorisationRecord,
  type ParcelRecord,
  type VehicleRecord,
  holders,
  type Holder,
} from './chain.ts';
import type { Covenant } from './covenant.ts';
import type { Position } from './position.ts';
import { isRealId, landGate, type Asset, type Coin, type Collection, type LandGate } from './vehicle.ts';

/** A read in flight, or its result. */
export type Load<T> = { loading: true } | { loading: false; outcome: Outcome<T> };

function useRead<T>(run: () => Promise<Outcome<T>>, deps: unknown[]): [Load<T>, () => void] {
  const [state, setState] = useState<Load<T>>({ loading: true });
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let live = true;
    setState({ loading: true });
    run().then((outcome) => { if (live) setState({ loading: false, outcome }); });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, nonce]);

  return [state, useCallback(() => setNonce((n) => n + 1), [])];
}

/* -------------------------------------------------------------- the clock */

/**
 * The chain's head, refreshed while the page is open.
 *
 * Thirty seconds rather than every block: nothing on these screens changes
 * within a block, and a page that polls a devnet at block speed is a page that
 * is mostly network traffic.
 */
export function useHead(): Head {
  const [state, setState] = useState<Head>({ known: false });

  useEffect(() => {
    let live = true;
    const tick = () => { head().then((h) => { if (live) setState(h); }); };
    tick();
    const timer = setInterval(tick, 30_000);
    return () => { live = false; clearInterval(timer); };
  }, []);

  return state;
}

/**
 * The clock every deadline on these screens is measured against.
 *
 * The chain's block time when it is known, the browser's otherwise — and the
 * fallback is stated in the return so a screen can say which it used rather
 * than quietly presenting one as the other.
 */
export function chainNow(h: Head): { at: Date; fromChain: boolean } {
  if (h.known && !Number.isNaN(h.at.getTime())) return { at: h.at, fromChain: true };
  return { at: new Date(), fromChain: false };
}

/* ---------------------------------------------------------------- indexes */

export function useCollections(): [Load<Collection[]>, () => void] {
  return useRead(fetchCollections, []);
}

export function useAssets(collectionId = ''): [Load<Asset[]>, () => void] {
  return useRead(() => fetchAssets(collectionId), [collectionId]);
}

/* ------------------------------------------------------------- one vehicle */

export function useVehicle(assetId: string): [Load<VehicleRecord>, () => void] {
  return useRead(() => fetchVehicle(assetId), [assetId]);
}

/**
 * The parcel behind a vehicle, and whether the registry's permission stands.
 *
 * Both, or neither. A parcel shown without its authorisation is a title shown
 * without the one fact that decides whether shares in it can lawfully be
 * issued, and this app exists partly to stop that page existing.
 */
export interface LandView {
  gate: LandGate;
  parcel: ParcelRecord | null;
  /** Set when the parcel itself could not be read, which is not the same as absent. */
  parcelUnreadable: boolean;
}

export function useLand(parcelId: string, nowSeconds: number): Load<LandView> {
  const [state, setState] = useState<Load<LandView>>({ loading: true });

  useEffect(() => {
    let live = true;

    if (!isRealId(parcelId)) {
      setState({
        loading: false,
        outcome: {
          ok: true,
          value: { gate: { kind: 'not-land' }, parcel: null, parcelUnreadable: false },
          height: 0,
        },
      });
      return;
    }

    setState({ loading: true });
    Promise.all([landAuthorisation(parcelId), fetchParcel(parcelId)]).then(([auth, parcel]) => {
      if (!live) return;

      const found: AuthorisationRecord | 'absent' | 'unreachable' = auth.ok
        ? auth.value
        : auth.reason === 'not-found' ? 'absent' : 'unreachable';

      setState({
        loading: false,
        outcome: {
          ok: true,
          value: {
            gate: landGate(parcelId, found, nowSeconds),
            parcel: parcel.ok ? parcel.value : null,
            parcelUnreadable: !parcel.ok && parcel.reason === 'unreachable',
          },
          height: auth.ok ? auth.height : parcel.ok ? parcel.height : 0,
        },
      });
    });

    return () => { live = false; };
    // nowSeconds deliberately excluded: it changes every thirty seconds and the
    // gate's answer only changes when the chain's does. Re-reading on the clock
    // would put two queries a minute against the registry for every open tab.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [parcelId]);

  return state;
}

/* ---------------------------------------------------------- what was filed */

/**
 * The register of what has been published against one tokenisation.
 *
 * Its failure is kept whole rather than flattened to an empty list, and on this
 * panel that matters more than most: "this operator has filed nothing" is an
 * accusation, and rendering it because a node did not answer would make it
 * against an operator who may have filed everything. The three outcomes stay
 * distinct all the way to the render.
 *
 * A vehicle that has never been fractionalised still gets asked. An operator
 * can publish against title before shares exist — a valuation, an independent
 * report — and skipping the query for those would hide exactly the filings a
 * reader wants before they subscribe.
 */
export function useCovenants(assetId: string): [Load<Covenant[]>, () => void] {
  return useRead(() => fetchCovenants(assetId), [assetId]);
}

/* ------------------------------------------------------------- one holding */

export interface Holding {
  /** Shares held, in base units. Null when there is no shareholding to hold. */
  balance: string | null;
  /** Total shares in issue — the denominator of every percentage. */
  supply: string | null;
  /** What may be taken out of the vault right now, from Query/Entitlement. */
  owed: Coin | null;
  /** Set when the entitlement query itself failed, so a zero is not implied. */
  owedUnknown: boolean;
  /**
   * The chain's position record: what was paid, and what is accruing.
   *
   * Null means the chain answered that there is no such position, which is a
   * finding — nobody ever recorded a cost — and NOT the absence of an answer.
   * `positionUnknown` carries that separately for the same reason `owedUnknown`
   * exists: rendering a failed read as "cost not recorded" would tell somebody
   * the chain has forgotten what they paid when it has not been asked.
   */
  position: Position | null;
  positionUnknown: boolean;
}

/**
 * What one address owns of one vehicle.
 *
 * The entitlement is queried rather than derived, and its failure is carried
 * rather than flattened to zero. It is not computable from a balance: it
 * depends on what has been paid into the vault, what this holder has already
 * taken, and what they held over each stretch between distributions. Showing a
 * holder zero because a query timed out would be telling them the truth about
 * the network and a lie about their money.
 */
export function useHolding(
  assetId: string,
  fractionDenom: string,
  address: string,
): [Load<Holding>, () => void] {
  const [state, setState] = useState<Load<Holding>>({ loading: true });
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let live = true;

    if (!fractionDenom) {
      setState({
        loading: false,
        outcome: {
          ok: true,
          value: {
            balance: null, supply: null, owed: null, owedUnknown: false,
            position: null, positionUnknown: false,
          },
          height: 0,
        },
      });
      return;
    }

    setState({ loading: true });
    Promise.all([
      supplyOf(fractionDenom),
      address ? balanceOf(address, fractionDenom) : Promise.resolve(null),
      address ? fetchEntitlement(assetId, address) : Promise.resolve(null),
      address ? fetchPosition(assetId, address) : Promise.resolve(null),
    ]).then(([supply, balance, owed, position]) => {
      if (!live) return;
      setState({
        loading: false,
        outcome: {
          ok: true,
          value: {
            balance,
            supply,
            owed: owed && owed.ok ? owed.value : null,
            // Only a real failure counts. No address means nothing was asked,
            // which is not the same as an unanswered question.
            owedUnknown: owed !== null && !owed.ok && owed.reason === 'unreachable',
            position: position && position.ok ? position.value : null,
            // A refusal counts here as well as a timeout, and deliberately: the
            // position is read out of the raw store, so a node that has stopped
            // serving that path answers 'refused' rather than failing to
            // answer. Both mean "we do not know what this cost", and neither
            // means "nothing was paid".
            positionUnknown: position !== null && !position.ok,
          },
          height: owed && owed.ok ? owed.height : 0,
        },
      });
    });

    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [assetId, fractionDenom, address, nonce]);

  return [state, useCallback(() => setNonce((n) => n + 1), [])];
}

/* ------------------------------------------------------ everything I own */

export function useBalances(address: string): [Load<Coin[]>, () => void] {
  const [state, setState] = useState<Load<Coin[]>>({ loading: true });
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let live = true;
    if (!address) {
      setState({ loading: false, outcome: { ok: true, value: [], height: 0 } });
      return;
    }
    setState({ loading: true });
    fetchBalances(address).then((coins) => {
      if (!live) return;
      setState({
        loading: false,
        outcome: coins === null
          ? { ok: false, reason: 'unreachable', detail: 'balances' }
          : { ok: true, value: coins, height: 0 },
      });
    });
    return () => { live = false; };
  }, [address, nonce]);

  return [state, useCallback(() => setNonce((n) => n + 1), [])];
}

/**
 * The register of holders of one tokenisation.
 *
 * Read from the bank rather than from x/tokenisation, which has no holders
 * query: the shareholding is an ordinary bank denomination, so who owns it is
 * bank's question. See chain.ts holders().
 *
 * An unfractionalised asset has no denomination and therefore no register, and
 * that is an answer rather than a gap: nothing has been issued, so nobody holds
 * anything. It resolves immediately without asking the chain.
 */
export function useHolders(denom: string): Load<{ rows: Holder[]; more: boolean }> {
  const [state, setState] = useState<Load<{ rows: Holder[]; more: boolean }>>({ loading: true });

  useEffect(() => {
    let live = true;
    if (!denom) {
      setState({ loading: false, outcome: { ok: true, value: { rows: [], more: false }, height: 0 } });
      return;
    }
    setState({ loading: true });
    holders(denom).then((outcome) => { if (live) setState({ loading: false, outcome }); });
    return () => { live = false; };
  }, [denom]);

  return state;
}
