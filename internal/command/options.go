package command

import (
	"github.com/spf13/cobra"
)

func AnnotateCommand(cmd *cobra.Command, key, value string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}

	cmd.Annotations[key] = value
}

func TagV1Command(cmd *cobra.Command) {
	AnnotateCommand(cmd, "apps_v1", "1")
}

func IsAppsV1Command(cmd *cobra.Command) bool {
	_, ok := cmd.Annotations["apps_v1"]
	return ok
}
