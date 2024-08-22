package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/xeipuuv/gojsonschema"
)

func Run(operationID, file, args string) (string, bool, error) {
	if args == "" {
		args = "{}"
	}
	schemaJSON, opInfo, found, err := GetSchema(operationID, file)
	if err != nil {
		return "", false, err
	} else if !found {
		return "", false, nil
	}

	// Validate args against the schema.
	validationResult, err := gojsonschema.Validate(gojsonschema.NewStringLoader(schemaJSON), gojsonschema.NewStringLoader(args))
	if err != nil {
		return "", false, err
	}

	if !validationResult.Valid() {
		return "", false, fmt.Errorf("invalid arguments for operation %s: %s", operationID, validationResult.Errors())
	}

	// Construct and execute the HTTP request.

	// Handle path parameters.
	opInfo.Path = handlePathParameters(opInfo.Path, opInfo.PathParams, args)

	// Parse the URL
	path, err := url.JoinPath(opInfo.Server, opInfo.Path)
	if err != nil {
		return "", false, fmt.Errorf("failed to join server and path: %w", err)
	}

	u, err := url.Parse(path)
	if err != nil {
		return "", false, fmt.Errorf("failed to parse server URL %s: %w", opInfo.Server+opInfo.Path, err)
	}

	// Set up the request
	req, err := http.NewRequest(opInfo.Method, u.String(), nil)
	if err != nil {
		return "", false, fmt.Errorf("failed to create request: %w", err)
	}

	// TODO - check for auth
	if os.Getenv("OPENAPI_BEARER") != "" {
		req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAPI_BEARER"))
	}

	// Handle query parameters
	req.URL.RawQuery = handleQueryParameters(req.URL.Query(), opInfo.QueryParams, args).Encode()

	if os.Getenv("OPENAPI_QUERY_KEY") != "" {
		req.URL.RawQuery += "&" + "key=" + url.QueryEscape(os.Getenv("OPENAPI_QUERY_KEY"))
	}

	// Handle header and cookie parameters
	handleHeaderParameters(req, opInfo.HeaderParams, args)
	handleCookieParameters(req, opInfo.CookieParams, args)

	// Handle request body
	if opInfo.BodyContentMIME != "" {
		res := gjson.Get(args, "requestBodyContent")
		var body bytes.Buffer
		switch opInfo.BodyContentMIME {
		case "application/json":
			var reqBody interface{}

			reqBody = struct{}{}
			if res.Exists() {
				reqBody = res.Value()
			}
			if err := json.NewEncoder(&body).Encode(reqBody); err != nil {
				return "", false, fmt.Errorf("failed to encode JSON: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

		case "text/plain":
			reqBody := ""
			if res.Exists() {
				reqBody = res.String()
			}
			body.WriteString(reqBody)

			req.Header.Set("Content-Type", "text/plain")

		case "multipart/form-data":
			multiPartWriter := multipart.NewWriter(&body)
			req.Header.Set("Content-Type", multiPartWriter.FormDataContentType())
			if res.Exists() && res.IsObject() {
				for k, v := range res.Map() {
					if err := multiPartWriter.WriteField(k, v.String()); err != nil {
						return "", false, fmt.Errorf("failed to write multipart field: %w", err)
					}
				}
			} else {
				return "", false, fmt.Errorf("multipart/form-data requires an object as the requestBodyContent")
			}
			if err := multiPartWriter.Close(); err != nil {
				return "", false, fmt.Errorf("failed to close multipart writer: %w", err)
			}

		default:
			return "", false, fmt.Errorf("unsupported MIME type: %s", opInfo.BodyContentMIME)
		}
		req.Body = io.NopCloser(&body)
	}

	// Make the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("failed to read response: %w", err)
	}

	return string(result), true, nil
}

// handlePathParameters extracts each path parameter from the input JSON and replaces its placeholder in the URL path.
func handlePathParameters(path string, params []Parameter, input string) string {
	for _, param := range params {
		res := gjson.Get(input, param.Name)
		if res.Exists() {
			// If it's an array or object, handle the serialization style
			if res.IsArray() {
				switch param.Style {
				case "simple", "": // simple is the default style for path parameters
					// simple looks the same regardless of whether explode is true
					strs := make([]string, len(res.Array()))
					for i, item := range res.Array() {
						strs[i] = item.String()
					}
					path = strings.Replace(path, "{"+param.Name+"}", strings.Join(strs, ","), 1)
				case "label":
					strs := make([]string, len(res.Array()))
					for i, item := range res.Array() {
						strs[i] = item.String()
					}

					if param.Explode == nil || !*param.Explode { // default is to not explode
						path = strings.Replace(path, "{"+param.Name+"}", "."+strings.Join(strs, ","), 1)
					} else {
						path = strings.Replace(path, "{"+param.Name+"}", "."+strings.Join(strs, "."), 1)
					}
				case "matrix":
					strs := make([]string, len(res.Array()))
					for i, item := range res.Array() {
						strs[i] = item.String()
					}

					if param.Explode == nil || !*param.Explode { // default is to not explode
						path = strings.Replace(path, "{"+param.Name+"}", ";"+param.Name+"="+strings.Join(strs, ","), 1)
					} else {
						s := ""
						for _, str := range strs {
							s += ";" + param.Name + "=" + str
						}
						path = strings.Replace(path, "{"+param.Name+"}", s, 1)
					}
				}
			} else if res.IsObject() {
				switch param.Style {
				case "simple", "":
					if param.Explode == nil || !*param.Explode { // default is to not explode
						var strs []string
						for k, v := range res.Map() {
							strs = append(strs, k, v.String())
						}
						path = strings.Replace(path, "{"+param.Name+"}", strings.Join(strs, ","), 1)
					} else {
						var strs []string
						for k, v := range res.Map() {
							strs = append(strs, k+"="+v.String())
						}
						path = strings.Replace(path, "{"+param.Name+"}", strings.Join(strs, ","), 1)
					}
				case "label":
					if param.Explode == nil || !*param.Explode { // default is to not explode
						var strs []string
						for k, v := range res.Map() {
							strs = append(strs, k, v.String())
						}
						path = strings.Replace(path, "{"+param.Name+"}", "."+strings.Join(strs, ","), 1)
					} else {
						s := ""
						for k, v := range res.Map() {
							s += "." + k + "=" + v.String()
						}
						path = strings.Replace(path, "{"+param.Name+"}", s, 1)
					}
				case "matrix":
					if param.Explode == nil || !*param.Explode { // default is to not explode
						var strs []string
						for k, v := range res.Map() {
							strs = append(strs, k, v.String())
						}
						path = strings.Replace(path, "{"+param.Name+"}", ";"+param.Name+"="+strings.Join(strs, ","), 1)
					} else {
						s := ""
						for k, v := range res.Map() {
							s += ";" + k + "=" + v.String()
						}
						path = strings.Replace(path, "{"+param.Name+"}", s, 1)
					}
				}
			} else {
				// Serialization is handled slightly differently even for basic types.
				// Explode doesn't do anything though.
				switch param.Style {
				case "simple", "":
					path = strings.Replace(path, "{"+param.Name+"}", res.String(), 1)
				case "label":
					path = strings.Replace(path, "{"+param.Name+"}", "."+res.String(), 1)
				case "matrix":
					path = strings.Replace(path, "{"+param.Name+"}", ";"+param.Name+"="+res.String(), 1)
				}
			}
		}
	}
	return path
}

// handleQueryParameters extracts each query parameter from the input JSON and adds it to the URL query.
func handleQueryParameters(q url.Values, params []Parameter, input string) url.Values {
	for _, param := range params {
		res := gjson.Get(input, param.Name)
		if res.Exists() {
			// If it's an array or object, handle the serialization style
			if res.IsArray() {
				switch param.Style {
				case "form", "": // form is the default style for query parameters
					if param.Explode == nil || *param.Explode { // default is to explode
						for _, item := range res.Array() {
							q.Add(param.Name, item.String())
						}
					} else {
						var strs []string
						for _, item := range res.Array() {
							strs = append(strs, item.String())
						}
						q.Add(param.Name, strings.Join(strs, ","))
					}
				case "spaceDelimited":
					if param.Explode == nil || *param.Explode {
						for _, item := range res.Array() {
							q.Add(param.Name, item.String())
						}
					} else {
						var strs []string
						for _, item := range res.Array() {
							strs = append(strs, item.String())
						}
						q.Add(param.Name, strings.Join(strs, " "))
					}
				case "pipeDelimited":
					if param.Explode == nil || *param.Explode {
						for _, item := range res.Array() {
							q.Add(param.Name, item.String())
						}
					} else {
						var strs []string
						for _, item := range res.Array() {
							strs = append(strs, item.String())
						}
						q.Add(param.Name, strings.Join(strs, "|"))
					}
				}
			} else if res.IsObject() {
				switch param.Style {
				case "form", "": // form is the default style for query parameters
					if param.Explode == nil || *param.Explode { // default is to explode
						for k, v := range res.Map() {
							q.Add(k, v.String())
						}
					} else {
						var strs []string
						for k, v := range res.Map() {
							strs = append(strs, k, v.String())
						}
						q.Add(param.Name, strings.Join(strs, ","))
					}
				case "deepObject":
					for k, v := range res.Map() {
						q.Add(param.Name+"["+k+"]", v.String())
					}
				}
			} else {
				q.Add(param.Name, res.String())
			}
		}
	}
	return q
}

// handleHeaderParameters extracts each header parameter from the input JSON and adds it to the request headers.
func handleHeaderParameters(req *http.Request, params []Parameter, input string) {
	for _, param := range params {
		res := gjson.Get(input, param.Name)
		if res.Exists() {
			if res.IsArray() {
				strs := make([]string, len(res.Array()))
				for i, item := range res.Array() {
					strs[i] = item.String()
				}
				req.Header.Add(param.Name, strings.Join(strs, ","))
			} else if res.IsObject() {
				// Handle explosion
				var strs []string
				if param.Explode == nil || !*param.Explode { // default is to not explode
					for k, v := range res.Map() {
						strs = append(strs, k, v.String())
					}
				} else {
					for k, v := range res.Map() {
						strs = append(strs, k+"="+v.String())
					}
				}
				req.Header.Add(param.Name, strings.Join(strs, ","))
			} else { // basic type
				req.Header.Add(param.Name, res.String())
			}
		}
	}
}

// handleCookieParameters extracts each cookie parameter from the input JSON and adds it to the request cookies.
func handleCookieParameters(req *http.Request, params []Parameter, input string) {
	for _, param := range params {
		res := gjson.Get(input, param.Name)
		if res.Exists() {
			if res.IsArray() {
				strs := make([]string, len(res.Array()))
				for i, item := range res.Array() {
					strs[i] = item.String()
				}
				req.AddCookie(&http.Cookie{
					Name:  param.Name,
					Value: strings.Join(strs, ","),
				})
			} else if res.IsObject() {
				var strs []string
				for k, v := range res.Map() {
					strs = append(strs, k, v.String())
				}
				req.AddCookie(&http.Cookie{
					Name:  param.Name,
					Value: strings.Join(strs, ","),
				})
			} else { // basic type
				req.AddCookie(&http.Cookie{
					Name:  param.Name,
					Value: res.String(),
				})
			}
		}
	}
}
