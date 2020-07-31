package runtimesupport

import "fmt"

type Runtime struct {
	Name        string
	Description string
	FileText    string
}

var runtimes map[string]Runtime

func GetRuntime(runtimename string) (*Runtime, error) {
	if len(runtimes) == 0 {
		// Load runtimes
		initRuntimes()
	}

	runtime, ok := runtimes[runtimename]

	if !ok {
		return nil, fmt.Errorf("no runtime with %s name supported", runtimename)
	}

	return &runtime, nil
}

func initRuntimes() {
	runtimes = make(map[string]Runtime)

	runtimes["node"] = Runtime{Name: "node",
		Description: "A Nodejs Runtime",
		FileText: `
			FROM node:current-alpine
			WORKDIR /app			
			COPY package.json .
			COPY package-lock.json .
			RUN npm install --production
			COPY . .
			ENV PORT=8080
			CMD [ "npm","start" ]
	`}

	runtimes["ruby"] = Runtime{Name: "ruby",
		Description: "A Ruby Runtime - runs app.rb",
		FileText: `
			FROM ruby:2.7
			WORKDIR /usr/src/app
			COPY Gemfile ./
			RUN bundle install
			COPY . .
			ENV PORT=8080
			EXPOSE 8080
			CMD ["bundle", "exec", "rackup", "--host", "0.0.0.0", "-p", "8080"]
	`}

	runtimes["deno"] = Runtime{Name: "deno",
		Description: "A deno Runtime - runs main.ts, requires deps.ts",
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
	`}

	runtimes["go"] = Runtime{Name: "go",
		Description: "A Go Runtime - Builds app.go uses go modules",
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
	`}

}
