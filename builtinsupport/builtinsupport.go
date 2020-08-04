package builtinsupport

import "fmt"

type Builtin struct {
	Name        string
	Description string
	FileText    string
}

var builtins map[string]Builtin

func GetBuiltin(builtinname string) (*Builtin, error) {
	initBuiltins()

	builtin, ok := builtins[builtinname]

	if !ok {
		return nil, fmt.Errorf("no builtin with %s name supported", builtinname)
	}

	return &builtin, nil
}

func GetBuiltins() []Builtin {
	initBuiltins()

	var builtarray []Builtin

	for _, v := range builtins {
		builtarray = append(builtarray, v)
	}

	return builtarray
}

func initBuiltins() {
	if len(builtins) != 0 {
		return
	}
	builtins = make(map[string]Builtin)

	for _, rt := range basicbuiltins {
		builtins[rt.Name] = rt
	}
}

var basicbuiltins = []Builtin{
	Builtin{Name: "node",
		Description: "Builtin Nodejs",
		FileText: `
			FROM node:current-alpine
			WORKDIR /app			
			COPY package.json .
			COPY package-lock.json .
			RUN npm install --production
			COPY . .
			ENV PORT=8080
			CMD [ "npm","start" ]
	`},
	Builtin{Name: "ruby",
		Description: "Builtin Ruby - runs app.rb",
		FileText: `
			FROM ruby:2.7
			WORKDIR /usr/src/app
			COPY Gemfile ./
			RUN bundle install
			COPY . .
			ENV PORT=8080
			EXPOSE 8080
			CMD ["bundle", "exec", "rackup", "--host", "0.0.0.0", "-p", "8080"]
`},
	Builtin{Name: "deno",
		Description: "Builtin Deno - runs main.ts, requires deps.ts",
		FileText: `
			FROM hayd/alpine-deno:1.2.1
			EXPOSE 8080
			WORKDIR /app
			USER deno
			COPY deps.ts .
			RUN deno cache deps.ts
			ADD . .
			RUN deno cache main.ts
			CMD ["run", "--allow-net", "main.ts"]
`},
	Builtin{Name: "go",
		Description: "Builtin Go - Builds app.go uses go modules",
		FileText: `
			FROM golang:1.13 as builder
			WORKDIR /go/src/app
			COPY . .
			RUN go mod download
			RUN CGO_ENABLED=0 GOOS=linux go build -v -o app
			FROM alpine:latest  
			COPY --from=builder /go/src/app/app /app
			COPY ./resources/ /resources/
			RUN apk --no-cache add ca-certificates
			EXPOSE 8080
			CMD ["/app"]
`},
	Builtin{Name: "static",
		Description: "Builtin Static - Static web server. All files are copied across and served",
		FileText: `
			FROM pierrezemb/gostatic
			COPY . /srv/http/
			CMD ["-port","8080"]
	`},
}
