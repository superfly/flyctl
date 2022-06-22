// Package flag implements flag-related functionality.
package flag

import (
	"context"

	"github.com/spf13/cobra"
)

const (
	// AccessTokenName denotes the name of the access token flag.
	AccessTokenName = "access-token"

	// VerboseName denotes the name of the verbose flag.
	VerboseName = "verbose"

	// JSONOutputName denotes the name of the json output flag.
	JSONOutputName = "json"

	// LocalOnlyName denotes the name of the local-only flag.
	LocalOnlyName = "local-only"

	// OrgName denotes the name of the org flag.
	OrgName = "org"

	// RegionName denotes the name of the region flag.
	RegionName = "region"

	// YesName denotes the name of the yes flag.
	YesName = "yes"

	// AppName denotes the name of the app flag.
	AppName = "app"

	// AppConfigFilePathName denotes the name of the app config file path flag.
	AppConfigFilePathName = "config"

	// ImageName denotes the name of the image flag.
	ImageName = "image"

	// NowName denotes the name of the now flag.
	NowName = "now"

	// NoDeploy denotes the name of the no deploy flag.
	NoDeployName = "no-deploy"

	// GenerateName denotes the name of the generate name flag.
	GenerateNameFlagName = "generate-name"

	// DetachName denotes the name of the detach flag.
	DetachName = "detach"
)

// Flag wraps the set of flags.
type Flag interface {
	addTo(*cobra.Command)
}

// Add adds flag to cmd, binding them on v should v not be nil.
func Add(cmd *cobra.Command, flags ...Flag) {
	for _, flag := range flags {
		flag.addTo(cmd)
	}
}

// Bool wraps the set of boolean flags.
type Bool struct {
	Name        string
	Shorthand   string
	Description string
	Default     bool
	Hidden      bool
}

func (b Bool) addTo(cmd *cobra.Command) {
	flags := cmd.Flags()

	if b.Shorthand != "" {
		_ = flags.BoolP(b.Name, b.Shorthand, b.Default, b.Description)
	} else {
		_ = flags.Bool(b.Name, b.Default, b.Description)
	}

	f := flags.Lookup(b.Name)
	f.Hidden = b.Hidden
}

// String wraps the set of string flags.
type String struct {
	Name        string
	Shorthand   string
	Description string
	Default     string
	ConfName    string
	EnvName     string
	Hidden      bool
}

func (s String) addTo(cmd *cobra.Command) {
	flags := cmd.Flags()

	if s.Shorthand != "" {
		_ = flags.StringP(s.Name, s.Shorthand, s.Default, s.Description)
	} else {
		_ = flags.String(s.Name, s.Default, s.Description)
	}

	f := flags.Lookup(s.Name)
	f.Hidden = s.Hidden
}

// Int wraps the set of int flags.
type Int struct {
	Name        string
	Shorthand   string
	Description string
	Default     int
	Hidden      bool
}

func (i Int) addTo(cmd *cobra.Command) {
	flags := cmd.Flags()

	if i.Shorthand != "" {
		_ = flags.IntP(i.Name, i.Shorthand, i.Default, i.Description)
	} else {
		_ = flags.Int(i.Name, i.Default, i.Description)
	}

	f := flags.Lookup(i.Name)
	f.Hidden = i.Hidden
}

// StringSlice wraps the set of string slice flags.
type StringSlice struct {
	Name        string
	Shorthand   string
	Description string
	Default     []string
	ConfName    string
	EnvName     string
	Hidden      bool
}

func (ss StringSlice) addTo(cmd *cobra.Command) {
	flags := cmd.Flags()

	if ss.Shorthand != "" {
		_ = flags.StringSliceP(ss.Name, ss.Shorthand, ss.Default, ss.Description)
	} else {
		_ = flags.StringSlice(ss.Name, ss.Default, ss.Description)
	}

	f := flags.Lookup(ss.Name)
	f.Hidden = ss.Hidden
}

// Org returns an org string flag.
func Org() String {
	return String{
		Name:        OrgName,
		Description: "The organization to operate on",
		Shorthand:   "o",
	}
}

// Region returns a region string flag.
func Region() String {
	return String{
		Name:        RegionName,
		Description: "The region to operate on",
		Shorthand:   "r",
	}
}

// Yes returns a yes bool flag.
func Yes() Bool {
	return Bool{
		Name:        YesName,
		Shorthand:   "y",
		Description: "Accept all confirmations",
	}
}

// App returns an app string flag.
func App() String {
	return String{
		Name:        AppName,
		Shorthand:   "a",
		Description: "Application name",
	}
}

// AppConfig returns an app config string flag.
func AppConfig() String {
	return String{
		Name:        AppConfigFilePathName,
		Shorthand:   "c",
		Description: "Path to application configuration file",
	}
}

// Image returns a Docker image config string flag.
func Image() String {
	return String{
		Name:        ImageName,
		Shorthand:   "i",
		Description: "The image tag or ID to deploy",
	}
}

// Now returns a boolean flag for deploying immediately
func Now() Bool {
	return Bool{
		Name:        NowName,
		Description: "Deploy now without confirmation",
		Default:     false,
	}
}

// GenerateName returns a boolean flag for generating an application name
func GenerateName() Bool {
	return Bool{
		Name:        GenerateNameFlagName,
		Description: "Always generate a name for the app",
		Default:     false,
	}
}

const remoteOnlyName = "remote-only"

// RemoteOnly returns a boolean flag for deploying remotely
func RemoteOnly(defaultValue bool) Bool {
	return Bool{
		Name:        remoteOnlyName,
		Description: "Perform builds on a remote builder instance instead of using the local docker daemon",
		Default:     defaultValue,
	}
}

func GetRemoteOnly(ctx context.Context) bool {
	return GetBool(ctx, remoteOnlyName)
}

const localOnlyName = "local-only"

// RemoteOnly returns a boolean flag for deploying remotely
func LocalOnly() Bool {
	return Bool{
		Name:        localOnlyName,
		Description: "Only perform builds locally using the local docker daemon",
	}
}

func GetLocalOnly(ctx context.Context) bool {
	return GetBool(ctx, localOnlyName)
}

const detachName = "detach"

// Detach returns a boolean flag for detaching during deployment
func Detach() Bool {
	return Bool{
		Name:        detachName,
		Description: "Return immediately instead of monitoring deployment progress",
	}
}

func GetDetach(ctx context.Context) bool {
	return GetBool(ctx, detachName)
}

const buildOnlyName = "build-only"

// BuildOnly returns a boolean flag for building without a deployment
func BuildOnly() Bool {
	return Bool{
		Name:        buildOnlyName,
		Description: "Build but do not deploy",
	}
}

func GetBuildOnly(ctx context.Context) bool {
	return GetBool(ctx, buildOnlyName)
}

const pushName = "push"

// Push returns a boolean flag to force pushing a build image to the registry
func Push() Bool {
	return Bool{
		Name:        pushName,
		Description: "Push image to registry after build is complete",
		Default:     false,
	}
}

const dockerfileName = "dockerfile"

func Dockerfile() String {
	return String{
		Name:        dockerfileName,
		Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
	}
}
