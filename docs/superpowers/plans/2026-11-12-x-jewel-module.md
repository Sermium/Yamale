# x/jewel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A chain module, `x/jewel`, that registers jewels as `x/nft` tokens under admitted jewelers, moves them only by offer and acceptance, carries benefits that travel with the jewel, and is proven end to end on a local devnet.

**Architecture:** `x/jewel` keeps the rules and records; `x/nft` holds the tokens (one class per jeweler, one token per jewel) and is the only source of ownership. `x/circuit` disables x/nft's bare send at the message router, and `x/jewel` refuses to admit a jeweler or register a jewel while that send is still open. The module follows `x/herd`'s shape: collections, explicit counters, rebuilt indexes, no Begin/EndBlocker.

**Tech Stack:** Go 1.25, Cosmos SDK 0.53.6, `cosmossdk.io/collections`, `cosmossdk.io/x/nft` v0.1.0, `cosmossdk.io/x/circuit` v0.1.1, gogoproto via `buf`, bash for the devnet seeder.

**Spec:** `docs/superpowers/specs/2026-11-12-jewel-loyalty-design.md`. This plan covers its delivery steps 1 and 2. The app and `loyalty-api` (step 3) and the devnet-2 upgrade (step 4) are separate plans.

## Global Constraints

- Offers lapse after **14 days** (`offer_ttl` 1,209,600 s).
- Benefit vouchers live at most **5 minutes** (`voucher_max_ttl` 300 s).
- A reassignment stays open to the owner's veto for **30 days** (`reassignment_window` 2,592,000 s).
- x/nft class id `jewel-<jeweler id>`, token id `jewel-<jewel id>`.
- No id 0 is ever issued; counters are exported in genesis, never derived.
- Every collections prefix must not be a byte-prefix of another (`jewel` is a prefix of `jeweler`: hence `maker` and `piece`).
- No Begin or EndBlocker. Expiry is checked when someone acts.
- The module is compiled out of the `settlement` build profile, like `x/herd`.
- Commit messages carry **no Co-Authored-By trailer**.
- Windows: put Go on PATH in every shell (`export PATH="/c/Program Files/Go/bin:$PATH"`). `make` is not installed: run the target bodies quoted in this plan. Application control intermittently blocks Go-built binaries with `fork/exec … stratégie de contrôle`. That is not a test failure: rerun, or build the binary into the session scratchpad and run it from there.

---

## File Structure

```
proto/blockchain/jewel/module/v1/module.proto   module config (authority)
proto/blockchain/jewel/v1/params.proto          Params
proto/blockchain/jewel/v1/jewel.proto           Jeweler, Jewel, Amendment, Offer, Benefit, Reassignment, Voucher
proto/blockchain/jewel/v1/genesis.proto         GenesisState, UsedNonce
proto/blockchain/jewel/v1/tx.proto              Msg service
proto/blockchain/jewel/v1/query.proto           Query service

x/jewel/types/keys.go              names, prefixes, ClassID/TokenID
x/jewel/types/errors.go            registered errors
x/jewel/types/codec.go             interface registration
x/jewel/types/expected_keepers.go  GroupKeeper, NFTKeeper, CircuitKeeper
x/jewel/types/params.go            DefaultParams, Validate
x/jewel/types/inscription.go       NormaliseInscription, ValidateRecord
x/jewel/types/signing.go           VoucherSignBytes, ClaimSignBytes, VerifySecp256k1
x/jewel/types/genesis.go           DefaultGenesis, Validate
x/jewel/keeper/keeper.go           Keeper, NewKeeper, shared helpers
x/jewel/keeper/msg_server.go       msgServer, UpdateParams
x/jewel/keeper/msg_server_jeweler.go   Admit, Suspend, Reinstate, SetStaff
x/jewel/keeper/msg_server_jewel.go     Register, Amend
x/jewel/keeper/msg_server_offer.go     Offer, Cancel, Accept, Claim, ReportStolen, ClearStolen
x/jewel/keeper/msg_server_benefit.go   Attach, Redeem
x/jewel/keeper/msg_server_reassign.go  Propose, Veto, Complete
x/jewel/keeper/query.go            query server
x/jewel/keeper/genesis.go          InitGenesis, ExportGenesis
x/jewel/module/module.go           AppModule
x/jewel/module/depinject.go        ProvideModule
x/jewel/module/autocli.go          CLI descriptors
x/jewel/keeper/*_test.go, x/jewel/types/*_test.go

app/profile_jewel_on.go, app/profile_jewel_off.go   build-profile wiring
app/jewel_circuit_test.go          the router refuses a bare NFT send, also inside authz

tools/jewelsign/main.go            signs claims and vouchers for the seeder
scripts/devnet/seed-jewel.sh       end-to-end run on the local devnet
```

---

### Task 1: Protos, generated code and pure types

**Files:**
- Create: the six `.proto` files above
- Create: `x/jewel/types/keys.go`, `errors.go`, `codec.go`, `expected_keepers.go`, `params.go`, `inscription.go`, `signing.go`, `genesis.go` (the last with `DefaultGenesis` only; `Validate` comes in Task 8)
- Test: `x/jewel/types/types_test.go`

**Interfaces:**
- Produces: generated types `Params`, `Jeweler`, `Jewel`, `Amendment`, `Offer`, `Benefit`, `Reassignment`, `Voucher`, `GenesisState`, `UsedNonce`, all `Msg*` and `Query*` types, `MsgServer`, `QueryServer`, `UnimplementedMsgServer`; constants `JEWELER_ACTIVE`, `JEWELER_SUSPENDED`; `ClassID(uint64) string`, `TokenID(uint64) string`, `NormaliseInscription(string) (string, error)`, `ValidateRecord(hash, uri string) error`, `VoucherSignBytes(chainID string, v Voucher) []byte`, `ClaimSignBytes(chainID string, jewelID uint64, claimer string) []byte`, `VerifySecp256k1(pubKey, msg, sig []byte) (sdk.AccAddress, bool)`, `DefaultParams() Params`, `(Params) Validate() error`, `DefaultGenesis() *GenesisState`.

- [ ] **Step 1: Write the module config proto**

`proto/blockchain/jewel/module/v1/module.proto`:

```proto
syntax = "proto3";
package blockchain.jewel.module.v1;

import "cosmos/app/v1alpha1/module.proto";

option go_package = "yamale/blockchain/x/jewel/types";

// Module is the config object for the jewel register.
message Module {
  option (cosmos.app.v1alpha1.module) = {go_import: "yamale/blockchain/x/jewel"};

  // authority defines the custom module authority.
  //
  // If not set, defaults to the governance module. Governance appoints the
  // office, and the office admits jewelers: an office that could appoint
  // itself could admit a jeweler that mints forgeries under a real name.
  string authority = 1;
}
```

- [ ] **Step 2: Write params.proto and jewel.proto**

`proto/blockchain/jewel/v1/params.proto`:

```proto
syntax = "proto3";
package blockchain.jewel.v1;

import "amino/amino.proto";
import "cosmos_proto/cosmos.proto";
import "gogoproto/gogo.proto";

option go_package = "yamale/blockchain/x/jewel/types";

// Params are the register's settings. Governance sets them.
message Params {
  option (amino.name) = "blockchain/x/jewel/Params";
  option (gogoproto.equal) = true;

  // office is the x/group policy account that admits and suspends jewelers
  // and proposes reassignments. Empty until governance appoints one, and no
  // jeweler can be admitted before then.
  string office = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  // offer_ttl is how long an offer stays open, in seconds.
  int64 offer_ttl = 2;
  // voucher_max_ttl is the longest a benefit voucher may live, in seconds.
  int64 voucher_max_ttl = 3;
  // reassignment_window is how long a proposed reassignment stays open to
  // the owner's veto, in seconds.
  int64 reassignment_window = 4;
}
```

`proto/blockchain/jewel/v1/jewel.proto`:

```proto
syntax = "proto3";
package blockchain.jewel.v1;

import "cosmos_proto/cosmos.proto";
import "gogoproto/gogo.proto";

option go_package = "yamale/blockchain/x/jewel/types";

// JewelerStatus says whether a jeweler may still act.
enum JewelerStatus {
  option (gogoproto.goproto_enum_prefix) = false;

  // Never stored.
  JEWELER_STATUS_UNSPECIFIED = 0;
  // May register jewels, sell stock and honour benefits.
  JEWELER_ACTIVE = 1;
  // Its jewels and their histories stay. It registers nothing, and its
  // benefits are no longer honoured.
  JEWELER_SUSPENDED = 2;
}

// Jeweler is a business the office has admitted.
message Jeweler {
  uint64 id = 1;
  string name = 2;
  // business holds the jeweler's stock and manages its staff.
  string business = 3 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  // staff act for the jeweler. A person works for one jeweler at a time.
  repeated string staff = 4 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  JewelerStatus status = 5;
}

// Amendment is a correction appended to a jewel's record. The original and
// every correction stay visible.
message Amendment {
  string record_hash = 1;
  string record_uri = 2;
  string note = 3;
  string staff = 4 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  // time is the block time, in unix seconds.
  int64 time = 5;
}

// Jewel is one registered piece. Its token is x/nft class jewel-<jeweler_id>,
// id jewel-<id>. Ownership is x/nft's and is not copied here.
message Jewel {
  uint64 id = 1;
  uint64 jeweler_id = 2;
  // inscription is the normalised laser inscription: upper case, separators
  // removed.
  string inscription = 3;
  // record_hash is the SHA-256, in lower-case hex, of the record file
  // (photos, materials, origin) as first registered.
  string record_hash = 4;
  string record_uri = 5;
  // registered_at is the block time of registration, in unix seconds.
  int64 registered_at = 6;
  // owner_count counts owners since the jeweler sold it. Zero in stock.
  uint32 owner_count = 7;
  bool stolen = 8;
  repeated Amendment amendments = 9 [(gogoproto.nullable) = false];
}

// Offer proposes handing a jewel to someone. Exactly one of to and claim_key
// is set.
message Offer {
  uint64 jewel_id = 1;
  // from is the owner when the offer was made.
  string from = 2 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  // to is the recipient, when they already have an account.
  string to = 3 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  // claim_key is the lower-case hex compressed secp256k1 public key of a
  // one-time key sent by email, when the recipient has no account yet. The
  // claimer proves it by signing their own address with it.
  string claim_key = 4;
  // expires_at is in unix seconds.
  int64 expires_at = 5;
}

// Benefit is a service a jeweler attaches to a jewel. It passes with the
// jewel to each new owner.
message Benefit {
  uint64 id = 1;
  uint64 jewel_id = 2;
  uint64 jeweler_id = 3;
  // kind is the service as the jeweler words it, e.g. "cleaning".
  string kind = 4;
  // allowance is how many uses fit in one period.
  uint32 allowance = 5;
  // period is the rolling window, in seconds, the allowance applies to. Zero
  // means the allowance covers the benefit's whole life.
  int64 period = 6;
  // expires_at is in unix seconds. Zero means it does not expire.
  int64 expires_at = 7;
  // uses are the block times, in unix seconds, of every use.
  repeated int64 uses = 8;
}

// Reassignment is the office's proposal to move a jewel whose owner has lost
// their key. The owner may veto it until window_ends.
message Reassignment {
  uint64 jewel_id = 1;
  // from is the owner when it was proposed. A later change of owner voids it.
  string from = 2 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  string to = 3 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  string reason = 4;
  // window_ends is in unix seconds.
  int64 window_ends = 5;
}

// Voucher is what the owner's app signs to consent to one use of a benefit.
message Voucher {
  uint64 benefit_id = 1;
  uint64 jewel_id = 2;
  // nonce is single-use, at most 64 characters.
  string nonce = 3;
  // expires_at is in unix seconds.
  int64 expires_at = 4;
}
```

- [ ] **Step 3: Write genesis.proto**

`proto/blockchain/jewel/v1/genesis.proto`:

```proto
syntax = "proto3";
package blockchain.jewel.v1;

import "blockchain/jewel/v1/jewel.proto";
import "blockchain/jewel/v1/params.proto";
import "gogoproto/gogo.proto";

option go_package = "yamale/blockchain/x/jewel/types";

// UsedNonce is a voucher already spent.
message UsedNonce {
  uint64 benefit_id = 1;
  string nonce = 2;
}

// GenesisState carries the whole register. Indexes are rebuilt from it. The
// counters are exported rather than derived, so an import keeps its numbering.
message GenesisState {
  Params params = 1 [(gogoproto.nullable) = false];
  repeated Jeweler jewelers = 2 [(gogoproto.nullable) = false];
  repeated Jewel jewels = 3 [(gogoproto.nullable) = false];
  repeated Offer offers = 4 [(gogoproto.nullable) = false];
  repeated Benefit benefits = 5 [(gogoproto.nullable) = false];
  repeated UsedNonce used_nonces = 6 [(gogoproto.nullable) = false];
  repeated Reassignment reassignments = 7 [(gogoproto.nullable) = false];
  uint64 next_jeweler_id = 8;
  uint64 next_jewel_id = 9;
  uint64 next_benefit_id = 10;
}
```

- [ ] **Step 4: Write tx.proto**

`proto/blockchain/jewel/v1/tx.proto`:

```proto
syntax = "proto3";
package blockchain.jewel.v1;

import "amino/amino.proto";
import "blockchain/jewel/v1/jewel.proto";
import "blockchain/jewel/v1/params.proto";
import "cosmos/msg/v1/msg.proto";
import "cosmos_proto/cosmos.proto";
import "gogoproto/gogo.proto";

option go_package = "yamale/blockchain/x/jewel/types";

// Msg is the write side of the jewel register.
service Msg {
  option (cosmos.msg.v1.service) = true;

  // UpdateParams sets the settings and appoints the office. Governance only;
  // the office must be a group account.
  rpc UpdateParams(MsgUpdateParams) returns (MsgUpdateParamsResponse);
  // AdmitJeweler admits a business and opens its collection. Office only.
  rpc AdmitJeweler(MsgAdmitJeweler) returns (MsgAdmitJewelerResponse);
  // SuspendJeweler stops a jeweler acting. Office only.
  rpc SuspendJeweler(MsgSuspendJeweler) returns (MsgSuspendJewelerResponse);
  // ReinstateJeweler lets a suspended jeweler act again. Office only.
  rpc ReinstateJeweler(MsgReinstateJeweler) returns (MsgReinstateJewelerResponse);
  // SetStaff adds and removes the people who act for a jeweler.
  rpc SetStaff(MsgSetStaff) returns (MsgSetStaffResponse);
  // RegisterJewel mints a jewel into its jeweler's stock.
  rpc RegisterJewel(MsgRegisterJewel) returns (MsgRegisterJewelResponse);
  // AmendJewel appends a correction to a jewel's record.
  rpc AmendJewel(MsgAmendJewel) returns (MsgAmendJewelResponse);
  // OfferJewel opens an offer to an account or a claim key.
  rpc OfferJewel(MsgOfferJewel) returns (MsgOfferJewelResponse);
  // CancelOffer closes an open offer.
  rpc CancelOffer(MsgCancelOffer) returns (MsgCancelOfferResponse);
  // AcceptOffer takes a jewel offered to the signer's account.
  rpc AcceptOffer(MsgAcceptOffer) returns (MsgAcceptOfferResponse);
  // ClaimOffer takes a jewel offered to a claim key.
  rpc ClaimOffer(MsgClaimOffer) returns (MsgClaimOfferResponse);
  // ReportStolen flags a jewel and closes its offer.
  rpc ReportStolen(MsgReportStolen) returns (MsgReportStolenResponse);
  // ClearStolen lifts the flag.
  rpc ClearStolen(MsgClearStolen) returns (MsgClearStolenResponse);
  // AttachBenefit adds a benefit to a jewel.
  rpc AttachBenefit(MsgAttachBenefit) returns (MsgAttachBenefitResponse);
  // RedeemBenefit records one use, with the owner's signed voucher.
  rpc RedeemBenefit(MsgRedeemBenefit) returns (MsgRedeemBenefitResponse);
  // ProposeReassignment opens the public window. Office only.
  rpc ProposeReassignment(MsgProposeReassignment) returns (MsgProposeReassignmentResponse);
  // VetoReassignment closes it. The current owner only.
  rpc VetoReassignment(MsgVetoReassignment) returns (MsgVetoReassignmentResponse);
  // CompleteReassignment moves the jewel once the window has passed. Anyone.
  rpc CompleteReassignment(MsgCompleteReassignment) returns (MsgCompleteReassignmentResponse);
}

message MsgUpdateParams {
  option (cosmos.msg.v1.signer) = "authority";
  option (amino.name) = "blockchain/x/jewel/MsgUpdateParams";
  string authority = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  Params params = 2 [(gogoproto.nullable) = false, (amino.dont_omitempty) = true];
}
message MsgUpdateParamsResponse {}

message MsgAdmitJeweler {
  option (cosmos.msg.v1.signer) = "office";
  option (amino.name) = "blockchain/x/jewel/MsgAdmitJeweler";
  string office = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  string name = 2;
  string business = 3 [(cosmos_proto.scalar) = "cosmos.AddressString"];
}
message MsgAdmitJewelerResponse {
  uint64 id = 1;
}

message MsgSuspendJeweler {
  option (cosmos.msg.v1.signer) = "office";
  option (amino.name) = "blockchain/x/jewel/MsgSuspendJeweler";
  string office = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jeweler_id = 2;
  string reason = 3;
}
message MsgSuspendJewelerResponse {}

message MsgReinstateJeweler {
  option (cosmos.msg.v1.signer) = "office";
  option (amino.name) = "blockchain/x/jewel/MsgReinstateJeweler";
  string office = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jeweler_id = 2;
}
message MsgReinstateJewelerResponse {}

message MsgSetStaff {
  option (cosmos.msg.v1.signer) = "business";
  option (amino.name) = "blockchain/x/jewel/MsgSetStaff";
  string business = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jeweler_id = 2;
  repeated string add = 3 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  repeated string remove = 4 [(cosmos_proto.scalar) = "cosmos.AddressString"];
}
message MsgSetStaffResponse {}

message MsgRegisterJewel {
  option (cosmos.msg.v1.signer) = "staff";
  option (amino.name) = "blockchain/x/jewel/MsgRegisterJewel";
  string staff = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  string inscription = 2;
  string record_hash = 3;
  string record_uri = 4;
}
message MsgRegisterJewelResponse {
  uint64 id = 1;
}

message MsgAmendJewel {
  option (cosmos.msg.v1.signer) = "staff";
  option (amino.name) = "blockchain/x/jewel/MsgAmendJewel";
  string staff = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
  string record_hash = 3;
  string record_uri = 4;
  string note = 5;
}
message MsgAmendJewelResponse {}

message MsgOfferJewel {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "blockchain/x/jewel/MsgOfferJewel";
  // owner is the jewel's owner, or a staff member while it is stock.
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
  string to = 3 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  string claim_key = 4;
}
message MsgOfferJewelResponse {
  int64 expires_at = 1;
}

message MsgCancelOffer {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "blockchain/x/jewel/MsgCancelOffer";
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
}
message MsgCancelOfferResponse {}

message MsgAcceptOffer {
  option (cosmos.msg.v1.signer) = "recipient";
  option (amino.name) = "blockchain/x/jewel/MsgAcceptOffer";
  string recipient = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
}
message MsgAcceptOfferResponse {}

message MsgClaimOffer {
  option (cosmos.msg.v1.signer) = "claimer";
  option (amino.name) = "blockchain/x/jewel/MsgClaimOffer";
  string claimer = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
  // claim_pub_key is the compressed secp256k1 public key of the claim key.
  bytes claim_pub_key = 3;
  // claim_signature signs ClaimSignBytes(chain id, jewel id, claimer).
  bytes claim_signature = 4;
}
message MsgClaimOfferResponse {}

message MsgReportStolen {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "blockchain/x/jewel/MsgReportStolen";
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
}
message MsgReportStolenResponse {}

message MsgClearStolen {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "blockchain/x/jewel/MsgClearStolen";
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
}
message MsgClearStolenResponse {}

message MsgAttachBenefit {
  option (cosmos.msg.v1.signer) = "staff";
  option (amino.name) = "blockchain/x/jewel/MsgAttachBenefit";
  string staff = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
  string kind = 3;
  uint32 allowance = 4;
  int64 period = 5;
  int64 expires_at = 6;
}
message MsgAttachBenefitResponse {
  uint64 id = 1;
}

message MsgRedeemBenefit {
  option (cosmos.msg.v1.signer) = "staff";
  option (amino.name) = "blockchain/x/jewel/MsgRedeemBenefit";
  string staff = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  Voucher voucher = 2 [(gogoproto.nullable) = false];
  // owner_pub_key is the compressed secp256k1 public key of the owner.
  bytes owner_pub_key = 3;
  // owner_signature signs VoucherSignBytes(chain id, voucher).
  bytes owner_signature = 4;
}
message MsgRedeemBenefitResponse {}

message MsgProposeReassignment {
  option (cosmos.msg.v1.signer) = "office";
  option (amino.name) = "blockchain/x/jewel/MsgProposeReassignment";
  string office = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
  string to = 3 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  string reason = 4;
}
message MsgProposeReassignmentResponse {
  int64 window_ends = 1;
}

message MsgVetoReassignment {
  option (cosmos.msg.v1.signer) = "owner";
  option (amino.name) = "blockchain/x/jewel/MsgVetoReassignment";
  string owner = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
}
message MsgVetoReassignmentResponse {}

message MsgCompleteReassignment {
  option (cosmos.msg.v1.signer) = "creator";
  option (amino.name) = "blockchain/x/jewel/MsgCompleteReassignment";
  string creator = 1 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  uint64 jewel_id = 2;
}
message MsgCompleteReassignmentResponse {}
```

