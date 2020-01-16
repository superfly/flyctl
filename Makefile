cmddocs:
	go run helpgen/helpgen.go helpgen/flyctlhelp.toml | gofmt -s > docstrings/gen.go

generate:
	go generate ./...

build: generate
	go build -o bin/flyctl . 
