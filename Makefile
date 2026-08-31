.PHONY: m0-gate m0-offline m0-node-preflight m0-oracle m0-source-drift m0-freeze

PI_CODE_COMMIT := 936aff00918de1187f085f123c2812d8f2d67745
PI_TYPESCRIPT_VERSION := 5.9.3

m0-gate: m0-offline

m0-offline:
	go test ./... -count=1
	go test -race ./... -count=1
	go vet ./...
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...
	go run ./examples/m0-contracts

m0-node-preflight:
	@node --experimental-strip-types -e "" >/dev/null 2>&1 || (echo "freeze checks require Node >=22.6.0 with --experimental-strip-types" >&2; exit 2)

m0-oracle: m0-node-preflight
	@test -n "$(PIG_PI_ORACLE_CHECKOUT)" || (echo "PIG_PI_ORACLE_CHECKOUT must name a prepared locked Pi checkout" >&2; exit 2)
	@test "$$(git -C "$(PIG_PI_ORACLE_CHECKOUT)" rev-parse HEAD)" = "$(PI_CODE_COMMIT)" || (echo "Pi Oracle checkout is not at $(PI_CODE_COMMIT)" >&2; exit 2)
	@test -z "$$(git -C "$(PIG_PI_ORACLE_CHECKOUT)" status --porcelain=v1 --untracked-files=no)" || (echo "Pi Oracle checkout has tracked changes" >&2; exit 2)
	@test -f "$(PIG_PI_ORACLE_CHECKOUT)/node_modules/openai/package.json" || (echo "Pi Oracle dependencies are absent; run npm ci --ignore-scripts --prefix $(PIG_PI_ORACLE_CHECKOUT)" >&2; exit 2)
	@test -f "$(PIG_PI_ORACLE_CHECKOUT)/node_modules/partial-json/package.json" || (echo "Pi Oracle dependencies are incomplete" >&2; exit 2)
	@test -f "$(PIG_PI_ORACLE_CHECKOUT)/node_modules/chalk/package.json" || (echo "Pi Oracle dependencies are incomplete" >&2; exit 2)
	@test -f "$(PIG_PI_ORACLE_CHECKOUT)/packages/coding-agent/dist/cli.js" || (echo "Pi Oracle dist is absent; run npm run build:offline --prefix $(PIG_PI_ORACLE_CHECKOUT)" >&2; exit 2)
	node --experimental-strip-types parity/oracle/run.mjs "$(abspath $(PIG_PI_ORACLE_CHECKOUT))" --check
	node parity/oracle/codingagent-auth-help.mjs "$(abspath $(PIG_PI_ORACLE_CHECKOUT))" --check
	node --experimental-strip-types parity/oracle/openai-completions-m0-no-op.mjs "$(abspath $(PIG_PI_ORACLE_CHECKOUT))" --check
	node --experimental-strip-types parity/oracle/openai-completions-text.mjs "$(abspath $(PIG_PI_ORACLE_CHECKOUT))" --check
	node --experimental-strip-types parity/oracle/openai-completions-sse.mjs "$(abspath $(PIG_PI_ORACLE_CHECKOUT))" --check

m0-source-drift: m0-node-preflight
	@test -n "$(PIG_PI_SOURCE_CHECKOUT)" || (echo "PIG_PI_SOURCE_CHECKOUT must name a clean locked Pi checkout" >&2; exit 2)
	@test "$$(git -C "$(PIG_PI_SOURCE_CHECKOUT)" rev-parse HEAD)" = "$(PI_CODE_COMMIT)" || (echo "Pi checkout is not at $(PI_CODE_COMMIT)" >&2; exit 2)
	@test -z "$$(git -C "$(PIG_PI_SOURCE_CHECKOUT)" status --porcelain=v1 --untracked-files=all --ignored)" || (echo "Pi source checkout must have no tracked, untracked, or ignored files" >&2; exit 2)
	@test "$$(node -p 'require("./parity/extract/node_modules/typescript/package.json").version' 2>/dev/null)" = "$(PI_TYPESCRIPT_VERSION)" || (echo "surface extraction requires typescript@$(PI_TYPESCRIPT_VERSION); run npm ci --prefix parity/extract" >&2; exit 2)
	PIG_INVENTORY_DRIFT=1 PIG_PI_CHECKOUT="$(PIG_PI_SOURCE_CHECKOUT)" go test ./internal/inventory -run '^TestInventoryDriftAgainstUpstream$$' -count=1
	@set -eu; tmp=$$(mktemp -d /tmp/pig-m0-surface.XXXXXX); \
		trap 'rm -rf "$$tmp"' EXIT; \
		node parity/extract/surface.mjs "$(PIG_PI_SOURCE_CHECKOUT)" --out "$$tmp/symbols.jsonl"; \
		cmp parity/surface/symbols.jsonl "$$tmp/symbols.jsonl"; \
		node --experimental-strip-types parity/baseline/export-image-models.mjs "$(PIG_PI_SOURCE_CHECKOUT)" > "$$tmp/image-models.json"; \
		cmp parity/baseline/catalog/image/models.json "$$tmp/image-models.json"

m0-freeze: m0-gate m0-oracle m0-source-drift