- [ ] **Step 5: Write query.proto**

`proto/blockchain/jewel/v1/query.proto`:

```proto
syntax = "proto3";
package blockchain.jewel.v1;

import "blockchain/jewel/v1/jewel.proto";
import "blockchain/jewel/v1/params.proto";
import "cosmos/base/query/v1beta1/pagination.proto";
import "gogoproto/gogo.proto";
import "google/api/annotations.proto";

option go_package = "yamale/blockchain/x/jewel/types";

// Query is the read side, and what the public verification page calls.
service Query {
  // Params returns the settings and the office.
  rpc Params(QueryParamsRequest) returns (QueryParamsResponse) {
    option (google.api.http).get = "/yamale/blockchain/jewel/v1/params";
  }
  // Jeweler returns one jeweler.
  rpc Jeweler(QueryJewelerRequest) returns (QueryJewelerResponse) {
    option (google.api.http).get = "/yamale/blockchain/jewel/v1/jeweler/{id}";
  }
  // Jewelers lists the jewelers, suspended ones included.
  rpc Jewelers(QueryJewelersRequest) returns (QueryJewelersResponse) {
    option (google.api.http).get = "/yamale/blockchain/jewel/v1/jewelers";
  }
  // Jewel returns one jewel with its owner, open offer, pending reassignment
  // and benefits.
  rpc Jewel(QueryJewelRequest) returns (QueryJewelResponse) {
    option (google.api.http).get = "/yamale/blockchain/jewel/v1/jewel/{id}";
  }
  // JewelsByInscription finds every jewel carrying an inscription, whichever
  // jeweler registered it. At most 50.
  rpc JewelsByInscription(QueryJewelsByInscriptionRequest) returns (QueryJewelsByInscriptionResponse) {
    option (google.api.http).get = "/yamale/blockchain/jewel/v1/inscription/{inscription}";
  }
}

message QueryParamsRequest {}
message QueryParamsResponse {
  Params params = 1 [(gogoproto.nullable) = false];
}

message QueryJewelerRequest {
  uint64 id = 1;
}
message QueryJewelerResponse {
  Jeweler jeweler = 1 [(gogoproto.nullable) = false];
}

message QueryJewelersRequest {
  cosmos.base.query.v1beta1.PageRequest pagination = 1;
}
message QueryJewelersResponse {
  repeated Jeweler jewelers = 1 [(gogoproto.nullable) = false];
  cosmos.base.query.v1beta1.PageResponse pagination = 2;
}

message QueryJewelRequest {
  uint64 id = 1;
}
message QueryJewelResponse {
  Jewel jewel = 1 [(gogoproto.nullable) = false];
  string owner = 2;
  // offer is set while an offer is open, expired or not.
  Offer offer = 3;
  // reassignment is set while one is pending.
  Reassignment reassignment = 4;
  repeated Benefit benefits = 5 [(gogoproto.nullable) = false];
}

message QueryJewelsByInscriptionRequest {
  string inscription = 1;
}
message QueryJewelsByInscriptionResponse {
  repeated Jewel jewels = 1 [(gogoproto.nullable) = false];
}
```

- [ ] **Step 6: Lint and generate**

Run (the bodies of `make proto-lint` and `make proto-gen`):

```bash
export PATH="/c/Program Files/Go/bin:$PATH"
cd proto && go tool github.com/bufbuild/buf/cmd/buf lint && cd ..
rm -rf yamale
go tool github.com/bufbuild/buf/cmd/buf generate --template proto/buf.gen.gogo.yaml
cp -r yamale/blockchain/x/. x/
rm -rf yamale
ls x/jewel/types
```

Expected: no lint output. `x/jewel/types` holds `genesis.pb.go jewel.pb.go module.pb.go params.pb.go query.pb.go query.pb.gw.go tx.pb.go`. If `buf lint` is run from the repo root and fails to find the module, run it from `proto/` as shown. If `go tool` is blocked by application control, copy the built `buf.exe` into the session scratchpad and run it from there.

- [ ] **Step 7: Write the failing test for the pure types**

`x/jewel/types/types_test.go`:

```go
package types_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// An inscription is read off metal through a loupe and typed by a person, so
// the same code arrives with spaces, dashes and lower case. Normalising is
// what makes "one inscription, one jewel" mean anything.
func TestInscriptionsNormaliseToOneForm(t *testing.T) {
	for _, in := range []string{"gia 2141-438-1", "GIA2141.438.1", " gia/2141_438 1 "} {
		got, err := types.NormaliseInscription(in)
		require.NoError(t, err, in)
		require.Equal(t, "GIA21414381", got, in)
	}
}

func TestInscriptionsThatCannotBeEngravedAreRefused(t *testing.T) {
	for _, in := range []string{"", "AB", "MAISON-É-2024", "AB#1234", "A123456789012345678901234567890123456789X"} {
		_, err := types.NormaliseInscription(in)
		require.ErrorIs(t, err, types.ErrInvalidInscription, in)
	}
}

func TestARecordMustBeAFingerprintAndALocation(t *testing.T) {
	good := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	require.NoError(t, types.ValidateRecord(good, "https://loyalty.yamaletech.com/r/"+good))
	require.ErrorIs(t, types.ValidateRecord("9F86D081884C7D659A2FEAA0C55AD015A3BF4F1B2B0B822CD15D6C15B0F00A08", "x"), types.ErrInvalidRecord)
	require.ErrorIs(t, types.ValidateRecord(good[:63], "x"), types.ErrInvalidRecord)
	require.ErrorIs(t, types.ValidateRecord(good, ""), types.ErrInvalidRecord)
}

func TestTheDefaultsAreTheSpecsNumbers(t *testing.T) {
	p := types.DefaultParams()
	require.Equal(t, int64(14*24*3600), p.OfferTtl)
	require.Equal(t, int64(300), p.VoucherMaxTtl)
	require.Equal(t, int64(30*24*3600), p.ReassignmentWindow)
	require.Empty(t, p.Office)
	require.NoError(t, p.Validate())
}

func TestParamsThatDisableAProtectionAreRefused(t *testing.T) {
	for _, mutate := range []func(*types.Params){
		func(p *types.Params) { p.OfferTtl = 0 },
		func(p *types.Params) { p.VoucherMaxTtl = 0 },
		func(p *types.Params) { p.VoucherMaxTtl = 3601 },
		func(p *types.Params) { p.ReassignmentWindow = 0 },
	} {
		p := types.DefaultParams()
		mutate(&p)
		require.Error(t, p.Validate())
	}
}

// A signature by the right key over the right bytes verifies, and names the
// key's address. Anything else does not verify.
func TestSignaturesVerifyOnlyOverTheirOwnBytes(t *testing.T) {
	priv := secp256k1.GenPrivKey()
	msg := types.ClaimSignBytes("yamale-local-1", 7, "yml1claimer")
	sig, err := priv.Sign(msg)
	require.NoError(t, err)

	addr, ok := types.VerifySecp256k1(priv.PubKey().Bytes(), msg, sig)
	require.True(t, ok)
	require.Equal(t, sdk.AccAddress(priv.PubKey().Address()), addr)

	_, ok = types.VerifySecp256k1(priv.PubKey().Bytes(), types.ClaimSignBytes("yamale-local-1", 7, "yml1thief"), sig)
	require.False(t, ok, "a claim signed for one address must not verify for another")
	_, ok = types.VerifySecp256k1(priv.PubKey().Bytes(), types.ClaimSignBytes("other-chain", 7, "yml1claimer"), sig)
	require.False(t, ok, "a claim signed on one chain must not verify on another")
	_, ok = types.VerifySecp256k1([]byte{1, 2, 3}, msg, sig)
	require.False(t, ok)
}

func TestTokenIdsNameTheirJeweler(t *testing.T) {
	require.Equal(t, "jewel-3", types.ClassID(3))
	require.Equal(t, "jewel-41", types.TokenID(41))
}
```

- [ ] **Step 8: Run it to see it fail**

Run: `go test ./x/jewel/types/...`
Expected: FAIL to compile with `undefined: types.NormaliseInscription` (and the other helpers).

- [ ] **Step 9: Write keys.go, errors.go, codec.go, expected_keepers.go**

`x/jewel/types/keys.go`:

```go
package types

import (
	"fmt"

	"cosmossdk.io/collections"
)

const (
	// ModuleName defines the module name.
	ModuleName = "jewel"

	// StoreKey defines the primary module store key.
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency.
	GovModuleName = "gov"

	// NFTSendTypeURL is x/nft's own send. It skips the offer and acceptance,
	// the stolen flag and the owner count, so x/circuit must disable it, and
	// this module refuses to start registering while it is open.
	NFTSendTypeURL = "/cosmos.nft.v1beta1.MsgSend"

	// MaxSearchResults bounds JewelsByInscription. Two jewelers may share a
	// code; two hundred sharing one is not a search, it is a scan.
	MaxSearchResults = 50
)

// Prefixes. "jewel" is a byte-prefix of "jeweler", and collections refuses
// overlapping prefixes, so the records are "maker" and "piece".
var (
	ParamsKey       = collections.NewPrefix("p_jewel")
	JewelerKey      = collections.NewPrefix("maker")
	JewelKey        = collections.NewPrefix("piece")
	OfferKey        = collections.NewPrefix("offer")
	BenefitKey      = collections.NewPrefix("benefit")
	ReassignmentKey = collections.NewPrefix("reassign")
	UsedNonceKey    = collections.NewPrefix("used_nonce")

	// The indexes, each one there because the alternative is a scan.
	ByBusinessKey      = collections.NewPrefix("idx_business")
	ByStaffKey         = collections.NewPrefix("idx_staff")
	ByInscriptionKey   = collections.NewPrefix("idx_inscription")
	SearchKey          = collections.NewPrefix("idx_search")
	BenefitsByJewelKey = collections.NewPrefix("idx_benefits_by_piece")

	NextJewelerIDKey = collections.NewPrefix("next_maker_id")
	NextJewelIDKey   = collections.NewPrefix("next_piece_id")
	NextBenefitIDKey = collections.NewPrefix("next_benefit_id")
)

// ClassID is the x/nft class holding one jeweler's jewels.
func ClassID(jewelerID uint64) string { return fmt.Sprintf("jewel-%d", jewelerID) }

// TokenID is the x/nft id of one jewel.
func TokenID(jewelID uint64) string { return fmt.Sprintf("jewel-%d", jewelID) }
```

`x/jewel/types/errors.go`:

```go
package types

import "cosmossdk.io/errors"

// Errors are worded for the person at the counter or on the phone, not for
// the node operator: each says what is wrong in terms they can act on.
var (
	ErrNotOffice      = errors.Register(ModuleName, 1, "only the register's office may do this")
	ErrNoOffice       = errors.Register(ModuleName, 2, "no office is appointed yet; governance must appoint one before any jeweler is admitted")
	ErrOfficeNotGroup = errors.Register(ModuleName, 3, "the office must be a group account, so its decisions need several signatures")
	ErrInvalidAddress = errors.Register(ModuleName, 4, "that is not a valid account")
	ErrNoName         = errors.Register(ModuleName, 5, "a name is required")
	ErrBusinessTaken  = errors.Register(ModuleName, 6, "this business account already belongs to a jeweler")
	ErrNoJeweler      = errors.Register(ModuleName, 7, "no such jeweler")
	ErrJewelerSuspended = errors.Register(ModuleName, 8, "this jeweler is suspended and can no longer act")
	ErrNotBusiness    = errors.Register(ModuleName, 9, "only the jeweler's business account may manage its staff")
	ErrStaffTaken     = errors.Register(ModuleName, 10, "this person already works for another jeweler")
	ErrNotStaff       = errors.Register(ModuleName, 11, "you do not work for this jeweler")
	ErrBareSendOpen   = errors.Register(ModuleName, 12,
		"this chain still lets an NFT be sent without an offer and acceptance; disable /cosmos.nft.v1beta1.MsgSend in x/circuit first")
	ErrInvalidInscription = errors.Register(ModuleName, 13,
		"the inscription must be 4 to 40 letters and digits; spaces, dashes, dots, slashes and underscores are ignored")
	ErrInscriptionTaken = errors.Register(ModuleName, 14, "this jeweler has already registered a jewel with this inscription")
	ErrInvalidRecord    = errors.Register(ModuleName, 15,
		"the record needs its SHA-256 fingerprint in lower-case hex and a location of at most 512 characters")
	ErrNoJewel      = errors.Register(ModuleName, 16, "no such jewel")
	ErrNotOwner     = errors.Register(ModuleName, 17, "only the jewel's owner may do this")
	ErrOfferOpen    = errors.Register(ModuleName, 18, "this jewel already has an open offer; cancel it first")
	ErrNoOffer      = errors.Register(ModuleName, 19, "this jewel has no open offer")
	ErrOfferExpired = errors.Register(ModuleName, 20, "this offer has expired; ask the owner for a new one")
	ErrOfferTarget  = errors.Register(ModuleName, 21, "an offer names either an account or a claim key, exactly one")
	ErrSelfOffer    = errors.Register(ModuleName, 22, "the recipient already holds this jewel")
	ErrNotRecipient = errors.Register(ModuleName, 23, "this offer was made to someone else")
	ErrBadClaim     = errors.Register(ModuleName, 24, "the claim key or its signature does not match this offer")
	ErrStolen       = errors.Register(ModuleName, 25, "this jewel is reported stolen")
	ErrNotStolen    = errors.Register(ModuleName, 26, "this jewel is not reported stolen")
	ErrInvalidBenefit = errors.Register(ModuleName, 27,
		"a benefit needs a kind of at most 80 characters, an allowance of at least 1, a period of zero or more, and an expiry in the future or none")
	ErrNoBenefit        = errors.Register(ModuleName, 28, "no such benefit")
	ErrBenefitExpired   = errors.Register(ModuleName, 29, "this benefit has expired")
	ErrAllowanceUsed    = errors.Register(ModuleName, 30, "this benefit's allowance for the period is used up")
	ErrBadVoucher       = errors.Register(ModuleName, 31, "the voucher is not valid; ask the owner to show a fresh code")
	ErrVoucherUsed      = errors.Register(ModuleName, 32, "this voucher has already been used")
	ErrReassignmentOpen = errors.Register(ModuleName, 33, "a reassignment of this jewel is already pending")
	ErrNoReassignment   = errors.Register(ModuleName, 34, "no reassignment of this jewel is pending")
	ErrWindowOpen       = errors.Register(ModuleName, 35, "the owner's veto window has not closed yet")
	ErrReassignmentStale = errors.Register(ModuleName, 36, "the jewel changed owner after the reassignment was proposed")
	ErrNoReason         = errors.Register(ModuleName, 37, "a reason is required")
	ErrInvalidSigner    = errors.Register(ModuleName, 38, "only governance may change these settings")
)
```

`x/jewel/types/codec.go`:

```go
package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// RegisterInterfaces registers every message the register accepts.
func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
		&MsgAdmitJeweler{},
		&MsgSuspendJeweler{},
		&MsgReinstateJeweler{},
		&MsgSetStaff{},
		&MsgRegisterJewel{},
		&MsgAmendJewel{},
		&MsgOfferJewel{},
		&MsgCancelOffer{},
		&MsgAcceptOffer{},
		&MsgClaimOffer{},
		&MsgReportStolen{},
		&MsgClearStolen{},
		&MsgAttachBenefit{},
		&MsgRedeemBenefit{},
		&MsgProposeReassignment{},
		&MsgVetoReassignment{},
		&MsgCompleteReassignment{},
	)

	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
```

`x/jewel/types/expected_keepers.go`:

```go
package types

import (
	"context"

	"cosmossdk.io/x/nft"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/group"
)

// GroupKeeper answers one question, at appointment: is the office a group
// policy? An office that is one key is one bribe away from admitting a
// forger under a real jeweler's name.
type GroupKeeper interface {
	GroupPolicyInfo(
		ctx context.Context, req *group.QueryGroupPolicyInfoRequest,
	) (*group.QueryGroupPolicyInfoResponse, error)
}

// NFTKeeper is the slice of x/nft the register drives. x/nft holds the
// tokens and is the only record of who owns them.
type NFTKeeper interface {
	SaveClass(ctx context.Context, class nft.Class) error
	Mint(ctx context.Context, token nft.NFT, receiver sdk.AccAddress) error
	Update(ctx context.Context, token nft.NFT) error
	Transfer(ctx context.Context, classID, nftID string, receiver sdk.AccAddress) error
	GetOwner(ctx context.Context, classID, nftID string) sdk.AccAddress
}

// CircuitKeeper says whether a message type may still run.
type CircuitKeeper interface {
	IsAllowed(ctx context.Context, typeURL string) (bool, error)
}
```

- [ ] **Step 10: Write params.go, inscription.go, signing.go, genesis.go**

`x/jewel/types/params.go`:

```go
package types

import "fmt"

// DefaultParams are the spec's numbers. The office stays empty: appointing it
// is governance's act, not a default.
func DefaultParams() Params {
	return Params{
		OfferTtl:           14 * 24 * 3600,
		VoucherMaxTtl:      5 * 60,
		ReassignmentWindow: 30 * 24 * 3600,
	}
}

// Validate refuses settings that would switch a protection off.
func (p Params) Validate() error {
	if p.OfferTtl <= 0 {
		return fmt.Errorf("offer_ttl must be positive; at zero no offer could ever be accepted, got %d", p.OfferTtl)
	}
	if p.VoucherMaxTtl <= 0 || p.VoucherMaxTtl > 3600 {
		return fmt.Errorf("voucher_max_ttl must be between 1 and 3600 seconds; a long-lived voucher is a benefit someone can spend without the owner present, got %d", p.VoucherMaxTtl)
	}
	if p.ReassignmentWindow <= 0 {
		return fmt.Errorf("reassignment_window must be positive; at zero the office could move a jewel before its owner could object, got %d", p.ReassignmentWindow)
	}
	return nil
}
```

`x/jewel/types/inscription.go`:

```go
package types

import (
	"encoding/hex"
	"strings"
)

// NormaliseInscription reduces an inscription to the form it is indexed by:
// upper-case letters and digits only. Separators a person types between
// groups (space, dash, dot, slash, underscore) are dropped; anything else is
// refused rather than repaired, because a laser does not engrave it.
func NormaliseInscription(s string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '.' || r == '/' || r == '_':
		default:
			return "", ErrInvalidInscription.Wrapf("%q contains %q", s, r)
		}
	}
	out := b.String()
	if len(out) < 4 || len(out) > 40 {
		return "", ErrInvalidInscription.Wrapf("%q has %d characters once separators are removed", s, len(out))
	}
	return out, nil
}

// ValidateRecord checks a record fingerprint and its location. Lower case
// only, so one file has one fingerprint string.
func ValidateRecord(hash, uri string) error {
	if len(hash) != 64 || hash != strings.ToLower(hash) {
		return ErrInvalidRecord.Wrapf("fingerprint %q", hash)
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return ErrInvalidRecord.Wrapf("fingerprint %q", hash)
	}
	if uri == "" || len(uri) > 512 {
		return ErrInvalidRecord.Wrap("location")
	}
	return nil
}
```

`x/jewel/types/signing.go`:

```go
package types

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// VoucherSignBytes is what an owner signs to consent to one benefit use. The
// chain id is in it so a voucher from a test chain cannot be spent here.
func VoucherSignBytes(chainID string, v Voucher) []byte {
	return []byte(fmt.Sprintf("yamale-jewel-voucher/v1:%s:%d:%d:%s:%d",
		chainID, v.BenefitId, v.JewelId, v.Nonce, v.ExpiresAt))
}

// ClaimSignBytes is what a claim key signs. It names the claimer, which is
// what makes a claim copied out of the mempool useless to anyone else.
func ClaimSignBytes(chainID string, jewelID uint64, claimer string) []byte {
	return []byte(fmt.Sprintf("yamale-jewel-claim/v1:%s:%d:%s", chainID, jewelID, claimer))
}

// VerifySecp256k1 checks a signature by a compressed secp256k1 key and
// returns the key's account address.
func VerifySecp256k1(pubKey, msg, sig []byte) (sdk.AccAddress, bool) {
	if len(pubKey) != secp256k1.PubKeySize {
		return nil, false
	}
	pk := &secp256k1.PubKey{Key: pubKey}
	if !pk.VerifySignature(msg, sig) {
		return nil, false
	}
	return sdk.AccAddress(pk.Address()), true
}
```

`x/jewel/types/genesis.go` (Validate is filled in Task 8):

```go
package types

// DefaultGenesis returns an empty register with the default settings.
func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

// Validate refuses a genesis the module could not have produced.
func (gs GenesisState) Validate() error {
	return gs.Params.Validate()
}
```

- [ ] **Step 11: Run the tests to see them pass**

Run: `go test ./x/jewel/types/...`
Expected: `ok  	yamale/blockchain/x/jewel/types`

- [ ] **Step 12: Commit**

```bash
git add proto/blockchain/jewel x/jewel/types
git commit -m "x/jewel: protos, generated types, inscription and signing rules"
```

---

### Task 2: Keeper, module and app wiring

**Files:**
- Create: `x/jewel/keeper/keeper.go`, `x/jewel/keeper/msg_server.go`, `x/jewel/keeper/query.go` (Params only), `x/jewel/keeper/genesis.go`
- Create: `x/jewel/module/module.go`, `x/jewel/module/depinject.go`, `x/jewel/module/autocli.go`
- Create: `app/profile_jewel_on.go`, `app/profile_jewel_off.go`
- Modify: `app/app.go` (embed `jewelKeepers` beside `herdKeepers` at line 119; append `app.jewelDepinjectOutputs()` after `app.herdDepinjectOutputs()` around line 238)
- Modify: `app/app_config.go` (append `jewelInitGenesis` after `herdInitGenesis` around line 305; `jewelModuleConfigs` after `herdModuleConfigs` around line 409)
- Modify: `app/profile_modules_on_test.go`, `app/profile_modules_off_test.go`
- Test: `x/jewel/keeper/fixture_test.go`, `x/jewel/keeper/keeper_test.go`

**Interfaces:**
- Consumes: Task 1's types.
- Produces: `keeper.NewKeeper(storeService corestore.KVStoreService, cdc codec.Codec, addressCodec address.Codec, authority []byte, groupKeeper types.GroupKeeper, nftKeeper types.NFTKeeper, circuit types.CircuitKeeper) Keeper`; `Keeper` fields `Params, Jeweler, Jewel, Offer, Benefit, Reassignment, UsedNonce, ByBusiness, ByStaff, ByInscription, Search, BenefitsByJewel, NextJewelerID, NextJewelID, NextBenefitID`; helpers `nextID`, `isNotFound`, `(Keeper) now`, `requireOffice`, `requireBareSendClosed`, `getJeweler`, `activeJeweler`, `requireStaff`, `getJewel`, `Owner`, `actsForOwner`, `transfer`; `keeper.NewMsgServerImpl(Keeper) types.MsgServer`; `keeper.NewQueryServerImpl(Keeper) types.QueryServer`; test fixture `setup(t) *fixture` with `f.admit`, `f.register`, `f.owner`, `f.advance`.

- [ ] **Step 1: Write the test fixture and a failing wiring test**

`x/jewel/keeper/fixture_test.go`:

```go
package keeper_test

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/x/nft"
	nftkeeper "cosmossdk.io/x/nft/keeper"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/jewel/keeper"
	"yamale/blockchain/x/jewel/types"
)

// fakeCircuit stands in for x/circuit. Closed by default, because a chain
// running this module is supposed to have the bare send disabled; the test
// that proves the module checks opens it.
type fakeCircuit struct{ open bool }

func (c *fakeCircuit) IsAllowed(_ context.Context, url string) (bool, error) {
	if url == types.NFTSendTypeURL {
		return c.open, nil
	}
	return true, nil
}

const recordHash = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

type fixture struct {
	env     *integration.Env
	k       keeper.Keeper
	srv     types.MsgServer
	qs      types.QueryServer
	nft     nftkeeper.Keeper
	circuit *fakeCircuit
	office  string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	env := integration.NewWith(t, []string{types.ModuleName, nft.ModuleName})
	env.Ctx = env.Ctx.WithBlockTime(time.Unix(1_788_000_000, 0)).WithBlockHeight(1).WithChainID("yamale-test-1")

	nk := nftkeeper.NewKeeper(env.Store(nft.ModuleName), env.Codec, env.AuthKeeper, env.BankKeeper)
	circuit := &fakeCircuit{}
	// nil group keeper: admission is tested by writing the office into the
	// params, which is what a successful appointment produces. The group
	// check itself has its own test with a fake.
	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, nil, nk, circuit)

	_, office := env.Addr(t)
	p := types.DefaultParams()
	p.Office = office
	require.NoError(t, k.Params.Set(env.Ctx, p))

	return &fixture{env: env, k: k, srv: keeper.NewMsgServerImpl(k), qs: keeper.NewQueryServerImpl(k),
		nft: nk, circuit: circuit, office: office}
}

// admit admits a jeweler with one staff member and returns its id, its
// business account and the staff member.
func (f *fixture) admit(t *testing.T, name string) (uint64, string, string) {
	t.Helper()
	_, business := f.env.Addr(t)
	res, err := f.srv.AdmitJeweler(f.env.Ctx, &types.MsgAdmitJeweler{Office: f.office, Name: name, Business: business})
	require.NoError(t, err)
	_, staff := f.env.Addr(t)
	_, err = f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: business, JewelerId: res.Id, Add: []string{staff}})
	require.NoError(t, err)
	return res.Id, business, staff
}

// register registers a jewel through the given staff member.
func (f *fixture) register(t *testing.T, staff, inscription string) uint64 {
	t.Helper()
	res, err := f.srv.RegisterJewel(f.env.Ctx, &types.MsgRegisterJewel{
		Staff: staff, Inscription: inscription, RecordHash: recordHash,
		RecordUri: "https://loyalty.yamaletech.com/r/" + recordHash,
	})
	require.NoError(t, err)
	return res.Id
}

func (f *fixture) jewel(t *testing.T, id uint64) types.Jewel {
	t.Helper()
	j, err := f.k.Jewel.Get(f.env.Ctx, id)
	require.NoError(t, err)
	return j
}

func (f *fixture) owner(t *testing.T, id uint64) string {
	t.Helper()
	o, err := f.k.Owner(f.env.Ctx, f.jewel(t, id))
	require.NoError(t, err)
	return o
}

func (f *fixture) advance(d time.Duration) {
	f.env.Ctx = f.env.Ctx.WithBlockTime(f.env.Ctx.BlockTime().Add(d))
}
```

The fixture's helpers call handlers written in Tasks 3 and 4. For this task, write only one test, in `x/jewel/keeper/keeper_test.go`:

```go
package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// The keeper builds its schema without overlapping prefixes and reads back
// the params the fixture wrote.
func TestTheKeeperBuildsAndHoldsItsParams(t *testing.T) {
	f := setup(t)
	res, err := f.qs.Params(f.env.Ctx, &types.QueryParamsRequest{})
	require.NoError(t, err)
	require.Equal(t, f.office, res.Params.Office)
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./x/jewel/keeper/... -run TestTheKeeperBuilds`
Expected: FAIL to compile, `undefined: keeper.NewKeeper`.

- [ ] **Step 3: Write keeper.go**

`x/jewel/keeper/keeper.go`:

```go
package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"yamale/blockchain/x/jewel/types"
)

// Keeper holds the jewel register. x/nft holds the tokens; this keeps the
// rules about who may mint, move and use them.
type Keeper struct {
	storeService corestore.KVStoreService
	cdc          codec.Codec
	addressCodec address.Codec

	// The x/gov module account. It appoints the office; the office admits
	// jewelers.
	authority []byte

	groupKeeper types.GroupKeeper
	nft         types.NFTKeeper
	circuit     types.CircuitKeeper

	Schema collections.Schema
	Params collections.Item[types.Params]

	Jeweler      collections.Map[uint64, types.Jeweler]
	Jewel        collections.Map[uint64, types.Jewel]
	Offer        collections.Map[uint64, types.Offer] // by jewel id; one open offer per jewel
	Benefit      collections.Map[uint64, types.Benefit]
	Reassignment collections.Map[uint64, types.Reassignment] // by jewel id
	// (benefit id, nonce): vouchers already spent.
	UsedNonce collections.KeySet[collections.Pair[uint64, string]]

	// business address -> jeweler id. One business, one jeweler.
	ByBusiness collections.Map[string, uint64]
	// staff address -> jeweler id. A person works for one jeweler at a time.
	ByStaff collections.Map[string, uint64]
	// (jeweler id, normalised inscription) -> jewel id. The uniqueness rule.
	ByInscription collections.Map[collections.Pair[uint64, string], uint64]
	// (normalised inscription, jewel id): the public search across jewelers.
	Search collections.KeySet[collections.Pair[string, uint64]]
	// (jewel id, benefit id): one jewel's benefits as a prefix walk.
	BenefitsByJewel collections.KeySet[collections.Pair[uint64, uint64]]

	NextJewelerID collections.Sequence
	NextJewelID   collections.Sequence
	NextBenefitID collections.Sequence
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,
	groupKeeper types.GroupKeeper,
	nftKeeper types.NFTKeeper,
	circuit types.CircuitKeeper,
) Keeper {
	if _, err := addressCodec.BytesToString(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)
	pairU64Str := collections.PairKeyCodec(collections.Uint64Key, collections.StringKey)

	k := Keeper{
		storeService: storeService,
		cdc:          cdc,
		addressCodec: addressCodec,
		authority:    authority,
		groupKeeper:  groupKeeper,
		nft:          nftKeeper,
		circuit:      circuit,

		Params: collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),

		Jeweler: collections.NewMap(sb, types.JewelerKey, "jeweler",
			collections.Uint64Key, codec.CollValue[types.Jeweler](cdc)),
		Jewel: collections.NewMap(sb, types.JewelKey, "jewel",
			collections.Uint64Key, codec.CollValue[types.Jewel](cdc)),
		Offer: collections.NewMap(sb, types.OfferKey, "offer",
			collections.Uint64Key, codec.CollValue[types.Offer](cdc)),
		Benefit: collections.NewMap(sb, types.BenefitKey, "benefit",
			collections.Uint64Key, codec.CollValue[types.Benefit](cdc)),
		Reassignment: collections.NewMap(sb, types.ReassignmentKey, "reassignment",
			collections.Uint64Key, codec.CollValue[types.Reassignment](cdc)),
		UsedNonce: collections.NewKeySet(sb, types.UsedNonceKey, "used_nonce", pairU64Str),

		ByBusiness: collections.NewMap(sb, types.ByBusinessKey, "by_business",
			collections.StringKey, collections.Uint64Value),
		ByStaff: collections.NewMap(sb, types.ByStaffKey, "by_staff",
			collections.StringKey, collections.Uint64Value),
		ByInscription: collections.NewMap(sb, types.ByInscriptionKey, "by_inscription",
			pairU64Str, collections.Uint64Value),
		Search: collections.NewKeySet(sb, types.SearchKey, "search",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		BenefitsByJewel: collections.NewKeySet(sb, types.BenefitsByJewelKey, "benefits_by_jewel",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),

		NextJewelerID: collections.NewSequence(sb, types.NextJewelerIDKey, "next_jeweler_id"),
		NextJewelID:   collections.NewSequence(sb, types.NextJewelIDKey, "next_jewel_id"),
		NextBenefitID: collections.NewSequence(sb, types.NextBenefitIDKey, "next_benefit_id"),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

// GetAuthority returns the governance authority.
func (k Keeper) GetAuthority() []byte { return k.authority }

// nextID hands out an id, skipping zero: in proto3 an id of 0 is
// indistinguishable from an unset field.
func nextID(ctx context.Context, seq collections.Sequence) (uint64, error) {
	id, err := seq.Next(ctx)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return seq.Next(ctx)
	}
	return id, nil
}

func isNotFound(err error) bool { return errors.Is(err, collections.ErrNotFound) }

func (k Keeper) now(ctx context.Context) int64 {
	return sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
}

func (k Keeper) chainID(ctx context.Context) string {
	return sdk.UnwrapSDKContext(ctx).ChainID()
}

// requireOffice refuses anyone but the appointed office.
func (k Keeper) requireOffice(ctx context.Context, signer string) error {
	p, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}
	if p.Office == "" {
		return types.ErrNoOffice
	}
	if signer != p.Office {
		return types.ErrNotOffice.Wrapf("%s", signer)
	}
	return nil
}

// requireBareSendClosed refuses to admit or mint while x/nft's own send can
// still move a jewel past every rule here.
func (k Keeper) requireBareSendClosed(ctx context.Context) error {
	allowed, err := k.circuit.IsAllowed(ctx, types.NFTSendTypeURL)
	if err != nil {
		return err
	}
	if allowed {
		return types.ErrBareSendOpen
	}
	return nil
}

func (k Keeper) getJeweler(ctx context.Context, id uint64) (types.Jeweler, error) {
	j, err := k.Jeweler.Get(ctx, id)
	if isNotFound(err) {
		return types.Jeweler{}, types.ErrNoJeweler.Wrapf("%d", id)
	}
	return j, err
}

func (k Keeper) activeJeweler(ctx context.Context, id uint64) (types.Jeweler, error) {
	j, err := k.getJeweler(ctx, id)
	if err != nil {
		return j, err
	}
	if j.Status != types.JEWELER_ACTIVE {
		return j, types.ErrJewelerSuspended.Wrapf("%s", j.Name)
	}
	return j, nil
}

// requireStaff returns the jeweler, active, that signer works for, and
// refuses if that is not jewelerID.
func (k Keeper) requireStaff(ctx context.Context, signer string, jewelerID uint64) (types.Jeweler, error) {
	got, err := k.ByStaff.Get(ctx, signer)
	if isNotFound(err) || (err == nil && got != jewelerID) {
		return types.Jeweler{}, types.ErrNotStaff.Wrapf("%s", signer)
	}
	if err != nil {
		return types.Jeweler{}, err
	}
	return k.activeJeweler(ctx, jewelerID)
}

func (k Keeper) getJewel(ctx context.Context, id uint64) (types.Jewel, error) {
	j, err := k.Jewel.Get(ctx, id)
	if isNotFound(err) {
		return types.Jewel{}, types.ErrNoJewel.Wrapf("%d", id)
	}
	return j, err
}

// Owner reads the jewel's owner from x/nft, the only record of it.
func (k Keeper) Owner(ctx context.Context, j types.Jewel) (string, error) {
	addr := k.nft.GetOwner(ctx, types.ClassID(j.JewelerId), types.TokenID(j.Id))
	if addr.Empty() {
		return "", types.ErrNoJewel.Wrapf("%d has no owner in x/nft", j.Id)
	}
	return k.addressCodec.BytesToString(addr)
}

// actsForOwner accepts the owner, or, while the jewel is still the jeweler's
// stock, an active member of that jeweler's staff. It returns the owner.
func (k Keeper) actsForOwner(ctx context.Context, signer string, j types.Jewel) (string, error) {
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return "", err
	}
	if signer == owner {
		return owner, nil
	}
	jr, err := k.getJeweler(ctx, j.JewelerId)
	if err != nil {
		return "", err
	}
	if owner == jr.Business && jr.Status == types.JEWELER_ACTIVE {
		if got, err := k.ByStaff.Get(ctx, signer); err == nil && got == jr.Id {
			return owner, nil
		}
	}
	return "", types.ErrNotOwner.Wrapf("%s", signer)
}

// transfer moves the token, counts the new owner and closes any offer.
func (k Keeper) transfer(ctx context.Context, j *types.Jewel, to string) error {
	addr, err := k.addressCodec.StringToBytes(to)
	if err != nil {
		return types.ErrInvalidAddress.Wrapf("%s", to)
	}
	if err := k.nft.Transfer(ctx, types.ClassID(j.JewelerId), types.TokenID(j.Id), addr); err != nil {
		return err
	}
	j.OwnerCount++
	if err := k.Offer.Remove(ctx, j.Id); err != nil {
		return err
	}
	return k.Jewel.Set(ctx, j.Id, *j)
}
```

- [ ] **Step 4: Write msg_server.go, query.go and genesis.go stubs**

`x/jewel/keeper/msg_server.go`:

```go
package keeper

import (
	"bytes"
	"context"

	"github.com/cosmos/cosmos-sdk/x/group"

	"yamale/blockchain/x/jewel/types"
)

type msgServer struct {
	Keeper
	// Embedded until every handler exists; removed in Task 9.
	types.UnimplementedMsgServer
}

// NewMsgServerImpl returns the register's Msg service.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// UpdateParams sets the settings and appoints the office. Governance only.
func (k msgServer) UpdateParams(ctx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	authority, err := k.addressCodec.StringToBytes(req.Authority)
	if err != nil || !bytes.Equal(k.GetAuthority(), authority) {
		return nil, types.ErrInvalidSigner.Wrapf("%s", req.Authority)
	}
	if err := req.Params.Validate(); err != nil {
		return nil, err
	}
	if req.Params.Office != "" {
		if _, err := k.addressCodec.StringToBytes(req.Params.Office); err != nil {
			return nil, types.ErrInvalidAddress.Wrapf("office %s", req.Params.Office)
		}
		if k.groupKeeper != nil {
			if _, err := k.groupKeeper.GroupPolicyInfo(ctx,
				&group.QueryGroupPolicyInfoRequest{Address: req.Params.Office}); err != nil {
				return nil, types.ErrOfficeNotGroup.Wrapf("%s", req.Params.Office)
			}
		}
	}
	return &types.MsgUpdateParamsResponse{}, k.Params.Set(ctx, req.Params)
}
```

`x/jewel/keeper/query.go`:

```go
package keeper

import (
	"context"

	"yamale/blockchain/x/jewel/types"
)

type queryServer struct{ k Keeper }

// NewQueryServerImpl returns the register's Query service.
func NewQueryServerImpl(k Keeper) types.QueryServer { return queryServer{k: k} }

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: p}, nil
}
```

Until Task 7, `queryServer` does not implement the other four RPCs. Embed `types.UnimplementedQueryServer` in it for now:

