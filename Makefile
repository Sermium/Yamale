BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
COMMIT := $(shell git log -1 --format='%H')
APPNAME := blockchain

# do not override user values
ifeq (,$(VERSION))
  VERSION := $(shell git describe --exact-match 2>/dev/null)
  # if VERSION is empty, then populate it with branch name and raw commit hash
  ifeq (,$(VERSION))
    VERSION := $(BRANCH)-$(COMMIT)
  endif
endif

# Update the ldflags with the app, client & server names
ldflags = -X github.com/cosmos/cosmos-sdk/version.Name=$(APPNAME) \
	-X github.com/cosmos/cosmos-sdk/version.AppName=$(APPNAME)d \
	-X github.com/cosmos/cosmos-sdk/version.Version=$(VERSION) \
	-X github.com/cosmos/cosmos-sdk/version.Commit=$(COMMIT)

BUILD_FLAGS := -ldflags '$(ldflags)'

##############
###  Test  ###
##############

test-unit:
	@echo Running unit tests...
	@go test -mod=readonly -v -timeout 30m ./...

test-race:
	@echo Running unit tests with race condition reporting...
	@go test -mod=readonly -v -race -timeout 30m ./...

test-cover:
	@echo Running unit tests and creating coverage report...
	@go test -mod=readonly -v -timeout 30m -coverprofile=$(COVER_FILE) -covermode=atomic ./...
	@go tool cover -html=$(COVER_FILE) -o $(COVER_HTML_FILE)
	@rm $(COVER_FILE)

bench:
	@echo Running unit tests with benchmarking...
	@go test -mod=readonly -v -timeout 30m -bench=. ./...

# The simulation tests are gated behind -Enabled=true, so a plain `go test ./...`
# skips them entirely. These targets are the only thing that actually exercises
# the modules' randomized operations.
SIM_NUM_BLOCKS ?= 100
SIM_BLOCK_SIZE ?= 200
SIM_SEED ?= 7

test-sim:
	@echo Running the full application simulation...
	@go test -mod=readonly ./app -run TestFullAppSimulation -Enabled=true \
		-NumBlocks=$(SIM_NUM_BLOCKS) -BlockSize=$(SIM_BLOCK_SIZE) -Commit=true \
		-Seed=$(SIM_SEED) -Period=1 -v -timeout 30m

test-sim-import-export:
	@echo Running the simulation export/import round trip...
	@go test -mod=readonly ./app -run TestAppImportExport -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Seed=$(SIM_SEED) -Period=1 -timeout 30m

test-sim-determinism:
	@echo Checking simulation state determinism...
	@go test -mod=readonly ./app -run TestAppStateDeterminism -Enabled=true \
		-NumBlocks=30 -BlockSize=100 -Commit=true -Period=1 -timeout 60m

test-sim-all: test-sim test-sim-import-export test-sim-determinism

test: govet govulncheck test-unit

.PHONY: test test-unit test-race test-cover bench test-sim test-sim-import-export test-sim-determinism test-sim-all

#################
###  Install  ###
#################

all: install

install:
	@echo "--> ensure dependencies have not been modified"
	@go mod verify
	@echo "--> installing $(APPNAME)d"
	@go install $(BUILD_FLAGS) -mod=readonly -tags "$(PROFILE_TAGS)" ./cmd/$(APPNAME)d

.PHONY: all install

##################
###  Profiles  ###
##################

# Build profiles are build tags, not configuration. A module disabled by a
# config flag is still linked in, still reachable if the flag is wrong, and
# still inside the scope an auditor has to be paid to read. A module excluded
# by a tag is in none of those.
#
# PROFILES is every combination CI has to keep compiling. Adding a tag without
# adding it here is how a profile rots: it breaks on the commit that breaks it
# and nobody finds out until someone tries to cut a release from it.
#
#   (empty)         every chain module, no IBC — the default
#   ibc             the above plus IBC; reproduces the pre-profile build exactly
#   settlement      the sovereign settlement profile, no IBC: x/paymsg,
#                   x/stablecoin, x/treasury, x/oracle, x/enforcement, x/alias,
#                   x/validatorgov and governance, with fees in the issued
#                   currency routed to a treasury operating account
#   settlement,ibc  the above, with IBC
PROFILES ?= "" ibc settlement settlement,ibc

