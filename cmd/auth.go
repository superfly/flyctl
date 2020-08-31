package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
)

func newAuthCommand() *Command {

	authStrings := docstrings.Get("auth")

	cmd := BuildCommandKS(nil, nil, authStrings, os.Stdout)

	authWhoamiStrings := docstrings.Get("auth.whoami")
	BuildCommand(cmd, runWhoami, authWhoamiStrings.Usage, authWhoamiStrings.Short, authWhoamiStrings.Long, os.Stdout, requireSession)

	authTokenStrings := docstrings.Get("auth.token")
	BuildCommand(cmd, runAuthToken, authTokenStrings.Usage, authTokenStrings.Short, authTokenStrings.Long, os.Stdout, requireSession)

	authLoginStrings := docstrings.Get("auth.login")
	login := BuildCommand(cmd, runLogin, authLoginStrings.Usage, authLoginStrings.Short, authLoginStrings.Long, os.Stdout)

	authDockerStrings := docstrings.Get("auth.docker")
	BuildCommand(cmd, runAuthDocker, authDockerStrings.Usage, authDockerStrings.Short, authDockerStrings.Long, os.Stdout)

	// TODO: Move flag descriptions into the docStrings
	login.AddBoolFlag(BoolFlagOpts{
		Name:        "interactive",
		Shorthand:   "i",
		Description: "Log in with an email and password interactively",
	})
	login.AddStringFlag(StringFlagOpts{
		Name:        "email",
		Description: "Login email",
	})
	login.AddStringFlag(StringFlagOpts{
		Name:        "password",
		Description: "Login password",
	})
	login.AddStringFlag(StringFlagOpts{
		Name:        "otp",
		Description: "One time password",
	})

	authLogoutStrings := docstrings.Get("auth.logout")
	BuildCommand(cmd, runLogout, authLogoutStrings.Usage, authLogoutStrings.Short, authLogoutStrings.Long, os.Stdout, requireSession)

	authSignupStrings := docstrings.Get("auth.signup")
	BuildCommand(cmd, runSignup, authSignupStrings.Usage, authSignupStrings.Short, authSignupStrings.Long, os.Stdout)

	return cmd
}

func runWhoami(ctx *cmdctx.CmdContext) error {
	user, err := ctx.Client.API().GetCurrentUser()
	if err != nil {
		return err
	}
	fmt.Printf("Current user: %s\n", user.Email)
	return nil
}

func runLogin(ctx *cmdctx.CmdContext) error {
	if ctx.Config.GetBool("interactive") {
		return runInteractiveLogin(ctx)
	}
	if val, _ := ctx.Config.GetString("email"); val != "" {
		return runInteractiveLogin(ctx)
	}
	if val, _ := ctx.Config.GetString("password"); val != "" {
		return runInteractiveLogin(ctx)
	}
	if val, _ := ctx.Config.GetString("otp"); val != "" {
		return runInteractiveLogin(ctx)
	}

	return runWebLogin(ctx, false)
}

func runSignup(ctx *cmdctx.CmdContext) error {
	return runWebLogin(ctx, true)
}

func runWebLogin(ctx *cmdctx.CmdContext, signup bool) error {
	name, _ := os.Hostname()

	cliAuth, err := api.StartCLISessionWebAuth(name, signup)
	if err != nil {
		return err
	}

	//fmt.Fprintln(ctx.Out, "Opening browser to url", aurora.Bold(cliAuth.AuthURL))

	if err := open.Run(cliAuth.AuthURL); err != nil {
		terminal.Error("Error opening browser. Copy the url " + cliAuth.AuthURL + " into a browser and continue")
	}

	select {
	case <-time.After(15 * time.Minute):
		return errors.New("Login expired, please try again")
	case cliAuth = <-waitForCLISession(cliAuth.ID):
	}

	if cliAuth.AccessToken == "" {
		return errors.New("Unable to log in, please try again")
	}

	viper.Set(flyctl.ConfigAPIToken, cliAuth.AccessToken)
	if err := flyctl.SaveConfig(); err != nil {
		return err
	}

	if !ctx.Client.InitApi() {
		return client.ErrNoAuthToken
	}

	user, err := ctx.Client.API().GetCurrentUser()
	if err != nil {
		return err
	}

	fmt.Println("Successfully logged in as", aurora.Bold(user.Email))

	return nil
}

func waitForCLISession(id string) <-chan api.CLISessionAuth {
	done := make(chan api.CLISessionAuth, 0)

	go func() {
		s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		s.Writer = os.Stderr
		s.Prefix = "Waiting for session..."
		s.FinalMSG = "Waiting for session...Done\n"
		s.Start()
		defer s.Stop()

		for {
			time.Sleep(1 * time.Second)
			cliAuth, _ := api.GetAccessTokenForCLISession(id)

			if cliAuth.AccessToken != "" {
				done <- cliAuth
				break
			}
		}
	}()

	return done
}

func runInteractiveLogin(ctx *cmdctx.CmdContext) error {
	email, _ := ctx.Config.GetString("email")
	if email == "" {
		prompt := &survey.Input{
			Message: "Email:",
		}
		if err := survey.AskOne(prompt, &email, survey.WithValidator(survey.Required)); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	password, _ := ctx.Config.GetString("password")
	if password == "" {
		prompt := &survey.Password{
			Message: "Password:",
		}
		if err := survey.AskOne(prompt, &password, survey.WithValidator(survey.Required)); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	otp, _ := ctx.Config.GetString("otp")
	if otp == "" {
		prompt := &survey.Password{
			Message: "One Time Password (if any):",
		}
		if err := survey.AskOne(prompt, &otp); err != nil {
			if isInterrupt(err) {
				return nil
			}
		}
	}

	accessToken, err := api.GetAccessToken(email, password, otp)

	if err != nil {
		return err
	}

	viper.Set(flyctl.ConfigAPIToken, accessToken)

	return flyctl.SaveConfig()
}

func runLogout(ctx *cmdctx.CmdContext) error {
	viper.Set(flyctl.ConfigAPIToken, "")

	if err := flyctl.SaveConfig(); err != nil {
		return err
	}

	fmt.Println("Session removed")

	return nil
}

func runAuthToken(ctx *cmdctx.CmdContext) error {
	token, _ := ctx.GlobalConfig.GetString(flyctl.ConfigAPIToken)

	if ctx.OutputJSON() {
		ctx.WriteJSON(map[string]string{"flyctlAuthToken": token})
		return nil
	}
	fmt.Fprintln(ctx.Out, token)

	return nil
}

func runAuthDocker(ctx *cmdctx.CmdContext) error {
	cc := createCancellableContext()

	binary, err := exec.LookPath("docker")
	if err != nil {
		return errors.Wrap(err, "docker cli not found - make sure it's installed and try again")
	}

	token, _ := ctx.GlobalConfig.GetString(flyctl.ConfigAPIToken)

	cmd := exec.CommandContext(cc, binary, "login", "--username=x", "--password-stdin", "registry.fly.io")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		defer stdin.Close()
		fmt.Fprint(stdin, token)
	}()

	if err := cmd.Wait(); err != nil {
		return err
	}

	if !cmd.ProcessState.Success() {
		output, err := cmd.CombinedOutput()
		if err != nil {
			return err
		}
		fmt.Println(output)
		return errors.New("error authenticating with registry.fly.io")
	}

	fmt.Println("Authentication successful. You can now tag and push images to registry.fly.io/{your-app}")

	return nil
}
