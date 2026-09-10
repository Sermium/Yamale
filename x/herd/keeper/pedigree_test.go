package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"yamale/blockchain/testutil/integration"
	"yamale/blockchain/x/herd/keeper"
	"yamale/blockchain/x/herd/types"
)

// The pedigree refusals, which are the reason this module exists rather than a
// spreadsheet.
//
// A Ladoum birth certificate issued today carries a line reading
// "Classico / Atomique / Palmarin". Those are names in free text: nobody can
// walk them and — the part that matters — nobody can contradict them. A breeder
// can declare a sire that was dead, a dam that gave birth a fortnight earlier,
// or an ancestor that is the animal itself, and the document looks exactly as
// good as a true one.
//
// Each test below is one of those declarations, refused.

const jurisdiction = "SN"

type fixture struct {
	env    *integration.Env
	k      keeper.Keeper
	srv    types.MsgServer
	office string
	herd   uint64
}

func setup(t *testing.T) *fixture {
	t.Helper()
	env := integration.NewWith(t, []string{types.ModuleName})

	// nil group keeper: these tests exercise the pedigree rules, not admission.
	// The office is written straight into state, which is what admission would
	// have produced.
	k := keeper.NewKeeper(env.StoreService, env.Codec, env.AddressCodec, env.Authority, nil)
	require.NoError(t, k.Params.Set(env.Ctx, types.DefaultParams()))

	_, office := env.Addr(t)
	require.NoError(t, k.Authority.Set(env.Ctx, office, types.Authority{
		Address: office, Name: "Direction de l'Elevage", Jurisdiction: jurisdiction, Active: true,
	}))

	f := &fixture{env: env, k: k, srv: keeper.NewMsgServerImpl(k), office: office}

	_, herdKeeper := env.Addr(t)
	res, err := f.srv.EnrolHerd(env.Ctx, &types.MsgEnrolHerd{
		Creator:      office,
		Reference:    "SN-DK-H-000147",
		Keeper:       herdKeeper,
		Species:      types.SPECIES_SHEEP,
		DeclaredSize: 40,
		Locality:     "Niacoulrab",
	})
	require.NoError(t, err)
	f.herd = res.Id
	return f
}

// animal records one at level 2 and returns its id.
func (f *fixture) animal(t *testing.T, nationalID, tag, born string, sex types.Sex, birthType string) uint64 {
	t.Helper()
	res, err := f.srv.IdentifyAnimal(f.env.Ctx, &types.MsgIdentifyAnimal{
		Creator:    f.office,
		HerdId:     f.herd,
		NationalId: nationalID,
		Tag:        tag,
		Sex:        sex,
		BornOn:     born,
		BirthType:  birthType,
	})
	require.NoError(t, err)
	return res.Id
}

func (f *fixture) certify(t *testing.T, animal, sire, dam uint64) error {
	t.Helper()
	_, err := f.srv.CertifyAnimal(f.env.Ctx, &types.MsgCertifyAnimal{
		Creator:   f.office,
		AnimalId:  animal,
		SireId:    sire,
		DamId:     dam,
		DnaHash:   "dna-" + string(rune('a'+animal)),
		PhotoHash: "photo",
	})
	return err
}

// A line that is simply true is recorded, which has to be established before
// any refusal means anything.
func TestACoherentPedigreeIsAccepted(t *testing.T) {
	f := setup(t)
	sire := f.animal(t, "221/682/000001/L", "982000000000001", "2020-01-10", types.SEX_MALE, "single")
	dam := f.animal(t, "221/682/000002/L", "982000000000002", "2020-03-04", types.SEX_FEMALE, "single")
	lamb := f.animal(t, "221/682/021029/L", "982000000000003", "2026-08-25", types.SEX_FEMALE, "single")

	require.NoError(t, f.certify(t, lamb, sire, dam))

	got, err := f.k.Animal.Get(f.env.Ctx, lamb)
	require.NoError(t, err)
	require.Equal(t, types.LEVEL_CERTIFIED, got.Level)
	require.Equal(t, sire, got.SireId)
	require.Equal(t, dam, got.DamId)
}

