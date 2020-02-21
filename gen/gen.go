package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"log"
	"os"
	"path"
	"reflect"
	"strings"
	"text/template"
	"time"

	"github.com/ghodss/yaml"
	"github.com/go-openapi/spec"
	"github.com/swaggo/swag"
)

// Gen presents a generate tool for swag.
type Gen struct {
	jsonIndent func(data interface{}) ([]byte, error)
	jsonToYAML func(data []byte) ([]byte, error)
}

// New creates a new Gen.
func New() *Gen {
	return &Gen{
		jsonIndent: func(data interface{}) ([]byte, error) {
			return json.MarshalIndent(data, "", "    ")
		},
		jsonToYAML: yaml.JSONToYAML,
	}
}

// Config presents Gen configurations.
type Config struct {
	// SearchDir the swag would be parse
	SearchDir string

	// OutputDir represents the output directory for all the generated files
	OutputDir string

	// MainAPIFile the Go file path in which 'swagger general API Info' is written
	MainAPIFile string

	// PropNamingStrategy represents property naming strategy like snakecase,camelcase,pascalcase
	PropNamingStrategy string

	// ParseVendor whether swag should be parse vendor folder
	ParseVendor bool

	// ParseDependencies whether swag should be parse outside dependency folder
	ParseDependency bool

	// MarkdownFilesDir used to find markdownfiles, which can be used for tag descriptions
	MarkdownFilesDir string

	// GeneratedTime whether swag should generate the timestamp at the top of docs.go
	GeneratedTime bool
}

