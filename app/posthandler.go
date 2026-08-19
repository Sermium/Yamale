package app

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// newPostHandler builds the app's PostHandler from whatever the profile
// contributes. The SDK's own default (empty) post handler is disabled via
// SkipPostHandler in app_config.go, so this is the only one in effect.
//
// A profile that contributes nothing gets a nil handler rather than an empty
// chain: baseapp skips a nil post handler, whereas an empty chain built by
// ChainPostDecorators is a call per transaction that does nothing.
func newPostHandler(app *App) (sdk.PostHandler, error) {
	decorators := app.builderfeePostDecorators()
	if len(decorators) == 0 {
		return nil, nil
	}

	return sdk.ChainPostDecorators(decorators...), nil
}
