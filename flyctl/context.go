package flyctl

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/terminal"
)

type AppContext struct {
	Path string
	cfg  *viper.Viper
}

func (c *AppContext) Init(cmd *cobra.Command) error {
	v := viper.New()

	// fmt.Print(v.AllSettings())
	v.SetEnvPrefix("FLY")
	v.AutomaticEnv()
	// v.BindEnv("APP")

	v.BindPFlag("app", cmd.Flags().Lookup("app"))

	v.SetConfigName("fly")
	v.AddConfigPath(".")

	// If a config file is found, read it in.
	if err := v.ReadInConfig(); err != nil {
		return err
		//
	} else {
		terminal.Debug("Using config file:", viper.ConfigFileUsed())
	}

	// fmt.Print(c.cfg.AllSettings())
	v.Debug()
	c.cfg = v

	if c.AppName() == "" {
		return errors.New("no app specified")
	}

	return nil
}

func (c *AppContext) AppName() string {
	return c.cfg.GetString("app")
}

func (c *AppContext) HasBuildConfig() bool {
	return c.cfg.IsSet("build")
}

func CurrentAppName() string {
	appName := os.Getenv("FLY_APP")
	if appName != "" {
		return appName
	}

	if manifest, err := LoadManifest(DefaultManifestPath()); err == nil {
		return manifest.AppName
	}

	return ""
}
