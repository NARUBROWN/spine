package endpoint

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	validation "github.com/NARUBROWN/spine/contract/validator"
)

func (c *catalog) conditionalSchema(t reflect.Type, mode string, fields []wireField, rules []validation.ConditionalRequired) []any {
	if len(rules) == 0 {
		return nil
	}
	names := make(map[string]string, len(fields))
	switch mode {
	case "request/json", "request/form", "request/multipart", "request/xml":
		for _, field := range fields {
			if !field.quoted && !customCodecType(field.typeOf) {
				names[fmt.Sprint(field.index)] = field.name
			}
		}
	}
	var branches []any
	for _, rule := range rules {
		branch := conditionalBranch(names, rule)
		if branch == nil {
			c.diagnostic("VALIDATION_CONDITION_UNRESOLVED", t.String(), "A conditional requirement could not be mapped to fields in "+mode+".")
			continue
		}
		branches = append(branches, branch)
	}
	return branches
}

func conditionalBranch(names map[string]string, rule validation.ConditionalRequired) map[string]any {
	target := names[fmt.Sprint(rule.Index)]
	if target == "" || len(rule.When) == 0 {
		return nil
	}
	properties := make(map[string]any, len(rule.When))
	required := make([]string, 0, len(rule.When))
	for _, condition := range rule.When {
		name := names[fmt.Sprint(condition.Index)]
		value := strings.TrimSpace(string(condition.Value))
		if name == "" || properties[name] != nil || !json.Valid(condition.Value) || value[0] == '[' || value[0] == '{' {
			return nil
		}
		required = append(required, name)
		properties[name] = map[string]any{"const": condition.Value}
	}
	then := map[string]any{"required": []string{target}}
	if len(rule.Schema) > 0 {
		var schema map[string]json.RawMessage
		if err := json.Unmarshal(rule.Schema, &schema); err != nil || schema == nil {
			return nil
		}
		then["properties"] = map[string]any{target: rule.Schema}
	}
	return map[string]any{
		"if":                           map[string]any{"required": required, "properties": properties},
		"then":                         then,
		"x-spine-conditional-required": true,
	}
}
