package builtinsupport

import "fmt"

type Builtin struct {
	Name        string
	Description string
	Details     string
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
	{Name: "node",
		Description: "Nodejs builtin",
		Details: `Requires package.json, package-lock.json and an app in server.js. 
Runs a production npm install and copies all files across. 
When run will call npm start to start the application.
Uses and exposes port 8080 internally.`,
		FileText: `
FROM node:current-slim
WORKDIR /app			
COPY package.json .
COPY package-lock.json .
RUN npm install --production
COPY . .
RUN npm run build --if-present
ENV PORT=8080
EXPOSE 8080
CMD [ "npm","start" ]
	`},
	{Name: "ruby",
		Description: "Ruby builtin",
		Details: `Builtin for a Ruby application with a Gemfile. Runs bundle install to build. 
At runtime, it uses rackup to run config.ru and start the application as configured.
Uses and exposes port 8080 internally.`,
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
	{Name: "deno",
		Description: "Deno builtin",
		Details: `Uses Alpine image from https://github.com/hayd/deno-docker.
runs main.ts with --allow-net set and requires deps.ts for dependencies.
Uses and exposes port 8080 internally.`,
		FileText: `
FROM hayd/debian-deno:1.4.0
ENV PORT=8080
EXPOSE 8080
WORKDIR /app
USER deno
COPY deps.ts .
RUN deno cache deps.ts
ADD . .
RUN deno cache main.ts
CMD ["run", "--allow-net", "main.ts"]
`},
	{Name: "go",
		Description: "Go Builtin",
		Details: `Builds main.go from the directory, the app should use go modules.
Uses and exposes port 8080 internally.
`,
		FileText: `
FROM golang:1.14 as builder
WORKDIR /go/src/app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -v -o app
FROM alpine:latest  
COPY --from=builder /go/src/app/app /goapp/app
WORKDIR /goapp
COPY . /throwaway
RUN cp -r /throwaway/resources ./resources || echo "No resources to copy"
RUN rm -rf /throwaway
RUN apk --no-cache add ca-certificates
ENV PORT=8080
EXPOSE 8080
CMD ["/goapp/app"]
`},
	{Name: "static",
		Description: "Web server builtin",
		Details:     `All files are copied to the image and served, except files with executable permission set.`,
		FileText: `
FROM pierrezemb/gostatic
COPY . /srv/http/
CMD ["-port","8080"]
	`},
	{Name: "hugo-static",
		Description: "Hugo static build with web server builtin",
		Details:     `Hugo static build, then all public files are copied to the image and served, except files with executable permission set.`,
		FileText: `
FROM klakegg/hugo:0.75.1-onbuild AS hugo
FROM pierrezemb/gostatic
COPY --from=hugo /target /srv/http/
CMD ["-port","8080"]
`},
	{Name: "staticplus",
		Description: "Web server builtin",
		Details:     `All files are copied to the image and served, except files with executable permission set.`,
		FileText: `
FROM flyio/gostatic-fly
COPY . /srv/http/
CMD ["-port","8080"]
`},
}
