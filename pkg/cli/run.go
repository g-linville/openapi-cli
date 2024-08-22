package cli

import (
	"fmt"

	"github.com/gptscript-ai/openapi-cli/pkg/openapi"
	"github.com/spf13/cobra"
)

type Run struct {
	DefaultHost string `json:"defaultHost"`
}

func (r *Run) Run(_ *cobra.Command, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("not enough args")
	}

	operationID := args[0]
	input := args[1]
	files := args[2:]

	for _, file := range files {
		output, found, err := openapi.Run(operationID, file, input)
		if err != nil {
			return fmt.Errorf("failed to run operation %s in file %s: %w", operationID, file, err)
		}

		if found {
			fmt.Println(output)
			return nil
		}
	}

	return fmt.Errorf("operation %s not found in any file", operationID)
}