// A sire that is not male is not a mistake about this animal, it is a mistake
// about that one — and it would propagate into every line passing through it.
func TestASireMustBeMale(t *testing.T) {
	f := setup(t)
	notASire := f.animal(t, "221/682/000010/L", "982000000000010", "2020-01-10", types.SEX_FEMALE, "single")
	dam := f.animal(t, "221/682/000011/L", "982000000000011", "2020-03-04", types.SEX_FEMALE, "single")
	lamb := f.animal(t, "221/682/000012/L", "982000000000012", "2026-08-25", types.SEX_MALE, "single")

	err := f.certify(t, lamb, notASire, dam)
	require.ErrorIs(t, err, types.ErrSireNotMale)
}

func TestADamMustBeFemale(t *testing.T) {
	f := setup(t)
	sire := f.animal(t, "221/682/000020/L", "982000000000020", "2020-01-10", types.SEX_MALE, "single")
	notADam := f.animal(t, "221/682/000021/L", "982000000000021", "2020-03-04", types.SEX_MALE, "single")
	lamb := f.animal(t, "221/682/000022/L", "982000000000022", "2026-08-25", types.SEX_MALE, "single")

	err := f.certify(t, lamb, sire, notADam)
	require.ErrorIs(t, err, types.ErrDamNotFemale)
}

// The declaration nobody checks on paper: a parent born after its child.
func TestAParentCannotBeYoungerThanItsChild(t *testing.T) {
	f := setup(t)
	sire := f.animal(t, "221/682/000030/L", "982000000000030", "2026-01-10", types.SEX_MALE, "single")
	lamb := f.animal(t, "221/682/000031/L", "982000000000031", "2020-08-25", types.SEX_FEMALE, "single")

	err := f.certify(t, lamb, sire, 0)
	require.ErrorIs(t, err, types.ErrParentYounger)
}

// Inventing ancestry usually means attaching several animals to one prestigious
// dam without thinking about the calendar. A ewe cannot produce two unrelated
// lambs a fortnight apart.
func TestOneDamCannotHaveTwoBirthsAFortnightApart(t *testing.T) {
	f := setup(t)
	dam := f.animal(t, "221/682/000040/L", "982000000000040", "2019-02-01", types.SEX_FEMALE, "single")
	first := f.animal(t, "221/682/000041/L", "982000000000041", "2026-08-01", types.SEX_FEMALE, "single")
	second := f.animal(t, "221/682/000042/L", "982000000000042", "2026-08-15", types.SEX_MALE, "single")

	require.NoError(t, f.certify(t, first, 0, dam))

	err := f.certify(t, second, 0, dam)
	require.ErrorIs(t, err, types.ErrBirthTooClose)
}

// Twins are the exception the check has to allow, or it refuses real records —
// and a rule that refuses the truth gets switched off within a season.
func TestTwinsAreOneBirthNotTwo(t *testing.T) {
	f := setup(t)
	dam := f.animal(t, "221/682/000050/L", "982000000000050", "2019-02-01", types.SEX_FEMALE, "single")
	first := f.animal(t, "221/682/000051/L", "982000000000051", "2026-08-01", types.SEX_FEMALE, "twin")
	second := f.animal(t, "221/682/000052/L", "982000000000052", "2026-08-01", types.SEX_MALE, "twin")

	require.NoError(t, f.certify(t, first, 0, dam))
	require.NoError(t, f.certify(t, second, 0, dam),
		"two animals with the same dam, the same birth date and birth type twin are one birth")
}

// An animal that is its own ancestor is not merely wrong: it is a cycle that
// would make every later pedigree walk run until a bound saved it.
//
// Note which animals this test uses, because it took a failure to see it. With
// both birth dates known a cycle is unreachable: it would need A born before B
// and B born before A, so the date check refuses the declaration first and the
// cycle check never runs. The cycle check earns its keep on exactly the records
// the date check cannot judge — the ones enrolled from a paper file that says
// "about 2019" — and those are the common ones here, not the exotic ones. So
// these animals carry no birth date, which is the only state in which this rule
// is the one standing between the registry and a loop.
func TestAnAnimalCannotBeItsOwnAncestor(t *testing.T) {
	f := setup(t)
	grandsire := f.animal(t, "221/682/000060/L", "982000000000060", "", types.SEX_MALE, "single")
	sire := f.animal(t, "221/682/000061/L", "982000000000061", "", types.SEX_MALE, "single")

	require.NoError(t, f.certify(t, sire, grandsire, 0))

	// Declaring the grandsire's sire to be its own grandson closes the loop.
	err := f.certify(t, grandsire, sire, 0)
	require.ErrorIs(t, err, types.ErrPedigreeCycle)
}