// Build builds swagger json file  for given searchDir and mainAPIFile. Returns json
func (g *Gen) Build(config *Config) error {
	if _, err := os.Stat(config.SearchDir); os.IsNotExist(err) {
		return fmt.Errorf("dir: %s is not exist", config.SearchDir)
	}

	log.Println("Generate swagger docs....")
	p := swag.New(swag.SetMarkdownFileDirectory(config.MarkdownFilesDir))
	p.PropNamingStrategy = config.PropNamingStrategy
	p.ParseVendor = config.ParseVendor
	p.ParseDependency = config.ParseDependency

	if err := p.ParseAPI(config.SearchDir, config.MainAPIFile); err != nil {
		return err
	}
	swagger := p.GetSwagger()
	pathMap := swagger.SwaggerProps.Paths.Paths
	//newPathMap := make(map[string]spec.PathItem)
	for key, _ := range pathMap {
		item := pathMap[key]
		// 获取get post等path
		rv := reflect.ValueOf(&item.PathItemProps).Elem()
		for i := 0; i < rv.NumField(); i++ {
			x := rv.Field(i)
			if !x.IsNil() && reflect.TypeOf(x.Interface()).Kind() == reflect.Ptr {
				get, ok := x.Interface().(*spec.Operation)
				if ok {
					getResp := get.OperationProps.Responses
					if getResp == nil {
						continue
					}
					responseMap := getResp.ResponsesProps.StatusCodeResponses
					if responseMap == nil {
						continue
					}
					for keyCode, valResponse := range responseMap {
						if valResponse.ResponseProps.Schema.SchemaProps.Type == nil {
							newResponse := responseMap[keyCode]
							urn := newResponse.ResponseProps.Schema.SchemaProps.Ref.Ref.String()
							baseRef, _ := spec.NewRef(urn + "Auto")
							newResponse.ResponseProps.Schema.SchemaProps.Ref = baseRef
							responseMap[keyCode] = newResponse
							addAutoObject(swagger, urn)
						}
					}
					baseRef, _ := spec.NewRef("#/definitions/yidAutoResponse")
					if _, ok := responseMap[200]; !ok {
						responseMap[200] = spec.Response{
							ResponseProps: spec.ResponseProps{
								Description: "auto response",
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Ref: baseRef,
									},
								},
							},
						}
					}
				}
			}
		}
		//get:=item.PathItemProps.Get
		//if get != nil{
		//	getResp:=get.OperationProps.Responses
		//	if getResp == nil{
		//		continue
		//	}
		//	responseMap:=getResp.ResponsesProps.StatusCodeResponses
		//	if responseMap == nil{
		//		continue
		//	}
		//	for keyr,valr:= range responseMap{
		//		if valr.ResponseProps.Schema.SchemaProps.Type==nil{
		//			newResponse:=responseMap[keyr]
		//			urn:=newResponse.ResponseProps.Schema.SchemaProps.Ref.Ref.String()
		//			baseRef, _ := spec.NewRef(urn+"Auto")
		//			newResponse.ResponseProps.Schema.SchemaProps.Ref=baseRef
		//			responseMap[keyr]=newResponse
		//			addAutoObject(swagger,urn)
		//		}
		//	}
		//}
		pathMap[key] = item
	}
	swagger.SwaggerProps.Paths.Paths = pathMap

	// 默认返回
	swagger.Definitions["yidAutoResponse"] = spec.Schema{
		SchemaProps: spec.SchemaProps{
			Type: []string{"object"},
			Properties: map[string]spec.Schema{
				"code": {
					SchemaProps: spec.SchemaProps{
						Description: "返回码",
						Type:        []string{"string"},
					}},
				"message": {
					SchemaProps: spec.SchemaProps{
						Description: "返回信息",
						Type:        []string{"string"},
					}},
				"request_id": {
					SchemaProps: spec.SchemaProps{
						Description: "requestID",
						Type:        []string{"string"},
					}},
				"data": {
					SchemaProps: spec.SchemaProps{
						Description: "data",
						Type:        []string{"object"},
					}},
			},
		},
	}

	//baseRef, _ := spec.NewRef("#/definitions/api.Data")
	//swagger.Definitions["api.ResponseTestAuto"] = spec.Schema{
	//	SchemaProps: spec.SchemaProps{
	//		Type: []string{"object"},
	//		Properties: map[string]spec.Schema{
	//			"code": {
	//				SchemaProps: spec.SchemaProps{
	//					Description: "返回码",
	//					Type:        []string{"string"},
	//				}},
	//			"message": {
	//				SchemaProps: spec.SchemaProps{
	//					Description: "返回信息",
	//					Type:        []string{"string"},
	//				}},
	//			"status": {
	//				SchemaProps: spec.SchemaProps{
	//					Description: "返回状态",
	//					Type:        []string{"string"},
	//				}},
	//			"request_id": {
	//				SchemaProps: spec.SchemaProps{
	//					Description: "requestID",
	//					Type:        []string{"string"},
	//				}},
	//			"data": {
	//				SchemaProps: spec.SchemaProps{
	//					Description: "数据",
	//					Type:        []string{"object"},
	//					Ref: baseRef,
	//					},
	//				},
	//		},
	//	},
	//}

	b, err := g.jsonIndent(swagger)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(config.OutputDir, os.ModePerm); err != nil {
		return err
	}

	packageName := path.Base(config.OutputDir)
	docFileName := path.Join(config.OutputDir, "docs.go")
	jsonFileName := path.Join(config.OutputDir, "swagger.json")
	yamlFileName := path.Join(config.OutputDir, "swagger.yaml")

	docs, err := os.Create(docFileName)
	if err != nil {
		return err
	}
	defer docs.Close()

	err = g.writeFile(b, jsonFileName)
	if err != nil {
		return err
	}

	y, err := g.jsonToYAML(b)
	if err != nil {
		return fmt.Errorf("cannot convert json to yaml error: %s", err)
	}

	err = g.writeFile(y, yamlFileName)
	if err != nil {
		return err
	}

	// Write doc
	err = g.writeGoDoc(packageName, docs, swagger, config)
	if err != nil {
		return err
	}

	log.Printf("create docs.go at %+v", docFileName)
	log.Printf("create swagger.json at %+v", jsonFileName)
	log.Printf("create swagger.yaml at %+v", yamlFileName)

	return nil
}

func addAutoObject(swagger *spec.Swagger, uri string) {
	baseRef, _ := spec.NewRef(uri)
	splitedUri := strings.Split(uri, `/`)
	name := splitedUri[len(splitedUri)-1]
	swagger.Definitions[name+"Auto"] = spec.Schema{
		SchemaProps: spec.SchemaProps{
			Type: []string{"object"},
			Properties: map[string]spec.Schema{
				"code": {
					SchemaProps: spec.SchemaProps{
						Description: "返回码",
						Type:        []string{"string"},
					}},
				"message": {
					SchemaProps: spec.SchemaProps{
						Description: "返回信息",
						Type:        []string{"string"},
					}},
				"request_id": {
					SchemaProps: spec.SchemaProps{
						Description: "requestID",
						Type:        []string{"string"},
					}},
				"data": {
					SchemaProps: spec.SchemaProps{
						Description: "数据",
						Type:        []string{"object"},
						Ref:         baseRef,
					},
				},
			},
		},
	}
}