# Overridable so `make install PROFILE_TAGS=settlement` produces the profile
# binary. Empty is the default build.
PROFILE_TAGS ?=

# Compile every profile. Catches the usual failure, which is a reference to an
# excluded module added to a shared file where the default build never sees it.
build-profiles:
	@for tags in $(PROFILES); do \
		echo "--> building profile [$$tags]"; \
		go build -tags "$$tags" ./... || exit 1; \
	done

# Run the app tests under every profile. The app package is where the wiring
# lives, so it is the package a profile actually breaks.
test-profiles:
	@for tags in $(PROFILES); do \
		echo "--> testing profile [$$tags]"; \
		go test -tags "$$tags" ./app/... || exit 1; \
	done

# Prove exclusion rather than assert it. `go list -deps` reports the packages
# actually linked into the binary, so a module that is merely unregistered —
# still compiled in, still in audit scope — fails here even though it builds
# and every test passes. This is the check the profiles exist for.

# The modules scope §3 keeps out of the settlement profile. x/enforcement and
# x/alias are deliberately absent from this list: §4 keeps enforcement oversight
# and calls it critical for sovereign sale, and the jurisdictional perimeter
# x/alias carries is load-bearing in every profile.
SETTLEMENT_EXCLUDED := emission amm builderfee custody tokenisation land

check-profiles:
	@for m in $(SETTLEMENT_EXCLUDED); do \
		echo "--> x/$$m must be absent from the settlement profile"; \
		if go list -tags settlement -deps ./cmd/$(APPNAME)d | grep -q "yamale/blockchain/x/$$m"; then \
			echo "FAIL: x/$$m is linked into the settlement build"; exit 1; \
		fi; \
		echo "--> x/$$m must be present without it"; \
		go list -deps ./cmd/$(APPNAME)d | grep -q "yamale/blockchain/x/$$m" || \
			{ echo "FAIL: x/$$m is missing from the default build"; exit 1; }; \
	done
	@echo "--> the modules the settlement profile does carry must still be linked"
	@for m in paymsg stablecoin treasury oracle enforcement alias validatorgov constitution; do \
		go list -tags settlement -deps ./cmd/$(APPNAME)d | grep -q "yamale/blockchain/x/$$m" || \
			{ echo "FAIL: x/$$m is missing from the settlement build"; exit 1; }; \
	done
	@echo "--> ibc-go must be absent unless the ibc tag is passed"
	@if go list -deps ./cmd/$(APPNAME)d | grep -q "ibc-go"; then \
		echo "FAIL: ibc-go is linked into a build that did not ask for it"; exit 1; \
	fi
	@echo "--> ibc-go must be present with it"
	@go list -tags ibc -deps ./cmd/$(APPNAME)d | grep -q "ibc-go" || \
		{ echo "FAIL: ibc-go is missing from the ibc build"; exit 1; }
	@echo "all profile exclusions hold"

.PHONY: build-profiles test-profiles check-profiles

##################
###  Protobuf  ###
##################

# Use this target if you do not want to use Ignite for generating proto files

proto-deps:
	@echo "Installing proto deps"
	@echo "Proto deps present, run 'go tool' to see them"

