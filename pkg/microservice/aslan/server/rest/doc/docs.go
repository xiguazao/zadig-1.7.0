/*
Copyright 2021 The KodeRover Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// GENERATED BY THE COMMAND ABOVE; DO NOT EDIT
// This file was generated by swaggo/swag

package doc

import (
	"bytes"
	"encoding/json"
	"strings"
	"text/template"

	"github.com/swaggo/swag"
)

var doc = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{.Description}}",
        "title": "{{.Title}}",
        "contact": {
            "email": "contact@koderover.com"
        },
        "license": {
            "name": "Apache 2.0",
            "url": "http://www.apache.org/licenses/LICENSE-2.0.html"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/workflow/v2/pipelines": {
            "get": {
                "produces": [
                    "application/json"
                ],
                "summary": "Return all workflows (also called pipelines)",
                "responses": {
                    "200": {
                        "description": "response type follows list of microservice/aslan/core/common/repository/models#Pipeline",
                        "schema": {
                            "type": "object"
                        }
                    }
                }
            }
        },
        "/workflow/v2/pipelines/{name}": {
            "get": {
                "produces": [
                    "application/json"
                ],
                "summary": "Get the relevant workflow (also called pipeline) information with the specified workflow name",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Name of the workflow",
                        "name": "name",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "response type follows microservice/aslan/core/common/repository/models#Pipeline",
                        "schema": {
                            "type": "object"
                        }
                    }
                }
            }
        },
        "/workflow/webhook/gerritHook": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Process webhook for gerrit",
                "responses": {
                    "200": {
                        "description": "map[string]string - {message: success}",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/workflow/webhook/githubWebHook": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Process webhook for github",
                "responses": {
                    "200": {
                        "description": "map[string]string - {message: 'success information'}",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/workflow/webhook/gitlabhook": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "summary": "Process webhook for giblab",
                "responses": {
                    "200": {
                        "description": "map[string]string - {message: success}",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        }
    }
}`

type swaggerInfo struct {
	Version     string
	Host        string
	BasePath    string
	Schemes     []string
	Title       string
	Description string
}

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = swaggerInfo{
	Version:     "1.0",
	Host:        "",
	BasePath:    "/api/aslan",
	Schemes:     []string{},
	Title:       "Zadig aslan service REST APIs",
	Description: "The API doc is targeting for Zadig developers rather than Zadig users.\nThe majority of these APIs are not designed for public use, there is no guarantee on version compatibility.\nPlease reach out to contact@koderover.com before you decide to depend on these APIs directly.",
}

type s struct{}

func (s *s) ReadDoc() string {
	sInfo := SwaggerInfo
	sInfo.Description = strings.Replace(sInfo.Description, "\n", "\\n", -1)

	t, err := template.New("swagger_info").Funcs(template.FuncMap{
		"marshal": func(v interface{}) string {
			a, _ := json.Marshal(v)
			return string(a)
		},
	}).Parse(doc)
	if err != nil {
		return doc
	}

	var tpl bytes.Buffer
	if err := t.Execute(&tpl, sInfo); err != nil {
		return doc
	}

	return tpl.String()
}

func init() {
	swag.Register(swag.Name, &s{})
}
