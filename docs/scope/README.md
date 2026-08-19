# Scope of record

[`revised-scope.md`](revised-scope.md) is the authoritative scope for Yamale. It supersedes every
earlier enhancement specification in this repository and in prior discussion.

Two decisions in it reframe the work rather than extend it:

- **Payments and land are separate deployments** sharing infrastructure, not a
  chain. Land is the most sovereign asset class there is; no state will place
  its cadastre on a ledger validated by other states. The shared component is
  therefore the stack, not the validator set.
- **Yamale is a distribution, not a network** — a permissioned financial-ledger
  platform shipped as profiles and deployed by others. There is no network to
  operate and no continental rail to be granted permission to build.

That second point dissolves the hardest problem in the previous framing and
introduces obligations this repository does not yet meet: long-term support
branches, a backport policy, upgrade tooling, deployment certification, and a
legal entity able to sign warranties.

Anything in `docs/guides/` that assumes a single chain carrying every module
should be read against this document, not the other way round.

## What is in this directory

- [revised-scope.md](revised-scope.md) — the specification itself, as issued,
  with decisions taken since recorded at the end.
- [gaps.md](gaps.md) — what the tree does not yet meet, checked against the
  source rather than against the task list.
- [roles-and-perimeter.md](roles-and-perimeter.md) — how a stakeholder's powers
  get bounded by country, and why the country cannot live in the address.