```go
type queryServer struct {
	types.UnimplementedQueryServer
	k Keeper
}
```

`x/jewel/keeper/genesis.go` (Task 8 replaces it):

```go
package keeper

import (
	"context"

	"yamale/blockchain/x/jewel/types"
)

// InitGenesis writes the register in.
func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	return k.Params.Set(ctx, gs.Params)
}

// ExportGenesis writes the register out.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.GenesisState{Params: p}, nil
}
```

- [ ] **Step 5: Run the keeper test**

Run: `go test ./x/jewel/keeper/... -run TestTheKeeperBuilds`
Expected: PASS. The fixture's helpers call `AdmitJeweler`, `SetStaff` and `RegisterJewel`; `UnimplementedMsgServer` provides them, so the package compiles, and this test does not call them. If it fails with `schema has overlapping prefixes`, a prefix in `keys.go` is a byte-prefix of another: rename it.

- [ ] **Step 6: Write the module, depinject and autocli files**

`x/jewel/module/module.go`: copy `x/herd/module/module.go` and change only:
- the package name to `jewel`;
- imports to `yamale/blockchain/x/jewel/keeper` and `yamale/blockchain/x/jewel/types`;
- the `AppModule` doc comment to:

```go
// AppModule is the jewel register.
//
// No BeginBlocker or EndBlocker. Offers lapse and veto windows close when
// somebody acts on them, not on a sweep: a register that moved jewels in the
// background would move them while nobody was watching.
```

`x/jewel/module/depinject.go`:

```go
package jewel

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	circuitkeeper "cosmossdk.io/x/circuit/keeper"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"yamale/blockchain/x/jewel/keeper"
	"yamale/blockchain/x/jewel/types"
)

var _ depinject.OnePerModuleType = AppModule{}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (AppModule) IsOnePerModuleType() {}

func init() {
	appconfig.Register(&types.Module{}, appconfig.Provide(ProvideModule))
}

// ModuleInputs are what the register needs. No bank keeper: a jewel's sale
// is paid off the chain, and the register records ownership, not payment.
type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	GroupKeeper types.GroupKeeper
	NFTKeeper   types.NFTKeeper
	// The concrete keeper, because IsAllowed has a pointer receiver and
	// depinject hands out the value.
	CircuitKeeper circuitkeeper.Keeper
}

type ModuleOutputs struct {
	depinject.Out

	JewelKeeper keeper.Keeper
	Module      appmodule.AppModule
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}
	circuit := in.CircuitKeeper
	k := keeper.NewKeeper(in.StoreService, in.Cdc, in.AddressCodec, authority,
		in.GroupKeeper, in.NFTKeeper, &circuit)
	return ModuleOutputs{JewelKeeper: k, Module: NewAppModule(in.Cdc, k)}
}
```

`x/jewel/module/autocli.go` (Task 7 adds the jewel queries; Task 9 adds the transactions):

```go
package jewel

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"yamale/blockchain/x/jewel/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "Params", Use: "params", Short: "Shows the office and how long offers, vouchers and veto windows last"},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              types.Msg_serviceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "UpdateParams", Skip: true}, // governance only
			},
		},
	}
}
```

Check `x/herd/module/autocli.go` for the exact `Tx:` block shape it uses and match it; if herd sets `EnhanceCustomCommand` differently, follow herd.

- [ ] **Step 7: Wire the module into the app**

`app/profile_jewel_on.go`:

```go
//go:build !settlement

package app

import (
	appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"
	"cosmossdk.io/depinject/appconfig"

	jewelmodulekeeper "yamale/blockchain/x/jewel/keeper"
	_ "yamale/blockchain/x/jewel/module" // registers the module with appconfig
	jewelmoduletypes "yamale/blockchain/x/jewel/types"
)

// The half of the x/jewel contract that exists when the module is compiled
// in. See profile_jewel_off.go for the other half.

var (
	// After x/nft in the genesis order, which the SDK block already places
	// earlier: a jewel names an x/nft class and token.
	jewelInitGenesis = []string{jewelmoduletypes.ModuleName}

	jewelModuleConfigs = []*appv1alpha1.ModuleConfig{
		{
			Name:   jewelmoduletypes.ModuleName,
			Config: appconfig.WrapAny(&jewelmoduletypes.Module{}),
		},
	}
)

type jewelKeepers struct {
	JewelKeeper jewelmodulekeeper.Keeper
}

func (app *App) jewelDepinjectOutputs() []any {
	return []any{&app.JewelKeeper}
}
```

`app/profile_jewel_off.go`:

```go
//go:build settlement

package app

import appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"

// x/jewel is a commercial register for jewelers, not settlement
// infrastructure, so a settlement binary does not carry it.

var (
	jewelInitGenesis   []string
	jewelModuleConfigs []*appv1alpha1.ModuleConfig
)

type jewelKeepers struct{}

func (app *App) jewelDepinjectOutputs() []any { return nil }
```

In `app/app.go`, add `jewelKeepers` on the line after `herdKeepers` in the `App` struct, and `app.jewelDepinjectOutputs(),` after `app.herdDepinjectOutputs(),` in the outputs list.

In `app/app_config.go`, add `jewelInitGenesis,` after `herdInitGenesis,`, and change `landModuleConfigs, herdModuleConfigs, []*appv1alpha1.ModuleConfig{` to `landModuleConfigs, herdModuleConfigs, jewelModuleConfigs, []*appv1alpha1.ModuleConfig{`.

In `app/profile_modules_on_test.go`, after the x/land assertions add:

```go
	// x/jewel holds no coins either: the tokens are x/nft's.
	require.Contains(t, runtimeConfig.InitGenesis, "jewel")
	require.NotContains(t, macc, "jewel")
	require.Less(t, slices.Index(runtimeConfig.InitGenesis, "nft"), slices.Index(runtimeConfig.InitGenesis, "jewel"),
		"x/jewel names x/nft classes, so x/nft must be restored first")
```

In `app/profile_modules_off_test.go`, add `"jewel"` to the name list in `TestExcludedModulesAreUnwiredInThisProfile` and in `TestExcludedModulesAreGoneFromEveryOrdering` if that test has its own list.

- [ ] **Step 8: Build and run the app tests**

Run:

```bash
go build ./...
go test ./app/... -run 'TestEveryModuleIsWiredIntoThisProfile|TestTheOrderingsThatMatterAreHeld|TestTheUpgradeSequence'
go build -tags settlement ./...
```

Expected: both builds succeed; the tests PASS. A depinject error naming `types.NFTKeeper` means x/nft's provided keeper does not satisfy the interface: compare the method set in `expected_keepers.go` with `cosmossdk.io/x/nft@v0.1.0/keeper` and fix the interface, not the keeper.

- [ ] **Step 9: Commit**

```bash
git add x/jewel app
git commit -m "x/jewel: keeper, module and wiring; compiled out of the settlement build"
```

---

### Task 3: The office, jewelers and staff

**Files:**
- Create: `x/jewel/keeper/msg_server_jeweler.go`
- Test: `x/jewel/keeper/jeweler_test.go`

**Interfaces:**
- Consumes: Task 2's keeper helpers and fixture.
- Produces: handlers `AdmitJeweler`, `SuspendJeweler`, `ReinstateJeweler`, `SetStaff`.

- [ ] **Step 1: Write the failing tests**

`x/jewel/keeper/jeweler_test.go`:

```go
package keeper_test

import (
	"context"
	"errors"
	"testing"

	"github.com/cosmos/cosmos-sdk/x/group"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/keeper"
	"yamale/blockchain/x/jewel/types"
)

func TestTheOfficeAdmitsAJewelerAndOpensItsCollection(t *testing.T) {
	f := setup(t)
	id, business, staff := f.admit(t, "Maison Diop")

	j, err := f.k.Jeweler.Get(f.env.Ctx, id)
	require.NoError(t, err)
	require.Equal(t, business, j.Business)
	require.Equal(t, []string{staff}, j.Staff)
	require.Equal(t, types.JEWELER_ACTIVE, j.Status)
	require.True(t, f.nft.HasClass(f.env.Ctx, types.ClassID(id)))
}

func TestOnlyTheOfficeAdmits(t *testing.T) {
	f := setup(t)
	_, someone := f.env.Addr(t)
	_, err := f.srv.AdmitJeweler(f.env.Ctx, &types.MsgAdmitJeweler{Office: someone, Name: "X", Business: someone})
	require.ErrorIs(t, err, types.ErrNotOffice)
}

func TestNothingIsAdmittedBeforeAnOfficeIsAppointed(t *testing.T) {
	f := setup(t)
	require.NoError(t, f.k.Params.Set(f.env.Ctx, types.DefaultParams()))
	_, err := f.srv.AdmitJeweler(f.env.Ctx, &types.MsgAdmitJeweler{Office: f.office, Name: "X", Business: f.office})
	require.ErrorIs(t, err, types.ErrNoOffice)
}

// The guard that makes the circuit setting impossible to forget.
func TestNoJewelerIsAdmittedWhileTheBareSendIsOpen(t *testing.T) {
	f := setup(t)
	f.circuit.open = true
	_, business := f.env.Addr(t)
	_, err := f.srv.AdmitJeweler(f.env.Ctx, &types.MsgAdmitJeweler{Office: f.office, Name: "X", Business: business})
	require.ErrorIs(t, err, types.ErrBareSendOpen)
}

func TestABusinessAccountBelongsToOneJeweler(t *testing.T) {
	f := setup(t)
	_, business, _ := f.admit(t, "Maison Diop")
	_, err := f.srv.AdmitJeweler(f.env.Ctx, &types.MsgAdmitJeweler{Office: f.office, Name: "Copie", Business: business})
	require.ErrorIs(t, err, types.ErrBusinessTaken)
}

func TestAPersonWorksForOneJewelerAtATime(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	other, otherBusiness, _ := f.admit(t, "Atelier Ba")
	_, err := f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: otherBusiness, JewelerId: other, Add: []string{staff}})
	require.ErrorIs(t, err, types.ErrStaffTaken)
}

func TestOnlyTheBusinessManagesStaff(t *testing.T) {
	f := setup(t)
	id, _, staff := f.admit(t, "Maison Diop")
	_, newcomer := f.env.Addr(t)
	_, err := f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: staff, JewelerId: id, Add: []string{newcomer}})
	require.ErrorIs(t, err, types.ErrNotBusiness)
}

func TestRemovedStaffStopActing(t *testing.T) {
	f := setup(t)
	id, business, staff := f.admit(t, "Maison Diop")
	_, err := f.srv.SetStaff(f.env.Ctx, &types.MsgSetStaff{Business: business, JewelerId: id, Remove: []string{staff}})
	require.NoError(t, err)
	has, err := f.k.ByStaff.Has(f.env.Ctx, staff)
	require.NoError(t, err)
	require.False(t, has)
	j, _ := f.k.Jeweler.Get(f.env.Ctx, id)
	require.Empty(t, j.Staff)
}

func TestSuspensionNeedsAReasonAndReinstatementUndoesIt(t *testing.T) {
	f := setup(t)
	id, _, _ := f.admit(t, "Maison Diop")
	_, err := f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: id})
	require.ErrorIs(t, err, types.ErrNoReason)

	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: id, Reason: "complaint 2026-118"})
	require.NoError(t, err)
	j, _ := f.k.Jeweler.Get(f.env.Ctx, id)
	require.Equal(t, types.JEWELER_SUSPENDED, j.Status)

	_, err = f.srv.ReinstateJeweler(f.env.Ctx, &types.MsgReinstateJeweler{Office: f.office, JewelerId: id})
	require.NoError(t, err)
	j, _ = f.k.Jeweler.Get(f.env.Ctx, id)
	require.Equal(t, types.JEWELER_ACTIVE, j.Status)
}

type fakeGroups struct{ policies map[string]bool }

func (g fakeGroups) GroupPolicyInfo(_ context.Context, req *group.QueryGroupPolicyInfoRequest) (*group.QueryGroupPolicyInfoResponse, error) {
	if g.policies[req.Address] {
		return &group.QueryGroupPolicyInfoResponse{}, nil
	}
	return nil, errors.New("not found")
}

// An office that is one key is one bribe. Governance cannot appoint one.
func TestTheOfficeMustBeAGroupAccount(t *testing.T) {
	f := setup(t)
	_, policy := f.env.Addr(t)
	_, plainKey := f.env.Addr(t)
	k := keeper.NewKeeper(f.env.StoreService, f.env.Codec, f.env.AddressCodec, f.env.Authority,
		fakeGroups{policies: map[string]bool{policy: true}}, f.nft, f.circuit)
	srv := keeper.NewMsgServerImpl(k)
	gov, err := f.env.AddressCodec.BytesToString(f.env.Authority)
	require.NoError(t, err)

	p := types.DefaultParams()
	p.Office = plainKey
	_, err = srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{Authority: gov, Params: p})
	require.ErrorIs(t, err, types.ErrOfficeNotGroup)

	p.Office = policy
	_, err = srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{Authority: gov, Params: p})
	require.NoError(t, err)

	_, err = srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{Authority: policy, Params: p})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./x/jewel/keeper/... -run 'Office|Jeweler|Business|Staff|Suspension|BareSend'`
Expected: FAIL with `rpc error: code = Unimplemented desc = method AdmitJeweler not implemented`.

- [ ] **Step 3: Write the handlers**

`x/jewel/keeper/msg_server_jeweler.go`:

```go
package keeper

import (
	"context"
	"slices"
	"strings"

	"cosmossdk.io/x/nft"

	"yamale/blockchain/x/jewel/types"
)

// AdmitJeweler admits a business and opens its x/nft collection.
func (k msgServer) AdmitJeweler(ctx context.Context, req *types.MsgAdmitJeweler) (*types.MsgAdmitJewelerResponse, error) {
	if err := k.requireOffice(ctx, req.Office); err != nil {
		return nil, err
	}
	if err := k.requireBareSendClosed(ctx); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, types.ErrNoName
	}
	if _, err := k.addressCodec.StringToBytes(req.Business); err != nil {
		return nil, types.ErrInvalidAddress.Wrapf("business %s", req.Business)
	}
	if has, err := k.ByBusiness.Has(ctx, req.Business); err != nil {
		return nil, err
	} else if has {
		return nil, types.ErrBusinessTaken.Wrapf("%s", req.Business)
	}

	id, err := nextID(ctx, k.NextJewelerID)
	if err != nil {
		return nil, err
	}
	if err := k.nft.SaveClass(ctx, nft.Class{Id: types.ClassID(id), Name: name, Symbol: "JEWEL"}); err != nil {
		return nil, err
	}
	j := types.Jeweler{Id: id, Name: name, Business: req.Business, Status: types.JEWELER_ACTIVE}
	if err := k.Jeweler.Set(ctx, id, j); err != nil {
		return nil, err
	}
	if err := k.ByBusiness.Set(ctx, req.Business, id); err != nil {
		return nil, err
	}
	return &types.MsgAdmitJewelerResponse{Id: id}, nil
}

// SuspendJeweler stops a jeweler acting. Its jewels and histories stay.
func (k msgServer) SuspendJeweler(ctx context.Context, req *types.MsgSuspendJeweler) (*types.MsgSuspendJewelerResponse, error) {
	if err := k.requireOffice(ctx, req.Office); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Reason) == "" {
		return nil, types.ErrNoReason
	}
	j, err := k.getJeweler(ctx, req.JewelerId)
	if err != nil {
		return nil, err
	}
	j.Status = types.JEWELER_SUSPENDED
	return &types.MsgSuspendJewelerResponse{}, k.Jeweler.Set(ctx, j.Id, j)
}

// ReinstateJeweler lets a suspended jeweler act again.
func (k msgServer) ReinstateJeweler(ctx context.Context, req *types.MsgReinstateJeweler) (*types.MsgReinstateJewelerResponse, error) {
	if err := k.requireOffice(ctx, req.Office); err != nil {
		return nil, err
	}
	j, err := k.getJeweler(ctx, req.JewelerId)
	if err != nil {
		return nil, err
	}
	j.Status = types.JEWELER_ACTIVE
	return &types.MsgReinstateJewelerResponse{}, k.Jeweler.Set(ctx, j.Id, j)
}

// SetStaff adds and removes the people who act for a jeweler. Allowed while
// suspended, because removing someone matters most then.
func (k msgServer) SetStaff(ctx context.Context, req *types.MsgSetStaff) (*types.MsgSetStaffResponse, error) {
	j, err := k.getJeweler(ctx, req.JewelerId)
	if err != nil {
		return nil, err
	}
	if req.Business != j.Business {
		return nil, types.ErrNotBusiness.Wrapf("%s", req.Business)
	}
	for _, s := range req.Remove {
		if got, err := k.ByStaff.Get(ctx, s); err == nil && got == j.Id {
			if err := k.ByStaff.Remove(ctx, s); err != nil {
				return nil, err
			}
		}
		j.Staff = slices.DeleteFunc(j.Staff, func(x string) bool { return x == s })
	}
	for _, s := range req.Add {
		if _, err := k.addressCodec.StringToBytes(s); err != nil {
			return nil, types.ErrInvalidAddress.Wrapf("staff %s", s)
		}
		got, err := k.ByStaff.Get(ctx, s)
		switch {
		case err == nil && got == j.Id:
			continue
		case err == nil:
			return nil, types.ErrStaffTaken.Wrapf("%s", s)
		case !isNotFound(err):
			return nil, err
		}
		if err := k.ByStaff.Set(ctx, s, j.Id); err != nil {
			return nil, err
		}
		j.Staff = append(j.Staff, s)
	}
	return &types.MsgSetStaffResponse{}, k.Jeweler.Set(ctx, j.Id, j)
}
```

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./x/jewel/keeper/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add x/jewel/keeper
git commit -m "x/jewel: the office admits and suspends jewelers; staff belong to one jeweler"
```

---

### Task 4: Registering and amending jewels

**Files:**
- Create: `x/jewel/keeper/msg_server_jewel.go`
- Test: `x/jewel/keeper/jewel_test.go`

**Interfaces:**
- Produces: handlers `RegisterJewel`, `AmendJewel`.

- [ ] **Step 1: Write the failing tests**

`x/jewel/keeper/jewel_test.go`:

```go
package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

func TestARegisteredJewelIsTheJewelersStock(t *testing.T) {
	f := setup(t)
	jid, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "md 2026-0001")

	j := f.jewel(t, id)
	require.Equal(t, jid, j.JewelerId)
	require.Equal(t, "MD20260001", j.Inscription)
	require.Zero(t, j.OwnerCount, "stock is not an owner")
	require.Equal(t, business, f.owner(t, id))
}

func TestAnInscriptionIsRegisteredOncePerJeweler(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	first := f.register(t, staff, "MD-2026-0001")
	_, err := f.srv.RegisterJewel(f.env.Ctx, &types.MsgRegisterJewel{
		Staff: staff, Inscription: "md 2026 0001", RecordHash: recordHash, RecordUri: "u",
	})
	require.ErrorIs(t, err, types.ErrInscriptionTaken)
	require.ErrorContains(t, err, "jewel 1", "the error names the existing jewel")
	_ = first
}

// Two houses may use the same code; the search shows both, with their jeweler.
func TestTwoJewelersMayShareACode(t *testing.T) {
	f := setup(t)
	_, _, a := f.admit(t, "Maison Diop")
	_, _, b := f.admit(t, "Atelier Ba")
	f.register(t, a, "RING-0001")
	f.register(t, b, "RING-0001")
}

func TestOnlyActiveStaffRegister(t *testing.T) {
	f := setup(t)
	id, business, staff := f.admit(t, "Maison Diop")

	_, err := f.srv.RegisterJewel(f.env.Ctx, &types.MsgRegisterJewel{
		Staff: business, Inscription: "MD-0001", RecordHash: recordHash, RecordUri: "u",
	})
	require.ErrorIs(t, err, types.ErrNotStaff, "the business account is not staff")

	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: id, Reason: "audit"})
	require.NoError(t, err)
	_, err = f.srv.RegisterJewel(f.env.Ctx, &types.MsgRegisterJewel{
		Staff: staff, Inscription: "MD-0001", RecordHash: recordHash, RecordUri: "u",
	})
	require.ErrorIs(t, err, types.ErrJewelerSuspended)
}

