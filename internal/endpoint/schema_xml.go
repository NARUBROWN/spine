package endpoint

import (
	"encoding/xml"
	"reflect"
	"strings"
)

func (c *catalog) xmlFields(root reflect.Type) ([]wireField, map[string]any) {
	var fields []wireField
	var element map[string]any
	for _, field := range reflect.VisibleFields(root) {
		if !field.IsExported() || field.Tag.Get("xml") == "-" {
			continue
		}
		name, options, _ := strings.Cut(field.Tag.Get("xml"), ",")
		namespace, local, namespaced := strings.Cut(name, " ")
		if namespaced {
			name = local
		}
		if field.Name == "XMLName" && field.Type == reflect.TypeFor[xml.Name]() {
			if name != "" {
				element = map[string]any{"name": name}
				if namespaced {
					element["namespace"] = namespace
				}
			}
			continue
		}
		attribute := false
		unsupported := namespaced || field.Anonymous || len(field.Index) > 1 || strings.Contains(name, ">")
		for _, option := range strings.Split(options, ",") {
			switch option {
			case "attr":
				attribute = true
			case "", "omitempty":
			default:
				unsupported = true
			}
		}
		if unsupported {
			c.diagnostic("XML_FIELD_UNRESOLVED", root.String()+"."+field.Name, "The XML field path, embedding, namespace or decoder option requires an explicit schema.")
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields = append(fields, wireField{name: name, index: field.Index, typeOf: field.Type, xmlAttribute: attribute})
	}
	return fields, element
}
