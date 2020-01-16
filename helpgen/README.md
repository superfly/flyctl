# helpgen

Code generator for help strings. Generator is helpgen.go. Currently outputs to stdout. Takes the name of a .toml file.

Toml file format (currently):

```toml
[info]
usage="info"
shortHelp="Show detailed app information"
longHelp="""Shows information about the application on the Fly platform

Information includes the application's
* name, owner, version, status and hostname
* services
* IP addresses
"""
```

Help TOML file is flyctlhelp.toml

run

```
go run helpgen/helpgen.go helpgen/flyctlhelp.toml > docstrings/flyctldocstrings.go
```

To generate docstrings/flyctldocstrings.go

```go
package docstrings

var docstrings=map[string]KeyStrings{
"info":KeyStrings{"info","Show detailed app information",
    `Shows information about the application on the Fly platform

Information includes the application's
* name, owner, version, status and hostname
* services
* IP addresses
`,
},
}

```

This contains a literal initialised map of all the KeyStrings. Consumed by docstrings/docstrings.go

TODO: Add Flag and Example support