func TestNothingIsMintedWhileTheBareSendIsOpen(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	f.circuit.open = true
	_, err := f.srv.RegisterJewel(f.env.Ctx, &types.MsgRegisterJewel{
		Staff: staff, Inscription: "MD-0001", RecordHash: recordHash, RecordUri: "u",
	})
	require.ErrorIs(t, err, types.ErrBareSendOpen)
}

func TestAnAmendmentAppendsAndNeverOverwrites(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	corrected := "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"

	_, err := f.srv.AmendJewel(f.env.Ctx, &types.MsgAmendJewel{Staff: staff, JewelId: id,
		RecordHash: corrected, RecordUri: "u2"})
	require.ErrorIs(t, err, types.ErrNoReason)

	_, err = f.srv.AmendJewel(f.env.Ctx, &types.MsgAmendJewel{Staff: staff, JewelId: id,
		RecordHash: corrected, RecordUri: "u2", Note: "carat weight 1.02, not 1.20"})
	require.NoError(t, err)

	j := f.jewel(t, id)
	require.Equal(t, recordHash, j.RecordHash, "the original stays")
	require.Len(t, j.Amendments, 1)
	require.Equal(t, corrected, j.Amendments[0].RecordHash)
	token, _ := f.nft.GetNFT(f.env.Ctx, types.ClassID(j.JewelerId), types.TokenID(id))
	require.Equal(t, corrected, token.UriHash, "x/nft points at the latest record")
}

func TestAnotherJewelersStaffCannotAmend(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	_, _, other := f.admit(t, "Atelier Ba")
	id := f.register(t, staff, "MD-0001")
	_, err := f.srv.AmendJewel(f.env.Ctx, &types.MsgAmendJewel{Staff: other, JewelId: id,
		RecordHash: recordHash, RecordUri: "u", Note: "x"})
	require.ErrorIs(t, err, types.ErrNotStaff)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./x/jewel/keeper/... -run 'Jewel|Inscription|Register|Amend|Minted'`
Expected: FAIL with `method RegisterJewel not implemented`.

- [ ] **Step 3: Write the handlers**

`x/jewel/keeper/msg_server_jewel.go`:

```go
package keeper

import (
	"context"
	"strings"

	"cosmossdk.io/collections"
	"cosmossdk.io/x/nft"

	"yamale/blockchain/x/jewel/types"
)

// RegisterJewel mints a jewel into the signer's jeweler's stock.
func (k msgServer) RegisterJewel(ctx context.Context, req *types.MsgRegisterJewel) (*types.MsgRegisterJewelResponse, error) {
	jewelerID, err := k.ByStaff.Get(ctx, req.Staff)
	if isNotFound(err) {
		return nil, types.ErrNotStaff.Wrapf("%s", req.Staff)
	}
	if err != nil {
		return nil, err
	}
	jr, err := k.activeJeweler(ctx, jewelerID)
	if err != nil {
		return nil, err
	}
	if err := k.requireBareSendClosed(ctx); err != nil {
		return nil, err
	}
	code, err := types.NormaliseInscription(req.Inscription)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateRecord(req.RecordHash, req.RecordUri); err != nil {
		return nil, err
	}
	key := collections.Join(jr.Id, code)
	if existing, err := k.ByInscription.Get(ctx, key); err == nil {
		return nil, types.ErrInscriptionTaken.Wrapf("%s is jewel %d", code, existing)
	} else if !isNotFound(err) {
		return nil, err
	}

	id, err := nextID(ctx, k.NextJewelID)
	if err != nil {
		return nil, err
	}
	business, err := k.addressCodec.StringToBytes(jr.Business)
	if err != nil {
		return nil, err
	}
	if err := k.nft.Mint(ctx, nft.NFT{
		ClassId: types.ClassID(jr.Id), Id: types.TokenID(id),
		Uri: req.RecordUri, UriHash: req.RecordHash,
	}, business); err != nil {
		return nil, err
	}
	j := types.Jewel{
		Id: id, JewelerId: jr.Id, Inscription: code,
		RecordHash: req.RecordHash, RecordUri: req.RecordUri, RegisteredAt: k.now(ctx),
	}
	if err := k.Jewel.Set(ctx, id, j); err != nil {
		return nil, err
	}
	if err := k.ByInscription.Set(ctx, key, id); err != nil {
		return nil, err
	}
	if err := k.Search.Set(ctx, collections.Join(code, id)); err != nil {
		return nil, err
	}
	return &types.MsgRegisterJewelResponse{Id: id}, nil
}

// AmendJewel appends a correction and points the token at the new record.
func (k msgServer) AmendJewel(ctx context.Context, req *types.MsgAmendJewel) (*types.MsgAmendJewelResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if _, err := k.requireStaff(ctx, req.Staff, j.JewelerId); err != nil {
		return nil, err
	}
	if err := types.ValidateRecord(req.RecordHash, req.RecordUri); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Note) == "" {
		return nil, types.ErrNoReason
	}
	j.Amendments = append(j.Amendments, types.Amendment{
		RecordHash: req.RecordHash, RecordUri: req.RecordUri, Note: req.Note,
		Staff: req.Staff, Time: k.now(ctx),
	})
	if err := k.nft.Update(ctx, nft.NFT{
		ClassId: types.ClassID(j.JewelerId), Id: types.TokenID(j.Id),
		Uri: req.RecordUri, UriHash: req.RecordHash,
	}); err != nil {
		return nil, err
	}
	return &types.MsgAmendJewelResponse{}, k.Jewel.Set(ctx, j.Id, j)
}
```

Note: `TestAnInscriptionIsRegisteredOncePerJeweler` passes `RecordUri: "u"`. `ValidateRecord` accepts any non-empty location of at most 512 characters, so `"u"` is valid.

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./x/jewel/keeper/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add x/jewel/keeper
git commit -m "x/jewel: register jewels once per inscription; amendments append"
```

---

### Task 5: Offers, claims and the stolen flag

**Files:**
- Create: `x/jewel/keeper/msg_server_offer.go`
- Test: `x/jewel/keeper/offer_test.go`

**Interfaces:**
- Produces: handlers `OfferJewel`, `CancelOffer`, `AcceptOffer`, `ClaimOffer`, `ReportStolen`, `ClearStolen`; test helper `f.sell(t, staff, jewelID) string` (offers stock to a new account, accepts, returns the owner).

- [ ] **Step 1: Write the failing tests**

`x/jewel/keeper/offer_test.go`:

```go
package keeper_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// sell offers a jewel from stock to a fresh account, which accepts it.
func (f *fixture) sell(t *testing.T, staff string, id uint64) string {
	t.Helper()
	_, buyer := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: buyer})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.NoError(t, err)
	return buyer
}

func TestStaffSellStockAndTheBuyerBecomesTheFirstOwner(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	buyer := f.sell(t, staff, id)

	require.Equal(t, buyer, f.owner(t, id))
	require.Equal(t, uint32(1), f.jewel(t, id).OwnerCount)
	has, _ := f.k.Offer.Has(f.env.Ctx, id)
	require.False(t, has, "accepting closes the offer")
}

func TestAResaleNeedsTheOwnerNotTheJeweler(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	f.sell(t, staff, id)
	_, someone := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: someone})
	require.ErrorIs(t, err, types.ErrNotOwner, "once sold, the jeweler's staff are strangers to it")
}

func TestAnOfferIsForTheNamedRecipientAndLapses(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, buyer := f.env.Addr(t)
	_, thief := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: buyer})
	require.NoError(t, err)

	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: thief, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotRecipient)

	f.advance(14*24*time.Hour + time.Second)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.ErrorIs(t, err, types.ErrOfferExpired)
}

func TestOneOpenOfferAtATimeButALapsedOneIsReplaced(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, a := f.env.Addr(t)
	_, b := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: a})
	require.NoError(t, err)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: b})
	require.ErrorIs(t, err, types.ErrOfferOpen)

	f.advance(15 * 24 * time.Hour)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: b})
	require.NoError(t, err)
}

func TestAnOfferNamesAnAccountOrAClaimKeyNotBoth(t *testing.T) {
	f := setup(t)
	_, business, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, a := f.env.Addr(t)
	key := hex.EncodeToString(secp256k1.GenPrivKey().PubKey().Bytes())

	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id})
	require.ErrorIs(t, err, types.ErrOfferTarget)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: a, ClaimKey: key})
	require.ErrorIs(t, err, types.ErrOfferTarget)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, ClaimKey: "abcd"})
	require.ErrorIs(t, err, types.ErrOfferTarget)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: business})
	require.ErrorIs(t, err, types.ErrSelfOffer)
}

func TestTheSellerCancelsBeforeAcceptance(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, buyer := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: buyer})
	require.NoError(t, err)
	_, err = f.srv.CancelOffer(f.env.Ctx, &types.MsgCancelOffer{Owner: staff, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoOffer)
}

// The email path. The link carries a one-time key; the claimer signs their
// own address with it. A claim copied out of the mempool names someone else
// and is refused.
func TestAClaimKeyHandsTheJewelToWhoeverSignsWithIt(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	claimKey := secp256k1.GenPrivKey()
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id,
		ClaimKey: hex.EncodeToString(claimKey.PubKey().Bytes())})
	require.NoError(t, err)

	_, customer := f.env.Addr(t)
	_, thief := f.env.Addr(t)
	sig, err := claimKey.Sign(types.ClaimSignBytes("yamale-test-1", id, customer))
	require.NoError(t, err)

	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: thief, JewelId: id,
		ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: sig})
	require.ErrorIs(t, err, types.ErrBadClaim, "a copied signature names the customer, not the thief")

	wrongKey := secp256k1.GenPrivKey()
	wrongSig, _ := wrongKey.Sign(types.ClaimSignBytes("yamale-test-1", id, thief))
	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: thief, JewelId: id,
		ClaimPubKey: wrongKey.PubKey().Bytes(), ClaimSignature: wrongSig})
	require.ErrorIs(t, err, types.ErrBadClaim)

	_, err = f.srv.ClaimOffer(f.env.Ctx, &types.MsgClaimOffer{Claimer: customer, JewelId: id,
		ClaimPubKey: claimKey.PubKey().Bytes(), ClaimSignature: sig})
	require.NoError(t, err)
	require.Equal(t, customer, f.owner(t, id))
}

func TestAStolenJewelCannotBeOffered(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, buyer := f.env.Addr(t)
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: owner, JewelId: id, To: buyer})
	require.NoError(t, err)

	_, err = f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: owner, JewelId: id})
	require.NoError(t, err)
	require.True(t, f.jewel(t, id).Stolen)
	has, _ := f.k.Offer.Has(f.env.Ctx, id)
	require.False(t, has, "reporting a theft closes the open offer")

	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: owner, JewelId: id, To: buyer})
	require.ErrorIs(t, err, types.ErrStolen)

	_, err = f.srv.ClearStolen(f.env.Ctx, &types.MsgClearStolen{Owner: owner, JewelId: id})
	require.NoError(t, err)
	_, err = f.srv.ClearStolen(f.env.Ctx, &types.MsgClearStolen{Owner: owner, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotStolen)
}

func TestOnlyTheOwnerReportsATheft(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	f.sell(t, staff, id)
	_, stranger := f.env.Addr(t)
	_, err := f.srv.ReportStolen(f.env.Ctx, &types.MsgReportStolen{Owner: stranger, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotOwner)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./x/jewel/keeper/... -run 'Offer|Sell|Resale|Claim|Stolen|Theft|Cancel'`
Expected: FAIL with `method OfferJewel not implemented`.

- [ ] **Step 3: Write the handlers**

`x/jewel/keeper/msg_server_offer.go`:

```go
package keeper

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"yamale/blockchain/x/jewel/types"
)

// OfferJewel opens an offer to an account or to a claim key.
func (k msgServer) OfferJewel(ctx context.Context, req *types.MsgOfferJewel) (*types.MsgOfferJewelResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if j.Stolen {
		return nil, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	owner, err := k.actsForOwner(ctx, req.Owner, j)
	if err != nil {
		return nil, err
	}
	if (req.To == "") == (req.ClaimKey == "") {
		return nil, types.ErrOfferTarget
	}
	claimKey := strings.ToLower(req.ClaimKey)
	if req.To != "" {
		if _, err := k.addressCodec.StringToBytes(req.To); err != nil {
			return nil, types.ErrInvalidAddress.Wrapf("%s", req.To)
		}
		if req.To == owner {
			return nil, types.ErrSelfOffer
		}
	} else if b, err := hex.DecodeString(claimKey); err != nil || len(b) != secp256k1.PubKeySize {
		return nil, types.ErrOfferTarget.Wrap("the claim key must be a compressed secp256k1 public key in hex")
	}

	now := k.now(ctx)
	if existing, err := k.Offer.Get(ctx, j.Id); err == nil && existing.ExpiresAt > now {
		return nil, types.ErrOfferOpen
	} else if err != nil && !isNotFound(err) {
		return nil, err
	}
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	offer := types.Offer{JewelId: j.Id, From: owner, To: req.To, ClaimKey: claimKey, ExpiresAt: now + p.OfferTtl}
	if err := k.Offer.Set(ctx, j.Id, offer); err != nil {
		return nil, err
	}
	return &types.MsgOfferJewelResponse{ExpiresAt: offer.ExpiresAt}, nil
}

// CancelOffer closes the open offer.
func (k msgServer) CancelOffer(ctx context.Context, req *types.MsgCancelOffer) (*types.MsgCancelOfferResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if _, err := k.actsForOwner(ctx, req.Owner, j); err != nil {
		return nil, err
	}
	if has, err := k.Offer.Has(ctx, j.Id); err != nil {
		return nil, err
	} else if !has {
		return nil, types.ErrNoOffer
	}
	return &types.MsgCancelOfferResponse{}, k.Offer.Remove(ctx, j.Id)
}

// openOffer returns the jewel's offer if one exists and has not lapsed.
func (k msgServer) openOffer(ctx context.Context, j types.Jewel) (types.Offer, error) {
	if j.Stolen {
		return types.Offer{}, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	o, err := k.Offer.Get(ctx, j.Id)
	if isNotFound(err) {
		return o, types.ErrNoOffer
	}
	if err != nil {
		return o, err
	}
	if k.now(ctx) >= o.ExpiresAt {
		return o, types.ErrOfferExpired
	}
	return o, nil
}

// AcceptOffer takes a jewel offered to the signer's account.
func (k msgServer) AcceptOffer(ctx context.Context, req *types.MsgAcceptOffer) (*types.MsgAcceptOfferResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	o, err := k.openOffer(ctx, j)
	if err != nil {
		return nil, err
	}
	if o.To == "" || o.To != req.Recipient {
		return nil, types.ErrNotRecipient
	}
	return &types.MsgAcceptOfferResponse{}, k.transfer(ctx, &j, req.Recipient)
}

// ClaimOffer takes a jewel offered to a claim key. The key signs the
// claimer's own address, so a claim copied from the mempool is worthless.
func (k msgServer) ClaimOffer(ctx context.Context, req *types.MsgClaimOffer) (*types.MsgClaimOfferResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	o, err := k.openOffer(ctx, j)
	if err != nil {
		return nil, err
	}
	if o.ClaimKey == "" || hex.EncodeToString(req.ClaimPubKey) != o.ClaimKey {
		return nil, types.ErrBadClaim
	}
	msg := types.ClaimSignBytes(k.chainID(ctx), j.Id, req.Claimer)
	if _, ok := types.VerifySecp256k1(req.ClaimPubKey, msg, req.ClaimSignature); !ok {
		return nil, types.ErrBadClaim
	}
	if req.Claimer == o.From {
		return nil, types.ErrSelfOffer
	}
	return &types.MsgClaimOfferResponse{}, k.transfer(ctx, &j, req.Claimer)
}

// ReportStolen flags the jewel and closes its open offer.
func (k msgServer) ReportStolen(ctx context.Context, req *types.MsgReportStolen) (*types.MsgReportStolenResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if _, err := k.actsForOwner(ctx, req.Owner, j); err != nil {
		return nil, err
	}
	j.Stolen = true
	if err := k.Offer.Remove(ctx, j.Id); err != nil {
		return nil, err
	}
	return &types.MsgReportStolenResponse{}, k.Jewel.Set(ctx, j.Id, j)
}

// ClearStolen lifts the flag.
func (k msgServer) ClearStolen(ctx context.Context, req *types.MsgClearStolen) (*types.MsgClearStolenResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if _, err := k.actsForOwner(ctx, req.Owner, j); err != nil {
		return nil, err
	}
	if !j.Stolen {
		return nil, types.ErrNotStolen
	}
	j.Stolen = false
	return &types.MsgClearStolenResponse{}, k.Jewel.Set(ctx, j.Id, j)
}
```

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./x/jewel/keeper/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add x/jewel/keeper
git commit -m "x/jewel: offers, claim keys bound to the claimer, and the stolen flag"
```

---

### Task 6: Benefits and owner-signed vouchers

**Files:**
- Create: `x/jewel/keeper/msg_server_benefit.go`
- Test: `x/jewel/keeper/benefit_test.go`

**Interfaces:**
- Produces: handlers `AttachBenefit`, `RedeemBenefit`; test helpers `f.sellTo(t, staff, id, priv) string`, `f.voucher(t, priv, benefitID, jewelID, nonce, ttl) *types.MsgRedeemBenefit`.

- [ ] **Step 1: Write the failing tests**

`x/jewel/keeper/benefit_test.go`:

```go
package keeper_test

import (
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

// sellTo sells a jewel from stock to the account of a known key, so the test
// can sign vouchers as the owner.
func (f *fixture) sellTo(t *testing.T, staff string, id uint64, priv *secp256k1.PrivKey) string {
	t.Helper()
	buyer, err := f.env.AddressCodec.BytesToString(sdk.AccAddress(priv.PubKey().Address()))
	require.NoError(t, err)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: staff, JewelId: id, To: buyer})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.NoError(t, err)
	return buyer
}

// voucher builds the redemption a staff member submits after scanning the
// owner's code.
func (f *fixture) voucher(t *testing.T, staff string, priv *secp256k1.PrivKey, benefitID, jewelID uint64, nonce string, ttl time.Duration) *types.MsgRedeemBenefit {
	t.Helper()
	v := types.Voucher{BenefitId: benefitID, JewelId: jewelID, Nonce: nonce,
		ExpiresAt: f.env.Ctx.BlockTime().Add(ttl).Unix()}
	sig, err := priv.Sign(types.VoucherSignBytes(f.env.Ctx.ChainID(), v))
	require.NoError(t, err)
	return &types.MsgRedeemBenefit{Staff: staff, Voucher: v, OwnerPubKey: priv.PubKey().Bytes(), OwnerSignature: sig}
}

func (f *fixture) cleaning(t *testing.T, staff string, jewelID uint64, allowance uint32, period time.Duration) uint64 {
	t.Helper()
	res, err := f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: staff, JewelId: jewelID,
		Kind: "cleaning", Allowance: allowance, Period: int64(period / time.Second)})
	require.NoError(t, err)
	return res.Id
}

func TestTheOwnerConsentsToEachUse(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	bid := f.cleaning(t, staff, id, 2, 365*24*time.Hour)

	_, err := f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "n1", 4*time.Minute))
	require.NoError(t, err)
	b, _ := f.k.Benefit.Get(f.env.Ctx, bid)
	require.Len(t, b.Uses, 1)
}

func TestStaffCannotSpendABenefitWithoutTheOwner(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	f.sellTo(t, staff, id, secp256k1.GenPrivKey())
	bid := f.cleaning(t, staff, id, 2, 0)

	impostor := secp256k1.GenPrivKey()
	_, err := f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, impostor, bid, id, "n1", time.Minute))
	require.ErrorIs(t, err, types.ErrBadVoucher)
}