# buf and every protoc plugin are already `go tool` dependencies, so protobuf
# generation needs nothing installed beyond the Go toolchain. This target used
# to shell out to `ignite`, which meant anyone without the Ignite CLI simply
# could not regenerate protos; buf produces byte-identical output.
# buf emits into a nested tree mirroring the go_package path
# (./yamale/blockchain/x/<module>/types), so the generated files are relocated
# into x/ and the scratch tree removed. Ignite used to do this relocation
# invisibly; doing it here is what lets the target work without Ignite.
#
# The scratch directory has to match the go_package prefix in the .proto files.
# It said `sermium` after the module was renamed to `yamale`, which meant the
# copy silently found nothing and proto-gen appeared to succeed while changing
# nothing — and proto-check, which is the guard against a checked-in .pb.go
# drifting from its .proto, passed for the same reason.
proto-gen:
	@echo "Generating protobuf files..."
	@rm -rf yamale
	@go tool github.com/bufbuild/buf/cmd/buf generate --template proto/buf.gen.gogo.yaml
	@cp -r yamale/blockchain/x/. x/
	@rm -rf yamale

# Regenerate and fail if anything changed — catches a checked-in .pb.go that
# has drifted from its .proto.
proto-check: proto-gen
	@echo "Checking generated protobuf files are up to date..."
	@git diff --exit-code -- '*.pb.go' '*.pb.gw.go'

proto-lint:
	@echo "Linting protobuf files..."
	@go tool github.com/bufbuild/buf/cmd/buf lint

# The chain's own messages, generated into TypeScript for the client SDK.
# Without this the SDK can only sign the standard Cosmos messages, because
# signing means protobuf-encoding and CosmJS encodes only what it has a type
# for — payments, treasury spends and swaps were CLI-only for exactly that
# reason. Output is generated; do not hand-edit clients/sdk/src/generated.
proto-ts:
	@echo "Generating TypeScript types..."
	@rm -rf clients/sdk/src/generated
	@# x/feegrant is how an institution pays the network fee for its customers,
	@# so its messages have to be buildable from a browser. They come from the
	@# SDK rather than from this repository's protos, and nothing here imports
	@# them, so buf would not otherwise generate them.
	@#
	@# This runs FIRST, and the repository's own generation runs after it, because
	@# the two overlap: both emit google/protobuf/*.ts and cosmos/base/*.ts. A
	@# module named on the command line resolves its own dependencies rather than
	@# this repository's buf.lock, and the cosmos-sdk module pins an older
	@# well-known-types than we do — so generating it second silently downgraded
	@# descriptor.ts and timestamp.ts, and undid the sed below by writing
	@# descriptor.ts after it had already been corrected. Ordered this way, the
	@# pinned buf.lock wins every shared file and the SDK module contributes only
	@# what we do not generate ourselves: cosmos/feegrant, any.ts and duration.ts.
	@#
	@# Pinned to the same commit buf.lock records, so the output does not change
	@# under us when the module is updated upstream.
	@go tool github.com/bufbuild/buf/cmd/buf generate buf.build/cosmos/cosmos-sdk:65fa41963e6a41dd95a35934239029df --template proto/buf.gen.ts.yaml --path cosmos/feegrant/v1beta1
	@go tool github.com/bufbuild/buf/cmd/buf generate --template proto/buf.gen.ts.yaml
	@# importSuffix makes relative imports explicit, which Node's type stripping
	@# requires — but ts-proto applies it to the protobufjs import too, and that
	@# is a package, whose entry point is .js and stays .js. Corrected here
	@# rather than by hand, so the tree stays reproducible from `make proto-ts`.
	@find clients/sdk/src/generated -name '*.ts' -exec sed -i 's#protobufjs/minimal\.ts#protobufjs/minimal.js#' {} +

proto-ts-check: proto-ts
	@echo "Checking the generated TypeScript is up to date..."
	@git diff --exit-code -- clients/sdk/src/generated

.PHONY: proto-gen proto-check proto-lint proto-ts proto-ts-check

#####################
###  Documentation ###
#####################

# The reference documentation is generated from the protobuf descriptors, the
# modules' registered errors and their DefaultParams(). Hand-writing it would
# guarantee it drifts, and a reference that is subtly wrong is worse than none.
docs:
	@echo "Generating reference documentation..."
	@go tool github.com/bufbuild/buf/cmd/buf build --as-file-descriptor-set --exclude-imports \
		-o $(DOCS_DESCRIPTOR)
	@go run ./tools/docgen --descriptor $(DOCS_DESCRIPTOR) --out docs/reference --root .
	@rm -f $(DOCS_DESCRIPTOR)

