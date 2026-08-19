package app

import "strings"

// ProfileName names the build profile this binary was compiled as.
//
// It exists so that a failure caused by running the wrong binary against a
// genesis says which binary it is. "unknown section: emission" sends an
// operator looking for a corrupt file; "unknown section: emission, and this is
// the settlement profile" tells them they started the wrong build, which is
// what actually happened.
//
// The name is assembled from the tags rather than written out per combination,
// because a constant per combination is four constants that have to be kept in
// step with two independent tags — and the one that rots is the one nobody
// builds.
func ProfileName() string {
	tags := make([]string, 0, 2)
	for _, tag := range []string{profileTagSettlement, profileTagIBC} {
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		return "default"
	}
	return strings.Join(tags, ",")
}