func TestAVoucherIsSpentOnceAndLivesFiveMinutesAtMost(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	bid := f.cleaning(t, staff, id, 5, 0)

	v := f.voucher(t, staff, owner, bid, id, "n1", time.Minute)
	_, err := f.srv.RedeemBenefit(f.env.Ctx, v)
	require.NoError(t, err)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, v)
	require.ErrorIs(t, err, types.ErrVoucherUsed)

	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "n2", 10*time.Minute))
	require.ErrorIs(t, err, types.ErrBadVoucher, "a voucher valid for ten minutes is refused")

	old := f.voucher(t, staff, owner, bid, id, "n3", time.Minute)
	f.advance(2 * time.Minute)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, old)
	require.ErrorIs(t, err, types.ErrBadVoucher, "an expired voucher is refused")
}

func TestTheAllowanceIsPerRollingPeriod(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	year := 365 * 24 * time.Hour
	bid := f.cleaning(t, staff, id, 2, year)

	for _, n := range []string{"a", "b"} {
		_, err := f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, n, time.Minute))
		require.NoError(t, err)
	}
	_, err := f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "c", time.Minute))
	require.ErrorIs(t, err, types.ErrAllowanceUsed)

	f.advance(year + time.Hour)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "d", time.Minute))
	require.NoError(t, err)
}

// The benefit follows the jewel: the new owner's voucher works, the old
// owner's does not.
func TestABenefitPassesToTheNextOwner(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	first := secp256k1.GenPrivKey()
	firstAddr := f.sellTo(t, staff, id, first)
	bid := f.cleaning(t, staff, id, 5, 0)

	second := secp256k1.GenPrivKey()
	secondAddr, _ := f.env.AddressCodec.BytesToString(sdk.AccAddress(second.PubKey().Address()))
	_, err := f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: firstAddr, JewelId: id, To: secondAddr})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: secondAddr, JewelId: id})
	require.NoError(t, err)

	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, first, bid, id, "old", time.Minute))
	require.ErrorIs(t, err, types.ErrBadVoucher)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, second, bid, id, "new", time.Minute))
	require.NoError(t, err)
}

func TestASuspendedJewelersBenefitsAreNotHonoured(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	bid := f.cleaning(t, staff, id, 5, 0)
	_, err := f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "closed"})
	require.NoError(t, err)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "n", time.Minute))
	require.ErrorIs(t, err, types.ErrJewelerSuspended)
}

func TestOnlyTheIssuingJewelerAttachesBenefits(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	_, _, other := f.admit(t, "Atelier Ba")
	id := f.register(t, staff, "MD-0001")
	_, err := f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: other, JewelId: id, Kind: "cleaning", Allowance: 1})
	require.ErrorIs(t, err, types.ErrNotStaff)
	_, err = f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: staff, JewelId: id, Kind: "", Allowance: 1})
	require.ErrorIs(t, err, types.ErrInvalidBenefit)
	_, err = f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: staff, JewelId: id, Kind: "cleaning", Allowance: 0})
	require.ErrorIs(t, err, types.ErrInvalidBenefit)
}

func TestAnExpiredBenefitIsRefused(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	res, err := f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: staff, JewelId: id, Kind: "resizing",
		Allowance: 1, ExpiresAt: f.env.Ctx.BlockTime().Add(time.Hour).Unix()})
	require.NoError(t, err)
	f.advance(2 * time.Hour)
	_, err = f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, res.Id, id, "n", time.Minute))
	require.ErrorIs(t, err, types.ErrBenefitExpired)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./x/jewel/keeper/... -run 'Benefit|Voucher|Allowance|Consent|Spend'`
Expected: FAIL with `method AttachBenefit not implemented`.

- [ ] **Step 3: Write the handlers**

`x/jewel/keeper/msg_server_benefit.go`:

```go
package keeper

import (
	"context"
	"strings"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/jewel/types"
)

// AttachBenefit adds a benefit to a jewel. Its own jeweler's staff only.
func (k msgServer) AttachBenefit(ctx context.Context, req *types.MsgAttachBenefit) (*types.MsgAttachBenefitResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	jr, err := k.requireStaff(ctx, req.Staff, j.JewelerId)
	if err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(req.Kind)
	now := k.now(ctx)
	if kind == "" || len(kind) > 80 || req.Allowance == 0 || req.Period < 0 ||
		(req.ExpiresAt != 0 && req.ExpiresAt <= now) {
		return nil, types.ErrInvalidBenefit
	}
	id, err := nextID(ctx, k.NextBenefitID)
	if err != nil {
		return nil, err
	}
	b := types.Benefit{Id: id, JewelId: j.Id, JewelerId: jr.Id, Kind: kind,
		Allowance: req.Allowance, Period: req.Period, ExpiresAt: req.ExpiresAt}
	if err := k.Benefit.Set(ctx, id, b); err != nil {
		return nil, err
	}
	if err := k.BenefitsByJewel.Set(ctx, collections.Join(j.Id, id)); err != nil {
		return nil, err
	}
	return &types.MsgAttachBenefitResponse{Id: id}, nil
}

// RedeemBenefit records one use. Staff submit it; the owner's signature on
// the voucher is the consent.
func (k msgServer) RedeemBenefit(ctx context.Context, req *types.MsgRedeemBenefit) (*types.MsgRedeemBenefitResponse, error) {
	v := req.Voucher
	b, err := k.Benefit.Get(ctx, v.BenefitId)
	if isNotFound(err) {
		return nil, types.ErrNoBenefit.Wrapf("%d", v.BenefitId)
	}
	if err != nil {
		return nil, err
	}
	if _, err := k.requireStaff(ctx, req.Staff, b.JewelerId); err != nil {
		return nil, err
	}
	now := k.now(ctx)
	if b.ExpiresAt != 0 && now >= b.ExpiresAt {
		return nil, types.ErrBenefitExpired
	}
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if v.JewelId != b.JewelId || v.Nonce == "" || len(v.Nonce) > 64 {
		return nil, types.ErrBadVoucher
	}
	if v.ExpiresAt <= now || v.ExpiresAt > now+p.VoucherMaxTtl {
		return nil, types.ErrBadVoucher.Wrap("the code has expired, or was made to last longer than allowed")
	}
	nonceKey := collections.Join(b.Id, v.Nonce)
	if used, err := k.UsedNonce.Has(ctx, nonceKey); err != nil {
		return nil, err
	} else if used {
		return nil, types.ErrVoucherUsed
	}
	j, err := k.getJewel(ctx, b.JewelId)
	if err != nil {
		return nil, err
	}
	if j.Stolen {
		return nil, types.ErrStolen.Wrapf("jewel %d", j.Id)
	}
	signer, ok := types.VerifySecp256k1(req.OwnerPubKey, types.VoucherSignBytes(k.chainID(ctx), v), req.OwnerSignature)
	if !ok {
		return nil, types.ErrBadVoucher
	}
	signerStr, err := k.addressCodec.BytesToString(signer)
	if err != nil {
		return nil, err
	}
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	if signerStr != owner {
		return nil, types.ErrBadVoucher.Wrap("not signed by the jewel's owner")
	}
	used := 0
	for _, u := range b.Uses {
		if b.Period == 0 || u > now-b.Period {
			used++
		}
	}
	if used >= int(b.Allowance) {
		return nil, types.ErrAllowanceUsed
	}
	b.Uses = append(b.Uses, now)
	if err := k.Benefit.Set(ctx, b.Id, b); err != nil {
		return nil, err
	}
	return &types.MsgRedeemBenefitResponse{}, k.UsedNonce.Set(ctx, nonceKey)
}
```

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./x/jewel/keeper/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add x/jewel/keeper
git commit -m "x/jewel: benefits travel with the jewel; each use needs the owner's signed code"
```

---

### Task 7: Reassignment, and the queries

**Files:**
- Create: `x/jewel/keeper/msg_server_reassign.go`
- Modify: `x/jewel/keeper/query.go` (add four RPCs, drop `UnimplementedQueryServer`)
- Modify: `x/jewel/module/autocli.go` (add the query commands)
- Test: `x/jewel/keeper/reassign_test.go`, `x/jewel/keeper/query_test.go`

**Interfaces:**
- Produces: handlers `ProposeReassignment`, `VetoReassignment`, `CompleteReassignment`; queries `Jeweler`, `Jewelers`, `Jewel`, `JewelsByInscription`.

- [ ] **Step 1: Write the failing tests**

`x/jewel/keeper/reassign_test.go`:

```go
package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

const month = 30 * 24 * time.Hour

func TestALostKeyIsReassignedAfterAPublicWindow(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	f.sell(t, staff, id)
	_, newAccount := f.env.Addr(t)
	_, anyone := f.env.Addr(t)

	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id, To: newAccount})
	require.ErrorIs(t, err, types.ErrNoReason)
	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: newAccount, Reason: "identity checked at Maison Diop, 2026-11-20"})
	require.NoError(t, err)

	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: anyone, JewelId: id})
	require.ErrorIs(t, err, types.ErrWindowOpen)

	f.advance(month + time.Second)
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: anyone, JewelId: id})
	require.NoError(t, err, "anyone completes it, so the office cannot hold it hostage")
	require.Equal(t, newAccount, f.owner(t, id))
	require.Equal(t, uint32(2), f.jewel(t, id).OwnerCount)
}

func TestTheOwnerVetoesAReassignment(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, claimant := f.env.Addr(t)
	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: claimant, Reason: "claimant says the key is lost"})
	require.NoError(t, err)

	_, err = f.srv.VetoReassignment(f.env.Ctx, &types.MsgVetoReassignment{Owner: claimant, JewelId: id})
	require.ErrorIs(t, err, types.ErrNotOwner)
	_, err = f.srv.VetoReassignment(f.env.Ctx, &types.MsgVetoReassignment{Owner: owner, JewelId: id})
	require.NoError(t, err)

	f.advance(month + time.Second)
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: claimant, JewelId: id})
	require.ErrorIs(t, err, types.ErrNoReassignment)
	require.Equal(t, owner, f.owner(t, id))
}

func TestASaleDuringTheWindowVoidsTheReassignment(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, claimant := f.env.Addr(t)
	_, buyer := f.env.Addr(t)
	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: claimant, Reason: "r"})
	require.NoError(t, err)
	_, err = f.srv.OfferJewel(f.env.Ctx, &types.MsgOfferJewel{Owner: owner, JewelId: id, To: buyer})
	require.NoError(t, err)
	_, err = f.srv.AcceptOffer(f.env.Ctx, &types.MsgAcceptOffer{Recipient: buyer, JewelId: id})
	require.NoError(t, err)

	f.advance(month + time.Second)
	_, err = f.srv.CompleteReassignment(f.env.Ctx, &types.MsgCompleteReassignment{Creator: claimant, JewelId: id})
	require.ErrorIs(t, err, types.ErrReassignmentStale)

	// A stale proposal does not block a fresh one.
	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id,
		To: claimant, Reason: "r2"})
	require.NoError(t, err)
}

func TestOnlyTheOfficeProposes(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	_, err := f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: staff, JewelId: id,
		To: staff, Reason: "r"})
	require.ErrorIs(t, err, types.ErrNotOffice)
}
```

`x/jewel/keeper/query_test.go`:

```go
package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

func TestTheVerificationPageGetsEverythingInOneQuery(t *testing.T) {
	f := setup(t)
	_, _, staff := f.admit(t, "Maison Diop")
	id := f.register(t, staff, "MD-0001")
	owner := f.sell(t, staff, id)
	_, err := f.srv.AttachBenefit(f.env.Ctx, &types.MsgAttachBenefit{Staff: staff, JewelId: id, Kind: "cleaning", Allowance: 2})
	require.NoError(t, err)

	res, err := f.qs.Jewel(f.env.Ctx, &types.QueryJewelRequest{Id: id})
	require.NoError(t, err)
	require.Equal(t, "MD0001", res.Jewel.Inscription)
	require.Equal(t, owner, res.Owner)
	require.Nil(t, res.Offer)
	require.Nil(t, res.Reassignment)
	require.Len(t, res.Benefits, 1)

	_, err = f.qs.Jewel(f.env.Ctx, &types.QueryJewelRequest{Id: 999})
	require.ErrorIs(t, err, types.ErrNoJewel)
}

func TestASearchByInscriptionFindsEveryJewelerUsingIt(t *testing.T) {
	f := setup(t)
	_, _, a := f.admit(t, "Maison Diop")
	_, _, b := f.admit(t, "Atelier Ba")
	f.register(t, a, "RING-0001")
	f.register(t, b, "ring 0001")
	f.register(t, b, "RING-0002")

	res, err := f.qs.JewelsByInscription(f.env.Ctx, &types.QueryJewelsByInscriptionRequest{Inscription: "ring-0001"})
	require.NoError(t, err)
	require.Len(t, res.Jewels, 2)

	_, err = f.qs.JewelsByInscription(f.env.Ctx, &types.QueryJewelsByInscriptionRequest{Inscription: "#"})
	require.ErrorIs(t, err, types.ErrInvalidInscription)
}

func TestJewelersAreListedSuspendedOnesIncluded(t *testing.T) {
	f := setup(t)
	id, _, _ := f.admit(t, "Maison Diop")
	f.admit(t, "Atelier Ba")
	_, err := f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: id, Reason: "r"})
	require.NoError(t, err)

	res, err := f.qs.Jewelers(f.env.Ctx, &types.QueryJewelersRequest{})
	require.NoError(t, err)
	require.Len(t, res.Jewelers, 2)

	one, err := f.qs.Jeweler(f.env.Ctx, &types.QueryJewelerRequest{Id: id})
	require.NoError(t, err)
	require.Equal(t, types.JEWELER_SUSPENDED, one.Jeweler.Status)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./x/jewel/keeper/... -run 'Reassign|Veto|Window|Propose|Verification|Search|Jewelers'`
Expected: FAIL with `not implemented`.

- [ ] **Step 3: Write the reassignment handlers**

`x/jewel/keeper/msg_server_reassign.go`:

```go
package keeper

import (
	"context"
	"strings"

	"yamale/blockchain/x/jewel/types"
)

// ProposeReassignment opens the public window. A proposal left stale by a
// change of owner is replaced rather than blocking.
func (k msgServer) ProposeReassignment(ctx context.Context, req *types.MsgProposeReassignment) (*types.MsgProposeReassignmentResponse, error) {
	if err := k.requireOffice(ctx, req.Office); err != nil {
		return nil, err
	}
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if _, err := k.addressCodec.StringToBytes(req.To); err != nil {
		return nil, types.ErrInvalidAddress.Wrapf("%s", req.To)
	}
	if strings.TrimSpace(req.Reason) == "" {
		return nil, types.ErrNoReason
	}
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	if req.To == owner {
		return nil, types.ErrSelfOffer
	}
	if existing, err := k.Reassignment.Get(ctx, j.Id); err == nil && existing.From == owner {
		return nil, types.ErrReassignmentOpen
	} else if err != nil && !isNotFound(err) {
		return nil, err
	}
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	r := types.Reassignment{JewelId: j.Id, From: owner, To: req.To, Reason: req.Reason,
		WindowEnds: k.now(ctx) + p.ReassignmentWindow}
	if err := k.Reassignment.Set(ctx, j.Id, r); err != nil {
		return nil, err
	}
	return &types.MsgProposeReassignmentResponse{WindowEnds: r.WindowEnds}, nil
}

// VetoReassignment closes it. The current owner's key only: a jewel's stock
// is never reassigned, so staff have no say here.
func (k msgServer) VetoReassignment(ctx context.Context, req *types.MsgVetoReassignment) (*types.MsgVetoReassignmentResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	if has, err := k.Reassignment.Has(ctx, j.Id); err != nil {
		return nil, err
	} else if !has {
		return nil, types.ErrNoReassignment
	}
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	if req.Owner != owner {
		return nil, types.ErrNotOwner.Wrapf("%s", req.Owner)
	}
	return &types.MsgVetoReassignmentResponse{}, k.Reassignment.Remove(ctx, j.Id)
}

// CompleteReassignment moves the jewel once the window has passed. Anyone may
// call it, so the office cannot hold a reassignment hostage.
func (k msgServer) CompleteReassignment(ctx context.Context, req *types.MsgCompleteReassignment) (*types.MsgCompleteReassignmentResponse, error) {
	j, err := k.getJewel(ctx, req.JewelId)
	if err != nil {
		return nil, err
	}
	r, err := k.Reassignment.Get(ctx, j.Id)
	if isNotFound(err) {
		return nil, types.ErrNoReassignment
	}
	if err != nil {
		return nil, err
	}
	if k.now(ctx) < r.WindowEnds {
		return nil, types.ErrWindowOpen
	}
	owner, err := k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	if owner != r.From {
		return nil, types.ErrReassignmentStale
	}
	if err := k.Reassignment.Remove(ctx, j.Id); err != nil {
		return nil, err
	}
	return &types.MsgCompleteReassignmentResponse{}, k.transfer(ctx, &j, r.To)
}
```

- [ ] **Step 4: Write the queries**

Replace `x/jewel/keeper/query.go` with:

```go
package keeper

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/cosmos-sdk/types/query"

	"yamale/blockchain/x/jewel/types"
)

type queryServer struct{ k Keeper }

// NewQueryServerImpl returns the register's Query service.
func NewQueryServerImpl(k Keeper) types.QueryServer { return queryServer{k: k} }

var _ types.QueryServer = queryServer{}

func (q queryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	p, err := q.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: p}, nil
}

func (q queryServer) Jeweler(ctx context.Context, req *types.QueryJewelerRequest) (*types.QueryJewelerResponse, error) {
	j, err := q.k.getJeweler(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &types.QueryJewelerResponse{Jeweler: j}, nil
}

func (q queryServer) Jewelers(ctx context.Context, req *types.QueryJewelersRequest) (*types.QueryJewelersResponse, error) {
	jewelers, page, err := query.CollectionPaginate(ctx, q.k.Jeweler, req.Pagination,
		func(_ uint64, j types.Jeweler) (types.Jeweler, error) { return j, nil })
	if err != nil {
		return nil, err
	}
	return &types.QueryJewelersResponse{Jewelers: jewelers, Pagination: page}, nil
}

func (q queryServer) Jewel(ctx context.Context, req *types.QueryJewelRequest) (*types.QueryJewelResponse, error) {
	j, err := q.k.getJewel(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	owner, err := q.k.Owner(ctx, j)
	if err != nil {
		return nil, err
	}
	res := &types.QueryJewelResponse{Jewel: j, Owner: owner}
	if o, err := q.k.Offer.Get(ctx, j.Id); err == nil {
		res.Offer = &o
	} else if !isNotFound(err) {
		return nil, err
	}
	if r, err := q.k.Reassignment.Get(ctx, j.Id); err == nil {
		res.Reassignment = &r
	} else if !isNotFound(err) {
		return nil, err
	}
	err = q.k.BenefitsByJewel.Walk(ctx, collections.NewPrefixedPairRange[uint64, uint64](j.Id),
		func(key collections.Pair[uint64, uint64]) (bool, error) {
			b, err := q.k.Benefit.Get(ctx, key.K2())
			if err != nil {
				return true, err
			}
			res.Benefits = append(res.Benefits, b)
			return false, nil
		})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (q queryServer) JewelsByInscription(ctx context.Context, req *types.QueryJewelsByInscriptionRequest) (*types.QueryJewelsByInscriptionResponse, error) {
	code, err := types.NormaliseInscription(req.Inscription)
	if err != nil {
		return nil, err
	}
	res := &types.QueryJewelsByInscriptionResponse{}
	err = q.k.Search.Walk(ctx, collections.NewPrefixedPairRange[string, uint64](code),
		func(key collections.Pair[string, uint64]) (bool, error) {
			j, err := q.k.Jewel.Get(ctx, key.K2())
			if err != nil {
				return true, err
			}
			res.Jewels = append(res.Jewels, j)
			return len(res.Jewels) >= types.MaxSearchResults, nil
		})
	if err != nil {
		return nil, err
	}
	return res, nil
}
```

