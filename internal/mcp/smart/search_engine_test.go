package smart

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func decodeSchemaForTest(t *testing.T, raw string) any {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestFormatDescribeResultPreservesInputSchema(t *testing.T) {
	// questions reproduces evaluate's ID-to-question map. Also cover schema
	// constructs that cannot be represented by a top-level type/description list.
	const raw = `{
		"type": "object",
		"required": ["questions"],
		"additionalProperties": false,
		"properties": {
			"state": {"type": ["string", "object", "array", "null"]},
			"questions": {
				"type": "object",
				"additionalProperties": {
					"type": "object",
					"required": ["type", "instructions"],
					"properties": {
						"type": {"type": "string", "enum": ["noul", "choice", "score"]},
						"instructions": {"description": "A full question or structured definitions."},
						"criteria": {"oneOf": [
							{"type": "object", "additionalProperties": {"type": ["string", "null"]}},
							{"type": "array", "items": {"$ref": "#/$defs/level"}, "minItems": 2}
						]},
						"min_confidence": {"type": "number", "minimum": 0, "maximum": 1}
					}
				}
			},
			"record_id": {"const": 9007199254740993},
			"model": {"type": "string", "default": "jev-latest"}
		},
		"$defs": {"level": {"type": "string"}},
		"examples": [{"state": "Payment failed", "questions": {"q1": {
			"type": "noul", "instructions": "Is there a payment problem?"
		}}}]
	}`
	want := decodeSchemaForTest(t, raw)
	for _, input := range []struct {
		name   string
		schema any
	}{
		{"raw_message", json.RawMessage(raw)},
		{"bytes", []byte(raw)},
		{"string", raw},
		{"map", want},
	} {
		for _, scope := range []string{"tool", "service"} {
			t.Run(input.name+"/"+scope, func(t *testing.T) {
				tool := map[string]interface{}{
					"type": "tool", "service": "systemone_1", "name": "evaluate",
					"description": "Evaluate observed evidence.", "inputSchema": input.schema,
				}
				result := tool
				if scope == "service" {
					result = map[string]interface{}{
						"type": "service", "name": "systemone_1", "display_name": "Decisions",
						"tools_count": 1, "tools": []interface{}{tool},
					}
				}
				results := []map[string]interface{}{result}
				output := FormatDescribeResult(results, true)
				_, block, ok := strings.Cut(output, "```json\n")
				if !ok {
					t.Fatalf("missing full JSON schema in describe output:\n%s", output)
				}
				block, _, ok = strings.Cut(block, "\n```")
				if !ok {
					t.Fatalf("unclosed schema block:\n%s", output)
				}
				if got := decodeSchemaForTest(t, block); !reflect.DeepEqual(got, want) {
					t.Fatalf("schema changed during describe formatting:\n%s", block)
				}

				brief := FormatDescribeResult(results, false)
				if !strings.Contains(brief, "evaluate") || !strings.Contains(brief, "Evaluate observed evidence.") {
					t.Fatalf("brief description lost tool identity: %s", brief)
				}
				if strings.Contains(brief, "questions") || strings.Contains(brief, "```json") {
					t.Fatalf("includeSchema=false leaked schema: %s", brief)
				}
			})
		}
	}
}
