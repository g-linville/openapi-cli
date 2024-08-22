package openapi

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

type OperationList struct {
	Operations map[string]Operation `json:"operations"`
}

type Operation struct {
	Description string `json:"description,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

func List(file string) (OperationList, error) {
	loader := openapi3.NewLoader()
	t, err := loader.LoadFromFile(file)
	if err != nil {
		return OperationList{}, fmt.Errorf("failed to load OpenAPI file %s: %w", file, err)
	}

	operations := make(map[string]Operation)
	for _, pathItem := range t.Paths.Map() {
		for _, operation := range pathItem.Operations() {
			operations[operation.OperationID] = Operation{
				Description: operation.Description,
				Summary:     operation.Summary,
			}
		}
	}

	return OperationList{Operations: operations}, nil
}