DOCS_DESCRIPTOR := .docgen-descriptor.binpb

# Fails when the committed reference has fallen behind the code.
docs-check: docs
	@echo "Checking the reference documentation is up to date..."
	@git diff --exit-code -- docs/reference

# The published REST specification, merged from the per-proto documents
# protoc-gen-openapiv2 emits. Ignite used to run this merge, so when the project
# stopped using Ignite the committed spec silently stopped being regenerated and
# drifted two whole modules behind the chain.
OPENAPI_TMP := .openapi-tmp

openapi:
	@echo "Generating the OpenAPI specification..."
	@rm -rf $(OPENAPI_TMP)
	@go tool github.com/bufbuild/buf/cmd/buf generate --template proto/buf.gen.sta.yaml -o $(OPENAPI_TMP)
	@go run ./tools/openapi --in $(OPENAPI_TMP) --out docs/static/openapi.json
	@rm -rf $(OPENAPI_TMP)

openapi-check: openapi
	@echo "Checking the OpenAPI specification is up to date..."
	@git diff --exit-code -- docs/static/openapi.json

# The currencies this chain issues on testnet, from one table in
# tools/currencies into the four places that need them. Seeding genesis takes
# the issuer's address, so it is a ceremony step rather than a make target:
#
#   go run ./tools/currencies --genesis <path> --issuer <foundation address>
#
# This target only refreshes the client's display registry, which is the copy
# most likely to drift — a currency the chain mints and the SDK has never heard
# of renders as raw base units.
# The offline key tool. Built separately from the node because the whole point
# is that it can live on a machine the node never touches.
wallet:
	@echo "Building the wallet tool..."
	@go build -o yamale-wallet ./tools/wallet

.PHONY: wallet

currencies-ts:
	@echo "Generating the client currency registry..."
	@go run ./tools/currencies --emit-ts

# The website's copy of the documentation, rendered from the same Markdown the
# repository serves. clients/site/docs is generated output — regenerate it, do
# not edit it, for the same reason docs/reference is generated: two copies of an
# explanation drift, and the website is the copy nobody remembers to update.
site-docs:
	@node clients/site/build-docs.mjs

# The hosted ceremony's client bundle. tools/ceremony/hosted is generated output
# and it is COMMITTED rather than gitignored, which is unusual here and
# deliberate: the ceremony binary embeds it, so `go build ./...` has to work on a
# clone with no JavaScript toolchain, and a coordinator has to be able to serve
# the ceremony from one file. Rebuild it whenever clients/ceremony changes, and
# note that the digest `ceremony host` prints — the one a custodian compares
# against a published value — moves with it.
ceremony-ui:
	@echo "Building the hosted ceremony bundle..."
	@cd clients && npm run build --workspace @yamale/ceremony

# Fails when the committed bundle has fallen behind clients/ceremony. Not one of
# the drift guards CI runs, because it needs npm as well as Go; run it by hand
# after touching the ceremony client.
ceremony-ui-check: ceremony-ui
	@echo "Checking the hosted ceremony bundle is up to date..."
	@git diff --exit-code -- tools/ceremony/hosted

.PHONY: docs docs-check openapi openapi-check currencies-ts site-docs ceremony-ui ceremony-ui-check

#################
###  Linting  ###
#################

lint:
	@echo "--> Running linter"
	@go tool github.com/golangci/golangci-lint/cmd/golangci-lint run ./... --timeout 15m

lint-fix:
	@echo "--> Running linter and fixing issues"
	@go tool github.com/golangci/golangci-lint/cmd/golangci-lint run ./... --fix --timeout 15m

.PHONY: lint lint-fix

###################
### Development ###
###################

govet:
	@echo Running go vet...
	@go vet ./...

govulncheck:
	@echo Running govulncheck...
	@go tool golang.org/x/vuln/cmd/govulncheck@latest
	@govulncheck ./...

.PHONY: govet govulncheck