//go:build settlement

package app

import appv1alpha1 "cosmossdk.io/api/cosmos/app/v1alpha1"

// x/herd is not linked into the settlement profile, for the same reason x/land
// is not.
//
// A livestock registry is a sovereign register: it says who owns what inside
// one country, and it is the instrument a ministry uses to decide whether an
// animal may be exported or a herd has been vaccinated. No state will place
// that on a ledger other states validate, and a settlement binary that carried
// it would invite exactly the shared deployment scope 2 rules out.
//
// The argument is if anything stronger here than for land. A cadastre at least
// sits still; a herd crosses borders, so a shared livestock ledger would make
// two states' registries disagree about the same animal twice a year.

var (
	herdInitGenesis   []string
	herdModuleConfigs []*appv1alpha1.ModuleConfig
)

type herdKeepers struct{}

func (app *App) herdDepinjectOutputs() []any { return nil }
