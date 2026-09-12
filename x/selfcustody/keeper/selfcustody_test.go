package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/selfcustody/keeper"
	"yamale/blockchain/x/selfcustody/types"
)

// The door out.
//
// What is being proved here is not that the module stores records. It is that
// the pathway has the two properties that make it worth having:
//
//   - the operator cannot stall a holder who wants their key, and
//   - a stolen device cannot take one before its owner can react.
//
// Those pull in opposite directions, which is why the tests below spend most of
// their effort on the boundary between them rather than on the happy path.

const hour = 60 * 60

type fixture struct {
	env *integration.Env
	k   keeper.Keeper
	srv types.MsgServer
	q   types.QueryServer

	alice string
	bob   string
	gov   string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	env := integration.NewWith(t, []string{types.ModuleName})
	// A real clock. Every period in this module is one a frightened person
	// lives through; a zero time.Time is 1754 and every failure message that
	// quotes it reads as nonsense.
	env.Ctx = env.Ctx.WithBlockTime(time.Unix(1_788_000_000, 0)).WithBlockHeight(1)

	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority)
	require.NoError(t, k.Params.Set(env.Ctx, types.DefaultParams()))

	f := &fixture{
		env: env,
		k:   k,
		srv: keeper.NewMsgServerImpl(k),
		q:   keeper.NewQueryServerImpl(k),
	}
	_, f.alice = env.Addr(t)
	_, f.bob = env.Addr(t)

	gov, err := env.AddressCodec.BytesToString(env.Authority)
	require.NoError(t, err)
	f.gov = gov

	return f
}

// now returns the fixture's current block time.
func (f *fixture) now() int64 { return f.env.Ctx.BlockTime().Unix() }

// advance moves the clock and the height together.
//
// Both, because the disclosure record stores a height and a test that only
// moved the clock would write every record at height 1 — hiding exactly the
// kind of mistake the height is there to make checkable.
func (f *fixture) advance(seconds int64) {
	f.env.Ctx = f.env.Ctx.
		WithBlockTime(f.env.Ctx.BlockTime().Add(time.Duration(seconds) * time.Second)).
		WithBlockHeight(f.env.Ctx.BlockHeight() + seconds/5)
}

// ack is the acknowledgement this chain publishes, as a holder's client would
// have fetched it from the Params query.
func ack() (string, string) {
	p := types.DefaultParams()
	return p.AcknowledgementVersion, p.AcknowledgementHash
}

func (f *fixture) request(t *testing.T, account string) *types.MsgRequestDisclosureResponse {
	t.Helper()
	version, hash := ack()
	res, err := f.srv.RequestDisclosure(f.env.Ctx, &types.MsgRequestDisclosure{
		Account:                account,
		AcknowledgementVersion: version,
		AcknowledgementHash:    hash,
	})
	require.NoError(t, err)
	return res
}

func (f *fixture) confirm(t *testing.T, account string) error {
	t.Helper()
	version, hash := ack()
	_, err := f.srv.ConfirmDisclosure(f.env.Ctx, &types.MsgConfirmDisclosure{
		Account:                account,
		AcknowledgementVersion: version,
		AcknowledgementHash:    hash,
	})
	return err
}

/* ========================================================================== */
/*  The pathway                                                               */
/* ========================================================================== */

// A holder who asks, waits and asks again gets out.
//
// The operator is not a party to any of these messages, which is the point
// being recorded: there is no step in this sequence at which anybody but the
// account holder signs anything.
func TestDisclosureCompletes(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	askedAt := f.now()
	res := f.request(t, f.alice)
	require.Equal(t, askedAt+params.DisclosureDelay, res.EligibleAt)

	f.advance(params.DisclosureDelay)
	require.NoError(t, f.confirm(t, f.alice))

	d, err := f.k.Disclosure.Get(f.env.Ctx, f.alice)
	require.NoError(t, err)
	require.Equal(t, askedAt, d.RequestedAt)
	require.Equal(t, f.now(), d.DisclosedAt)
	require.Equal(t, f.env.Ctx.BlockHeight(), d.DisclosedAtHeight)

	// The exact text they agreed to, recoverable from the record alone.
	version, hash := ack()
	require.Equal(t, version, d.AcknowledgementVersion)
	require.Equal(t, hash, d.AcknowledgementHash)

	// The request is gone, so the account is not permanently marked as being
	// inside a waiting period it has already served.
	has, err := f.k.Request.Has(f.env.Ctx, f.alice)
	require.NoError(t, err)
	require.False(t, has)

	disclosed, err := f.k.IsDisclosed(f.env.Ctx, f.alice)
	require.NoError(t, err)
	require.True(t, disclosed)
}

