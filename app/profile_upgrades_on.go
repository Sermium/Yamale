//go:build !settlement

package app

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	herdkeeper "yamale/blockchain/x/herd/keeper"
	herdtypes "yamale/blockchain/x/herd/types"
	jeweltypes "yamale/blockchain/x/jewel/types"
	markettypes "yamale/blockchain/x/market/types"
	selfcustodytypes "yamale/blockchain/x/selfcustody/types"
	syndicatetypes "yamale/blockchain/x/syndicate/types"
)

// The half of the herd-syndicate-selfcustody upgrade that exists when all three
// modules are linked. See profile_upgrades_off.go for the other.

var profileUpgrades = []Upgrade{
	{
		// Three new modules, one halt.
		//
		// They ship together because they are one coordinated stop and because
		// splitting them buys nothing: none of the three reads another's state,
		// so there is no ordering between them to get wrong, and three halts on
		// a chain being demonstrated is three chances to be halfway through.
		//
		// All three are new stores. A store has to be added before any module
		// migration runs, or the module's own InitGenesis — which arrives through
		// the standard migration path — writes into a store the app has not
		// mounted. That is why there is no hand-written genesis state below: the
		// modules initialise themselves, and this handler exists for the
		// assertions instead.
		Name: "herd-syndicate-selfcustody",
		StoreUpgrades: storetypes.StoreUpgrades{Added: []string{
			herdtypes.StoreKey,
			syndicatetypes.StoreKey,
			selfcustodytypes.StoreKey,
		}},
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			// The door out must actually open.
			//
			// This is the one assertion here that is not ceremonial.
			// Params.AcknowledgementHash is what every RequestDisclosure and
			// ConfirmDisclosure is checked against. If the chain publishes a hash
			// that this binary's embedded acknowledgement text does not produce,
			// MatchesPublished refuses every holder and NOBODY CAN TAKE THEIR KEY
			// AT ALL — which is precisely the failure x/selfcustody exists to
			// prevent, arriving silently at the moment the module is installed.
			//
			// Checked after the migration, because InitGenesis is what writes the
			// parameters being checked.
			params, err := app.SelfCustodyKeeper.Params.Get(ctx)
			if err != nil {
				return nil, fmt.Errorf("reading the key-disclosure parameters this upgrade just wrote: %w", err)
			}
			if want := selfcustodytypes.AcknowledgementHash(); params.AcknowledgementHash != want {
				return nil, fmt.Errorf(
					"the chain would publish acknowledgement hash %s but this binary's embedded text "+
						"hashes to %s; every key disclosure would be refused and no holder could take "+
						"their own key", params.AcknowledgementHash, want)
			}

			// x/syndicate arrives with nobody admitted to operate a platform, and
			// x/herd with no office admitted to register an animal. Both are
			// governance decisions. An upgrade that seeded either would be
			// admitting a platform, or an authority over somebody's livestock,
			// that nobody voted for — so the emptiness is asserted rather than
			// assumed, and a future change to either DefaultGenesis cannot
			// quietly install one during a halt.
			operators := 0
			if err := app.SyndicateKeeper.Operator.Walk(ctx, nil,
				func(string, syndicatetypes.Operator) (bool, error) { operators++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the syndicate operators this upgrade just wrote: %w", err)
			}
			if operators != 0 {
				return nil, fmt.Errorf(
					"x/syndicate arrived with %d operator(s) admitted; a platform that takes other "+
						"people's money is admitted by a vote, not by a migration", operators)
			}

			authorities := 0
			if err := app.HerdKeeper.Authority.Walk(ctx, nil,
				func(string, herdtypes.Authority) (bool, error) { authorities++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the herd authorities this upgrade just wrote: %w", err)
			}
			if authorities != 0 {
				return nil, fmt.Errorf(
					"x/herd arrived with %d authority/authorities admitted; a registry office is "+
						"admitted by a vote, not by a migration", authorities)
			}

			// Said out loud, in every validator's log, at the moment it becomes
			// true. The first of these changes what a bank transfer can do — a
			// deal token will not move to an address that has not been onboarded,
			// and that restriction binds on an ordinary MsgSend, through authz and
			// through a treasury alike. An operator who has not read this will
			// diagnose a refused transfer as a broken node.
			ctx.Logger().Info(
				"three modules arrived",
				"herd", "a livestock register where parentage is refused at consensus; no office is "+
					"admitted yet, so nothing can be registered until governance admits one",
				"syndicate", "private placements with escrowed commitments; transfers of a deal token "+
					"are gated by a bank send restriction from this height, so both sides must have "+
					"completed onboarding however the tokens are moved",
				"selfcustody", "the key-disclosure pathway; a holder asks, waits "+
					"the chain-enforced delay, and asks again, and the operator can neither refuse "+
					"nor accelerate it",
			)

			return vm, nil
		},
	},
	{
		// Deal papers.
		//
		// x/syndicate could state a deal's economics to the last basis point and
		// could say nothing whatever about the documents an investor is actually
		// bound by: `Deal.reference` was a free-text string and there was no
		// field for a term sheet, a subscription agreement or a shareholders'
		// agreement. A platform that is precise about a 3% fee and silent about
		// the contract is not being careful, it is being selective.
		//
		// NO STORE UPGRADE, and that is not an oversight. The documents live in
		// collections under x/syndicate's EXISTING store key, so nothing new is
		// mounted. What makes this a halt at all is the message: a validator
		// running the old binary cannot decode MsgAttachDocument and would fork
		// the moment one is included.
		//
		// NO CONSENSUS VERSION BUMP either. A bump obliges a registered
		// migration, and there is nothing to migrate: every existing deal is
		// valid with no documents attached, the new collections start empty, and
		// the id sequence starts at zero on its own. A no-op migration
		// registered only to satisfy a version number is a lie about what the
		// upgrade did.
		Name: "syndicate-documents",
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			// The store must answer, and it must answer EMPTY.
			//
			// Answering proves the new collections were built into the schema
			// rather than silently absent — a keeper field left unregistered
			// reads as a zero value and fails at the first write, in a block,
			// rather than here. Empty proves this handler did not invent a
			// document: an attestation that a paper existed is worth exactly the
			// accountability of whoever made it, and nobody attested to anything
			// during a halt.
			documents := 0
			if err := app.SyndicateKeeper.Document.Walk(ctx, nil,
				func(uint64, syndicatetypes.Document) (bool, error) { documents++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the deal documents this upgrade just mounted: %w", err)
			}
			if documents != 0 {
				return nil, fmt.Errorf(
					"x/syndicate arrived with %d document(s) attached; a document is attested by a "+
						"named operator who can be asked about it, not written during a halt", documents)
			}

			next, err := app.SyndicateKeeper.NextDocumentID.Peek(ctx)
			if err != nil {
				return nil, fmt.Errorf("reading the document id sequence: %w", err)
			}
			if next != 0 {
				return nil, fmt.Errorf("the document id sequence starts at %d, not 0", next)
			}

			ctx.Logger().Info(
				"deal documents arrived",
				"syndicate", "an operator may now attach the SHA-256 digest of a term sheet, a "+
					"subscription agreement, accounts or an exit resolution to a deal, in any state, "+
					"and supersede one with a later version",
				"never", "the documents themselves. The chain holds a digest, a kind, a title and a "+
					"location; a subscription agreement names its investor and their cheque, and that "+
					"on a public chain is a disclosure no later correction can withdraw",
				"worth", "a file produced in a dispute can be proved to be, or not to be, the one "+
					"that was attested. It proves nothing about whether the document says what its "+
					"title claims, or whether anybody signed it",
			)

			return vm, nil
		},
	},
	{
		// Tokenisation: pro-rata temporis, cost basis, covenants, sector.
		//
		// NO STORE UPGRADE. Every new collection — period rates, covenants —
		// lives under x/tokenisation's EXISTING store key, so nothing new is
		// mounted. What makes this a halt is the state shape and the message
		// set: an old binary cannot decode MsgPublishCovenant, and it would
		// compute a different entitlement for the same position.
		//
		// The migration is the reason this handler is long. It is not field
		// initialisation: a position carrying unsettled income at the halt
		// height would simply stop being owed it, because nothing after this
		// upgrade reads the index delta that produced it. See
		// MigrateToProRata.
		Name: "tokenisation-prorata",
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			vaults, positions, settled, err := app.TokenisationKeeper.MigrateToProRata(ctx)
			if err != nil {
				return nil, fmt.Errorf("migrating tokenisation to pro-rata temporis: %w", err)
			}

			// Refused rather than reported. A position left with an unset
			// amount panics every validator on its first transfer, and a halt
			// height is the only place where stopping is cheaper than
			// discovering it in a block.
			if err := app.TokenisationKeeper.AssertProRataReady(ctx); err != nil {
				return nil, fmt.Errorf("the pro-rata migration left the module unsafe: %w", err)
			}

			ctx.Logger().Info(
				"tokenisation moved to pro rata temporis",
				"vaults", vaults,
				"positions", positions,
				"settled_at_the_old_rule", settled.String(),
				"what_changed", "a distribution is now apportioned by the time each holder "+
					"actually held, not by who was on the register when it was declared; a "+
					"mid-period sale splits it between seller and buyer",
				"the_discontinuity", "the first period of every existing vehicle starts at THIS "+
					"height rather than at issuance. The chain holds no record of when balances "+
					"moved before now, so token-seconds for the past cannot be reconstructed, and "+
					"assuming everyone held from issuance would credit last week's buyer with a "+
					"year of accrual",
				"also", "positions now carry what they cost, so a return can be computed against "+
					"something rather than assumed; a tokenisation carries a sector; and an "+
					"operator can publish covenants against it — accounts, structure changes, "+
					"dividend notices, assembly notices — as digests, never as documents",
			)
			return vm, nil
		},
	},
	{
		// The secondary market.
		//
		// A new store, so it is a halt: x/market keeps its listings, auctions and
		// order book under its own key, and a store has to be mounted before any
		// module migration runs or the module’s InitGenesis writes into a store
		// the app has not got.
		//
		// It ships alone rather than alongside anything else because it is the
		// first module on this chain that WRITES to another module’s state as a
		// matter of course — every fill records a cost basis in x/tokenisation —
		// and the wiring that makes that correct is not in this handler at all.
		// It is in app.registerMarketEscrow, which runs when the binary starts.
		// That is worth a halt of its own, because the failure mode if a binary
		// were missing it is not a crash: it is a chain that trades correctly and
		// reports every holder’s cost wrong.
		//
		// It also has to come after tokenisation-prorata, because the cost basis
		// every fill writes is the field that migration creates.
		Name: "market",
		StoreUpgrades: storetypes.StoreUpgrades{Added: []string{
			markettypes.StoreKey,
		}},
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			// The store must answer, which proves the collections were built into
			// the schema rather than silently absent — an unregistered keeper field
			// reads as a zero value and fails at the first write, in a block,
			// rather than here.
			params, err := app.MarketKeeper.Params.Get(ctx)
			if err != nil {
				return nil, fmt.Errorf("reading the market params this upgrade just mounted: %w", err)
			}

			// And the bound must be usable. At zero no incoming order could ever
			// cross another: every order would rest, the book would fill with
			// orders overlapping in price that never trade, and the market would
			// LOOK like it worked — orders accepted, book deep — while being a
			// machine for escrowing property and giving it back. A market that
			// refuses orders is visibly broken; one that matches none is not.
			if params.MaxFillsPerOrder == 0 {
				return nil, fmt.Errorf(
					"x/market arrived with max_fills_per_order at zero, so no order could ever "+
						"cross another and the book would rest everything it accepted")
			}

			// Every id sequence starts at zero, which is how the module knows it is
			// new. A counter arriving part-used would mean this handler had written
			// an order during the halt — an offer nobody made, escrowed with
			// property nobody committed.
			for name, peek := range map[string]func(context.Context) (uint64, error){
				"listing": app.MarketKeeper.NextListingID.Peek,
				"auction": app.MarketKeeper.NextAuctionID.Peek,
				"order":   app.MarketKeeper.NextOrderID.Peek,
			} {
				next, err := peek(ctx)
				if err != nil {
					return nil, fmt.Errorf("reading the %s id sequence: %w", name, err)
				}
				if next != 0 {
					return nil, fmt.Errorf(
						"the %s id sequence starts at %d, not 0; this upgrade wrote a %s nobody asked for",
						name, next, name)
				}
			}

			ctx.Logger().Info(
				"the secondary market arrived",
				"market", "a holder of a tokenised shareholding may now sell it three ways: a "+
					"fixed-price listing, an auction by deadline, or a resting order in a book",
				"why", "a bank send names no price, so the register knew what everybody held and "+
					"nothing about what any of it cost. Every fill here writes the buyer’s cost "+
					"basis from the trade, which is the one figure no party has to be trusted for",
				"escrowed", "shares behind an ask or a listing, money behind a bid or a standing "+
					"auction bid. The module account is blocked, so nothing reaches it except "+
					"through an order the module can return",
			)

			return vm, nil
		},
	},
	{
		// One identifier, one animal; and an emission that stops writing nothing.
		//
		// No store is added and no state is rewritten: both changes are in the
		// logic that runs after the halt, and a halt is needed only because both
		// change what a block computes.
		//
		// x/herd now indexes a national identifier and a transponder code in a
		// canonical form (serial padded to six digits, separators dropped), so
		// 221/682/1029/L and 221/682/001029/L are the same animal rather than two.
		// Records written before this upgrade were checked on devnet-2 on
		// 2026-10-11 and all twelve were canonical already, so nothing is
		// re-indexed. The handler checks again rather than trusting that, because
		// the old binary accepts non-canonical identifiers right up to the halt.
		// It reports what it finds and does NOT fail: a handler that errors stops
		// the chain at this height until somebody ships another binary, and a
		// non-canonical record is a lookup that misses, not money that moves.
		//
		// x/emission stops rewriting byte-identical state in every BeginBlocker.
		Name: "one-identifier-one-animal",
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			checked, stray := 0, 0
			if err := app.HerdKeeper.Animal.Walk(ctx, nil, func(id uint64, a herdtypes.Animal) (bool, error) {
				checked++
				if a.NationalId != "" {
					if c, err := herdkeeper.CanonicalNationalID(a.NationalId); err != nil || c != a.NationalId {
						stray++
						ctx.Logger().Error("an animal is indexed under a non-canonical national identifier; "+
							"a lookup by its canonical form will not find it",
							"animal", id, "stored", a.NationalId, "canonical", c)
					}
				}
				if a.Tag != "" {
					if c, err := herdkeeper.CanonicalTag(a.Tag); err != nil || c != a.Tag {
						stray++
						ctx.Logger().Error("an animal is indexed under a non-canonical transponder code; "+
							"a lookup by its canonical form will not find it",
							"animal", id, "stored", a.Tag, "canonical", c)
					}
				}
				return false, nil
			}); err != nil {
				return nil, fmt.Errorf("walking the herd register: %w", err)
			}

			ctx.Logger().Info(
				"one identifier now means one animal",
				"checked", checked, "non_canonical", stray,
				"rule", "a national identifier is indexed with its serial padded to six digits and "+
					"its breed upper-cased; a transponder code as its fifteen digits. A serial "+
					"longer than six digits is refused, never truncated",
				"emission", "the BeginBlocker no longer rewrites identical state on every block",
			)
			return vm, nil
		},
	},
	{
		// x/jewel: the register of jewels (traceability.yamaletech.com).
		//
		// A new store, so it is added before any migration runs, and x/jewel's
		// InitGenesis arrives through the standard migration path — fromVM must
		// NOT carry "jewel", or the module's genesis never runs and its params
		// stay unwritten.
		//
		// The handler shuts x/nft's own MsgSend in x/circuit BEFORE the module
		// arrives, in the same block. x/nft's send checks only that the sender
		// owns the token, so while it is open an owner could move a jewel
		// around every rule x/jewel exists for, and x/jewel itself refuses to
		// admit a jeweler until it is shut. Only the gov authority can reset a
		// breaker on devnet-2 (x/circuit has no accounts), so reopening it would
		// take a vote.
		//
		// The office arrives empty, asserted rather than assumed: it is appointed
		// by governance, and an upgrade that installed one would be admitting an
		// authority over other people's jewels that nobody voted for.
		Name:          "jewel-register",
		StoreUpgrades: storetypes.StoreUpgrades{Added: []string{jeweltypes.StoreKey}},
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			if err := app.CircuitBreakerKeeper.DisableList.Set(ctx, jeweltypes.NFTSendTypeURL); err != nil {
				return nil, fmt.Errorf("closing x/nft's own send: %w", err)
			}
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			params, err := app.JewelKeeper.Params.Get(ctx)
			if err != nil {
				return nil, fmt.Errorf("reading the jewel parameters this upgrade just wrote: %w", err)
			}
			if params.Office != "" {
				return nil, fmt.Errorf("x/jewel arrived with office %s appointed; the office is "+
					"appointed by a vote, not by a migration", params.Office)
			}
			jewelers := 0
			if err := app.JewelKeeper.Jeweler.Walk(ctx, nil,
				func(uint64, jeweltypes.Jeweler) (bool, error) { jewelers++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the jewelers this upgrade just wrote: %w", err)
			}
			if jewelers != 0 {
				return nil, fmt.Errorf("x/jewel arrived with %d jeweler(s) admitted; the office admits them", jewelers)
			}

			ctx.Logger().Info(
				"the register of jewels arrived",
				"jewel", "jewelers are admitted by an office governance appoints; until then nothing can be registered",
				"nft", "x/nft's own MsgSend is disabled in x/circuit from this height; a jewel moves only by "+
					"offer and acceptance, a claim key, or a reassignment",
				"params", fmt.Sprintf("offer_ttl=%ds voucher_max_ttl=%ds reassignment_window=%ds",
					params.OfferTtl, params.VoucherMaxTtl, params.ReassignmentWindow),
			)
			return vm, nil
		},
	},
	{
		// x/jewel: the ownership card (docs/superpowers/specs/2026-11-24-jewel-
		// ownership-card-design.md).
		//
		// NO STORE UPGRADE. The cards, the claims and the per-jeweler claim
		// index live under new collection prefixes inside x/jewel's EXISTING
		// store. What makes this a halt is the message set — an old binary
		// cannot decode MsgIssueCard and would fork the moment one is included —
		// and transfer, which now spends a card on every change of owner.
		//
		// NO CONSENSUS VERSION BUMP and no migration, for the reason
		// syndicate-documents gives: there is nothing to migrate. The jewels
		// already registered have no card, which is the correct starting state;
		// the shop prints theirs from stock afterwards. An upgrade that issued
		// cards would be inventing secrets nobody holds.
		//
		// The handler asserts that emptiness rather than assuming it, and reads
		// the new collections so a field left out of the schema fails here, at
		// the halt, instead of at the first write in a block.
		Name: "jewel-cards",
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			cards, claims, listed := 0, 0, 0
			if err := app.JewelKeeper.Card.Walk(ctx, nil,
				func(uint64, jeweltypes.Card) (bool, error) { cards++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the ownership cards this upgrade just mounted: %w", err)
			}
			if err := app.JewelKeeper.Claim.Walk(ctx, nil,
				func(uint64, jeweltypes.Claim) (bool, error) { claims++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the claims this upgrade just mounted: %w", err)
			}
			if err := app.JewelKeeper.ClaimsByJeweler.Walk(ctx, nil,
				func(collections.Pair[uint64, uint64]) (bool, error) { listed++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the claim index this upgrade just mounted: %w", err)
			}
			if cards != 0 || claims != 0 || listed != 0 {
				return nil, fmt.Errorf(
					"x/jewel arrived with %d card(s), %d claim(s) and %d indexed claim(s); a card's "+
						"secret is printed by the shop that holds the jewel, never written during a halt",
					cards, claims, listed)
			}

			jewels := 0
			if err := app.JewelKeeper.Jewel.Walk(ctx, nil,
				func(uint64, jeweltypes.Jewel) (bool, error) { jewels++; return false, nil },
			); err != nil {
				return nil, fmt.Errorf("reading the jewels: %w", err)
			}

			ctx.Logger().Info(
				"ownership cards arrived",
				"card", "a shop's staff may issue a card for a jewel in stock; the buyer files a claim "+
					"signed with the card's secret, and staff approve it once they know the buyer paid",
				"transfer", "every change of owner now spends the jewel's card and drops any pending claim, "+
					"whether it moved by an approved claim, an offer or a reassignment",
				"jewels_without_a_card", jewels,
			)
			return vm, nil
		},
	},
	{
		// x/jewel: a shop's business account can move to another account,
		// taking its stock with it (MsgProposeBusinessMove, then
		// MsgAcceptBusinessMove from the named account).
		//
		// NO STORE UPGRADE and no migration. The proposal lives in two new
		// fields of the Jeweler record, which every existing jeweler already
		// reads as empty: no move waiting. What makes this a halt is the
		// message set: an old binary cannot decode either message.
		Name: "jewel-business-move",
		Handler: func(ctx sdk.Context, app *App, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}
			jewelers := 0
			if err := app.JewelKeeper.Jeweler.Walk(ctx, nil,
				func(_ uint64, j jeweltypes.Jeweler) (bool, error) {
					jewelers++
					if j.PendingBusiness != "" || j.PendingBusinessUntil != 0 {
						return true, fmt.Errorf("jeweler %d arrived with a business move pending; nothing proposed one", j.Id)
					}
					return false, nil
				},
			); err != nil {
				return nil, fmt.Errorf("reading the jewelers: %w", err)
			}
			ctx.Logger().Info(
				"a shop's business account can move",
				"move", "the business account proposes an account; that account accepts and receives the stock; "+
					"the old account keeps any staff place, and can then own the shop's pieces like any customer",
				"jewelers", jewelers,
			)
			return vm, nil
		},
	},
}
