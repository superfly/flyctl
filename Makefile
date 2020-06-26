all: build cmddocs

generate:
	@echo Running Generate for Help
	go generate ./...

build: generate
	@echo Running Build
	go build -o bin/flyctl . 

cmddocs: generate
	@echo Running Docs Generation
	bash scripts/generate_docs.sh