// One second early is early.
//
// The boundary is tested rather than the middle because a delay implemented
// with the wrong comparison is off by exactly one block, and a test that waited
// half the period would pass against every wrong version of this check.
func TestConfirmBeforeTheDelayIsRefused(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	f.request(t, f.alice)

	f.advance(params.DisclosureDelay - 1)
	require.ErrorIs(t, f.confirm(t, f.alice), types.ErrTooSoon)

	f.advance(1)
	require.NoError(t, f.confirm(t, f.alice))
}

// Confirming without having asked is refused.
//
// Otherwise the waiting period would be optional for anybody who skipped
// straight to the second message — which is the shape a stolen phone would
// take, because the thief has no reason to send the message that notifies the
// owner.
func TestConfirmWithoutRequestIsRefused(t *testing.T) {
	f := setup(t)
	require.ErrorIs(t, f.confirm(t, f.alice), types.ErrNoRequest)
}

// The holder can stop it, and nothing is left behind.
//
// This is the message a person whose phone was taken an hour ago reaches for,
// and it is the reason the delay is worth anything. It must also be clean:
// somebody who thought about leaving and changed their mind should not be able
// to be identified afterwards as having thought about it.
func TestCancelStopsTheRequest(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	f.request(t, f.alice)
	_, err := f.srv.CancelDisclosure(f.env.Ctx, &types.MsgCancelDisclosure{
		Account: f.alice, Reason: "I did not make this request",
	})
	require.NoError(t, err)

	has, err := f.k.Request.Has(f.env.Ctx, f.alice)
	require.NoError(t, err)
	require.False(t, has)

	// And the wait cannot be resumed: confirming after the delay would have
	// elapsed still fails, because there is nothing to confirm.
	f.advance(params.DisclosureDelay)
	require.ErrorIs(t, f.confirm(t, f.alice), types.ErrNoRequest)
}

func TestCancelWithoutRequestIsRefused(t *testing.T) {
	f := setup(t)
	_, err := f.srv.CancelDisclosure(f.env.Ctx, &types.MsgCancelDisclosure{Account: f.alice})
	require.ErrorIs(t, err, types.ErrNoRequest)
}

/* ========================================================================== */
/*  Expiry                                                                    */
/* ========================================================================== */

// A request nobody completed lapses.
//
// An indefinite request is a standing authorisation to take somebody's key,
// sitting there waiting for the day an attacker acquires their device.
func TestExpiredRequestCannotBeConfirmed(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	f.request(t, f.alice)
	f.advance(params.RequestExpiry)
	require.ErrorIs(t, f.confirm(t, f.alice), types.ErrRequestExpired)
}

// A lapsed request does not block a new one.
//
// If it did, the only way out for a holder who forgot about a request a year
// ago would be to ask the operator to clear it — and needing the operator's
// cooperation to exercise this right is the exact thing the module exists to
// remove.
func TestExpiredRequestIsReplacedByANewOne(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	f.request(t, f.alice)
	f.advance(params.RequestExpiry)

	askedAgainAt := f.now()
	res := f.request(t, f.alice)
	require.Equal(t, askedAgainAt+params.DisclosureDelay, res.EligibleAt)

	r, err := f.k.Request.Get(f.env.Ctx, f.alice)
	require.NoError(t, err)
	require.Equal(t, askedAgainAt, r.RequestedAt)

	// And the new request's own delay must be served in full: the lapsed one
	// does not carry time forward.
	f.advance(params.DisclosureDelay - 1)
	require.ErrorIs(t, f.confirm(t, f.alice), types.ErrTooSoon)
}

// A live request is not restarted by asking again.
//
// The trap this closes is the opposite of the one above: if a second request
// overwrote the first, a holder repeatedly tapping the button would reset their
// own waiting period for ever and never reach the end of it.
func TestSecondRequestWhileOneIsLiveIsRefused(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	askedAt := f.now()
	f.request(t, f.alice)

	f.advance(hour)
	version, hash := ack()
	_, err := f.srv.RequestDisclosure(f.env.Ctx, &types.MsgRequestDisclosure{
		Account: f.alice, AcknowledgementVersion: version, AcknowledgementHash: hash,
	})
	require.ErrorIs(t, err, types.ErrRequestPending)

	r, err := f.k.Request.Get(f.env.Ctx, f.alice)
	require.NoError(t, err)
	require.Equal(t, askedAt, r.RequestedAt, "the original wait must still be running")

	f.advance(params.DisclosureDelay - hour)
	require.NoError(t, f.confirm(t, f.alice))
}

/* ========================================================================== */
/*  The acknowledgement                                                       */
/* ========================================================================== */

// A holder shown some other text cannot start the pathway.
//
// The entire evidentiary value of the record is that we know exactly what they
// read. A request naming a hash that is not the published one means their
// client showed them something else, and recording it would produce a receipt
// that proves nothing.
func TestWrongAcknowledgementIsRefused(t *testing.T) {
	f := setup(t)
	version, _ := ack()

	_, err := f.srv.RequestDisclosure(f.env.Ctx, &types.MsgRequestDisclosure{
		Account:                f.alice,
		AcknowledgementVersion: version,
		AcknowledgementHash:    "00000000000000000000000000000000000000000000000000000000000000ff",
	})
	require.ErrorIs(t, err, types.ErrWrongAcknowledgement)
}

// A warning published during the waiting period sends the holder back to read it.
//
// This is why the acknowledgement is repeated on the confirmation at all. The
// terms they are accepting have changed; a confirmation that carried the old
// hash through would record them as having agreed to text they never saw.
func TestNewWarningDuringTheWaitBlocksConfirmation(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	f.request(t, f.alice)

	updated := params
	updated.AcknowledgementVersion = "2026-11-1"
	updated.AcknowledgementHash =
		"1111111111111111111111111111111111111111111111111111111111111111"
	_, err := f.srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{
		Authority: f.gov, Params: updated,
	})
	require.NoError(t, err)

	f.advance(params.DisclosureDelay)
	require.ErrorIs(t, f.confirm(t, f.alice), types.ErrWrongAcknowledgement)

	// Accepting the new text works, and — this is the part that matters — the
	// waiting period already served is NOT restarted. Governance changing the
	// wording must not be a way to reset everybody's clock.
	_, err = f.srv.ConfirmDisclosure(f.env.Ctx, &types.MsgConfirmDisclosure{
		Account:                f.alice,
		AcknowledgementVersion: updated.AcknowledgementVersion,
		AcknowledgementHash:    updated.AcknowledgementHash,
	})
	require.NoError(t, err)

	d, err := f.k.Disclosure.Get(f.env.Ctx, f.alice)
	require.NoError(t, err)
	require.Equal(t, updated.AcknowledgementHash, d.AcknowledgementHash)
}

// The published hash is the hash of the text actually shipped in the binary.
//
// If this fails, somebody edited the acknowledgement document without the hash
// following it, and every holder on the chain would be unable to acknowledge
// anything until governance caught up.
func TestPublishedHashMatchesTheShippedText(t *testing.T) {
	require.Equal(t, types.AcknowledgementHash(), types.DefaultParams().AcknowledgementHash)
	require.NotEmpty(t, types.AcknowledgementText)

	// The text must say what it is for. A warning that omits the one fact that
	// makes this irreversible is not the warning this module claims to publish.
	require.Contains(t, types.AcknowledgementText, "There is no seed phrase")
}

