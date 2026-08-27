/**
 * The message the chain will actually receive, field by field.
 *
 * The same idea as Yamale Pay's instruction table, applied to the three
 * messages a shareholder signs. The claim it makes is narrow and checkable:
 * here is every field of the protobuf message, here is the value this action
 * would put in it, and here is where that value ends up.
 *
 * The interesting rows are the ones that are *not* fields. A dispute bond is
 * nowhere in `MsgDisputeSale` — the keeper computes it from the collection and
 * takes it from the challenger's account in the same block — and a redemption's
 * payout is nowhere in `MsgRedeem`. Both are consequences of signing that the
 * signed bytes do not mention, which makes them exactly the things somebody
 * would otherwise discover afterwards. They are marked `derived` and shown
 * beside the fields rather than in a footnote.
 *
 * Pure, and tested, because a table that misdescribed what was about to be
 * signed would be worse than no table at all.
 */

export type Carried =
  /** Literally in the signed bytes, in this field, with this value. */
  | 'ledger'
  /** Not in the message. The keeper computes it and it happens anyway. */
  | 'derived';

export interface PlanRow {
  /** The protobuf field name, or a bracketed name for a derived consequence. */
  field: string;
  /** The value, as a string. Amounts stay in base units here; the view formats. */
  value: string;
  /** Set when the value is an amount, so the view knows to convert it. */
  denom?: string;
  carried: Carried;
  /** Catalogue key for a sentence explaining a derived row. */
  noteKey?: string;
}

export interface Plan {
  typeUrl: string;
  rows: PlanRow[];
}

export type Draft =
  | {
    kind: 'claim';
    holder: string;
    assetId: string;
    /** What the chain will pay out, from Query/Entitlement. */
    owed: string;
    owedDenom: string;
  }
  | {
    kind: 'redeem';
    holder: string;
    assetId: string;
    /** Shares to burn, in base units — which for shares is a plain count. */
    amount: string;
    shareDenom: string;
    /** What the burn will pay, computed the way the keeper computes it. */
    payout: string;
    payoutDenom: string;
  }
  | {
    kind: 'dispute';
    challenger: string;
    assetId: string;
    reason: string;
    /** The stake, computed the way the keeper computes it. */
    bond: string;
    bondDenom: string;
  };

/**
 * Type URLs, written out rather than built from a template.
 *
 * The prefix is `blockchain.`, not `yamale.blockchain.` — the proto package
 * declared in every one of this module's .proto files. A template that
 * assembled these would make the wrong one just as easy to produce as the right
 * one, and CosmJS's failure for an unregistered type URL names the URL it was
 * given, which reads as a missing encoder rather than as a typo.
 */
export const TYPE_URLS = {
  claim: '/blockchain.tokenisation.v1.MsgClaim',
  redeem: '/blockchain.tokenisation.v1.MsgRedeem',
  dispute: '/blockchain.tokenisation.v1.MsgDisputeSale',
} as const;

export function messagePlan(draft: Draft): Plan {
  switch (draft.kind) {
    case 'claim':
      return {
        typeUrl: TYPE_URLS.claim,
        rows: [
          { field: 'holder', value: draft.holder, carried: 'ledger' },
          { field: 'asset_id', value: draft.assetId, carried: 'ledger' },
          {
            field: 'paid',
            value: draft.owed,
            denom: draft.owedDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.paidNote',
          },
        ],
      };

    case 'redeem':
      return {
        typeUrl: TYPE_URLS.redeem,
        rows: [
          { field: 'holder', value: draft.holder, carried: 'ledger' },
          { field: 'asset_id', value: draft.assetId, carried: 'ledger' },
          { field: 'amount', value: draft.amount, denom: draft.shareDenom, carried: 'ledger' },
          {
            field: 'burned',
            value: draft.amount,
            denom: draft.shareDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.burnNote',
          },
          {
            field: 'paid',
            value: draft.payout,
            denom: draft.payoutDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.payoutNote',
          },
        ],
      };

    case 'dispute':
      return {
        typeUrl: TYPE_URLS.dispute,
        rows: [
          { field: 'challenger', value: draft.challenger, carried: 'ledger' },
          { field: 'asset_id', value: draft.assetId, carried: 'ledger' },
          // Public, permanently, and read by whoever resolves the dispute.
          // Marked `ledger` rather than left to be assumed: somebody typing a
          // reason should know it is being published, not filed.
          { field: 'reason', value: draft.reason, carried: 'ledger' },
          {
            field: 'bond',
            value: draft.bond,
            denom: draft.bondDenom,
            carried: 'derived',
            noteKey: 'rwa.msg.bondNote',
          },
        ],
      };
  }
}

/** The value object the signer encodes, matching the generated field names. */
export function messageValue(draft: Draft): Record<string, string> {
  switch (draft.kind) {
    case 'claim':
      return { holder: draft.holder, assetId: draft.assetId };
    case 'redeem':
      return { holder: draft.holder, assetId: draft.assetId, amount: draft.amount };
    case 'dispute':
      return { challenger: draft.challenger, assetId: draft.assetId, reason: draft.reason };
  }
}
