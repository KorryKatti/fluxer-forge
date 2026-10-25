// Package schema emits a machine-readable description of config.toml.
//
// The Go structs in internal/config are the only definition: this package
// walks them and writes JSON. Nothing here may be edited by hand, and
// schema/schema.json is regenerated with `bot --print-schema` in CI.
package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
)

// Document is the top level of schema.json.
type Document struct {
	SchemaVersion int              `json:"schema_version"`
	Title         string           `json:"title"`
	Description   string           `json:"description"`
	Type          string           `json:"type"`
	MaxDepth      int              `json:"max_depth"`
	Required      []string         `json:"required,omitempty"`
	Properties    map[string]*Node `json:"properties"`
}

// Node is one property in the schema. It deliberately mirrors the subset of
// JSON Schema the site and the future renderer need, plus nothing else.
type Node struct {
	Type       string           `json:"type"`
	Format     string           `json:"format,omitempty"`
	Minimum    *float64         `json:"minimum,omitempty"`
	MaxLength  *int             `json:"maxLength,omitempty"`
	MinItems   *int             `json:"minItems,omitempty"`
	Const      any              `json:"const,omitempty"`
	Items      *Node            `json:"items,omitempty"`
	Required   []string         `json:"required,omitempty"`
	Properties map[string]*Node `json:"properties,omitempty"`
}

// Marshal renders the schema as indented JSON with a trailing newline, ready
// to be written to schema/schema.json.
func Marshal() ([]byte, error) {
	doc, err := Generate()
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Generate builds the document from the config structs.
func Generate() (*Document, error) {
	root, err := nodeFor(reflect.TypeOf(config.Config{}))
	if err != nil {
		return nil, err
	}
	return &Document{
		SchemaVersion: config.SupportedSchemaVersion,
		Title:         "Fluxer Forge configuration",
		Description:   "Generated from runtime/internal/config. Do not edit by hand.",
		Type:          "object",
		MaxDepth:      measure(root, 0),
		Required:      root.Required,
		Properties:    root.Properties,
	}, nil
}

// nodeFor turns one Go type into a schema node. The `schema` struct tag adds
// the constraints the Go type alone cannot express:
//
//	required            the field must be present
//	const=1             a single allowed value
//	format=uri          a string is a URI/snowflake
//	snowflake           shorthand for format=snowflake (on a list: item format)
//	min=0               numeric or item minimum (also minimum for arrays)
//	maxLength=4         string length
//	minItems=1          array length
func nodeFor(t reflect.Type) (*Node, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return &Node{Type: "string"}, nil
	case reflect.Bool:
		return &Node{Type: "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Node{Type: "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return &Node{Type: "number"}, nil
	case reflect.Slice, reflect.Array:
		items, err := nodeFor(t.Elem())
		if err != nil {
			return nil, err
		}
		return &Node{Type: "array", Items: items}, nil
	case reflect.Struct:
		return objectNode(t)
	default:
		return nil, fmt.Errorf("schema: unsupported config type %s", t)
	}
}

func objectNode(t reflect.Type) (*Node, error) {
	node := &Node{Type: "object", Properties: map[string]*Node{}}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := strings.Split(field.Tag.Get("toml"), ",")[0]
		if name == "" || name == "-" || !field.IsExported() {
			continue
		}
		meta := parseTag(field.Tag.Get("schema"))

		child, err := nodeFor(field.Type)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", t.Name(), field.Name, err)
		}
		applyMeta(child, meta)

		node.Properties[name] = child
		if meta.required {
			node.Required = append(node.Required, name)
		}
	}
	return node, nil
}

type metadata struct {
	required  bool
	format    string
	minimum   *float64
	maxLength *int
	minItems  *int
	constVal  any
}

func parseTag(tag string) metadata {
	var meta metadata
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case part == "required":
			meta.required = true
		case part == "snowflake":
			meta.format = "snowflake"
		case strings.HasPrefix(part, "format="):
			meta.format = strings.TrimPrefix(part, "format=")
		case strings.HasPrefix(part, "const="):
			raw := strings.TrimPrefix(part, "const=")
			if n, err := strconv.Atoi(raw); err == nil {
				meta.constVal = n
			} else {
				meta.constVal = raw
			}
		case strings.HasPrefix(part, "min="):
			raw := strings.TrimPrefix(part, "min=")
			if n, err := strconv.ParseFloat(raw, 64); err == nil {
				meta.minimum = &n
			}
		case strings.HasPrefix(part, "maxLength="):
			raw := strings.TrimPrefix(part, "maxLength=")
			if n, err := strconv.Atoi(raw); err == nil {
				meta.maxLength = &n
			}
		case strings.HasPrefix(part, "minItems="):
			raw := strings.TrimPrefix(part, "minItems=")
			if n, err := strconv.Atoi(raw); err == nil {
				meta.minItems = &n
			}
		}
	}
	return meta
}

// applyMeta puts constraints in the place JSON Schema expects them: array
// item formats live on items, everything else on the node itself.
func applyMeta(node *Node, meta metadata) {
	if meta.constVal != nil {
		node.Const = meta.constVal
	}
	if meta.minimum != nil {
		node.Minimum = meta.minimum
	}
	if meta.maxLength != nil {
		node.MaxLength = meta.maxLength
	}
	if meta.minItems != nil {
		node.MinItems = meta.minItems
	}
	if meta.format != "" {
		if node.Type == "array" && node.Items != nil {
			node.Items.Format = meta.format
		} else {
			node.Format = meta.format
		}
	}
}

// measure reports how deeply containers nest. Scalar fields live at their
// containing object's depth, so the deepest module section plus its fields is
// the number checked against the depth rule.
func measure(node *Node, depth int) int {
	max := depth
	for _, child := range node.Properties {
		switch {
		case child.Type == "object":
			max = maxInt(max, measure(child, depth+1))
		case child.Type == "array" && child.Items != nil && child.Items.Type == "object":
			max = maxInt(max, measure(child.Items, depth+1))
		default:
			max = maxInt(max, depth)
		}
	}
	return max
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