/* ========================================================================== */
/*  Once out, out                                                             */
/* ========================================================================== */

// A key cannot be disclosed twice.
//
// Not pedantry. A second record would carry a later date on which the
// platform's obligations supposedly ended, when in fact they ended on the
// first — and the later date is the one a dispute would reach for.
func TestDisclosureCannotBeRepeated(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	f.request(t, f.alice)
	f.advance(params.DisclosureDelay)
	require.NoError(t, f.confirm(t, f.alice))

	version, hash := ack()
	_, err := f.srv.RequestDisclosure(f.env.Ctx, &types.MsgRequestDisclosure{
		Account: f.alice, AcknowledgementVersion: version, AcknowledgementHash: hash,
	})
	require.ErrorIs(t, err, types.ErrAlreadyDisclosed)

	require.ErrorIs(t, f.confirm(t, f.alice), types.ErrAlreadyDisclosed)
}

/* ========================================================================== */
/*  Governance                                                                */
/* ========================================================================== */

func TestOnlyGovernanceSetsTheDelay(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()
	params.DisclosureDelay = hour

	_, err := f.srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{
		Authority: f.alice, Params: params,
	})
	require.ErrorIs(t, err, types.ErrNotAuthority)

	_, err = f.srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{
		Authority: f.gov, Params: params,
	})
	require.NoError(t, err)
}

// Governance cannot set a delay of zero.
//
// A zero delay lets a request be made and confirmed in one block, which removes
// the only defence a person with a stolen phone has. The parameter validation
// refuses it, so an operator who captured governance still could not turn the
// pathway into a single tap.
func TestZeroDelayIsRefused(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()
	params.DisclosureDelay = 0

	_, err := f.srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{
		Authority: f.gov, Params: params,
	})
	require.ErrorIs(t, err, types.ErrInvalidParams)
}

// An expiry at or below the delay would make every request lapse before it
// could ever be confirmed — a refusal dressed as a parameter.
func TestExpiryBelowDelayIsRefused(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()
	params.RequestExpiry = params.DisclosureDelay

	_, err := f.srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{
		Authority: f.gov, Params: params,
	})
	require.ErrorIs(t, err, types.ErrInvalidParams)
}

// A longer delay published mid-wait applies to requests already in flight.
//
// The honest behaviour, and the one the Request query must report: if
// governance decides two days was too short, somebody eleven hours in really
// does have to wait longer, and telling them otherwise would send them to a
// confirmation the chain refuses.
func TestLongerDelayAppliesToRequestsInFlight(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	askedAt := f.now()
	f.request(t, f.alice)

	longer := params
	longer.DisclosureDelay = params.DisclosureDelay * 2
	_, err := f.srv.UpdateParams(f.env.Ctx, &types.MsgUpdateParams{
		Authority: f.gov, Params: longer,
	})
	require.NoError(t, err)

	f.advance(params.DisclosureDelay)
	require.ErrorIs(t, f.confirm(t, f.alice), types.ErrTooSoon)

	res, err := f.q.Request(f.env.Ctx, &types.QueryRequestRequest{Account: f.alice})
	require.NoError(t, err)
	require.True(t, res.Pending)
	require.Equal(t, askedAt+longer.DisclosureDelay, res.EligibleAt)

	f.advance(params.DisclosureDelay)
	require.NoError(t, f.confirm(t, f.alice))
}

/* ========================================================================== */
/*  The read side                                                             */
/* ========================================================================== */

// An account still in custody answers, it does not fail.
//
// "This account is in custody" and "the query failed" are opposite conclusions
// for a counterparty deciding whether to rely on the platform's safeguards, and
// a NotFound makes them identical at the call site.
func TestDisclosureQueryAnswersForAccountsInCustody(t *testing.T) {
	f := setup(t)

	res, err := f.q.Disclosure(f.env.Ctx, &types.QueryDisclosureRequest{Account: f.alice})
	require.NoError(t, err)
	require.False(t, res.Disclosed)

	params := types.DefaultParams()
	f.request(t, f.alice)
	f.advance(params.DisclosureDelay)
	require.NoError(t, f.confirm(t, f.alice))

	res, err = f.q.Disclosure(f.env.Ctx, &types.QueryDisclosureRequest{Account: f.alice})
	require.NoError(t, err)
	require.True(t, res.Disclosed)
	require.Equal(t, f.alice, res.Disclosure.Account)
}

