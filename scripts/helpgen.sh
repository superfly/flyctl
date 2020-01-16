echo "generating cli help"

go run ../helpgen/helpgen.go ../helpgen/flyctlhelp.toml | gofmt -s > ../docstrings/gen.go
