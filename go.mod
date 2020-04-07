module github.com/superfly/flyctl

go 1.13

// thrift (a dep through go-getter) moved to github. This is needed until it's updated
replace git.apache.org/thrift.git => github.com/apache/thrift v0.0.0-20180902110319-2566ecd5d999

require (
	github.com/AlecAivazis/survey/v2 v2.0.2
	github.com/BurntSushi/toml v0.3.1
	github.com/PuerkitoBio/rehttp v1.0.0
	github.com/aybabtme/iocontrol v0.0.0-20150809002002-ad15bcfc95a0 // indirect
	github.com/benbjohnson/clock v1.0.0 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/briandowns/spinner v1.6.1
	github.com/buildpacks/pack v0.9.0
	github.com/containerd/console v0.0.0-20191219165238-8375c3424e4d
	github.com/docker/docker v1.4.2-0.20200227233006-38f52c9fec82
	github.com/dustin/go-humanize v1.0.0
	github.com/getsentry/sentry-go v0.5.1
	github.com/hashicorp/go-multierror v0.0.0-20161216184304-ed905158d874
	github.com/hashicorp/hcl/v2 v2.3.0
	github.com/logrusorgru/aurora v0.0.0-20190428105938-cea283e61946
	github.com/machinebox/graphql v0.2.3-0.20181106130121-3a9253180225
	github.com/matryer/is v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/mattn/go-runewidth v0.0.4 // indirect
	github.com/moby/buildkit v0.7.0
	github.com/morikuni/aec v1.0.0
	github.com/novln/docker-parser v0.0.0-20190306203532-b3f122c6978e
	github.com/olekukonko/tablewriter v0.0.1
	github.com/pelletier/go-toml v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/segmentio/textio v1.2.0
	github.com/skratchdot/open-golang v0.0.0-20190402232053-79abb63cd66e
	github.com/spf13/cobra v0.0.5
	github.com/spf13/viper v1.4.0
	github.com/stretchr/testify v1.4.0
	github.com/zclconf/go-cty v1.3.1
	golang.org/x/net v0.0.0-20200226121028-0de0cce0169b
	gopkg.in/yaml.v2 v2.2.7
)

replace github.com/BurntSushi/toml => github.com/michaeldwan/toml v0.3.2-0.20191213213541-3c5ced72b6f3

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305

replace github.com/containerd/containerd => github.com/containerd/containerd v1.3.1-0.20200227195959-4d242818bf55

replace github.com/docker/docker => github.com/docker/docker v1.4.2-0.20200227233006-38f52c9fec82
