cmddocs: generate
	sh scripts/generate_docs.sh

generate:
	go generate ./...

build: generate
	go build -o bin/flyctl . 
