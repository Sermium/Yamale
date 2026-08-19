package types

import (
	"net/url"
	"strings"

	errorsmod "cosmossdk.io/errors"
)

// MaxPayloadStoreURLLength bounds the directory entry.
//
// The field is written to state by a permissionless-to-the-participant message,
// so an unbounded string is state bloat priced at one transaction fee. 2048 is
// the length every HTTP implementation already handles, so a value that would
// not fit here would not survive being fetched.
const MaxPayloadStoreURLLength = 2048

// ValidatePayloadStoreURL checks a participant's payload store address.
//
// Validated on-chain rather than left to the clients, because the failure it
// prevents does not land on whoever made the mistake. A value that is not a URL
// is not a store, and the payee — who is entitled to read the detail — sees it
// reported as unavailable with nothing to tell them the reason is a typo in
// somebody else's registration.
//
// Empty is valid and means the participant runs no store. That is the honest
// default: most participants will not run one on day one, and a client must
// render that as detail being unavailable, never as a payment with no detail.
func ValidatePayloadStoreURL(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > MaxPayloadStoreURLLength {
		return errorsmod.Wrapf(ErrInvalidPayloadStore,
			"url must be at most %d characters, got %d", MaxPayloadStoreURLLength, len(raw))
	}

	u, err := url.Parse(raw)
	if err != nil {
		return errorsmod.Wrapf(ErrInvalidPayloadStore, "url is unparseable: %s", err)
	}
	// http is permitted alongside https because a deployment stands up on a
	// private network before it has certificates, and refusing it would push
	// operators to register a URL they cannot serve. It is not a weakening: the
	// payload behind it is already sealed to keys the transport never sees, and
	// a store that can be read by whoever is on the wire learns only which
	// envelopes were fetched.
	if u.Scheme != "http" && u.Scheme != "https" {
		return errorsmod.Wrapf(ErrInvalidPayloadStore,
			"url must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errorsmod.Wrap(ErrInvalidPayloadStore,
			"url names no host, so nothing could ever be fetched from it")
	}
	// A base URL, so the client owns the path it appends. A registered query or
	// fragment would be silently dropped or would corrupt the request path, and
	// either way the payee's retrieval fails against a value that looks
	// deliberate in state.
	if u.RawQuery != "" || u.Fragment != "" {
		return errorsmod.Wrap(ErrInvalidPayloadStore,
			"url must be a base URL with no query string or fragment")
	}
	if strings.Contains(u.Path, "..") {
		return errorsmod.Wrap(ErrInvalidPayloadStore,
			"url path must not contain '..'")
	}
	return nil
}

// PayloadStoreEndpoint joins a registered base URL to a payment's path.
//
// One function, so the payer's wallet, the payee's client, the regulator's
// tooling and the store's own tests all derive the same URL. Two
// implementations of this that disagree by a slash produce a 404, and a 404
// here renders as detail that has been erased — which is a materially different
// statement from a path that was built wrong.
func PayloadStoreEndpoint(base, instructingParticipant, endToEndID string) string {
	return strings.TrimSuffix(base, "/") + "/payloads/" +
		url.PathEscape(instructingParticipant) + "/" + url.PathEscape(endToEndID)
}
