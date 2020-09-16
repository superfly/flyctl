package docstrings

// Get - Get a document string
func Get(key string) KeyStrings {
	switch key {
	case "flyctl":
		return KeyStrings{"flyctl", "The Fly CLI",
			`flyctl is a command line interface to the Fly.io platform.

It allows users to manage authentication, application initialization, 
deployment, network configuration, logging and more with just the 
one command.

Initialize an App with the init command
Deploy an App with the deploy command
View a Deployed web application with the open command
Check the status of an application with the status command

To read more, use the docs command to view Fly's help on the web.`,
		}
	case "destroy":
		return KeyStrings{"destroy [APPNAME]", "Permanently destroys an App",
			`The DESTROY command will remove an application 
from the Fly platform.`,
		}
	case "curl":
		return KeyStrings{"curl <url>", "Run a performance test against a url",
			`Run a performance test againt a url.`,
		}
	case "deploy":
		return KeyStrings{"deploy [<workingdirectory>]", "Deploy an App to the Fly platform",
			`Deploy an application to the Fly platform. The application can be a local 
image, remote image, defined in a Dockerfile or use a CNB Buildpack.

Use the --config/-c flag to select a specific toml configuration file.

Use the --image/-i flag to specify a local or remote image to deploy.

Use the --detach flag to return immediately from starting the deployment rather
than monitoring the deployment progress.

Use flyctl monitor to restart monitoring deployment progress`,
		}
	case "config":
		return KeyStrings{"config", "Manage an Apps configuration",
			`The CONFIG commands allow you to work with an application's configuration.`,
		}
	case "config.display":
		return KeyStrings{"display", "Display an App's configuration",
			`Display an application's configuration. The configuration is presented 
in JSON format. The configuration data is retrieved from the Fly service.`,
		}
	case "config.save":
		return KeyStrings{"save", "Save an App's config file",
			`Save an application's configuration locally. The configuration data is 
retrieved from the Fly service and saved in TOML format.`,
		}
	case "config.validate":
		return KeyStrings{"validate", "Validate an App's config file",
			`Validates an application's config file against the Fly platform to 
ensure it is correct and meaningful to the platform.`,
		}
	case "orgs":
		return KeyStrings{"orgs", "Commands for managing Fly organizations",
			`Commands for managing Fly organizations. list, create, show and 
destroy organizations. 
Organization admins can also invite or remove users from Organizations.`,
		}
	case "orgs.list":
		return KeyStrings{"list", "Lists organizations for current user",
			`Lists organizations available to current user.`,
		}
	case "orgs.invite":
		return KeyStrings{"invite <org> <email>", "Invite user (by email) to organization",
			`Invite a user, by email, to join organization. The invitation will be
sent, and the user will be pending until they respond. See also orgs revoke.`,
		}
	case "orgs.revoke":
		return KeyStrings{"revoke <org> <email>", "Revoke a pending invitation to an organization",
			`Revokes an invitation to join an organization that has been sent to a 
user by email.`,
		}
	case "orgs.remove":
		return KeyStrings{"remove <org> <email>", "Remove a user from an organization",
			`Remove a user from an organization. User must have accepted a previous
invitation to join (if not, see orgs revoke).`,
		}
	case "orgs.show":
		return KeyStrings{"show <org>", "Show information about an organization",
			`Shows information about an organization.
Includes name, slug and type. Summarizes user permissions, DNS zones and
associated member. Details full list of members and roles.`,
		}
	case "orgs.create":
		return KeyStrings{"create <org>", "Create an organization",
			`Create a new organization. Other users can be invited to join the 
organization later.`,
		}
	case "orgs.delete":
		return KeyStrings{"delete <org>", "Delete an organization",
			`Delete an existing organization.`,
		}
	case "docs":
		return KeyStrings{"docs", "View Fly documentation",
			`View Fly documentation on the Fly.io website. This command will open a 
browser to view the content.`,
		}
	case "platform":
		return KeyStrings{"platform", "Fly platform information",
			`The PLATFORM commands are for users looking for information 
about the Fly platform.`,
		}
	case "platform.regions":
		return KeyStrings{"regions", "List regions",
			`View a list of regions where Fly has edges and/or datacenters`,
		}
	case "platform.vmsizes":
		return KeyStrings{"vm-sizes", "List VM Sizes",
			`View a list of VM sizes which can be used with the FLYCTL SCALE VM command`,
		}
	case "platform.status":
		return KeyStrings{"status", "Show current platform status",
			`Show current Fly platform status in a browser`,
		}
	case "ips":
		return KeyStrings{"ips", "Manage IP addresses for Apps",
			`The IPS commands manage IP addresses for applications. An application 
can have a number of IP addresses associated with it and this family of commands 
allows you to list, allocate and release those addresses. It supports both IPv4 
and IPv6 addresses.`,
		}
	case "ips.list":
		return KeyStrings{"list", "List allocated IP addresses",
			`Lists the IP addresses allocated to the application.`,
		}
	case "ips.allocate-v4":
		return KeyStrings{"allocate-v4", "Allocate an IPv4 address",
			`Allocates an IPv4 address to the application.`,
		}
	case "ips.allocate-v6":
		return KeyStrings{"allocate-v6", "Allocate an IPv6 address",
			`Allocates an IPv6 address to the application.`,
		}
	case "ips.release":
		return KeyStrings{"release [ADDRESS]", "Release an IP address",
			`Releases an IP address from the application.`,
		}
	case "version":
		return KeyStrings{"version", "Show version information for the flyctl command",
			`Shows version information for the flyctl command itself, 
including version number and build date.`,
		}
	case "version.update":
		return KeyStrings{"update", "Checks for available updates and automatically updates",
			`Checks for update and if one is available, runs the appropriate
command to update the application.`,
		}
	case "suspend":
		return KeyStrings{"suspend [APPNAME]", "Suspend an application",
			`The SUSPEND command will suspend an application. 
All allocations will be deallocated leaving the application running nowhere.
It will continue to consume networking resources (IP address). See RESUME
for details on restarting it.`,
		}
	case "logs":
		return KeyStrings{"logs", "View App logs",
			`View application logs as generated by the application running on 
the Fly platform.

Logs can be filtered to a specific instance using the --instance/-i flag or 
to all instances running in a specific region using the --region/-r flag.`,
		}
	case "scale":
		return KeyStrings{"scale", "Scale App resources",
			`Scale application resources`,
		}
	case "scale.set":
		return KeyStrings{"set", "Set current models scaling parameters",
			`Allows the setting of the current models scaling parameters:

min=int - minimum number of instances to be allocated from region pool. 
max=int - maximum number of instances to be allocated from region pool.`,
		}
	case "scale.vm":
		return KeyStrings{"vm [flags] [SIZENAME]", "Change an App's VM to a named size (eg micro-1x, micro-2x, cpu1mem1...",
			`Change an application's VM size to one of the named VM sizes. Shows the application's current VM size if no arguments are given. 

Size names include micro-1x, micro-2x, cpu1mem1, cpu2mem2, cpu4mem4 and cpu8mem8.

For a full list of supported sizes, with names and pricing details, use the command FLYCTL PLATFORM VM-SIZES.`,
		}
	case "scale.balanced":
		return KeyStrings{"balanced", "Configure a traffic balanced App with params (min=int max=int)",
			`Configure the App to balance regions based on traffic with given parameters:

min=int - minimum number of instances to be allocated from region pool. 
max=int - maximum number of instances to be allocated from region pool.`,
		}
	case "scale.standard":
		return KeyStrings{"standard", "Configure a standard balanced App with params (min=int max=int)",
			`Configure the App without traffic balancing with the given parameters:

min=int - minimum number of instances to be allocated from region pool. 
max=int - maximum number of instances to be allocated from region pool.`,
		}
	case "scale.show":
		return KeyStrings{"show", "Show current scaling configuration",
			`Show current scaling configuration`,
		}
	case "open":
		return KeyStrings{"open [PATH]", "Open browser to current deployed application",
			`Open browser to current deployed application. If an optional path is specified, this is appended to the
URL for deployed application.`,
		}
	case "builtins":
		return KeyStrings{"builtins", "View and manage Flyctl deployment builtins",
			`View and manage Flyctl deployment builtins.`,
		}
	case "builtins.list":
		return KeyStrings{"list", "List available Flyctl deployment builtins",
			`List available Flyctl deployment builtins and their
descriptions.`,
		}
	case "builtins.show":
		return KeyStrings{"show", "Show details of a builtins",
			`Show details of a Flyctl deployment builtins, including
the builtin "Dockerfile" and other information.`,
		}
	case "certs":
		return KeyStrings{"certs", "Manage certificates",
			`Manages the certificates associated with a deployed application. 
Certificates are created by associating a hostname/domain with the application. 
When Fly is then able to validate that hostname/domain, the platform gets 
certificates issued for the hostname/domain by Let's Encrypt.`,
		}
	case "certs.list":
		return KeyStrings{"list", "List certificates for an App.",
			`List the certificates associated with a deployed application.`,
		}
	case "certs.add":
		return KeyStrings{"add <hostname>", "Add a certificate for an App.",
			`Add a certificate for an application. Takes a hostname 
as a parameter for the certificate.`,
		}
	case "certs.remove":
		return KeyStrings{"remove <hostname>", "Removes a certificate from an App",
			`Removes a certificate from an application. Takes hostname 
as a parameter to locate the certificate.`,
		}
	case "certs.show":
		return KeyStrings{"show <hostname>", "Shows certificate information",
			`Shows certificate information for an application. 
Takes hostname as a parameter to locate the certificate.`,
		}
	case "certs.check":
		return KeyStrings{"check <hostname>", "Checks DNS configuration",
			`Checks the DNS configuration for the specified hostname. 
Displays results in the same format as the SHOW command.`,
		}
	case "dns":
		return KeyStrings{"dns", "Manage DNS",
			`Manage DNS`,
		}
	case "dns.zones":
		return KeyStrings{"zones", "Manage DNS zones",
			`Manage DNS zones`,
		}
	case "dns.zones.delete":
		return KeyStrings{"delete [<org> <domain>]", "Delete a DNS zone",
			`Delete a DNS zone`,
		}
	case "dns.zones.list":
		return KeyStrings{"list [<org>]", "List DNS zones",
			`List DNS zones for an organization`,
		}
	case "dns.zones.create":
		return KeyStrings{"create [<org> <domain>]", "Create a DNS zone",
			`Create a DNS zone for an organization`,
		}
	case "dns.records":
		return KeyStrings{"records", "Manage DNS records",
			`Manage DNS records within a zone`,
		}
	case "dns.records.export":
		return KeyStrings{"export [<org> <domain>]", "Export DNS records",
			`Export DNS records`,
		}
	case "dns.records.import":
		return KeyStrings{"import <org> <domain> <path>", "Import DNS records",
			`Import DNS records`,
		}
	case "dns.records.list":
		return KeyStrings{"list [<org> <domain>]", "List DNS records",
			`List DNS records within a zone`,
		}
	case "info":
		return KeyStrings{"info", "Show detailed App information",
			`Shows information about the application on the Fly platform

Information includes the application's
* name, owner, version, status and hostname
* services
* IP addresses`,
		}
	case "apps":
		return KeyStrings{"apps", "Manage Apps",
			`The APPS commands focus on managing your Fly applications.
Start with the CREATE command to register your application.
The LIST command will list all currently registered applications.`,
		}
	case "apps.resume":
		return KeyStrings{"resume [APPNAME]", "Resume an application",
			`The APPS RESUME command will restart a previously suspended application. 
The application will resume with its original region pool and a min count of one
meaning there will be one running instance once restarted. Use SCALE SET MIN= to raise
the number of configured instances.`,
		}
	case "apps.destroy":
		return KeyStrings{"destroy [APPNAME]", "Permanently destroys an App",
			`The APPS DESTROY command will remove an application 
from the Fly platform.`,
		}
	case "apps.suspend":
		return KeyStrings{"suspend [APPNAME]", "Suspend an application",
			`The APPS SUSPEND command will suspend an application. 
All allocations will be deallocated leaving the application running nowhere.
It will continue to consume networking resources (IP address). See APPS RESUME
for details on restarting it.`,
		}
	case "apps.list":
		return KeyStrings{"list", "List applications",
			`The APPS LIST command will show the applications currently
registered and available to this user. The list will include applications 
from all the organizations the user is a member of. Each application will 
be shown with its name, owner and when it was last deployed.`,
		}
	case "apps.create":
		return KeyStrings{"create [APPNAME]", "Create a new application",
			`The APPS CREATE command will both register a new application 
with the Fly platform and create the fly.toml file which controls how 
the application will be deployed. The --builder flag allows a cloud native 
buildpack to be specified which will be used instead of a Dockerfile to 
create the application image when it is deployed.`,
		}
	case "apps.move":
		return KeyStrings{"move [APPNAME]", "Move an App to another organization",
			`The APPS MOVE command will move an application to another 
organization the current user belongs to.`,
		}
	case "apps.restart":
		return KeyStrings{"restart [APPNAME]", "Restart an application",
			`The APPS RESTART command will restart all running vms.`,
		}
	case "releases":
		return KeyStrings{"releases", "List App releases",
			`List all the releases of the application onto the Fly platform, 
including type, when, success/fail and which user triggered the release.`,
		}
	case "status":
		return KeyStrings{"status", "Show App status",
			`Show the application's current status including application 
details, tasks, most recent deployment details and in which regions it is 
currently allocated.`,
		}
	case "status.alloc":
		return KeyStrings{"alloc [alloc-id]", "Show allocation status",
			`Show the allocation's current status including logs, checks, 
and events.`,
		}
	case "builds":
		return KeyStrings{"builds", "Work with Fly Builds",
			`Fly Builds are templates to make developing Fly applications easier.`,
		}
	case "builds.logs":
		return KeyStrings{"logs", "Show logs associated with builds",
			``,
		}
	case "builds.list":
		return KeyStrings{"list", "List builds",
			``,
		}
	case "secrets":
		return KeyStrings{"secrets", "Manage App secrets",
			`Manage application secrets with the set and unset commands.

Secrets are provided to applications at runtime as ENV variables. Names are
case sensitive and stored as-is, so ensure names are appropriate for
the application and vm environment.`,
		}
	case "secrets.set":
		return KeyStrings{"set [flags] NAME=VALUE NAME=VALUE ...", "Set one or more encrypted secrets for an App",
			`Set one or more encrypted secrets for an application.

Secrets are provided to application at runtime as ENV variables. Names are
case sensitive and stored as-is, so ensure names are appropriate for
the application and vm environment.

Any value that equals "-" will be assigned from STDIN instead of args.`,
		}
	case "secrets.unset":
		return KeyStrings{"unset [flags] NAME NAME ...", "Remove encrypted secrets from an App",
			`Remove encrypted secrets from the application. Unsetting a 
secret removes its availability to the application.`,
		}
	case "secrets.list":
		return KeyStrings{"list", "Lists the secrets available to the App",
			`List the secrets available to the application. It shows each 
secret's name, a digest of the its value and the time the secret was last set. 
The actual value of the secret is only available to the application.`,
		}
	case "resume":
		return KeyStrings{"resume [APPNAME]", "Resume an application",
			`The RESUME command will restart a previously suspended application. 
The application will resume with its original region pool and a min count of one
meaning there will be one running instance once restarted. Use SCALE SET MIN= to raise
the number of configured instances.`,
		}
	case "monitor":
		return KeyStrings{"monitor", "Monitor Deployments",
			`Monitor Application Deployments and other activities. Use --verbose/-v
to get details of every instance allocation. Control-C to stop output.`,
		}
	case "list":
		return KeyStrings{"list", "Lists your Fly resources",
			`The list command is for listing your resources on has two subcommands, apps and orgs.

The apps command lists your applications. There are filtering options available.

The orgs command lists all the organizations you are a member of.`,
		}
	case "list.apps":
		return KeyStrings{"apps [text] [-o org] [-s status]", "Lists all your apps",
			`The list apps command lists all your applications. As this may be a 
long list, there are options to filter the results.

Specifying a text string as a parameter will only return applications where the
application name contains the text.

The --orgs/-o flag allows you to specify the name of an organization that the
application must be owned by. (see list orgs for organization names).

The --status/-s flag allows you to specify status applications should be at to be
returned in the results. e.g. -s running would only return running applications.`,
		}
	case "list.orgs":
		return KeyStrings{"orgs", "List all your organizations",
			`Lists all organizations which your are a member of. It will show the
short name of the organization and the long name.`,
		}
	case "init":
		return KeyStrings{"init [APPNAME]", "Initialize a new application",
			`The INIT command will both register a new application 
with the Fly platform and create the fly.toml file which controls how 
the application will be deployed. The --builder flag allows a cloud native 
buildpack to be specified which will be used instead of a Dockerfile to 
create the application image when it is deployed.`,
		}
	case "move":
		return KeyStrings{"move [APPNAME]", "Move an App to another organization",
			`The MOVE command will move an application to another 
organization the current user belongs to.`,
		}
	case "restart":
		return KeyStrings{"restart [APPNAME]", "Restart an application",
			`The RESTART command will restart all running vms.`,
		}
	case "dashboard":
		return KeyStrings{"dashboard", "Open web browser on Fly Web UI for this app",
			`Open web browser on Fly Web UI for this application`,
		}
	case "dashboard.metrics":
		return KeyStrings{"metrics", "Open web browser on Fly Web UI for this app's metrics",
			`Open web browser on Fly Web UI for this application's metrics`,
		}
	case "auth":
		return KeyStrings{"auth", "Manage authentication",
			`Authenticate with Fly (and logout if you need to).
If you do not have an account, start with the AUTH SIGNUP command.
If you do have and account, begin with the AUTH LOGIN subcommand.`,
		}
	case "auth.token":
		return KeyStrings{"token", "Show the current auth token",
			`Shows the authentication token that is currently in use. 
This can be used as an authentication token with API services, 
independent of flyctl.`,
		}
	case "auth.signup":
		return KeyStrings{"signup", "Create a new fly account",
			`Creates a new fly account. The command opens the browser 
and sends the user to a form to provide appropriate credentials.`,
		}
	case "auth.whoami":
		return KeyStrings{"whoami", "Show the currently authenticated user",
			`Displays the users email address/service identity currently 
authenticated and in use.`,
		}
	case "auth.logout":
		return KeyStrings{"logout", "Logs out the currently logged in user",
			`Log the currently logged-in user out of the Fly platform. 
To continue interacting with Fly, the user will need to log in again.`,
		}
	case "auth.docker":
		return KeyStrings{"docker", "Authenticate docker",
			`Adds registry.fly.io to the docker daemon's authenticated 
registries. This allows you to push images directly to fly from 
the docker cli.`,
		}
	case "auth.login":
		return KeyStrings{"login", "Log in a user",
			`Logs a user into the Fly platform. Supports browser-based, 
email/password and one-time-password authentication. Defaults to using 
browser-based authentication.`,
		}
	case "history":
		return KeyStrings{"history", "List an App's change history",
			`List the history of changes in the application. Includes autoscaling 
events and their results.`,
		}
	case "regions":
		return KeyStrings{"regions", "Manage regions",
			`Configure the region placement rules for an application.`,
		}
	case "regions.set":
		return KeyStrings{"set REGION ...", "Sets the region pool with provided regions",
			`Sets the region pool with provided regions`,
		}
	case "regions.backup":
		return KeyStrings{"backup REGION ...", "Sets the backup region pool with provided regions",
			`Sets the backup region pool with provided regions`,
		}
	case "regions.list":
		return KeyStrings{"list", "Shows the list of regions the app is allowed to run in",
			`Shows the list of regions the app is allowed to run in.`,
		}
	case "regions.add":
		return KeyStrings{"add REGION ...", "Allow the app to run in the provided regions",
			`Allow the app to run in one or more regions`,
		}
	case "regions.remove":
		return KeyStrings{"remove REGION ...", "Prevent the app from running in the provided regions",
			`Prevent the app from running in the provided regions`,
		}
	}
	panic("unknown command key " + key)
}
