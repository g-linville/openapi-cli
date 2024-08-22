package cli

import (
	"fmt"

	"github.com/gptscript-ai/cmd"
	"github.com/spf13/cobra"
)

type OpenAPICLI struct {
}

func (o *OpenAPICLI) Customize(cmd *cobra.Command) {
}

func (o *OpenAPICLI) Run(*cobra.Command, []string) error {
	printUsage()
	return nil
}

func New() *cobra.Command {
	return cmd.Command(&OpenAPICLI{}, &List{}, &GetSchema{}, &Run{})
}

func printUsage() {
	fmt.Println("Usage: openapi-cli command files...")
}
