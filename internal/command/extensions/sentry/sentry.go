package sentry_ext

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

// Supported Sentry Platforms from https://github.com/getsentry/sentry/blob/master/src/sentry/utils/platform_categories.py
var Platforms = []string{
	"android",
	"apple-ios",
	"apple-macos",
	"capacitor",
	"cocoa-objc",
	"cocoa-swift",
	"cordova",
	"csharp",
	"dart",
	"dart-flutter",
	"dotnet",
	"dotnet-aspnet",
	"dotnet-aspnetcore",
	"dotnet-awslambda",
	"dotnet-gcpfunctions",
	"dotnet-maui",
	"dotnet-winforms",
	"dotnet-wpf",
	"dotnet-xamarin",
	"electron",
	"elixir",
	"flutter",
	"go",
	"go-http",
	"ionic",
	"java",
	"java-android",
	"java-appengine",
	"java-log4j",
	"java-log4j2",
	"java-logback",
	"java-logging",
	"java-spring",
	"java-spring-boot",
	"javascript",
	"javascript-angular",
	"javascript-angularjs",
	"javascript-backbone",
	"javascript-capacitor",
	"javascript-cordova",
	"javascript-electron",
	"javascript-ember",
	"javascript-gatsby",
	"javascript-nextjs",
	"javascript-react",
	"javascript-remix",
	"javascript-svelte",
	"javascript-vue",
	"kotlin",
	"minidump",
	"native",
	"native-breakpad",
	"native-crashpad",
	"native-minidump",
	"native-qt",
	"node",
	"node-awslambda",
	"node-azurefunctions",
	"node-connect",
	"node-express",
	"node-gcpfunctions",
	"node-koa",
	"perl",
	"php",
	"php-laravel",
	"php-monolog",
	"php-symfony2",
	"python",
	"python-awslambda",
	"python-azurefunctions",
	"python-bottle",
	"python-celery",
	"python-django",
	"python-fastapi",
	"python-flask",
	"python-gcpfunctions",
	"python-pylons",
	"python-pyramid",
	"python-rq",
	"python-sanic",
	"python-starlette",
	"python-tornado",
	"react-native",
	"ruby",
	"ruby-rack",
	"ruby-rails",
	"rust",
	"unity",
	"unreal",
}

var PlatformMap = map[string]string{
	"Bun":           "javascript",
	"Django":        "python-django",
	"Deno":          "node",
	".NET":          "dotnet",
	"Elixir":        "elixir",
	"Go":            "go",
	"NodeJS":        "node",
	"NodeJS/Prisma": "node",
	"Laravel":       "php-laravel",
	"NextJS":        "javascript-nextjs",
	"NuxtJS":        "javascript-vue",
	"Phoenix":       "elixir",
	"Python":        "python",
	"Rails":         "ruby-rails",
	"RedwoodJS":     "javascript-react",
	"Remix":         "javscript-remix",
	"Remix/Prisma":  "javscript-remix",
	"Ruby":          "ruby",
}

func New() (cmd *cobra.Command) {

	const (
		short = "Setup a Sentry project for this app"
		long  = short + "\n"
	)

	cmd = command.New("sentry", short, long, nil)
	cmd.AddCommand(create(), Dashboard())

	return cmd
}