Add to the `Query` block of `x/jewel/module/autocli.go`:

```go
				{RpcMethod: "Jeweler", Use: "jeweler [id]", Short: "Shows one jeweler: name, business account, staff and status",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "Jewelers", Use: "jewelers", Short: "Lists the jewelers, suspended ones included"},
				{RpcMethod: "Jewel", Use: "jewel [id]", Short: "What the verification page shows: the record, the owner, any open offer, pending reassignment and benefits",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "id"}}},
				{RpcMethod: "JewelsByInscription", Use: "inscription [code]", Short: "Finds the jewels carrying an inscription, as read through a loupe",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "inscription"}}},
```

- [ ] **Step 5: Run them to see them pass**

Run: `go test ./x/jewel/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add x/jewel
git commit -m "x/jewel: reassignment after a public veto window; the verification queries"
```

---

### Task 8: Genesis

**Files:**
- Modify: `x/jewel/types/genesis.go` (full `Validate`)
- Modify: `x/jewel/keeper/genesis.go` (full Init and Export)
- Test: `x/jewel/types/genesis_test.go`, `x/jewel/keeper/genesis_test.go`

**Interfaces:**
- Produces: `(GenesisState) Validate() error`; `(Keeper) InitGenesis(ctx, types.GenesisState) error`; `(Keeper) ExportGenesis(ctx) (*types.GenesisState, error)`.

- [ ] **Step 1: Write the failing tests**

`x/jewel/types/genesis_test.go`:

```go
package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/types"
)

func validGenesis() types.GenesisState {
	gs := *types.DefaultGenesis()
	gs.Jewelers = []types.Jeweler{{Id: 1, Name: "Maison Diop", Business: "b1", Staff: []string{"s1"}, Status: types.JEWELER_ACTIVE}}
	gs.Jewels = []types.Jewel{{Id: 1, JewelerId: 1, Inscription: "MD0001"}}
	gs.Benefits = []types.Benefit{{Id: 1, JewelId: 1, JewelerId: 1, Kind: "cleaning", Allowance: 1}}
	gs.UsedNonces = []types.UsedNonce{{BenefitId: 1, Nonce: "n"}}
	gs.Offers = []types.Offer{{JewelId: 1, From: "b1", To: "c1", ExpiresAt: 10}}
	gs.NextJewelerId, gs.NextJewelId, gs.NextBenefitId = 2, 2, 2
	return gs
}

func TestAGenesisTheModuleCouldHaveWrittenValidates(t *testing.T) {
	require.NoError(t, types.DefaultGenesis().Validate())
	gs := validGenesis()
	require.NoError(t, gs.Validate())
}

func TestAGenesisThatBreaksAGuaranteeIsRefused(t *testing.T) {
	for name, mutate := range map[string]func(*types.GenesisState){
		"counter behind a jeweler":    func(g *types.GenesisState) { g.NextJewelerId = 1 },
		"counter behind a jewel":      func(g *types.GenesisState) { g.NextJewelId = 1 },
		"counter behind a benefit":    func(g *types.GenesisState) { g.NextBenefitId = 1 },
		"jeweler zero":                func(g *types.GenesisState) { g.Jewelers[0].Id = 0 },
		"business shared":             func(g *types.GenesisState) { g.Jewelers = append(g.Jewelers, types.Jeweler{Id: 2, Business: "b1"}); g.NextJewelerId = 3 },
		"staff on two jewelers":       func(g *types.GenesisState) { g.Jewelers = append(g.Jewelers, types.Jeweler{Id: 2, Business: "b2", Staff: []string{"s1"}}); g.NextJewelerId = 3 },
		"jewel of an unknown jeweler": func(g *types.GenesisState) { g.Jewels[0].JewelerId = 9 },
		"inscription not normalised":  func(g *types.GenesisState) { g.Jewels[0].Inscription = "md-0001" },
		"inscription twice":           func(g *types.GenesisState) { g.Jewels = append(g.Jewels, types.Jewel{Id: 2, JewelerId: 1, Inscription: "MD0001"}); g.NextJewelId = 3 },
		"offer with both targets":     func(g *types.GenesisState) { g.Offers[0].ClaimKey = "ab" },
		"offer on no jewel":           func(g *types.GenesisState) { g.Offers[0].JewelId = 9 },
		"benefit on no jewel":         func(g *types.GenesisState) { g.Benefits[0].JewelId = 9 },
		"nonce of no benefit":         func(g *types.GenesisState) { g.UsedNonces[0].BenefitId = 9 },
		"reassignment of no jewel":    func(g *types.GenesisState) { g.Reassignments = []types.Reassignment{{JewelId: 9}} },
	} {
		gs := validGenesis()
		mutate(&gs)
		require.Error(t, gs.Validate(), name)
	}
}
```

`x/jewel/keeper/genesis_test.go`:

```go
package keeper_test

import (
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/require"

	"yamale/blockchain/x/jewel/keeper"
	"yamale/blockchain/x/jewel/types"
)

// The register exported and imported again must export identically, byte for
// byte, or the import is not the same register.
func TestTheRegisterSurvivesExportAndImport(t *testing.T) {
	f := setup(t)
	jid, _, staff := f.admit(t, "Maison Diop")
	f.admit(t, "Atelier Ba")
	id := f.register(t, staff, "MD-0001")
	f.register(t, staff, "MD-0002")
	owner := secp256k1.GenPrivKey()
	f.sellTo(t, staff, id, owner)
	bid := f.cleaning(t, staff, id, 2, 0)
	_, err := f.srv.RedeemBenefit(f.env.Ctx, f.voucher(t, staff, owner, bid, id, "n1", time.Minute))
	require.NoError(t, err)
	_, other := f.env.Addr(t)
	_, err = f.srv.ProposeReassignment(f.env.Ctx, &types.MsgProposeReassignment{Office: f.office, JewelId: id, To: other, Reason: "r"})
	require.NoError(t, err)
	_, err = f.srv.SuspendJeweler(f.env.Ctx, &types.MsgSuspendJeweler{Office: f.office, JewelerId: jid, Reason: "r"})
	require.NoError(t, err)

	first, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.NoError(t, first.Validate())
	firstJSON, err := f.env.Codec.MarshalJSON(first)
	require.NoError(t, err)

	g := setup(t)
	k2 := keeper.NewKeeper(g.env.StoreService, g.env.Codec, g.env.AddressCodec, g.env.Authority, nil, g.nft, g.circuit)
	require.NoError(t, k2.InitGenesis(g.env.Ctx, *first))
	second, err := k2.ExportGenesis(g.env.Ctx)
	require.NoError(t, err)
	secondJSON, err := g.env.Codec.MarshalJSON(second)
	require.NoError(t, err)
	require.JSONEq(t, string(firstJSON), string(secondJSON))

	// The indexes were rebuilt, not just the records.
	got, err := k2.ByInscription.Has(g.env.Ctx, collectionsJoin(jid, "MD0001"))
	require.NoError(t, err)
	require.True(t, got)
	has, err := k2.ByStaff.Has(g.env.Ctx, staff)
	require.NoError(t, err)
	require.True(t, has)

	// And the counters continue rather than restart.
	next, err := k2.NextJewelID.Peek(g.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, first.NextJewelId, next)
}
```

Add at the bottom of the same file:

```go
func collectionsJoin(a uint64, b string) collections.Pair[uint64, string] { return collections.Join(a, b) }
```

and `"cosmossdk.io/collections"` to its imports.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./x/jewel/... -run 'Genesis|Survives'`
Expected: FAIL: `TestAGenesisThatBreaksAGuaranteeIsRefused` reports mutations that validate, and the round trip loses jewelers.

- [ ] **Step 3: Write Validate**

Replace `x/jewel/types/genesis.go`:

```go
package types

import "fmt"

// DefaultGenesis returns an empty register with the default settings.
func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

// Validate refuses a genesis the module could not have produced. The counter
// checks matter most: a counter at or below an existing id would issue it
// again, and the second record would silently overwrite the first.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	jewelers := map[uint64]bool{}
	businesses := map[string]uint64{}
	staff := map[string]uint64{}
	for _, j := range gs.Jewelers {
		if j.Id == 0 || j.Id >= gs.NextJewelerId {
			return fmt.Errorf("jeweler %d is zero or not below next_jeweler_id %d", j.Id, gs.NextJewelerId)
		}
		if prev, ok := businesses[j.Business]; ok {
			return fmt.Errorf("jewelers %d and %d share the business account %s", prev, j.Id, j.Business)
		}
		businesses[j.Business] = j.Id
		for _, s := range j.Staff {
			if prev, ok := staff[s]; ok {
				return fmt.Errorf("%s is staff of jewelers %d and %d", s, prev, j.Id)
			}
			staff[s] = j.Id
		}
		jewelers[j.Id] = true
	}

	jewels := map[uint64]bool{}
	inscriptions := map[string]uint64{}
	for _, j := range gs.Jewels {
		if j.Id == 0 || j.Id >= gs.NextJewelId {
			return fmt.Errorf("jewel %d is zero or not below next_jewel_id %d", j.Id, gs.NextJewelId)
		}
		if !jewelers[j.JewelerId] {
			return fmt.Errorf("jewel %d names jeweler %d, which is not in this genesis", j.Id, j.JewelerId)
		}
		code, err := NormaliseInscription(j.Inscription)
		if err != nil || code != j.Inscription {
			return fmt.Errorf("jewel %d's inscription %q is not in normalised form", j.Id, j.Inscription)
		}
		key := fmt.Sprintf("%d/%s", j.JewelerId, code)
		if prev, ok := inscriptions[key]; ok {
			return fmt.Errorf("jewels %d and %d share inscription %s at jeweler %d", prev, j.Id, code, j.JewelerId)
		}
		inscriptions[key] = j.Id
		jewels[j.Id] = true
	}

	for _, o := range gs.Offers {
		if !jewels[o.JewelId] {
			return fmt.Errorf("an offer names jewel %d, which is not in this genesis", o.JewelId)
		}
		if (o.To == "") == (o.ClaimKey == "") {
			return fmt.Errorf("the offer on jewel %d must name exactly one of an account and a claim key", o.JewelId)
		}
	}

	benefits := map[uint64]bool{}
	for _, b := range gs.Benefits {
		if b.Id == 0 || b.Id >= gs.NextBenefitId {
			return fmt.Errorf("benefit %d is zero or not below next_benefit_id %d", b.Id, gs.NextBenefitId)
		}
		if !jewels[b.JewelId] || !jewelers[b.JewelerId] {
			return fmt.Errorf("benefit %d names a jewel or jeweler not in this genesis", b.Id)
		}
		benefits[b.Id] = true
	}
	for _, n := range gs.UsedNonces {
		if !benefits[n.BenefitId] {
			return fmt.Errorf("a used voucher names benefit %d, which is not in this genesis", n.BenefitId)
		}
	}
	for _, r := range gs.Reassignments {
		if !jewels[r.JewelId] {
			return fmt.Errorf("a reassignment names jewel %d, which is not in this genesis", r.JewelId)
		}
	}
	return nil
}
```

- [ ] **Step 4: Write Init and Export**

Replace `x/jewel/keeper/genesis.go`:

```go
package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"yamale/blockchain/x/jewel/types"
)

// InitGenesis writes the register in and rebuilds every index from the
// records. The indexes are not exported: they are functions of the records,
// and two copies of one fact eventually disagree.
func (k Keeper) InitGenesis(ctx context.Context, gs types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, j := range gs.Jewelers {
		if err := k.Jeweler.Set(ctx, j.Id, j); err != nil {
			return err
		}
		if err := k.ByBusiness.Set(ctx, j.Business, j.Id); err != nil {
			return err
		}
		for _, s := range j.Staff {
			if err := k.ByStaff.Set(ctx, s, j.Id); err != nil {
				return err
			}
		}
	}
	for _, j := range gs.Jewels {
		if err := k.Jewel.Set(ctx, j.Id, j); err != nil {
			return err
		}
		if err := k.ByInscription.Set(ctx, collections.Join(j.JewelerId, j.Inscription), j.Id); err != nil {
			return err
		}
		if err := k.Search.Set(ctx, collections.Join(j.Inscription, j.Id)); err != nil {
			return err
		}
	}
	for _, o := range gs.Offers {
		if err := k.Offer.Set(ctx, o.JewelId, o); err != nil {
			return err
		}
	}
	for _, b := range gs.Benefits {
		if err := k.Benefit.Set(ctx, b.Id, b); err != nil {
			return err
		}
		if err := k.BenefitsByJewel.Set(ctx, collections.Join(b.JewelId, b.Id)); err != nil {
			return err
		}
	}
	for _, n := range gs.UsedNonces {
		if err := k.UsedNonce.Set(ctx, collections.Join(n.BenefitId, n.Nonce)); err != nil {
			return err
		}
	}
	for _, r := range gs.Reassignments {
		if err := k.Reassignment.Set(ctx, r.JewelId, r); err != nil {
			return err
		}
	}
	// A zero counter is not written: a fresh chain has no key there, and
	// writing one would make the imported store differ from the exported one.
	for _, s := range []struct {
		seq collections.Sequence
		v   uint64
	}{{k.NextJewelerID, gs.NextJewelerId}, {k.NextJewelID, gs.NextJewelId}, {k.NextBenefitID, gs.NextBenefitId}} {
		if s.v != 0 {
			if err := s.seq.Set(ctx, s.v); err != nil {
				return err
			}
		}
	}
	return nil
}

