package types

import (
	"bytes"
	"fmt"
	"strings"
)

// ValidateDocument checks everything about a deal paper that can be checked
// without reading state.
//
// Separated from the handler so it is testable on its own, because every rule
// here exists to stop one specific mistake and each of those mistakes deserves
// a test that names it.
func ValidateDocument(kind DocumentKind, title string, digest []byte, uri, language string) error {
	// A kind is how a reader tells the agreement that binds them from the pack
	// that describes the deal. An unspecified kind puts the most important
	// document on the list into the same bucket as the marketing deck.
	if kind == DOCUMENT_KIND_UNSPECIFIED {
		return ErrInvalidDocument.Wrap("say what kind of document this is")
	}
	if _, known := DocumentKind_name[int32(kind)]; !known {
		return ErrInvalidDocument.Wrapf("unknown document kind %d", int32(kind))
	}

	if title == "" {
		return ErrInvalidDocument.Wrap("a document needs a title a person can read")
	}
	if len(title) > MaxDocumentTitle {
		return ErrInvalidDocument.Wrapf("a title may not exceed %d bytes", MaxDocumentTitle)
	}
	if len(uri) > MaxDocumentURI {
		return ErrInvalidDocument.Wrapf("a location may not exceed %d bytes", MaxDocumentURI)
	}

	if len(digest) != SHA256Len {
		return ErrInvalidDocument.Wrapf(
			"a SHA-256 digest is %d bytes; this one is %d", SHA256Len, len(digest))
	}
	// Thirty-two zero bytes is not a digest anybody computed. It is what an
	// unset field looks like once it has been padded to the right length, and
	// accepting it would put a document in the register whose hash matches
	// nothing at all while looking exactly like one that does.
	if bytes.Equal(digest, make([]byte, SHA256Len)) {
		return ErrInvalidDocument.Wrap("that digest is thirty-two zero bytes, which is not a hash")
	}

	// Optional, because plenty of documents have no single language. When it is
	// given it has to be an ISO 639-1 code: these deals are written across
	// French, English and Portuguese jurisdictions, and which language a
	// binding agreement is in is not a presentational detail.
	if language != "" {
		if len(language) != 2 {
			return ErrInvalidDocument.Wrapf(
				"language %q is not an ISO 639-1 two-letter code", language)
		}
		for _, r := range language {
			if r < 'a' || r > 'z' {
				return ErrInvalidDocument.Wrapf(
					"language %q is not an ISO 639-1 two-letter code", language)
			}
		}
	}
	return nil
}

// DocumentKindLabel is the kind in words, for anything that has to print it.
//
// Kept beside the enum rather than in each interface, so a new kind cannot be
// added in one place and rendered as its enum name in three others.
func DocumentKindLabel(kind DocumentKind) string {
	switch kind {
	case DOCUMENT_KIND_TERM_SHEET:
		return "term sheet"
	case DOCUMENT_KIND_SUBSCRIPTION_AGREEMENT:
		return "subscription agreement"
	case DOCUMENT_KIND_SHAREHOLDERS_AGREEMENT:
		return "shareholders' agreement"
	case DOCUMENT_KIND_CONSTITUTIONAL:
		return "constitutional documents"
	case DOCUMENT_KIND_DISCLOSURE:
		return "disclosure"
	case DOCUMENT_KIND_DILIGENCE:
		return "diligence pack"
	case DOCUMENT_KIND_REPORT:
		return "report"
	case DOCUMENT_KIND_RESOLUTION:
		return "resolution"
	default:
		return "unspecified"
	}
}

// DigestHex is the digest as a person would compare it, lower case.
//
// `sha256sum` prints lower case, which is what somebody verifying a file will
// have in front of them, and a comparison that fails on case is a comparison
// that teaches people to stop making it.
func DigestHex(digest []byte) string {
	var b strings.Builder
	for _, c := range digest {
		fmt.Fprintf(&b, "%02x", c)
	}
	return b.String()
}