func (g *Gen) writeFile(b []byte, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(b)
	return err
}

func (g *Gen) formatSource(src []byte) []byte {
	code, err := format.Source(src)
	if err != nil {
		code = src // Output the unformatted code anyway
	}
	return code
}

func (g *Gen) writeGoDoc(packageName string, output io.Writer, swagger *spec.Swagger, config *Config) error {
	generator, err := template.New("swagger_info").Funcs(template.FuncMap{
		"printDoc": func(v string) string {
			// Add schemes
			v = "{\n    \"schemes\": {{ marshal .Schemes }}," + v[1:]
			// Sanitize backticks
			return strings.Replace(v, "`", "`+\"`\"+`", -1)
		},
	}).Parse(packageTemplate)
	if err != nil {
		return err
	}

	swaggerSpec := &spec.Swagger{
		VendorExtensible: swagger.VendorExtensible,
		SwaggerProps: spec.SwaggerProps{
			ID:       swagger.ID,
			Consumes: swagger.Consumes,
			Produces: swagger.Produces,
			Swagger:  swagger.Swagger,
			Info: &spec.Info{
				VendorExtensible: swagger.Info.VendorExtensible,
				InfoProps: spec.InfoProps{
					Description:    "{{.Description}}",
					Title:          "{{.Title}}",
					TermsOfService: swagger.Info.TermsOfService,
					Contact:        swagger.Info.Contact,
					License:        swagger.Info.License,
					Version:        "{{.Version}}",
				},
			},
			Host:                "{{.Host}}",
			BasePath:            "{{.BasePath}}",
			Paths:               swagger.Paths,
			Definitions:         swagger.Definitions,
			Parameters:          swagger.Parameters,
			Responses:           swagger.Responses,
			SecurityDefinitions: swagger.SecurityDefinitions,
			Security:            swagger.Security,
			Tags:                swagger.Tags,
			ExternalDocs:        swagger.ExternalDocs,
		},
	}

	// crafted docs.json
	buf, err := g.jsonIndent(swaggerSpec)
	if err != nil {
		return err
	}

	buffer := &bytes.Buffer{}
	err = generator.Execute(buffer, struct {
		Timestamp     time.Time
		GeneratedTime bool
		Doc           string
		Host          string
		PackageName   string
		BasePath      string
		Schemes       []string
		Title         string
		Description   string
		Version       string
	}{
		Timestamp:     time.Now(),
		GeneratedTime: config.GeneratedTime,
		Doc:           string(buf),
		Host:          swagger.Host,
		PackageName:   packageName,
		BasePath:      swagger.BasePath,
		Schemes:       swagger.Schemes,
		Title:         swagger.Info.Title,
		Description:   swagger.Info.Description,
		Version:       swagger.Info.Version,
	})
	if err != nil {
		return err
	}

	code := g.formatSource(buffer.Bytes())

	// write
	_, err = output.Write(code)
	return err
}

var packageTemplate = `// GENERATED BY THE COMMAND ABOVE; DO NOT EDIT
// This file was generated by swaggo/swag{{ if .GeneratedTime }} at
// {{ .Timestamp }}{{ end }}

package {{.PackageName}}

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/alecthomas/template"
	"github.com/swaggo/swag"
)

var doc = ` + "`{{ printDoc .Doc}}`" + `

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
	Version:     {{ printf "%q" .Version}},
 	Host:        {{ printf "%q" .Host}},
	BasePath:    {{ printf "%q" .BasePath}},
	Schemes:     []string{ {{ range $index, $schema := .Schemes}}{{if gt $index 0}},{{end}}{{printf "%q" $schema}}{{end}} },
	Title:       {{ printf "%q" .Title}},
	Description: {{ printf "%q" .Description}},
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
`