// The same loop, closed three generations up rather than one. A cycle a breeder
// creates by accident is rarely the short one: the short one is visible on the
// page in front of them, and this one is not.
func TestACycleIsRefusedSeveralGenerationsUp(t *testing.T) {
	f := setup(t)
	first := f.animal(t, "221/682/000065/L", "982000000000065", "", types.SEX_MALE, "single")
	second := f.animal(t, "221/682/000066/L", "982000000000066", "", types.SEX_MALE, "single")
	third := f.animal(t, "221/682/000067/L", "982000000000067", "", types.SEX_MALE, "single")
	fourth := f.animal(t, "221/682/000068/L", "982000000000068", "", types.SEX_MALE, "single")

	require.NoError(t, f.certify(t, second, first, 0))
	require.NoError(t, f.certify(t, third, second, 0))
	require.NoError(t, f.certify(t, fourth, third, 0))

	// first's sire is fourth, whose line runs back through third and second to
	// first. Nothing on a single certificate would show this.
	err := f.certify(t, first, fourth, 0)
	require.ErrorIs(t, err, types.ErrPedigreeCycle)
}

// A dated line is refused before the cycle check is ever reached, and the error
// a breeder gets should be the one they can act on: "this parent was born after
// this animal" names the field to correct, where "your pedigree contains a
// cycle" asks them to work it out. This pins that ordering, because it is the
// reason the test above had to drop its dates.
func TestADatedLoopIsRefusedOnTheDatesFirst(t *testing.T) {
	f := setup(t)
	older := f.animal(t, "221/682/000062/L", "982000000000062", "2018-01-10", types.SEX_MALE, "single")
	younger := f.animal(t, "221/682/000063/L", "982000000000063", "2020-01-10", types.SEX_MALE, "single")

	require.NoError(t, f.certify(t, younger, older, 0))

	err := f.certify(t, older, younger, 0)
	require.ErrorIs(t, err, types.ErrParentYounger)
}

// A parent that is not a record here is a name, which is what the free-text
// certificate already had.
func TestAParentMustBeARecord(t *testing.T) {
	f := setup(t)
	lamb := f.animal(t, "221/682/000070/L", "982000000000070", "2026-08-25", types.SEX_FEMALE, "single")

	err := f.certify(t, lamb, 9999, 0)
	require.ErrorIs(t, err, types.ErrParentMissing)
}

// Unknown parentage is honest and common — a founder animal has none — and it
// must not be confused with animal zero, which is never issued.
func TestUnknownParentageIsAccepted(t *testing.T) {
	f := setup(t)
	founder := f.animal(t, "221/682/000080/L", "982000000000080", "2015-04-02", types.SEX_MALE, "single")

	require.NoError(t, f.certify(t, founder, 0, 0))

	got, err := f.k.Animal.Get(f.env.Ctx, founder)
	require.NoError(t, err)
	require.Zero(t, got.SireId)
	require.Zero(t, got.DamId)
	require.Equal(t, types.LEVEL_CERTIFIED, got.Level)
}

// A missing birth date is the ordinary state of an animal enrolled from a paper
// record that says "about 2019". The checks that need a date pass rather than
// refuse, because a registry that rejected those records would simply not be
// used.
func TestAMissingBirthDateDoesNotBlockCertification(t *testing.T) {
	f := setup(t)
	sire := f.animal(t, "221/682/000090/L", "982000000000090", "", types.SEX_MALE, "single")
	lamb := f.animal(t, "221/682/000091/L", "982000000000091", "", types.SEX_FEMALE, "single")

	require.NoError(t, f.certify(t, lamb, sire, 0))
}

// Without a laboratory the parentage is the declaring breeder's word, which is
// exactly what the paper certificates already are.
func TestCertificationRequiresADnaProfile(t *testing.T) {
	f := setup(t)
	lamb := f.animal(t, "221/682/000100/L", "982000000000100", "2026-08-25", types.SEX_FEMALE, "single")

	_, err := f.srv.CertifyAnimal(f.env.Ctx, &types.MsgCertifyAnimal{
		Creator:  f.office,
		AnimalId: lamb,
	})
	require.ErrorIs(t, err, types.ErrNoDNA)
}
