package natsapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/invopop/jsonschema"
)

const docServerHTML = `<html>
<head>
<style>
  body { margin: 0; }
</style>
</head>
<body>
    <redoc></redoc>
    <script src="https://files.wegroup.be/jsbundles/redoc.standalone.js"></script>
    <script>
        Redoc.init("/asyncapi.json", {
            sortTagsAlphabetically: true,
            sortOperationsAlphabetically: true,
        });
    </script>
</body>
</html>`

const refPrefix = "#/components/schemas/"

type AsyncApiSpec struct {
	AsyncAPI           string                     `json:"asyncapi"`
	Info               AsyncApiInfo               `json:"info"`
	DefaultContentType string                     `json:"defaultContentType"`
	Channels           map[string]AsyncApiChannel `json:"channels"`
	Components         AsyncApiComponents         `json:"components"`
}

type AsyncApiInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type AsyncApiComponents struct {
	Schemas map[string]*jsonschema.Schema `json:"schemas"`
}

type AsyncApiChannel struct {
	Request *AsyncApiOperation `json:"request,omitempty"`
	Publish *AsyncApiOperation `json:"publish,omitempty"`
}

type AsyncApiOperation struct {
	OperationID string            `json:"operationId"`
	Summary     string            `json:"summary"`
	Description string            `json:"description"`
	Tags        []AsyncApiTag     `json:"tags"`
	Message     AsyncApiMessage   `json:"message"`
	Replies     []AsyncApiMessage `json:"replies"`
}

type AsyncApiTag struct {
	Name string `json:"name"`
}

type AsyncApiMessage struct {
	Payload *AsyncApiRef `json:"payload,omitempty"`
}

type AsyncApiRef struct {
	Ref string `json:"$ref"`
}

var reflector = &jsonschema.Reflector{
	Anonymous:      true,
	ExpandedStruct: true,
	Mapper: func(t reflect.Type) *jsonschema.Schema {
		switch t {
		case reflect.TypeOf(uuid.UUID{}):
			return &jsonschema.Schema{Type: "string", Format: "uuid"}
		case reflect.TypeOf(json.RawMessage{}):
			return &jsonschema.Schema{Title: "Any"}
		}
		return nil
	},
}

// AsyncApiSpec generates a schema document from all registered routes,
// matching the natsapi Python library format.
func (a *NatsAPI) AsyncApiSpec() AsyncApiSpec {
	channels := make(map[string]AsyncApiChannel)
	schemas := make(map[string]*jsonschema.Schema)

	addSchema := func(t reflect.Type) string {
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		name := t.Name()
		if name == "" || t.Kind() != reflect.Struct || t.NumField() == 0 {
			name = "EmptyParams"
			if _, exists := schemas[name]; !exists {
				s := &jsonschema.Schema{}
				s.Title = name
				s.Type = "object"
				s.Properties = jsonschema.NewProperties()
				schemas[name] = s
			}
			return name
		}
		if _, exists := schemas[name]; !exists {
			s := reflector.ReflectFromType(t)
			s.Version = ""
			s.Title = name
			optionalPointers(t, s, s.Definitions)
			rewriteRefs(s)
			for defName, def := range s.Definitions {
				if _, exists := schemas[defName]; !exists {
					def.Title = defName
					schemas[defName] = def
				}
			}
			s.Definitions = nil
			schemas[name] = s
		}
		return name
	}

	errorName := addSchema(reflect.TypeOf(JsonRPCError{}))

	for subject, route := range a.routes {
		op := &AsyncApiOperation{
			OperationID: operationID(route.Subject),
			Summary:     operationID(route.Subject),
			Description: route.Description,
			Tags:        tagsToAsyncAPI(route.Tags),
			Replies:     []AsyncApiMessage{},
		}
		if route.ParamType != nil {
			name := addSchema(route.ParamType)
			op.Message = AsyncApiMessage{Payload: &AsyncApiRef{Ref: refPrefix + name}}
		}
		if route.ResultType != nil {
			name := addSchema(route.ResultType)
			op.Replies = []AsyncApiMessage{{Payload: &AsyncApiRef{Ref: refPrefix + name}}}
		}
		op.Replies = append(op.Replies, AsyncApiMessage{Payload: &AsyncApiRef{Ref: refPrefix + errorName}})
		channels[subject] = AsyncApiChannel{Request: op}
	}

	for subject, route := range a.publishRoutes {
		op := &AsyncApiOperation{
			OperationID: operationID(route.Subject),
			Summary:     operationID(route.Subject),
			Description: route.Description,
			Tags:        tagsToAsyncAPI(route.Tags),
			Replies:     []AsyncApiMessage{},
		}
		if route.ParamType != nil {
			name := addSchema(route.ParamType)
			op.Message = AsyncApiMessage{Payload: &AsyncApiRef{Ref: refPrefix + name}}
		}
		channels[subject] = AsyncApiChannel{Publish: op}
	}

	return AsyncApiSpec{
		AsyncAPI:           "2.0.0",
		Info:               AsyncApiInfo{Title: a.Title, Version: a.Version},
		DefaultContentType: "application/json",
		Channels:           channels,
		Components:         AsyncApiComponents{Schemas: schemas},
	}
}

