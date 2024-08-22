package cli

import (
	"encoding/json"
	"fmt"

	"github.com/gptscript-ai/openapi-cli/pkg/openapi"
	"github.com/spf13/cobra"
)

type List struct{}

func (l *List) Run(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no files provided")
	}

	for _, file := range args {
		operationList, err := openapi.List(file)
		if err != nil {
			return fmt.Errorf("failed to list operations for file %s: %w", file, err)
		}

		operationListJSON, err := json.MarshalIndent(operationList, "", "    ")
		if err != nil {
			return fmt.Errorf("failed to marshal operation list: %w", err)
		}

		fmt.Println(string(operationListJSON))
	}

	return nil
}
