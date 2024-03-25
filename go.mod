module github.com/superfly/flyctl

go 1.21.7

require (
	github.com/AlecAivazis/survey/v2 v2.3.7
	github.com/Khan/genqlient v0.6.0
	github.com/MakeNowJust/heredoc/v2 v2.0.1
	github.com/alecthomas/chroma v0.10.0
	github.com/avast/retry-go/v4 v4.5.1
	github.com/azazeal/pause v1.3.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/briandowns/spinner v1.23.0
	github.com/buildpacks/pack v0.33.2
	github.com/cavaliergopher/grab/v3 v3.0.1
	github.com/chzyer/readline v1.5.1
	github.com/cli/safeexec v1.0.1
	github.com/containerd/console v1.0.4
	github.com/docker/docker v25.0.3+incompatible
	github.com/dustin/go-humanize v1.0.1
	github.com/ejcx/sshcert v1.1.0
	github.com/getsentry/sentry-go v0.27.0
	github.com/gofrs/flock v0.8.1
	github.com/google/go-cmp v0.6.0
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/haileys/go-harlog v0.0.0-20230517070437-0f99204b5a57
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-version v1.6.0
	github.com/heroku/heroku-go/v5 v5.5.0
	github.com/inancgumus/screen v0.0.0-20190314163918-06e984b86ed3
	github.com/jinzhu/copier v0.4.0
	github.com/jpillora/backoff v1.0.0
	github.com/kr/text v0.2.0
	github.com/logrusorgru/aurora v2.0.3+incompatible
	github.com/mattn/go-colorable v0.1.13
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-zglob v0.0.4
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/miekg/dns v1.1.58
	github.com/moby/buildkit v0.12.5
	github.com/moby/patternmatcher v0.6.0
	github.com/morikuni/aec v1.0.0
	github.com/muesli/termenv v0.15.2
	github.com/nats-io/nats.go v1.34.0
	github.com/novln/docker-parser v1.0.0
	github.com/oklog/ulid/v2 v2.1.0
	github.com/olekukonko/tablewriter v0.0.5
	github.com/pelletier/go-toml/v2 v2.2.0
	github.com/pkg/errors v0.9.1
	github.com/pkg/sftp v1.13.6
	github.com/samber/lo v1.39.0
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/sourcegraph/conc v0.3.0
	github.com/spf13/cobra v1.8.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.18.2
	github.com/stretchr/testify v1.9.0
	github.com/superfly/fly-go v0.1.4
	github.com/superfly/graphql v0.2.4
	github.com/superfly/lfsc-go v0.1.1
	github.com/superfly/macaroon v0.2.12
	github.com/superfly/tokenizer v0.0.2
	github.com/vektah/gqlparser v1.3.1
	github.com/vektah/gqlparser/v2 v2.5.11
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.24.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.24.0
	golang.org/x/crypto v0.21.0
	golang.org/x/exp v0.0.0-20231219160207-73b9e39aefca
	golang.org/x/net v0.22.0
	golang.org/x/sync v0.6.0
	golang.org/x/term v0.18.0
	golang.zx2c4.com/wireguard v0.0.0-20231211153847-12269c276173
	google.golang.org/grpc v1.62.1
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	nhooyr.io/websocket v1.8.10
)

require (
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.10.4 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/prometheus/client_golang v1.17.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
)

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/Azure/azure-sdk-for-go v68.0.0+incompatible // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.29 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.23 // indirect
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.12 // indirect
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.6 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Microsoft/hcsshim v0.11.4 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20230828082145-3c4c8a2d2371 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/agnivade/levenshtein v1.1.1 // indirect
	github.com/alexflint/go-arg v1.4.2 // indirect
	github.com/alexflint/go-scalar v1.0.0 // indirect
	github.com/apex/log v1.9.0 // indirect
	github.com/aws/aws-sdk-go-v2 v1.24.1 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.26.6 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.16.16 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.14.11 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.2.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.5.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.7.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.24.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecrpublic v1.21.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.10.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.18.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.21.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.26.7 // indirect
	github.com/aws/smithy-go v1.19.0 // indirect
	github.com/awslabs/amazon-ecr-credential-helper/ecr-login v0.0.0-20231213181459-b0fcec718dc6 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/buildpacks/imgutil v0.0.0-20240118145509-e94a1b7de8a9 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/chrismellard/docker-credential-acr-env v0.0.0-20230304212654-82a0ddb27589 // indirect
	github.com/cloudflare/circl v1.3.7 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.14.3 // indirect
	github.com/containerd/typeurl/v2 v2.1.1 // indirect
	github.com/cyphar/filepath-securejoin v0.2.4 // indirect
	github.com/dimchansky/utfbom v1.1.1 // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/dlclark/regexp2 v1.4.0 // indirect
	github.com/docker/cli v25.0.3+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.0 // indirect
	github.com/elazarl/goproxy v0.0.0-20230808193330-2592e75ae04a // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/gdamore/encoding v1.0.0 // indirect
	github.com/gdamore/tcell/v2 v2.7.0 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.5.0 // indirect
	github.com/go-git/go-git/v5 v5.11.0 // indirect
	github.com/go-logr/logr v1.4.1
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/go-containerregistry v0.19.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.19.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/heroku/color v0.0.6 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/ioprogress v0.0.0-20180201004757-6a23b12fa88e // indirect
	github.com/moby/sys/sequential v0.5.0 // indirect
	github.com/moby/sys/signal v0.7.0 // indirect
	github.com/moby/sys/user v0.1.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/opencontainers/selinux v1.11.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.17 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/rivo/tview v0.0.0-20220307222120-9994674d60a8 // indirect
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/skeema/knownhosts v1.2.1 // indirect
	github.com/superfly/ltx v0.3.12 // indirect
	github.com/vbatts/tar-split v0.11.5 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.24.0 // indirect
	go.opentelemetry.io/otel/metric v1.24.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/tools v0.17.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240123012728-ef4313101c80 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240123012728-ef4313101c80 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gvisor.dev/gvisor v0.0.0-20230927004350-cbd86285d259 // indirect
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161
	github.com/Microsoft/go-winio v0.6.1
	github.com/PuerkitoBio/rehttp v1.4.0
	github.com/buildpacks/lifecycle v0.18.5 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/containerd/containerd v1.7.12 // indirect
	github.com/containerd/continuity v0.4.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/go-connections v0.5.0
	github.com/docker/go-units v0.5.0
	github.com/fatih/color v1.15.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/compress v1.17.2 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc5 // indirect
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/r3labs/diff v1.1.0
	github.com/rivo/uniseg v0.4.3 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tonistiigi/fsutil v0.0.0-20230629203738-36ef4d8c0dbb // indirect
	github.com/tonistiigi/units v0.0.0-20180711220420-6950e57a87ea // indirect
	github.com/tonistiigi/vt100 v0.0.0-20230623042737-f9a4f7ef6531 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.46.0 // indirect
	go.opentelemetry.io/otel v1.24.0
	go.opentelemetry.io/otel/sdk v1.24.0
	go.opentelemetry.io/otel/trace v1.24.0
	go.opentelemetry.io/proto/otlp v1.1.0 // indirect
	golang.org/x/mod v0.16.0
	golang.org/x/sys v0.18.0
	golang.org/x/text v0.14.0
	golang.org/x/time v0.5.0
	google.golang.org/genproto v0.0.0-20240123012728-ef4313101c80 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
)

replace github.com/loadsmart/calver-go => github.com/ndarilek/calver-go v0.0.0-20230710153822-893bbd83a936
