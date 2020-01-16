cmddocs:
	sh scripts/helpgen.sh

generate:
	go generate ./...

build: generate
	go build -o bin/flyctl . 
