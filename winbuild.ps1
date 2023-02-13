# winbuild
cmd /c "go run .\helpgen\helpgen.go .\helpgen\flyctlhelp.toml > .\docstrings\gen.go"

 go fmt .\docstrings\gen.go

 go build -o bin/flyctl.exe .
