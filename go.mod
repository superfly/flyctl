module github.com/superfly/flyctl

go 1.13

// thrift (a dep through go-getter) moved to github. This is needed until it's updated
replace git.apache.org/thrift.git => github.com/apache/thrift v0.0.0-20180902110319-2566ecd5d999

require (
	github.com/AlecAivazis/survey/v2 v2.0.2
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/hcsshim v0.8.7 // indirect
	github.com/PuerkitoBio/rehttp v1.0.0
	github.com/aybabtme/iocontrol v0.0.0-20150809002002-ad15bcfc95a0 // indirect
	github.com/benbjohnson/clock v1.0.0 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/briandowns/spinner v1.6.1
	github.com/buildpacks/pack v0.8.1
	github.com/containerd/continuity v0.0.0-20200107194136-26c1120b8d41 // indirect
	github.com/docker/docker v1.4.2-0.20200103225628-a9507c6f7662
	github.com/dustin/go-humanize v1.0.0
	github.com/hashicorp/go-getter v1.3.0
	github.com/logrusorgru/aurora v0.0.0-20190428105938-cea283e61946
	github.com/machinebox/graphql v0.2.3-0.20181106130121-3a9253180225
	github.com/matryer/is v1.2.0 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/morikuni/aec v1.0.0
	github.com/novln/docker-parser v0.0.0-20190306203532-b3f122c6978e
	github.com/olekukonko/tablewriter v0.0.1
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/pelletier/go-toml v1.2.0
	github.com/pkg/errors v0.8.1
	github.com/skratchdot/open-golang v0.0.0-20190402232053-79abb63cd66e
	github.com/spf13/cobra v0.0.5
	github.com/spf13/viper v1.4.0
	github.com/stretchr/testify v1.4.0
	golang.org/x/net v0.0.0-20190724013045-ca1201d0de80
	gopkg.in/yaml.v2 v2.2.2
)

replace github.com/BurntSushi/toml => github.com/michaeldwan/toml v0.3.2-0.20191213213541-3c5ced72b6f3
