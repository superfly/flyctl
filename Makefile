NOW_RFC3339 = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_BRANCH = $(shell git symbolic-ref --short HEAD 2>/dev/null ||:)

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

LDFLAGS = -X 'github.com/superfly/flyctl/internal/buildinfo.buildDate=$(NOW_RFC3339)'\
 -X 'github.com/superfly/flyctl/internal/buildinfo.branchName=$(GIT_BRANCH)'\
 -X 'github.com/superfly/flyctl/internal/buildinfo.buildVersion=${VERSION}'

all: build cmddocs

generate:
	go generate ./...

build: generate
	CGO_ENABLED=0 go build -o bin/flyctl -ldflags="$(LDFLAGS)" .

build_linux_amd64:
	build GOOS=linux GOARCH=amd64

build_macos_arm64:
	build GOOS=macos GOARCH=arm64

test: FORCE
	go test ./... -ldflags="$(LDFLAGS)" --run=$(T)

test-api: FORCE
	cd api && go test ./... --run=$(T)

test-api: FORCE
	cd ./api && go test ./... -ldflags="-X 'github.com/superfly/flyctl/internal/buildinfo.buildDate=$(NOW_RFC3339)'" --run=$(T)

raw-preflight-test:
	if [ -r .direnv/preflight ]; then . .direnv/preflight; fi; \
	go test ./test/preflight --tags=integration -v -timeout 10m --run="$(T)"

# to run one test, use: make preflight-test T=TestAppsV2ConfigSave
preflight-test: build
	$(MAKE) raw-preflight-test

ci-preflight:
	$(MAKE) preflight-test FLY_PREFLIGHT_TEST_NO_PRINT_HISTORY_ON_FAIL=true

cmddocs: generate
	@echo Running Docs Generation
	bash scripts/generate_docs.sh


pre:
	pre-commit run --all-files

FORCE:
