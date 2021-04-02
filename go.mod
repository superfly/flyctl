module github.com/superfly/flyctl

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.2.7
	github.com/BurntSushi/toml v0.3.1
	github.com/PuerkitoBio/rehttp v1.0.0
	github.com/aybabtme/iocontrol v0.0.0-20150809002002-ad15bcfc95a0 // indirect
	github.com/benbjohnson/clock v1.0.2 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/briandowns/spinner v1.12.0
	github.com/buildpacks/pack v0.17.0
	github.com/cli/safeexec v1.0.0
	github.com/containerd/console v1.0.1
	github.com/docker/cli v20.10.4+incompatible // indirect
	github.com/docker/docker v20.10.0-beta1.0.20201110211921-af34b94a78a1+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/dustin/go-humanize v1.0.0
	github.com/ejcx/sshcert v1.0.1
	github.com/getsentry/sentry-go v0.9.0
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/hashicorp/go-multierror v1.1.0
	github.com/inancgumus/screen v0.0.0-20190314163918-06e984b86ed3
	github.com/jpillora/backoff v1.0.0
	github.com/logrusorgru/aurora v2.0.3+incompatible
	github.com/machinebox/graphql v0.2.3-0.20181106130121-3a9253180225
	github.com/matryer/is v1.3.0 // indirect
	github.com/mattn/go-colorable v0.1.8
	github.com/mattn/go-isatty v0.0.12
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/moby/buildkit v0.8.1
	github.com/moby/term v0.0.0-20201216013528-df9cb8a40635
	github.com/morikuni/aec v1.0.0
	github.com/muesli/termenv v0.7.4
	github.com/novln/docker-parser v1.0.0
	github.com/olekukonko/tablewriter v0.0.5
	github.com/pelletier/go-toml v1.8.1
	github.com/pkg/errors v0.9.1
	github.com/segmentio/textio v1.2.0
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/cobra v1.1.3
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
	golang.org/x/net v0.0.0-20210331212208-0fccb6fa2b5c
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210402192133-700132347e07 // indirect
	golang.org/x/term v0.0.0-20210220032956-6a3ed077a48d
	golang.zx2c4.com/wireguard v0.0.20201118
	golang.zx2c4.com/wireguard/tun/netstack v0.0.0-20210402170708-10533c3e73cd
	gopkg.in/yaml.v2 v2.4.0
)

replace github.com/BurntSushi/toml => github.com/michaeldwan/toml v0.3.2-0.20191213213541-3c5ced72b6f3

// for buildkit https://github.com/moby/buildkit/blob/f5962fca5e7c589620ad2c41f5c6bcaece68f3dc/go.mod#L79
replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305
