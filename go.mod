module github.com/superfly/flyctl

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.2.7
	github.com/BurntSushi/toml v0.4.1
	github.com/azazeal/pause v1.0.6
	github.com/blang/semver v3.5.1+incompatible
	github.com/briandowns/spinner v1.12.0
	github.com/buildpacks/pack v0.21.0
	github.com/cli/safeexec v1.0.0
	github.com/containerd/console v1.0.2
	github.com/docker/docker v20.10.8+incompatible
	github.com/dustin/go-humanize v1.0.0
	github.com/ejcx/sshcert v1.0.1
	github.com/getsentry/sentry-go v0.12.0
	github.com/gofrs/flock v0.7.3
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-version v1.3.0
	github.com/heroku/heroku-go/v5 v5.4.0
	github.com/inancgumus/screen v0.0.0-20190314163918-06e984b86ed3
	github.com/jpillora/backoff v1.0.0
	github.com/logrusorgru/aurora v2.0.3+incompatible
	github.com/machinebox/graphql v0.2.3-0.20181106130121-3a9253180225 // indirect
	github.com/mattn/go-colorable v0.1.11
	github.com/mattn/go-isatty v0.0.14
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/miekg/dns v1.1.43
	github.com/moby/buildkit v0.9.0
	github.com/moby/term v0.0.0-20201216013528-df9cb8a40635
	github.com/morikuni/aec v1.0.0
	github.com/muesli/termenv v0.7.4
	github.com/nats-io/nats-server/v2 v2.5.0 // indirect
	github.com/nats-io/nats.go v1.12.1
	github.com/novln/docker-parser v1.0.0
	github.com/olekukonko/tablewriter v0.0.5
	github.com/pelletier/go-toml v1.9.4
	github.com/pkg/errors v0.9.1
	github.com/segmentio/textio v1.2.0
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/superfly/flyctl/api v0.0.0-00010101000000-000000000000
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	golang.org/x/net v0.0.0-20211008194852-3b03d305991f
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b
	golang.zx2c4.com/wireguard v0.0.20201118
	golang.zx2c4.com/wireguard/tun/netstack v0.0.0-20210402170708-10533c3e73cd
	google.golang.org/grpc v1.38.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

replace github.com/BurntSushi/toml => github.com/michaeldwan/toml v0.3.2-0.20191213213541-3c5ced72b6f3

replace github.com/superfly/flyctl/api => ./api
