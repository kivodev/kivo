# Tools run at pinned versions through `go run`, so nothing needs installing.
SQLC        := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0
STATICCHECK := go run honnef.co/go/tools/cmd/staticcheck@2026.1
DEADCODE    := go run golang.org/x/tools/cmd/deadcode@v0.51.0
DUPL        := go run github.com/mibk/dupl@v1.1.0
GOCYCLO     := go run github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0
GOCOGNIT    := go run github.com/uudashr/gocognit/cmd/gocognit@v1.2.1
GO_LICENSES := go run github.com/google/go-licenses/v2@v2.0.1
# The OpenAPI tools run at pinned versions through npx (Node 20 or later).
OPENAPI_TS  := npx --yes openapi-typescript@7.13.0
REDOCLY     := REDOCLY_TELEMETRY=off REDOCLY_SUPPRESS_UPDATE_NOTICE=true npx --yes @redocly/cli@2.53.3
# So do the duplication, unused-code and import-cycle tools.
JSCPD       := npx --yes jscpd@4.3.0
KNIP        := npx --yes knip@6.38.0
MADGE       := npx --yes madge@8.0.0
# openapi-generator writes the native apps' Kotlin client (Java 17 or later),
# from its release jar, fetched once into bin/ and checked against its hash.
OPENAPI_GENERATOR_VERSION := 7.26.0
OPENAPI_GENERATOR_SHA256  := 1760050094997b9cc790cc1350be095f15da8c08a996b184d3eadc4ccda5e35e
OPENAPI_GENERATOR         := bin/openapi-generator-cli-$(OPENAPI_GENERATOR_VERSION).jar
KOTLIN_API                := ../clients/api
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# The Go code the checks cover, leaving out web/node_modules, which can hold
# Go files of its own.
GO_PKGS     := ./cmd/... ./internal/... ./web
GO_FILES    := cmd internal $(wildcard web/*.go)

include ceilings.mk

.PHONY: build test budget lint ceilings ceilings-server ceilings-web ceilings-clients generate fmt check-generated web notices

# bin/THIRD-PARTY-NOTICES.txt, which the image ships: packaging's notices
# with the Dockerfile's jellyfin-ffmpeg, each Go module in the build with its
# license (go-licenses), Go's own (its link where a distribution's Go has no
# LICENSE file), then CLDR's Unicode license. The Windows build
# writes its own the same way (packaging/windows/build.ps1). A module whose
# license go-licenses can't read fails it. go-licenses doesn't know MIT-0,
# go-sqlite3-wasm's license, so that module's LICENSE is added as it is.
FFMPEG_VERSION := $(shell sed -n 's/^ARG JELLYFIN_FFMPEG=//p' Dockerfile)
SQLITE_WASM    := github.com/ncruces/go-sqlite3-wasm/v6
notices:
	@mkdir -p bin
	set -e; { \
		sed 's/{ffmpeg}/$(FFMPEG_VERSION)/g' packaging/THIRD-PARTY-NOTICES.txt; \
		$(GO_LICENSES) report ./cmd/kivo --ignore github.com/mmcdole/kivo --ignore $(SQLITE_WASM) --template packaging/go-licenses.tpl; \
		printf '\n%s\n%s (MIT-0)\n\n' '================================================================================' "$$(go list -m $(SQLITE_WASM))"; \
		cat "$$(go list -m -f '{{.Dir}}' $(SQLITE_WASM))/LICENSE"; \
		printf '\n%s\nThe Go standard library, %s (BSD-3-Clause)\n\n' '================================================================================' "$$(go env GOVERSION)"; \
		cat "$$(go env GOROOT)/LICENSE" 2>/dev/null || echo 'https://go.dev/LICENSE'; \
		printf '\n%s\nUnicode CLDR\n\n' '================================================================================'; \
		cat ../shared/credits/unicode-license.txt; \
	} > bin/THIRD-PARTY-NOTICES.txt.tmp
	mv bin/THIRD-PARTY-NOTICES.txt.tmp bin/THIRD-PARTY-NOTICES.txt

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/kivo ./cmd/kivo

test:
	go test -race $(GO_PKGS)

# The read budgets, timed alone and without the race detector;
# a plain go test skips them.
budget:
	KIVO_BUDGET=1 go test -count=1 -run TestBudgets ./internal/query

# The dependency and write-ownership lints are tests in internal/archtest;
# redocly checks the OpenAPI spec, including that every operation has an
# access level.
lint:
	$(REDOCLY) lint --config internal/api/redocly.yaml internal/api/openapi.yaml
	go vet $(GO_PKGS)
	$(STATICCHECK) $(GO_PKGS)
	go test ./internal/archtest
	@test -z "$$(gofmt -l $(GO_FILES))" || { gofmt -l $(GO_FILES); echo "gofmt needed"; exit 1; }

# The code-health counts, each held at its ceiling in ceilings.mk. They
# leave out tests, the web client's mocks and generated files, and
# ceilings-web needs web/node_modules.
ceilings: ceilings-server ceilings-web ceilings-clients

ceilings-server ceilings-web ceilings-clients: SHELL := bash
ceilings-server ceilings-web ceilings-clients: .SHELLFLAGS := -o pipefail -c

ceilings-server:
	@$(call ceiling,deadcode,$(DEADCODE_MAX),$(DEADCODE) ./cmd/... | wc -l)
	@$(call ceiling,dupl,$(DUPL_MAX),$(DUPL) -t 50 $(GO_SOURCES) | sed -n 's/^Found total \([0-9]*\) clone groups.*/\1/p')
	@$(call ceiling,gocyclo,$(GOCYCLO_MAX),$(GOCYCLO) $(GO_SOURCES) | awk '$$1 > 15' | wc -l)
	@$(call ceiling,gocognit,$(GOCOGNIT_MAX),$(GOCOGNIT) $(GO_SOURCES) | awk '$$1 > 20' | wc -l)
	@$(call ceiling,api handlers,$(API_HANDLERS_MAX),$(GOCOGNIT) $(filter internal/api/%,$(GO_SOURCES)) | awk '$$1 > 15 && (index($$3$(comma) "(*handlers).") == 1 || index($$3$(comma) "(*Server).serve") == 1)' | wc -l)
	@$(call ceiling,jscpd go,$(JSCPD_GO_MAX),$(call jscpd,--format go $(GO_SOURCES)))
	@$(call ceiling,jscpd sql,$(JSCPD_SQL_MAX),$(call jscpd,--format sql $(wildcard internal/*/queries/*.sql)))

ceilings-web:
	@$(call ceiling,knip,$(KNIP_MAX),cd web && $(KNIP) --reporter json --no-exit-code | node -e '$(KNIP_COUNT)')
	@$(call ceiling,madge cycles,$(MADGE_CYCLES_MAX),cd web && { $(MADGE) --circular --json --extensions ts$(comma)tsx --ts-config tsconfig.json src || true; } | node -p 'JSON.parse(require("fs").readFileSync(0)).length')
	@$(call ceiling,jscpd web,$(JSCPD_WEB_MAX),$(call jscpd,--format typescript$(comma)tsx$(comma)javascript --ignore "$(WEB_EXCLUDED)" web/src))

# The native apps' side of their boundary with the core, read from their
# sources: the Android app's and the Apple apps'.
ceilings-clients:
	@$(call ceiling,app api calls,$(APP_API_CALLS_MAX),$(call app_lines,'\b(connection\.(works|plays|setup|users|http)|http\.(get|post|put|delete|patch|request|prepare[A-Za-z]*))\b',$(APP_SOURCES)))
	@$(call ceiling,app core wiring,$(APP_WIRING_MAX),$(call app_lines,$(APP_WIRING),$(APP_SOURCES)))
	@$(call ceiling,app api urls,$(APP_API_URLS_MAX),$(call app_lines,'/api/v1/',$(APP_SOURCES)))
	@$(call ceiling,unanswered rules,$(UNANSWERED_MAX),$(call app_lines,'502\.\.\.?504',$(APP_SOURCES) --exclude=RequestProblem.kt $(CORE_SOURCES)))
	@$(call ceiling,swift Kt names,$(SWIFT_KT_MAX),{ grep -rhoE --include='*.swift' '\b[A-Z][A-Za-z0-9_]*Kt\.' $(APP_SOURCES) || true; } | wc -l)
	@$(call ceiling,app player timers,$(APP_PLAYER_TIMERS_MAX),$(call app_lines,$(APP_TIMERS),$(APP_PLAYERS)))
	@$(call ceiling,app formats,$(APP_FORMATS_MAX),$(call app_lines,$(APP_FORMATS),$(APP_SOURCES)))

# Sorted, since dupl's and jscpd's counts depend on the order of the files.
GO_SOURCES    = $(sort $(shell grep -rL --include='*.go' --exclude='*_test.go' '^// Code generated' $(GO_FILES)))
WEB_EXCLUDED := **/*.test.ts,**/*.test.tsx,**/src/test/**,**/src/api/mocks/**,**/src/api/schema.d.ts,**/src/i18n/locales/**
KNIP_COUNT    := let n = 0; for (const f of JSON.parse(require("fs").readFileSync(0)).issues) for (const [k, v] of Object.entries(f)) if (k !== "file") n += Object.keys(v).length; console.log(n)
APP_SOURCES   := ../clients/android/src/main ../clients/apple/Kivo ../clients/apple/KivoUI
# The core's session wiring, as an app would build it: its constructors.
APP_WIRING    := '\b(ServerConnection|LiveEvents|SignIn|Discovery|ServerList|ClientId|DeviceSettingsStore|Bandwidth|CapabilityReport|ServerPlays|PlayController|CastSender|UserSettings|LinkDevice|SubtitleSearcher|[A-Z][A-Za-z]*Repository)\('
CORE_SOURCES  := $(wildcard ../clients/core/src/*Main)
# The apps' video players, and the waits a timed rule would need.
APP_PLAYERS   := ../clients/android/src/main/kotlin/com/mmcdole/kivo/android/player ../clients/apple/Kivo/Player
APP_TIMERS    := '\b(delay\(|Task\.sleep|asyncAfter|Timer\.|withTimeout)'
# Numbers formatted, and units and time left worded, by the apps themselves.
APP_FORMATS   := '\b(NumberFormat|MeasureFormat)\.|NumberFormatter\(|ByteCountFormatter|DateComponentsFormatter|String\(format:|%02l?d|Mb/s|min left'
comma         := ,
space         := $(subst ,, )

# ceiling prints a count and fails when it's over its ceiling:
# $(call ceiling,name,ceiling,command printing the count).
ceiling = n=$$($(3)) && echo "$(1): $$n (ceiling $(2))" && \
	{ [ "$$n" -le $(2) ] || { echo "$(1) is over its ceiling in ceilings.mk"; exit 1; }; }

# app_lines prints how many lines of Kotlin and Swift code, leaving out
# comments, match a pattern: $(call app_lines,pattern,paths).
app_lines = { grep -rhE --include='*.kt' --include='*.swift' $(1) $(2) || true; } | { grep -vE '^\s*(//|\*|/\*)' || true; } | wc -l

# jscpd prints the lines in clones of 50 tokens or more: $(call jscpd,options and paths).
jscpd = rm -rf bin/jscpd && $(JSCPD) --silent --min-tokens 50 --reporters json --output bin/jscpd $(1) >/dev/null && \
	node -p 'require("./bin/jscpd/jscpd-report.json").statistics.total.duplicatedLines' && rm -rf bin/jscpd

# The Kotlin client reads the spec with its named enums' values left out, so
# they're open strings: the generator's enum classes fail to
# decode a value a later server adds. A named enum is a component schema
# whose enum list starts at its schema's top level.
KOTLIN_OPEN_ENUMS := /^  schemas:/ { s = 1 } s && /^      enum: \[/ { skip = 1 } skip { if (/\]/) skip = 0; next } { print }
# The enums the apps branch on are hand-written in OpenEnums.kt instead,
# with an Unknown case for a value a later server adds.
KOTLIN_ENUMS := TrackType TrackDelivery PlayDelivery DownloadDelivery SubtitleMode
KOTLIN_ENUM_MAPPINGS := $(subst $(space),$(comma),$(foreach e,$(KOTLIN_ENUMS),$(e)=com.mmcdole.kivo.api.models.$(e)))

# sqlc.yaml has a block per package with a queries/ folder; query runs its own
# statements through database/sql.
# The Kotlin client is written afresh, so a schema taken out of the spec
# leaves no file behind; .openapi-generator-ignore keeps the hand-written ones.
generate: $(OPENAPI_GENERATOR)
	go generate $(GO_PKGS)
	@if [ -f sqlc.yaml ]; then $(SQLC) generate; fi
	$(OPENAPI_TS) internal/api/openapi.yaml -o web/src/api/schema.d.ts
	@if [ -f $(KOTLIN_API)/.openapi-generator/FILES ]; then cd $(KOTLIN_API) && xargs rm -f < .openapi-generator/FILES; fi
	awk '$(KOTLIN_OPEN_ENUMS)' internal/api/openapi.yaml > bin/openapi-kotlin.yaml
	java -jar $(OPENAPI_GENERATOR) generate -g kotlin --library multiplatform \
		-i bin/openapi-kotlin.yaml -o $(KOTLIN_API) \
		--schema-mappings=$(KOTLIN_ENUM_MAPPINGS) \
		--additional-properties=packageName=com.mmcdole.kivo.api,dateLibrary=kotlinx-datetime,omitGradleWrapper=true

$(OPENAPI_GENERATOR):
	@mkdir -p $(@D)
	curl -fsSL -o $@.tmp https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/$(OPENAPI_GENERATOR_VERSION)/openapi-generator-cli-$(OPENAPI_GENERATOR_VERSION).jar
	echo "$(OPENAPI_GENERATOR_SHA256)  $@.tmp" | shasum -a 256 -c -
	mv $@.tmp $@

fmt:
	gofmt -w $(GO_FILES)

# Untracked files count too: a schema added to the spec is a new Kotlin file.
check-generated: generate
	@git diff --exit-code -- . $(KOTLIN_API) && test -z "$$(git ls-files --others --exclude-standard -- . $(KOTLIN_API))" || \
		{ git ls-files --others --exclude-standard -- . $(KOTLIN_API); echo "generated files are stale: run make generate"; exit 1; }

# The web client (Node 22): Biome with the layering rules, the React hooks
# lint and the raw-style lint, then tsc, Vitest, a check that the generated
# tokens and message catalog are current, and the production build.
web:
	cd web && npm ci && npm run lint && npm run typecheck && npm test && npm run check:generated && npm run build && npm run build:packaged
