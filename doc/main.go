package main

import (
	"log"
	"os"
	"path"
	"strings"

	"github.com/superfly/flyctl/cmd"

	"github.com/spf13/cobra/doc"
)

func main() {
	cmd := cmd.GetRootCommand()
	cmd.DisableAutoGenTag = true

	filePrepender := func(filename string) string {
		return ""
	}

	linkHandler := func(name string) string {
		base := strings.TrimSuffix(name, path.Ext(name))
		base = strings.Replace(base, "flyctl_", "", 1)
		if base == "flyctl" {
			base = "help"
		}
		base = strings.ReplaceAll(base, "_", "-") + "/"
		return "/docs/flyctl/" + strings.ToLower(base)
	}
	//kubectl := cmd.NewKubectlCommand(cmdutil.NewFactory(nil), os.Stdin, ioutil.Discard, ioutil.Discard)
	os.MkdirAll("out", 0700)
	err := doc.GenMarkdownTreeCustom(cmd, "./out", filePrepender, linkHandler)

	if err != nil {
		log.Fatal(err)
	}
}
