module github.com/superfly/flyctl

go 1.13

// thrift (a dep through go-getter) moved to github. This is needed until it's updated
replace git.apache.org/thrift.git => github.com/apache/thrift v0.0.0-20180902110319-2566ecd5d999

require (
	github.com/AlecAivazis/survey/v2 v2.0.7
	github.com/Azure/go-autorest v10.15.5+incompatible // indirect
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/hcsshim v0.8.9 // indirect
	github.com/PuerkitoBio/rehttp v1.0.0
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/apex/log v1.3.0 // indirect
	github.com/aybabtme/iocontrol v0.0.0-20150809002002-ad15bcfc95a0 // indirect
	github.com/benbjohnson/clock v1.0.2 // indirect
	github.com/blang/semver v3.5.1+incompatible
	github.com/briandowns/spinner v1.11.1
	github.com/buildpacks/imgutil v0.0.0-20200520132953-ba4f77a60397 // indirect
	github.com/buildpacks/lifecycle v0.7.5 // indirect
	github.com/buildpacks/pack v0.10.0
	github.com/containerd/console v1.0.0
	github.com/docker/docker v1.4.2-0.20200227233006-38f52c9fec82
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/color v1.9.0 // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/getsentry/sentry-go v0.6.1
	github.com/golang/protobuf v1.4.2 // indirect
	github.com/google/go-containerregistry v0.0.0-20200521151920-a873a21aff23 // indirect
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/hcl/v2 v2.5.1
	github.com/logrusorgru/aurora v0.0.0-20200102142835-e9ef32dff381
	github.com/machinebox/graphql v0.2.3-0.20181106130121-3a9253180225
	github.com/matryer/is v1.3.0 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/mitchellh/mapstructure v1.3.1 // indirect
	github.com/moby/buildkit v0.7.1
	github.com/morikuni/aec v1.0.0
	github.com/novln/docker-parser v1.0.0
	github.com/olekukonko/tablewriter v0.0.4
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/selinux v1.5.2 // indirect
	github.com/pelletier/go-toml v1.8.0
	github.com/pkg/errors v0.9.1
	github.com/segmentio/textio v1.2.0
	github.com/sirupsen/logrus v1.6.0 // indirect
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/spf13/cast v1.3.1 // indirect
	github.com/spf13/cobra v1.0.0
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/viper v1.7.0
	github.com/stretchr/testify v1.5.1
	github.com/zclconf/go-cty v1.4.1
	golang.org/x/crypto v0.0.0-20200510223506-06a226fb4e37 // indirect
	golang.org/x/net v0.0.0-20200520182314-0ba52f642ac2
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a // indirect
	golang.org/x/sys v0.0.0-20200523222454-059865788121 // indirect
	google.golang.org/grpc v1.29.1 // indirect
	google.golang.org/protobuf v1.24.0 // indirect
	gopkg.in/ini.v1 v1.56.0 // indirect
	gopkg.in/yaml.v2 v2.3.0
)

replace github.com/BurntSushi/toml => github.com/michaeldwan/toml v0.3.2-0.20191213213541-3c5ced72b6f3

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305

replace github.com/containerd/containerd => github.com/containerd/containerd v1.3.1-0.20200227195959-4d242818bf55

replace github.com/docker/docker => github.com/docker/docker v1.4.2-0.20200227233006-38f52c9fec82
