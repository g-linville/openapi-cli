package cli

import (
	"fmt"

	"github.com/gptscript-ai/openapi-cli/pkg/openapi"
	"github.com/spf13/cobra"
)

type GetSchema struct{}

func (g *GetSchema) Run(_ *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("not enough args")
	}

	operationID := args[0]
	files := args[1:]

	for _, file := range files {
		schema, _, found, err := openapi.GetSchema(operationID, file)
		if err != nil {
			return fmt.Errorf("failed to get schema for operation %s in file %s: %w", operationID, file, err)
		}
		if !found {
			continue
		}
		fmt.Println(schema)
		return nil
	}

	return fmt.Errorf("operation %s not found in any file", operationID)
}