func TestRequestQueryReportsBothDeadlines(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	res, err := f.q.Request(f.env.Ctx, &types.QueryRequestRequest{Account: f.alice})
	require.NoError(t, err)
	require.False(t, res.Pending)

	askedAt := f.now()
	f.request(t, f.alice)

	res, err = f.q.Request(f.env.Ctx, &types.QueryRequestRequest{Account: f.alice})
	require.NoError(t, err)
	require.True(t, res.Pending)
	require.Equal(t, askedAt+params.DisclosureDelay, res.EligibleAt)
	require.Equal(t, askedAt+params.RequestExpiry, res.ExpiresAt)
}

// The count is published; the roll is not.
//
// A list of accounts whose private keys are known to be sitting in somebody's
// downloads folder is a target roster. There is deliberately no query that
// returns one, and this test exists so that adding one is a decision somebody
// has to make on purpose.
func TestCountIsPublishedWithoutNames(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	f.request(t, f.alice)
	f.advance(params.DisclosureDelay)
	require.NoError(t, f.confirm(t, f.alice))
	f.request(t, f.bob)

	res, err := f.q.Count(f.env.Ctx, &types.QueryCountRequest{})
	require.NoError(t, err)
	require.Equal(t, uint64(1), res.Disclosed)
	require.Equal(t, uint64(1), res.Pending)
}

/* ========================================================================== */
/*  Genesis                                                                   */
/* ========================================================================== */

// Export then import reproduces the state exactly.
//
// Both collections are primary facts. A disclosure is the date somebody's
// custody ended; a request is time a person has already served. Dropping either
// at an upgrade would restart a waiting period silently, which to the holder is
// indistinguishable from the operator stalling them.
func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	params := types.DefaultParams()

	f.request(t, f.alice)
	f.advance(params.DisclosureDelay)
	require.NoError(t, f.confirm(t, f.alice))
	f.request(t, f.bob)

	exported, err := f.k.ExportGenesis(f.env.Ctx)
	require.NoError(t, err)
	require.Len(t, exported.Disclosures, 1)
	require.Len(t, exported.Requests, 1)
	require.NoError(t, exported.Validate())

	fresh := setup(t)
	require.NoError(t, fresh.k.InitGenesis(fresh.env.Ctx, *exported))

	again, err := fresh.k.ExportGenesis(fresh.env.Ctx)
	require.NoError(t, err)
	require.Equal(t, exported, again)

	// And Bob's wait resumes where it was rather than starting over.
	r, err := fresh.k.Request.Get(fresh.env.Ctx, f.bob)
	require.NoError(t, err)
	require.Equal(t, f.now(), r.RequestedAt)
}

func TestGenesisRefusesADisclosureConfirmedBeforeItWasAsked(t *testing.T) {
	gs := types.DefaultGenesis()
	version, hash := ack()
	gs.Disclosures = []types.Disclosure{{
		Account:                "yml1whoever",
		RequestedAt:            2_000,
		DisclosedAt:            1_000,
		AcknowledgementVersion: version,
		AcknowledgementHash:    hash,
	}}
	require.ErrorContains(t, gs.Validate(), "before it was requested")
}

func TestGenesisRefusesARequestAgainstADisclosedAccount(t *testing.T) {
	gs := types.DefaultGenesis()
	version, hash := ack()
	gs.Disclosures = []types.Disclosure{{
		Account:                "yml1whoever",
		RequestedAt:            1_000,
		DisclosedAt:            2_000,
		AcknowledgementVersion: version,
		AcknowledgementHash:    hash,
	}}
	gs.Requests = []types.Request{{Account: "yml1whoever", RequestedAt: 3_000}}
	require.ErrorContains(t, gs.Validate(), "pending request and an existing disclosure")
}
