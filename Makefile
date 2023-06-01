NOW_RFC3339 = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_BRANCH = $(shell git symbolic-ref --short HEAD 2>/dev/null ||:)

all: build cmddocs

generate:
	@echo Running Generate for Help and GraphQL client
	go generate ./...

build: generate
	@echo Running Build
	CGO_ENABLED=0 go build -o bin/flyctl -ldflags="-X 'github.com/superfly/flyctl/internal/buildinfo.buildDate=$(NOW_RFC3339)' -X 'github.com/superfly/flyctl/internal/buildinfo.branchName=$(GIT_BRANCH)'" .

test: FORCE
	go test ./... -ldflags="-X 'github.com/superfly/flyctl/internal/buildinfo.buildDate=$(NOW_RFC3339)'" --run=$(T)

# to run one test, use: make preflight-test T=TestAppsV2ConfigSave
preflight-test: build
	if [ -r .direnv/preflight ]; then . .direnv/preflight; fi; \
	go test ./test/preflight --tags=integration -v -timeout 30m --run="$(T)"

ci-preflight:
	$(MAKE) preflight-test FLY_PREFLIGHT_TEST_NO_PRINT_HISTORY_ON_FAIL=true

cmddocs: generate
	@echo Running Docs Generation
	bash scripts/generate_docs.sh


pre:
	pre-commit run --all-files

FORCE:
