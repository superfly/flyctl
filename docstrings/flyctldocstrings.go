package docstrings

var docstrings = map[string]KeyStrings{
	"flyctl": KeyStrings{"flyctl", "The Fly CLI",
		`flyctl is a command line interface to the Fly.io platform.

It allows users to manage authentication, application creation, deployment, network configuration, logging and more
with just the one command.
`,
	},
	"version": KeyStrings{"version", "Show flyctl version information",
		`Shows version information for the flyctl command itself, including version number and build date.
`,
	},
	"history": KeyStrings{"history", "List app's change history",
		`List the history of changes in the application.
`,
	},
	"auth": KeyStrings{"auth", "Manage authentication",
		`Authenticate with Fly (and logout if you need to).

Start with the "login" subcommand.

`,
	},
	"auth.logout": KeyStrings{"logout", "Log out the currently logged in user",
		`Log out the currently logged in user. To continue interacting with Fly, the user must log in again.
`,
	},
	"auth.whoami": KeyStrings{"whoami", "Show the currently authenticated user",
		``,
	},
	"auth.token": KeyStrings{"token", "Show the current auth token",
		`Shows the authentication token that is currently in use.
`,
	},
	"auth.login": KeyStrings{"login", "Log in a user",
		`Log in a user to the Fly platform. Supports browser-based, email/password and one-time-password
authentication. Defaults to using browser-based authentication.
`,
	},
	"docs": KeyStrings{"docs", "View documentation",
		`View documentation on the Fly.io website. This command will open a browser to view the content.
`,
	},
	"certs": KeyStrings{"certs", "Manage certificates",
		`Manage certificates associated with a deployed application.
`,
	},
	"certs.list": KeyStrings{"list", "List certificates for an application",
		`List the certificates associated with a deployed applications.
`,
	},
	"certs.create": KeyStrings{"create <hostname>", "Create a certificate for an application",
		`Creates a certificate for an application. Takes a hostname as a parameter for the certificate.
`,
	},
	"certs.delete": KeyStrings{"delete <hostname>", "Delete certificate",
		`Deletes a certificate from an application. Takes hostname as a parameter to locate the certificate.
`,
	},
	"certs.show": KeyStrings{"show <hostname>", "Shows detailed certificate information",
		`Shows detailed certificate information for an application. Takes hostname as a parameter to locate the
certificate.
`,
	},
	"certs.check": KeyStrings{"check <hostname>", "Checks DNS configuration",
		`Checks the DNS configuration for the specified hostname.
`,
	},
	"status": KeyStrings{"status", "Show app status",
		`Show the application's current status including application details, tasks, most recent deployment details
and in which regions it is currently allocated.
`,
	},
	"builds": KeyStrings{"builds", "Work with Fly Builds",
		`Fly Builds are templates to make developing Fly applications easier. The builds commands
`,
	},
	"builds.list": KeyStrings{"list", "List builds",
		``,
	},
	"builds.logs": KeyStrings{"logs", "Show logs associated with builds",
		``,
	},
	"secrets": KeyStrings{"secrets", "Manage app secrets",
		`Manage application secrets with the set and unset commands.

Secrets are provided to apps at runtime as ENV variables. Names are
case sensitive and stored as-is, so ensure names are appropriate for
the application and vm environment.
`,
	},
	"secrets.list": KeyStrings{"list", "Lists the secrets available to the App",
		`List the secrets available to the application. It shows each secret's name, a digest of the its value and
the time the secret was last set. The actual value of the secret is only available to the application.
`,
	},
	"secrets.set": KeyStrings{"set [flags] NAME=VALUE NAME=VALUE ...", "Set one or more encrypted secrets for an app",
		`Set one or more encrypted secrets for an application.

Secrets are provided to apps at runtime as ENV variables. Names are
case sensitive and stored as-is, so ensure names are appropriate for
the application and vm environment.

Any value that equals "-" will be assigned from STDIN instead of args.
`,
	},
	"secrets.unset": KeyStrings{"unset [flags] NAME NAME ...", "Remove encrypted secrets from app",
		`Remove encrypted secrets from the application. Unsetting a secret removes its availability to
the application.
`,
	},
	"info": KeyStrings{"info", "Show detailed app information",
		`Shows information about the application on the Fly platform

Information includes the application's
* name, owner, version, status and hostname
* services
* IP addresses
`,
	},
	"apps": KeyStrings{"apps", "Manage apps",
		`The Apps commands focus on managing your Fly applications.
Start with the "create" command to register your application.
The "list" command will list all currently registered applications.
`,
	},
	"apps.init-config": KeyStrings{"init-config [APP] [PATH]", "Initialize a fly.toml file from an existing app",
		`Using an existing app, create a fly.toml file
`,
	},
	"apps.list": KeyStrings{"list", "List applications",
		`List the applications currently registered for this user. The list will include all organizations the user
is a member of. Each application will be shown with tis name, owner and when it was last deployed.
`,
	},
	"apps.create": KeyStrings{"create", "Create a new application",
		`The create command will both register a new application with the Fly platform and create the fly.toml
file which controls how the application will be deployed.
`,
	},
	"apps.destroy": KeyStrings{"destroy", "Permanently destroys an app",
		`The ~~~apps destroy~~~ command will remove an application from the fly platform.
`,
	},
	"logs": KeyStrings{"logs", "View app logs",
		`View application logs as generated by the application running on the Fly platform.

Logs can be filtered to a specific instance using the instance/i flag or to all instances running in a specific
region using the region/r flag.
`,
	},
	"config": KeyStrings{"config", "Manage application configuration",
		``,
	},
	"config.display": KeyStrings{"display", "Display an app's configuration",
		`Display an application's configuration
`,
	},
	"config.save": KeyStrings{"save", "Update and save an app's config file",
		`Update and save an application's config file
`,
	},
	"config.validate": KeyStrings{"validate", "Validate an app's config file",
		`Validate an application's config file
`,
	},
	"ips": KeyStrings{"ips", "Manage IP addresses for apps",
		`Manage IP addresses for applications.
`,
	},
	"ips.release": KeyStrings{"release [ADDRESS]", "Release an IP address",
		`Releases an IP address from the application.
`,
	},
	"ips.list": KeyStrings{"list", "List allocated IP addresses",
		`Lists the IP addresses allocated to the application.
`,
	},
	"ips.allocate-v4": KeyStrings{"allocate-v4", "Allocate an IPv4 address",
		`Allocates an IPv4 address to the application.
`,
	},
	"ips.allocate-v6": KeyStrings{"allocate-v6", "Allocate an IPv6 address",
		`Allocates an IPv6 address to the application.
`,
	},
	"releases": KeyStrings{"releases", "List app releases",
		`List all the releases of the application onto the Fly platform, including type, when, success/fail and
which user triggered the release.
`,
	},
	"releases.latest": KeyStrings{"latest", "Show details for latest release",
		`Show details of the most recent release.
`,
	},
	"releases.show": KeyStrings{"show [VERSION]", "Show details for a specific release",
		`Show the release details for a specific release. A version parameter identifies which release is required.
Versions can be seen in the output of ~~~flyctl releases~~~.
`,
	},
	"deploy": KeyStrings{"deploy", "Deploy an application to the Fly platform",
		`Deploy an application to the Fly platform. The application can be a local image, remote image or defined in
a Dockerfile.

Use the image/i flag to specify a local or remote image to deploy.

Use the detach flag to return immediately from starting the deployment rather than monitoring the deployment progress.
`,
	},
}