// AsyncApiSpecJSON returns the AsyncAPI spec as indented JSON.
func (a *NatsAPI) AsyncApiSpecJSON() ([]byte, error) {
	return json.MarshalIndent(a.AsyncApiSpec(), "", "  ")
}

// registerSchemaHandler registers a schema.retrieve handler on the root path
// that returns the AsyncAPI spec for this app. The spec is computed once at
// startup so requests are just a map lookup + copy.
func (a *NatsAPI) registerSchemaHandler() {
	subject := "schema.retrieve"
	key := a.RootPath + "." + subject
	if _, exists := a.routes[key]; exists {
		return
	}
	// like the Python natsapi (include_schema=False), the schema route itself is not part of the spec
	spec, err := a.AsyncApiSpecJSON()
	if err != nil {
		panic("asyncapi: failed to generate schema: " + err.Error())
	}
	a.asyncApiSpec = spec
	a.routes[key] = Route{
		Subject:     subject,
		Description: "Returns the AsyncAPI schema for this service",
		Handler: func(_ context.Context, _ *NatsAPI, _ json.RawMessage) (json.RawMessage, error) {
			return a.asyncApiSpec, nil
		},
	}
}

// optionalPointers removes pointer fields from required, as a nil pointer is a missing or null value.
// A pointer stays required when it is tagged validate:"required" or jsonschema:"required".
// Field names follow encoding/json and must stay in line with how the reflector names properties.
func optionalPointers(t reflect.Type, s *jsonschema.Schema, defs jsonschema.Definitions) {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		if t.Kind() != reflect.Ptr && s.Items != nil {
			s = s.Items
		}
		t = t.Elem()
	}
	if def, ok := defs[strings.TrimPrefix(s.Ref, "#/$defs/")]; ok {
		s = def
	}
	if t.Kind() != reflect.Struct || s.Properties == nil {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if f.Anonymous && name == "" {
			optionalPointers(f.Type, s, defs)
			continue
		}
		if name == "-" || !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		explicit := slices.Contains(strings.Split(f.Tag.Get("validate"), ","), "required") ||
			slices.Contains(strings.Split(f.Tag.Get("jsonschema"), ","), "required")
		if f.Type.Kind() == reflect.Ptr && !explicit {
			s.Required = slices.DeleteFunc(s.Required, func(r string) bool { return r == name })
		}
		if prop, ok := s.Properties.Get(name); ok {
			optionalPointers(f.Type, prop, defs)
		}
	}
}

// rewriteRefs points the reflector's $defs refs to the components, where the definitions are moved to.
func rewriteRefs(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	s.Ref = strings.Replace(s.Ref, "#/$defs/", refPrefix, 1)
	if s.Properties != nil {
		for p := s.Properties.Oldest(); p != nil; p = p.Next() {
			rewriteRefs(p.Value)
		}
	}
	rewriteRefs(s.Items)
	rewriteRefs(s.AdditionalProperties)
	for _, x := range slices.Concat(s.AllOf, s.AnyOf, s.OneOf) {
		rewriteRefs(x)
	}
	for _, def := range s.Definitions {
		rewriteRefs(def)
	}
}

// operationID turns a subject into a name without dots, as wg-sdk skips operations whose name contains one.
func operationID(subject string) string {
	return strings.ReplaceAll(subject, ".", "_")
}

func newAsyncAPIDocServer(a *NatsAPI) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/asyncapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(a.asyncApiSpec)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(docServerHTML))
	})

	return &http.Server{Addr: a.docServerAddr, Handler: mux}
}

func tagsToAsyncAPI(tags []string) []AsyncApiTag {
	out := make([]AsyncApiTag, len(tags))
	for i, t := range tags {
		out[i] = AsyncApiTag{Name: t}
	}
	return out
}
