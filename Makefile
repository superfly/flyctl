NOW_RFC3339 = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_SHA = $(shell git rev-parse HEAD 2>/dev/null || no_git)
GIT_BRANCH = $(shell git symbolic-ref --short HEAD 2>/dev/null ||:)
ifneq ($(GIT_BRANCH),)
GIT_COMMIT = $(GIT_SHA) ($(GIT_BRANCH))
else
GIT_COMMIT = $(GIT_SHA)
endif

all: build cmddocs

generate:
	@echo Running Generate for Help and GraphQL client
	go generate ./...

build: generate
	@echo Running Build
	go build -o bin/flyctl -ldflags="-X 'github.com/superfly/flyctl/internal/buildinfo.buildDate=$(NOW_RFC3339)' -X 'github.com/superfly/flyctl/internal/buildinfo.commit=$(GIT_COMMIT)'" .

test: FORCE
	go test ./... -ldflags="-X 'github.com/superfly/flyctl/internal/buildinfo.buildDate=$(NOW_RFC3339)'"

preflight-test: build
	go test ./test/preflight --tags=integration -v

cmddocs: generate
	@echo Running Docs Generation
	bash scripts/generate_docs.sh


pre:
	pre-commit run --all-files

FORCE:
