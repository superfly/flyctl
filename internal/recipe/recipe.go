package recipe

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
)

type Recipe struct {
	Agent     *agent.Client
	App       *api.App
	Client    *client.Client
	Dialer    *agent.Dialer
	AuthToken string
}

func NewRecipe(ctx context.Context, app *api.App) (*Recipe, error) {
	client := client.New()

	agentclient, err := agent.Establish(ctx, client.API())
	if err != nil {
		return nil, errors.Wrap(err, "can't establish agent")
	}
	dialer, err := agentclient.Dialer(ctx, &app.Organization)
	if err != nil {
		return nil, fmt.Errorf("ssh: can't build tunnel for %s: %s\n", app.Organization.Slug, err)
	}

	authToken := flyctl.GetAPIToken()

	recipe := &Recipe{
		Client:    client,
		Agent:     agentclient,
		Dialer:    &dialer,
		App:       app,
		AuthToken: authToken,
	}

	if err = recipe.buildTunnel(ctx); err != nil {
		return recipe, err
	}

	return recipe, nil
}

// Helper for building tunnel
func (r *Recipe) buildTunnel(ctx context.Context) error {
	r.Client.IO.StartProgressIndicatorMsg("Connecting to tunnel")
	if err := r.Agent.WaitForTunnel(ctx, &r.App.Organization); err != nil {
		return errors.Wrapf(err, "tunnel unavailable")
	}
	r.Client.IO.StopProgressIndicator()

	return nil
}

func (r *Recipe) RunHTTPOperation(ctx context.Context, machine *api.Machine, method, endpoint string) (*RecipeOperation, error) {
	op := NewRecipeOperation(r, machine, "")
	if err := op.RunHTTPCommand(ctx, method, endpoint); err != nil {
		return nil, err
	}

	return op, nil
}

func (r *Recipe) RunSSHOperation(ctx context.Context, machine *api.Machine, command string) (*RecipeOperation, error) {
	op := NewRecipeOperation(r, machine, command)
	if err := op.RunSSHCommand(ctx); err != nil {
		return nil, err
	}

	return op, nil
}

func (r *Recipe) RunSSHAttachOperation(ctx context.Context, machine *api.Machine, command string) (*RecipeOperation, error) {
	op := NewRecipeOperation(r, machine, command)
	if err := op.RunSSHAttachCommand(ctx); err != nil {
		return nil, err
	}

	return op, nil
}
