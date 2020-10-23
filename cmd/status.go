package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/inancgumus/screen"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/segmentio/textio"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/docstrings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newStatusCommand() *Command {
	statusStrings := docstrings.Get("status")
	cmd := BuildCommandKS(nil, runStatus, statusStrings, os.Stdout, requireSession, requireAppNameAsArg)

	//TODO: Move flag descriptions to docstrings
	cmd.AddBoolFlag(BoolFlagOpts{Name: "all", Description: "Show completed allocations"})
	cmd.AddBoolFlag(BoolFlagOpts{Name: "watch", Description: "Refresh details"})
	cmd.AddIntFlag(IntFlagOpts{Name: "rate", Description: "Refresh Rate for --watch", Default: 5})

	allocStatusStrings := docstrings.Get("status.alloc")
	allocStatusCmd := BuildCommand(cmd, runAllocStatus, allocStatusStrings.Usage, allocStatusStrings.Short, allocStatusStrings.Long, os.Stdout, requireSession, requireAppName)
	allocStatusCmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runStatus(ctx *cmdctx.CmdContext) error {

	watch := ctx.Config.GetBool("watch")
	refreshRate := ctx.Config.GetInt("rate")
	refreshCount := 1

	if watch && ctx.OutputJSON() {
		return fmt.Errorf("--watch and --json are not supported together")
	}

	for true {
		var app *api.AppStatus
		var backupregions []api.Region
		var err error
		if watch {
			refreshCount = refreshCount - 1
			if refreshCount == 0 {
				refreshCount = refreshRate
				app, err = ctx.Client.API().GetAppStatus(ctx.AppName, ctx.Config.GetBool("all"))

				if err != nil {
					return err
				}
				if app.Deployed {
					_, backupregions, err = ctx.Client.API().ListAppRegions(ctx.AppName)

					if err != nil {
						return err
					}

				}
				screen.Clear()
				screen.MoveTopLeft()
				fmt.Printf("%s %s %s\n\n", aurora.Bold(app.Name), aurora.Italic("at:"), aurora.Bold(time.Now().UTC().Format("15:04:05")))
			} else {
				screen.MoveTopLeft()
				if app != nil {
					fmt.Printf("%s %s %s\n\n", aurora.Bold(app.Name), aurora.Italic("at:"), aurora.Bold(time.Now().UTC().Format("15:04:05")))
				} else {
					fmt.Printf("%s %s %s\n\n", aurora.Bold(ctx.AppName), aurora.Italic("at:"), aurora.Bold(time.Now().UTC().Format("15:04:05")))
				}
				time.Sleep(time.Second)
				continue
			}
		} else {
			app, err = ctx.Client.API().GetAppStatus(ctx.AppName, ctx.Config.GetBool("all"))
			if app.Deployed {
				_, backupregions, err = ctx.Client.API().ListAppRegions(ctx.AppName)

				if err != nil {
					return err
				}

			}
			if err != nil {
				return err
			}
		}

		err = ctx.Frender(cmdctx.PresenterOption{Presentable: &presenters.AppStatus{AppStatus: *app}, HideHeader: true, Vertical: true, Title: "App"})
		if err != nil {
			return err
		}

		// If JSON output, everything has been printed, so return
		if !watch && ctx.OutputJSON() {
			return nil
		}

		// Continue formatted output
		if !app.Deployed {
			fmt.Println(`App has not been deployed yet.`)
			// exit if not watching, stay looping if we are
			if !watch {
				return nil
			}
		}

		if app.DeploymentStatus != nil {
			err = ctx.Frender(cmdctx.PresenterOption{
				Presentable: &presenters.DeploymentStatus{Status: app.DeploymentStatus},
				Vertical:    true,
				Title:       "Deployment Status",
			})

			if err != nil {
				return err
			}
		}

		err = ctx.Frender(cmdctx.PresenterOption{
			Presentable: &presenters.Allocations{Allocations: app.Allocations, BackupRegions: backupregions},
			Title:       "Allocations",
		})

		if err != nil {
			return err
		}

		if !watch {
			return nil
		}
	}

	return nil
}

func runAllocStatus(ctx *cmdctx.CmdContext) error {
	alloc, err := ctx.Client.API().GetAllocationStatus(ctx.AppName, ctx.Args[0], 25)
	if err != nil {
		return err
	}

	if alloc == nil {
		return api.ErrNotFound
	}

	err = ctx.Frender(
		cmdctx.PresenterOption{
			Title: "Allocation",
			Presentable: &presenters.Allocations{
				Allocations: []*api.AllocationStatus{alloc},
			},
			Vertical: true,
		},
		cmdctx.PresenterOption{
			Title: "Recent Events",
			Presentable: &presenters.AllocationEvents{
				Events: alloc.Events,
			},
		},
		cmdctx.PresenterOption{
			Title: "Checks",
			Presentable: &presenters.AllocationChecks{
				Checks: alloc.Checks,
			},
		},
	)
	if err != nil {
		return err
	}

	var p io.Writer
	var pw *textio.PrefixWriter

	if !ctx.OutputJSON() {
		fmt.Println(aurora.Bold("Recent Logs"))
		pw = textio.NewPrefixWriter(ctx.Out, "  ")
		p = pw
	} else {
		p = ctx.Out
	}

	logPresenter := presenters.LogPresenter{HideAllocID: true, HideRegion: true, RemoveNewlines: true}
	logPresenter.FPrint(p, ctx.OutputJSON(), alloc.RecentLogs)

	if p != ctx.Out {
		_ = pw.Flush()
	}

	return nil
}
