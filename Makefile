generate:
	go generate ./...

build: generate
	go build -o bin/flyctl . 

cmddocs: generate
	sh scripts/generate_docs.sh