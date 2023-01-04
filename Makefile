NOW_RFC3339 = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_SHA = $(shell git rev-parse HEAD 2>/dev/null || no_git)

all: build cmddocs

generate:
	@echo Running Generate for Help and GraphQL client
	go generate ./...

build: generate
	@echo Running Build
	go build -o bin/flyctl -ldflags="-X 'github.com/superfly/flyctl/internal/buildinfo.buildDate=$(NOW_RFC3339)' -X 'github.com/superfly/flyctl/internal/buildinfo.commit=$(GIT_SHA)'" .

test:
	go test ./... -ldflags="-X 'github.com/superfly/flyctl/internal/buildinfo.buildDate=$(NOW_RFC3339)'"

cmddocs: generate
	@echo Running Docs Generation
	bash scripts/generate_docs.sh


pre:
	pre-commit run --all-files
