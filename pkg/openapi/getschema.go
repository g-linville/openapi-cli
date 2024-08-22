package openapi

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type Parameter struct {
	Name, Style string
	Explode     *bool
}

type OperationInfo struct {
	Server, Path, Method, BodyContentMIME string
	// TODO - security infos
	QueryParams, PathParams, HeaderParams, CookieParams []Parameter
}

var supportedMIMETypes = []string{"application/json", "application/x-www-form-urlencoded", "multipart/form-data"}

// GetSchema returns the JSONSchema and OperationInfo for a particular OpenAPI operation.
// Return values in order: JSONSchema (string), OperationInfo, found (bool), error.
func GetSchema(operationID, file string) (string, OperationInfo, bool, error) {
	loader := openapi3.NewLoader()
	t, err := loader.LoadFromFile(file)
	if err != nil {
		return "", OperationInfo{}, false, err
	}

	// We basically want to extract all the information that we need for the HTTP request,
	// like we do in GPTScript.
	arguments := &openapi3.Schema{
		Type:       &openapi3.Types{"object"},
		Properties: openapi3.Schemas{},
		Required:   []string{},
	}

	info := OperationInfo{}

	// Determine the default server.
	// TODO - take in a default host parameter? Like the source where the OpenAPI doc was downloaded from?
	var defaultServer string
	if len(t.Servers) > 0 {
		defaultServer, err = parseServer(t.Servers[0])
		if err != nil {
			return "", OperationInfo{}, false, err
		}
	}

	for path, pathItem := range t.Paths.Map() {
		// Handle path-level server override, if one exists.
		pathServer := defaultServer
		if pathItem.Servers != nil && len(pathItem.Servers) > 0 {
			pathServer, err = parseServer(pathItem.Servers[0])
			if err != nil {
				return "", OperationInfo{}, false, err
			}
		}

		for method, operation := range pathItem.Operations() {
			if operation.OperationID == operationID {
				// Handle operation-level server override, if one exists.
				operationServer := pathServer
				if operation.Servers != nil && len(*operation.Servers) > 0 {
					operationServer, err = parseServer((*operation.Servers)[0])
					if err != nil {
						return "", OperationInfo{}, false, err
					}
				}

				info.Server = operationServer
				info.Path = path
				info.Method = method

				// We found our operation. Now we need to process it and build the arguments.
				// Handle query, path, header, and cookie parameters first.
				for _, param := range append(operation.Parameters, pathItem.Parameters...) {
					removeRefs(param.Value.Schema)
					arg := param.Value.Schema.Value

					if arg.Description == "" {
						arg.Description = param.Value.Description
					}

					// Store the arg
					arguments.Properties[param.Value.Name] = &openapi3.SchemaRef{Value: arg}

					// Check whether it is required
					if param.Value.Required {
						arguments.Required = append(arguments.Required, param.Value.Name)
					}

					// Save the parameter to the correct set of params.
					p := Parameter{
						Name:    param.Value.Name,
						Style:   param.Value.Style,
						Explode: param.Value.Explode,
					}
					switch param.Value.In {
					case "query":
						info.QueryParams = append(info.QueryParams, p)
					case "path":
						info.PathParams = append(info.PathParams, p)
					case "header":
						info.HeaderParams = append(info.HeaderParams, p)
					case "cookie":
						info.CookieParams = append(info.CookieParams, p)
					}
				}

				// Next, handle the request body, if one exists.
				if operation.RequestBody != nil {
					for mime, content := range operation.RequestBody.Value.Content {
						// Each MIME type needs to be handled individually, so we keep a list of the ones we support.
						if !slices.Contains(supportedMIMETypes, mime) {
							continue
						}
						info.BodyContentMIME = mime

						removeRefs(content.Schema)

						arg := content.Schema.Value
						if arg.Description == "" {
							arg.Description = content.Schema.Value.Description
						}

						// Read Only cannot be sent in the request body, so we remove it
						for key, property := range arg.Properties {
							if property.Value.ReadOnly {
								delete(arg.Properties, key)
							}
						}

						// Unfortunately, the request body doesn't contain any good descriptor for it,
						// so we just use "requestBodyContent" as the name of the arg.
						arguments.Properties["requestBodyContent"] = &openapi3.SchemaRef{Value: arg}
						arguments.Required = append(arguments.Required, "requestBodyContent")
						break
					}

					if info.BodyContentMIME == "" {
						return "", OperationInfo{}, false, fmt.Errorf("no supported MIME type found for request body in operation %s", operationID)
					}
				}

				argumentsJSON, err := json.MarshalIndent(arguments, "", "    ")
				if err != nil {
					return "", OperationInfo{}, false, err
				}
				return string(argumentsJSON), info, true, nil
			}
		}
	}

	return "", OperationInfo{}, false, nil
}

func parseServer(server *openapi3.Server) (string, error) {
	s := server.URL
	for name, variable := range server.Variables {
		if variable == nil {
			continue
		}

		if variable.Default != "" {
			s = strings.Replace(s, "{"+name+"}", variable.Default, 1)
		} else if len(variable.Enum) > 0 {
			s = strings.Replace(s, "{"+name+"}", variable.Enum[0], 1)
		}
	}

	if !strings.HasPrefix(s, "http") {
		return "", fmt.Errorf("invalid server URL: %s (must use HTTP or HTTPS; relative URLs not supported)", s)
	}
	return s, nil
}

func removeRefs(r *openapi3.SchemaRef) {
	if r == nil {
		return
	}

	r.Ref = ""
	r.Value.Discriminator = nil // Discriminators are not very useful and can junk up the schema.

	for i := range r.Value.OneOf {
		removeRefs(r.Value.OneOf[i])
	}
	for i := range r.Value.AnyOf {
		removeRefs(r.Value.AnyOf[i])
	}
	for i := range r.Value.AllOf {
		removeRefs(r.Value.AllOf[i])
	}
	removeRefs(r.Value.Not)
	removeRefs(r.Value.Items)

	for i := range r.Value.Properties {
		removeRefs(r.Value.Properties[i])
	}
}