// ExportGenesis writes the register out, records in key order.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	p, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	gs := &types.GenesisState{Params: p}

	if err := k.Jeweler.Walk(ctx, nil, func(_ uint64, v types.Jeweler) (bool, error) {
		gs.Jewelers = append(gs.Jewelers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Jewel.Walk(ctx, nil, func(_ uint64, v types.Jewel) (bool, error) {
		gs.Jewels = append(gs.Jewels, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Offer.Walk(ctx, nil, func(_ uint64, v types.Offer) (bool, error) {
		gs.Offers = append(gs.Offers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Benefit.Walk(ctx, nil, func(_ uint64, v types.Benefit) (bool, error) {
		gs.Benefits = append(gs.Benefits, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.UsedNonce.Walk(ctx, nil, func(key collections.Pair[uint64, string]) (bool, error) {
		gs.UsedNonces = append(gs.UsedNonces, types.UsedNonce{BenefitId: key.K1(), Nonce: key.K2()})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Reassignment.Walk(ctx, nil, func(_ uint64, v types.Reassignment) (bool, error) {
		gs.Reassignments = append(gs.Reassignments, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if gs.NextJewelerId, err = k.NextJewelerID.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.NextJewelId, err = k.NextJewelID.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.NextBenefitId, err = k.NextBenefitID.Peek(ctx); err != nil {
		return nil, err
	}
	return gs, nil
}
```

- [ ] **Step 5: Run them to see them pass**

Run: `go test ./x/jewel/...`
Expected: PASS. The round trip imports into a fixture whose x/nft store is empty; that is fine, because x/jewel's genesis does not read x/nft. A failure there means `InitGenesis` or `ExportGenesis` drops a field: compare the two JSON documents in the failure output.

- [ ] **Step 6: Commit**

```bash
git add x/jewel
git commit -m "x/jewel: genesis validates the register's guarantees and round-trips exactly"
```

---

### Task 9: The router refuses a bare NFT send; CLI and reference docs

**Files:**
- Create: `app/jewel_circuit_test.go`
- Modify: `x/jewel/keeper/msg_server.go` (remove `types.UnimplementedMsgServer`)
- Modify: `x/jewel/module/autocli.go` (transaction commands)
- Generate: `docs/reference/jewel.md` and any index the generator updates

**Interfaces:**
- Consumes: the app's `CircuitBreakerKeeper` and `MsgServiceRouter()`.

- [ ] **Step 1: Write the failing app-level test**

`app/jewel_circuit_test.go`:

```go
//go:build !settlement

package app

import (
	"testing"

	"cosmossdk.io/x/nft"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	"github.com/stretchr/testify/require"

	jeweltypes "yamale/blockchain/x/jewel/types"
)

// x/nft's own send skips every rule x/jewel exists for. Disabling it in
// x/circuit must stop it at the router, which is where authz and interchain
// accounts also arrive; a check in the ante chain would see neither.
//
// The proof is the difference between two errors. With the send open, the
// router hands the message to x/nft, which answers that the sender does not
// own the token. With it disabled, the router refuses before x/nft looks.
func TestABareNFTSendIsRefusedAtTheRouterEvenInsideAuthz(t *testing.T) {
	app := newUpgradeTestApp(t)
	ctx := app.NewUncachedContext(false, cmtproto.Header{Height: 1})

	owner := sdk.AccAddress([]byte("jewel-owner-________"))
	ownerStr, err := app.AuthKeeper.AddressCodec().BytesToString(owner)
	require.NoError(t, err)
	send := &nft.MsgSend{ClassId: "jewel-1", Id: "jewel-1", Sender: ownerStr, Receiver: ownerStr}

	handler := app.MsgServiceRouter().Handler(send)
	require.NotNil(t, handler)
	const nftAnswer = "is not the owner of nft"
	_, err = handler(ctx, send)
	require.ErrorContains(t, err, nftAnswer, "control: with the send open, x/nft itself answers")

	require.NoError(t, app.CircuitBreakerKeeper.DisableList.Set(ctx, jeweltypes.NFTSendTypeURL))

	_, err = handler(ctx, send)
	require.Error(t, err)
	require.NotContains(t, err.Error(), nftAnswer, "the router must refuse before x/nft runs")

	exec := authz.NewMsgExec(owner, []sdk.Msg{send})
	_, err = app.MsgServiceRouter().Handler(&exec)(ctx, &exec)
	require.Error(t, err)
	require.NotContains(t, err.Error(), nftAnswer, "authz must not reach x/nft either")
}
```

- [ ] **Step 2: Run it**

Run: `go test ./app/... -run TestABareNFTSendIsRefusedAtTheRouter`
Expected: PASS. x/circuit installs itself on the router through depinject (`BaseappOptions` in `cosmossdk.io/x/circuit@v0.1.1/module.go`). If the disabled send still reaches x/nft, the breaker is not installed: stop and report it, because the module's central rule would not hold and the spec has to change.

- [ ] **Step 3: Remove the scaffolding and add the transaction commands**

In `x/jewel/keeper/msg_server.go`, delete the `types.UnimplementedMsgServer` line from `msgServer`. Run `go build ./...`: it must compile, which proves all 18 handlers exist.

Replace the `Tx:` block in `x/jewel/module/autocli.go` with:

```go
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: types.Msg_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{RpcMethod: "UpdateParams", Skip: true}, // governance only
				{RpcMethod: "AdmitJeweler", Use: "admit-jeweler [name] [business]", Short: "The office admits a jeweler",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "name"}, {ProtoField: "business"}}},
				{RpcMethod: "SuspendJeweler", Use: "suspend-jeweler [jeweler-id] [reason]", Short: "The office stops a jeweler acting",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jeweler_id"}, {ProtoField: "reason"}}},
				{RpcMethod: "ReinstateJeweler", Use: "reinstate-jeweler [jeweler-id]", Short: "The office lets a suspended jeweler act again",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jeweler_id"}}},
				{RpcMethod: "SetStaff", Use: "set-staff [jeweler-id]", Short: "Adds (--add) and removes (--remove) the jeweler's staff",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jeweler_id"}}},
				{RpcMethod: "RegisterJewel", Use: "register [inscription] [record-sha256] [record-uri]", Short: "Registers a jewel into the jeweler's stock",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "inscription"}, {ProtoField: "record_hash"}, {ProtoField: "record_uri"}}},
				{RpcMethod: "AmendJewel", Use: "amend [jewel-id] [record-sha256] [record-uri] [note]", Short: "Appends a correction to a jewel's record",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}, {ProtoField: "record_hash"}, {ProtoField: "record_uri"}, {ProtoField: "note"}}},
				{RpcMethod: "OfferJewel", Use: "offer [jewel-id]", Short: "Offers a jewel to an account (--to) or a claim key (--claim-key)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "CancelOffer", Use: "cancel-offer [jewel-id]", Short: "Closes the open offer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "AcceptOffer", Use: "accept [jewel-id]", Short: "Takes a jewel offered to you",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "ClaimOffer", Skip: true}, // needs a claim-key signature: tools/jewelsign
				{RpcMethod: "ReportStolen", Use: "report-stolen [jewel-id]", Short: "Flags a jewel as stolen and closes its offer",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "ClearStolen", Use: "clear-stolen [jewel-id]", Short: "Lifts the stolen flag",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "AttachBenefit", Use: "attach-benefit [jewel-id] [kind] [allowance]", Short: "Attaches a benefit (--period, --expires-at in unix seconds)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}, {ProtoField: "kind"}, {ProtoField: "allowance"}}},
				{RpcMethod: "RedeemBenefit", Skip: true}, // needs the owner's voucher signature: tools/jewelsign
				{RpcMethod: "ProposeReassignment", Use: "propose-reassignment [jewel-id] [to] [reason]", Short: "The office proposes moving a jewel whose owner lost their key",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}, {ProtoField: "to"}, {ProtoField: "reason"}}},
				{RpcMethod: "VetoReassignment", Use: "veto-reassignment [jewel-id]", Short: "The owner refuses a proposed reassignment",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
				{RpcMethod: "CompleteReassignment", Use: "complete-reassignment [jewel-id]", Short: "Anyone completes a reassignment once its window has passed",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "jewel_id"}}},
			},
		},
```

- [ ] **Step 4: Prove the CLI builds**

A field named after an SDK tx flag panics the binary at start-up, and no test constructs the CLI. Build the binary into the scratchpad and run its help:

```bash
SCRATCH="$(cygpath -u "$TEMP")/claude/<project>/<session>/scratchpad"   # the session scratchpad path
go build -o "$SCRATCH/blockchaind.exe" ./cmd/blockchaind
"$SCRATCH/blockchaind.exe" tx jewel --help
"$SCRATCH/blockchaind.exe" query jewel --help
```

Expected: both print command lists; no panic. A panic naming a flag means a proto field collides with an SDK flag: rename the field in `tx.proto`, regenerate (Task 1 Step 6), and rebuild.

- [ ] **Step 5: Regenerate the reference docs**

`docs/reference` is generated, never hand-edited. Run the body of `make docs`:

```bash
go tool github.com/bufbuild/buf/cmd/buf build --as-file-descriptor-set --exclude-imports -o .docgen-descriptor.binpb
go build -o "$SCRATCH/docgen.exe" ./tools/docgen
"$SCRATCH/docgen.exe" --descriptor .docgen-descriptor.binpb --out docs/reference --root .
rm -f .docgen-descriptor.binpb
git status --short docs/reference
```

Expected: `docs/reference/jewel.md` is new; the index may change. Read `jewel.md` once: every message must show its signer and its errors. If a message reads badly, improve the comment in the `.proto` and regenerate; do not edit the markdown.

- [ ] **Step 6: Run everything**

Run: `go test -p 1 ./x/jewel/... ./app/...` and `go build -tags settlement ./...`
Expected: PASS and a clean build. Rerun a package that fails with `fork/exec … stratégie de contrôle`.

- [ ] **Step 7: Commit**

```bash
git add app/jewel_circuit_test.go x/jewel docs/reference
git commit -m "x/jewel: the router refuses a bare NFT send, also through authz; CLI and reference"
```

---

### Task 10: Signing tool and local devnet genesis

**Files:**
- Create: `tools/jewelsign/main.go`
- Modify: `scripts/devnet/init-local-devnet.sh` (the node genesis-edit block that starts around line 293)

**Interfaces:**
- Produces: `jewelsign claimkey` → prints `<private hex> <public hex>`; `jewelsign claim <claim-private-hex> <chain-id> <jewel-id> <claimer>` → prints a `MsgClaimOffer` JSON; `jewelsign voucher <owner-private-hex> <chain-id> <staff> <benefit-id> <jewel-id> <nonce> <expires-at>` → prints a `MsgRedeemBenefit` JSON. Both JSON forms carry `@type` and base64 bytes, ready for a transaction body.

- [ ] **Step 1: Write the tool**

`tools/jewelsign/main.go`:

```go
// Command jewelsign produces the two signatures x/jewel takes that a chain
// key in a keyring cannot make through the CLI: a claim key's signature over
// the claimer's address, and an owner's consent to a benefit use.
//
// It exists for scripts/devnet/seed-jewel.sh. The real app signs both in the
// browser with the account service's key.
package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"yamale/blockchain/x/jewel/types"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "claimkey":
		k := secp256k1.GenPrivKey()
		fmt.Println(hex.EncodeToString(k.Key), hex.EncodeToString(k.PubKey().Bytes()))
	case "claim":
		need(6)
		k := key(os.Args[2])
		jewelID := u64(os.Args[4])
		sig := sign(k, types.ClaimSignBytes(os.Args[3], jewelID, os.Args[5]))
		emit(map[string]any{
			"@type": "/blockchain.jewel.v1.MsgClaimOffer", "claimer": os.Args[5],
			"jewel_id": os.Args[4], "claim_pub_key": b64(k.PubKey().Bytes()), "claim_signature": b64(sig),
		})
	case "voucher":
		need(9)
		k := key(os.Args[2])
		v := types.Voucher{BenefitId: u64(os.Args[5]), JewelId: u64(os.Args[6]), Nonce: os.Args[7], ExpiresAt: i64(os.Args[8])}
		sig := sign(k, types.VoucherSignBytes(os.Args[3], v))
		emit(map[string]any{
			"@type": "/blockchain.jewel.v1.MsgRedeemBenefit", "staff": os.Args[4],
			"voucher": map[string]any{"benefit_id": os.Args[5], "jewel_id": os.Args[6], "nonce": v.Nonce, "expires_at": os.Args[8]},
			"owner_pub_key": b64(k.PubKey().Bytes()), "owner_signature": b64(sig),
		})
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: jewelsign claimkey | claim <priv-hex> <chain-id> <jewel-id> <claimer> | voucher <priv-hex> <chain-id> <staff> <benefit-id> <jewel-id> <nonce> <expires-at>")
	os.Exit(2)
}

func need(n int) {
	if len(os.Args) != n {
		usage()
	}
}

func key(h string) *secp256k1.PrivKey {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != 32 {
		fmt.Fprintln(os.Stderr, "the private key must be 32 bytes of hex")
		os.Exit(1)
	}
	return &secp256k1.PrivKey{Key: b}
}

func sign(k *secp256k1.PrivKey, msg []byte) []byte {
	sig, err := k.Sign(msg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return sig
}

func u64(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return v
}

func i64(s string) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return v
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func emit(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Check the tool against the module's own verifier**

Run:

```bash
go build -o "$SCRATCH/jewelsign.exe" ./tools/jewelsign
"$SCRATCH/jewelsign.exe" claimkey
```

Expected: two hex strings, 64 and 66 characters. The signature rules themselves are covered by `TestSignaturesVerifyOnlyOverTheirOwnBytes` and the claim and voucher tests; Task 11 proves the tool's output against a running chain.

- [ ] **Step 3: Disable the bare send and appoint the office in the local genesis**

In `scripts/devnet/init-local-devnet.sh`, inside the node script written to `$EDIT` (the block that sets `g.app_state.group` and the constitution), add after the group assignment:

```js
// x/jewel refuses to admit a jeweler while x/nft's own send is open, so the
// devnet disables it at height zero, as the devnet-2 upgrade will.
g.app_state.circuit = g.app_state.circuit || {};
g.app_state.circuit.disabled_type_urls = [
  ...new Set([...(g.app_state.circuit.disabled_type_urls || []), "/cosmos.nft.v1beta1.MsgSend"]),
];
// The foundation's 3-of-5 is the register's office on a local devnet.
if (g.app_state.jewel) g.app_state.jewel.params.office = policy;
```

`policy` is already the third argument of that script (`const [genesisPath, ceremonyDir, policy]`).

- [ ] **Step 4: Commit**

```bash
git add tools/jewelsign scripts/devnet/init-local-devnet.sh
git commit -m "x/jewel: signing tool for claims and vouchers; local devnet closes the bare NFT send"
```

---

### Task 11: The end-to-end run on a local devnet

**Files:**
- Create: `scripts/devnet/seed-jewel.sh`

**Interfaces:**
- Consumes: `tools/jewelsign` (Task 10); the local devnet from `init-local-devnet.sh` with the foundation 3-of-5 policy as office; the `as_office` mechanics documented in `scripts/devnet/seed-herd.sh`.

- [ ] **Step 1: Write the seeder**

`scripts/devnet/seed-jewel.sh`:

```bash
#!/usr/bin/env bash
#
# Puts a jeweler, a ring and a resale through the jewel register on the local
# devnet, and shows the refusals that make it worth having.
#
# What it demonstrates, in order:
#   1. the foundation office admits "Maison Diop" (a 3-of-5 group decision)
#   2. the business adds a staff member; staff register ring MD-2026-0001
#   3. a second registration of the same inscription is refused
#   4. a bare `tx nft send` is refused at the router
#   5. staff sell the ring to a customer by email: a claim key
#   6. the customer resells it to a buyer, who accepts
#   7. a cleaning benefit is attached and used with the buyer's signed code
#   8. the buyer reports it stolen; an offer is refused; the flag is cleared
#
# Usage, against scripts/devnet/init-local-devnet.sh:
#
#   BIN=$SCRATCH/blockchaind.exe SIGN=$SCRATCH/jewelsign.exe \
#   HOME_DIR=.devnet/local/node CHAIN=yamale-local-1 NODE=tcp://127.0.0.1:26957 \
#   M1=custodian1 M2=custodian2 M3=custodian3 scripts/devnet/seed-jewel.sh
set -euo pipefail

BIN=${BIN:?the chain binary}
SIGN=${SIGN:?the jewelsign binary}
HOME_DIR=${HOME_DIR:?the node home}
CHAIN=${CHAIN:?the chain id}
NODE=${NODE:?the node RPC}
KEYRING=${KEYRING:-test}
M1=${M1:-custodian1}; M2=${M2:-custodian2}; M3=${M3:-custodian3}

TX="--keyring-backend $KEYRING --home $HOME_DIR --chain-id $CHAIN --node $NODE --gas auto --gas-adjustment 1.5 -y"
Q="--home $HOME_DIR --node $NODE -o json"

say()  { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
ok()   { printf '  \033[32mok\033[0m   %s\n' "$*"; }
bad()  { printf '  \033[31mFAIL\033[0m %s\n' "$*"; FAILED=1; }
addr() { "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR"; }
json() { grep -oE "\"$1\":[[:space:]]*\"[^\"]*\"" | head -1 | cut -d'"' -f4 || true; }
ensure_key() {
  "$BIN" keys show "$1" -a --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1 \
    || "$BIN" keys add "$1" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1
}
privhex() { yes | "$BIN" keys export "$1" --unarmored-hex --unsafe --keyring-backend "$KEYRING" --home "$HOME_DIR" 2>/dev/null | tail -1; }
FAILED=0

# A transaction's code 0 is only CheckTx. Read the block's verdict.
committed() {
  local out=$1 hash code
  hash=$(printf '%s' "$out" | json txhash)
  [ -n "$hash" ] || return 1
  for _ in $(seq 1 20); do
    code=$("$BIN" query tx "$hash" $Q 2>/dev/null | grep -oE '"code":[[:space:]]*[0-9]+' | head -1 | grep -oE '[0-9]+$' || true)
    [ -n "$code" ] && { [ "$code" = 0 ]; return; }
    sleep 1
  done
  return 1
}
run() { local out; out=$("$BIN" tx "$@" $TX -o json 2>&1) || return 1; committed "$out"; }

# Broadcast one message built by jewelsign, signed by a keyring key.
run_json() {
  local msg=$1 from=$2 body="$HOME_DIR/.jewel-tx.json" signed="$HOME_DIR/.jewel-signed.json" out
  cat > "$body" <<JSON
{"body":{"messages":[$msg],"memo":"","timeout_height":"0","extension_options":[],"non_critical_extension_options":[]},
 "auth_info":{"signer_infos":[],"fee":{"amount":[],"gas_limit":"400000","payer":"","granter":""}},"signatures":[]}
JSON
  "$BIN" tx sign "$body" --from "$from" --keyring-backend "$KEYRING" --home "$HOME_DIR" --chain-id "$CHAIN" --node "$NODE" > "$signed" || return 1
  out=$("$BIN" tx broadcast "$signed" --node "$NODE" -o json 2>&1) || return 1
  committed "$out"
}

# Act as the office: propose, both members vote, the second with --exec try.
# Success is the proposal being pruned. See seed-herd.sh for why each part.
OFFICE=$("$BIN" query jewel params $Q | json office)
[ -n "$OFFICE" ] || { echo "no office in x/jewel params; rebuild the devnet with init-local-devnet.sh" >&2; exit 1; }
as_office() {
  local body=$1 msg="$HOME_DIR/.jewel-office.json" id proposal result
  cat > "$msg" <<JSON
{"group_policy_address":"$OFFICE","messages":[$body],"metadata":"seed-jewel","proposers":["$(addr "$M1")"]}
JSON
  "$BIN" tx group submit-proposal "$msg" --from "$M1" $TX >/dev/null 2>&1 || return 1
  sleep 3
  id=$("$BIN" query group proposals-by-group-policy "$OFFICE" $Q 2>/dev/null \
    | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4 || true)
  [ -n "$id" ] || return 1
  "$BIN" tx group vote "$id" "$(addr "$M1")" VOTE_OPTION_YES "" --from "$M1" $TX >/dev/null 2>&1 || return 1
  sleep 2
  "$BIN" tx group vote "$id" "$(addr "$M2")" VOTE_OPTION_YES "" --exec try --from "$M2" $TX >/dev/null 2>&1 || return 1
  for _ in $(seq 1 20); do
    proposal=$("$BIN" query group proposal "$id" $Q 2>/dev/null || true)
    [ -n "$proposal" ] || return 0
    result=$(printf '%s' "$proposal" | json executor_result)
    case "$result" in
      PROPOSAL_EXECUTOR_RESULT_SUCCESS) return 0 ;;
      PROPOSAL_EXECUTOR_RESULT_FAILURE) return 1 ;;
    esac
    sleep 1
  done
  return 1
}

for k in jw-business jw-staff jw-customer jw-buyer; do ensure_key "$k"; done
BUSINESS=$(addr jw-business); STAFF=$(addr jw-staff); CUSTOMER=$(addr jw-customer); BUYER=$(addr jw-buyer)
for a in "$BUSINESS" "$STAFF" "$CUSTOMER" "$BUYER"; do
  run bank send "$(addr "$M3")" "$a" 10000000uyml --from "$M3" >/dev/null || true
done

say "1  the office admits Maison Diop"
as_office "{\"@type\":\"/blockchain.jewel.v1.MsgAdmitJeweler\",\"office\":\"$OFFICE\",\"name\":\"Maison Diop\",\"business\":\"$BUSINESS\"}" \
  && ok "admitted" || bad "admission"
JEWELER=$("$BIN" query jewel jewelers $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | tail -1 | cut -d'"' -f4)

say "2  staff, then a ring"
run jewel set-staff "$JEWELER" --add "$STAFF" --from jw-business && ok "staff added" || bad "staff"
HASH=9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
run jewel register "MD-2026-0001" "$HASH" "https://loyalty.yamaletech.com/r/$HASH" --from jw-staff && ok "ring registered" || bad "register"
RING=$("$BIN" query jewel inscription MD20260001 $Q | grep -oE '"id":[[:space:]]*"[0-9]+"' | head -1 | cut -d'"' -f4)

say "3  the same inscription again"
run jewel register "md 2026 0001" "$HASH" "u" --from jw-staff && bad "a duplicate was accepted" || ok "refused"

say "4  a bare x/nft send"
run nft send "jewel-$JEWELER" "jewel-$RING" "$BUYER" --from jw-business && bad "the bare send went through" || ok "refused at the router"

say "5  sold by email: a claim key"
read -r CLAIM_PRIV CLAIM_PUB < <("$SIGN" claimkey)
run jewel offer "$RING" --claim-key "$CLAIM_PUB" --from jw-staff && ok "offered to a claim key" || bad "offer"
run_json "$("$SIGN" claim "$CLAIM_PRIV" "$CHAIN" "$RING" "$CUSTOMER")" jw-customer && ok "claimed by the customer" || bad "claim"

say "6  resold, owner to owner"
run jewel offer "$RING" --to "$BUYER" --from jw-customer && ok "offered" || bad "offer"
run jewel accept "$RING" --from jw-buyer && ok "accepted" || bad "accept"

say "7  a cleaning, used with the buyer's code"
run jewel attach-benefit "$RING" cleaning 2 --period 31536000 --from jw-staff && ok "attached" || bad "attach"
BENEFIT=$("$BIN" query jewel jewel "$RING" $Q | grep -oE '"benefits":[^]]*' | grep -oE '"id":[[:space:]]*"[0-9]+"' | head -1 | cut -d'"' -f4)
EXPIRES=$(( $(date +%s) + 240 ))
VOUCHER=$("$SIGN" voucher "$(privhex jw-buyer)" "$CHAIN" "$STAFF" "$BENEFIT" "$RING" "seed-1" "$EXPIRES")
run_json "$VOUCHER" jw-staff && ok "used" || bad "redeem"
run_json "$VOUCHER" jw-staff && bad "the same code was used twice" || ok "a second use of the same code is refused"

say "8  stolen"
run jewel report-stolen "$RING" --from jw-buyer && ok "reported" || bad "report"
run jewel offer "$RING" --to "$CUSTOMER" --from jw-buyer && bad "a stolen jewel was offered" || ok "an offer is refused"
run jewel clear-stolen "$RING" --from jw-buyer && ok "cleared" || bad "clear"

say "the record"
"$BIN" query jewel jewel "$RING" $Q
[ "$FAILED" = 0 ] && echo "all stages behaved" || { echo "some stages did not behave; see FAIL above" >&2; exit 1; }
```

- [ ] **Step 2: Build, start a fresh local devnet, run the seeder**

The chain binary must be built and run in the same sitting: application control blocks a new binary's hash once it has been seen to change. Build both binaries into the scratchpad:

```bash
go build -o "$SCRATCH/blockchaind.exe" ./cmd/blockchaind
go build -o "$SCRATCH/jewelsign.exe" ./tools/jewelsign
BIN="$SCRATCH/blockchaind.exe" bash scripts/devnet/init-local-devnet.sh
```

Then, with the node running as that script leaves it:

```bash
BIN="$SCRATCH/blockchaind.exe" SIGN="$SCRATCH/jewelsign.exe" HOME_DIR=.devnet/local/node \
CHAIN=yamale-local-1 NODE=tcp://127.0.0.1:26957 M1=custodian1 M2=custodian2 M3=custodian3 \
bash scripts/devnet/seed-jewel.sh
```

Expected: every stage prints `ok`, the last line is `all stages behaved`, and the final record shows `owner_count` 2, `stolen` false, and one use on the benefit.

If a stage fails, read its transaction's raw log (`query tx <hash>`) before changing anything. If the binary cannot be started (`Permission denied`), say so in the report: the module's rules are then proven by the keeper tests only, not on a running chain.

- [ ] **Step 3: Commit**

```bash
git add scripts/devnet/seed-jewel.sh
git commit -m "x/jewel: end-to-end seeder for the local devnet"
```

---

## What the next plans cover

- **Plan 2, the app and loyalty-api:** the three faces at loyalty.yamaletech.com, sign-in through the account service, the record store, the emails, and deployment on yamale-us.
- **Plan 3, devnet-2:** a coordinated upgrade that adds `x/jewel`, writes `/cosmos.nft.v1beta1.MsgSend` into x/circuit's disable list, and has governance appoint the office. Claude prepares the proposal; the user casts the vote.
