package ui

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestPresentationCompactsOnlyProvenRequiredScalars(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed to execute the presentation script")
	}
	script := `const spec = {paths:{"/users":{get:{parameters:[
 {in:"query",required:true,schema:{description:"ID",example:1,allOf:[{anyOf:[{type:"integer",format:"int64",minimum:-100,maximum:100,"x-spine-type-range":true},{type:"null"}]},{allOf:[{not:{type:"null"}},{exclusiveMinimum:0}]}]}},
 {in:"query",required:false,schema:{anyOf:[{type:"integer"},{type:"null"}]}},
 {in:"query",required:true,schema:{allOf:[{anyOf:[{type:"integer"},{type:"null"}]},{not:{type:"null"}},{"unevaluatedProperties":false}]}}
 ]}}}};` + presentationScript + `process.stdout.write(JSON.stringify({presentation, spec}));`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("script: %v: %s", err, output)
	}
	var result struct {
		Presentation struct {
			Paths map[string]struct {
				Get struct {
					Parameters []struct {
						Schema      map[string]any
						Description string
					}
				}
			}
		}
		Spec struct {
			Paths map[string]struct {
				Get struct {
					Parameters []struct{ Schema map[string]any }
				}
			}
		}
	}
	if err = json.Unmarshal(output, &result); err != nil {
		t.Fatal(err)
	}
	p := result.Presentation.Paths["/users"].Get.Parameters
	if p[0].Schema["type"] != "integer" || p[0].Schema["format"] != "int64" || p[0].Schema["exclusiveMinimum"] != float64(0) || !strings.Contains(p[0].Description, "Constraints: > 0") {
		t.Fatalf("compacted=%#v", p[0])
	}
	if p[0].Schema["minimum"] != nil || p[0].Schema["maximum"] != nil {
		t.Fatal("type range leaked")
	}
	if p[1].Schema["anyOf"] == nil || p[2].Schema["allOf"] == nil {
		t.Fatal("ambiguous schema was changed")
	}
	original := result.Spec.Paths["/users"].Get.Parameters[0].Schema
	if original["allOf"] == nil {
		t.Fatal("original document was changed")
	}
}
