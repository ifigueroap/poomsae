package identityaudit

import (
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"testing"
)

func TestValidateRejectsForbiddenKeyVariantsInsideFieldValues(t *testing.T) {
	for _, collection := range []string{"fields", "expected"} {
		for _, key := range []string{
			"poomsae_uid", "poomsaeUid", "poomsae-uid", "poomsae.uid",
			"poomsae_ref", "poomsaeRef", "poomsae-ref", "poomsae.ref",
			"poomsae_project_id", "poomsaeProjectId", "poomsae-project-id", "poomsae.project.id",
			"poomsae_binding_id", "poomsaeBindingId", "poomsae-binding-id", "poomsae.binding.id",
			"poomsae_work_branch", "poomsaeWorkBranch", "poomsae-work-branch", "poomsae.work.branch",
		} {
			params := map[string]any{
				"root_key": "root-example",
				"fields":   map[string]any{"poomsae_uid": map[string]any{"kind": "null"}},
				"expected": map[string]any{"poomsae_uid": map[string]any{"kind": "null"}},
			}
			params[collection].(map[string]any)["poomsae_uid"] = map[string]any{
				"kind": "date", "value": "2026-08-20", key: "neutral-forbidden",
			}
			raw, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			err = Validate("write_fields", raw, Options{ExternalRootKey: "root-example"})
			var auditErr *Error
			if !errors.As(err, &auditErr) || auditErr.Code != CodeForbiddenKey {
				t.Fatalf("%s key %q error = %#v, want forbidden-key error", collection, key, err)
			}
		}
	}
}

func TestValidateAllowsOpaqueFieldIDs(t *testing.T) {
	raw := jsontext.Value(`{"root_key":"root-example","fields":{"poomsae_uid":{"kind":"date","value":"2026-08-20"},"poomsaekana_start":{"kind":"null"}},"expected":{"poomsae_uid":{"kind":"null"},"poomsaekana_start":{"kind":"date","value":"2026-08-19"}}}`)
	if err := Validate("write_fields", raw, Options{ExternalRootKey: "root-example"}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsArbitraryStructuralPoomsaeKeys(t *testing.T) {
	for _, key := range []string{"poomsaeOwnerId", "poomsae-owner-ref", "poomsae.project.token", "poomsae"} {
		raw, err := json.Marshal(map[string]any{
			"root_key": "root-example",
			"fields": map[string]any{
				"poomsaekana_start": map[string]any{"kind": "date", "value": "2026-08-20", key: "neutral-forbidden"},
			},
			"expected": map[string]any{"poomsaekana_start": map[string]any{"kind": "null"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		err = Validate("write_fields", raw, Options{ExternalRootKey: "root-example"})
		var auditErr *Error
		if !errors.As(err, &auditErr) || auditErr.Code != CodeForbiddenKey {
			t.Fatalf("key %q error = %#v, want forbidden-key error", key, err)
		}
	}

	raw := jsontext.Value(`{"root_key":"root-example","fields":{"poomsaekana_start":{"kind":"null"}},"expected":{"poomsaekana_start":{"kind":"date","value":"2026-08-20"}}}`)
	if err := Validate("write_fields", raw, Options{ExternalRootKey: "root-example"}); err != nil {
		t.Fatalf("poomsaekana field rejected: %v", err)
	}
}

func TestValidateRejectsWrongParameterTypes(t *testing.T) {
	tests := []struct {
		method string
		raw    jsontext.Value
	}{
		{method: "resolve_root", raw: jsontext.Value(`{"locator":7}`)},
		{method: "read_root", raw: jsontext.Value(`{"root_key":false}`)},
		{method: "list_comments", raw: jsontext.Value(`{"root_key":[]}`)},
		{method: "publish_comment", raw: jsontext.Value(`{"root_key":"root-example","body":{},"operation_id":"operation-example"}`)},
		{method: "publish_comment", raw: jsontext.Value(`{"root_key":3,"body":"example","operation_id":"operation-example"}`)},
		{method: "publish_comment", raw: jsontext.Value(`{"root_key":"root-example","body":"example","operation_id":3}`)},
		{method: "complete_root", raw: jsontext.Value(`{"root_key":null}`)},
		{method: "read_fields", raw: jsontext.Value(`{"root_key":{},"field_ids":[]}`)},
		{method: "read_fields", raw: jsontext.Value(`{"root_key":"root-example","field_ids":"field-example"}`)},
		{method: "read_fields", raw: jsontext.Value(`{"root_key":"root-example","field_ids":[3]}`)},
		{method: "write_fields", raw: jsontext.Value(`{"root_key":true,"fields":{},"expected":{}}`)},
		{method: "write_fields", raw: jsontext.Value(`{"root_key":"root-example","fields":"field-example","expected":{}}`)},
		{method: "write_fields", raw: jsontext.Value(`{"root_key":"root-example","fields":{"field-example":null},"expected":{}}`)},
		{method: "write_fields", raw: jsontext.Value(`{"root_key":"root-example","fields":{"field-example":{}},"expected":{}}`)},
		{method: "write_fields", raw: jsontext.Value(`{"root_key":"root-example","fields":{"field-example":{"kind":4}},"expected":{}}`)},
		{method: "write_fields", raw: jsontext.Value(`{"root_key":"root-example","fields":{"field-example":{"kind":"date","value":{}}},"expected":{}}`)},
		{method: "write_fields", raw: jsontext.Value(`{"root_key":"root-example","fields":{"field-example":{"kind":"instant","timezone":[]}},"expected":{}}`)},
		{method: "write_fields", raw: jsontext.Value(`{"root_key":"root-example","fields":{"field-example":{"kind":"null","metadata":"unexpected"}},"expected":{}}`)},
	}
	for _, test := range tests {
		err := Validate(test.method, test.raw, Options{ExternalRootKey: "root-example"})
		var auditErr *Error
		if !errors.As(err, &auditErr) || auditErr.Code != CodeInvalidJSON {
			t.Errorf("%s params %s error = %#v, want invalid-json error", test.method, test.raw, err)
		}
	}
}

func TestValidateTreatsReadFieldSelectorsAsOpaque(t *testing.T) {
	options := Options{
		ExternalRootKey: "root-example",
		LongUIDs:        []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		ShortIDs:        []string{"root-short"},
	}
	raw := jsontext.Value(`{"root_key":"root-example","field_ids":["root-short","prefix-01ARZ3NDEKTSV4RRFFQ69G5FAV-suffix","poomsaeOwnerId"]}`)
	if err := Validate("read_fields", raw, options); err != nil {
		t.Fatalf("opaque field selectors rejected: %v", err)
	}

	for _, invalid := range []jsontext.Value{
		jsontext.Value(`{"root_key":"root-short","field_ids":[]}`),
		jsontext.Value(`{"root_key":"root-example","field_ids":[],"poomsaeOwnerId":"neutral-forbidden"}`),
	} {
		if err := Validate("read_fields", invalid, options); err == nil {
			t.Fatalf("surrounding identity channel accepted: %s", invalid)
		}
	}
}

func TestValidateLocalIdentityMatching(t *testing.T) {
	options := Options{
		ExternalRootKey: "root-example",
		LongUIDs:        []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV", "01ARZ3NDEKTSV4RRFFQ69G5FAW"},
		ShortIDs:        []string{"root-short", "child-short"},
	}
	for _, body := range []string{
		"prefix-01ARZ3NDEKTSV4RRFFQ69G5FAV-suffix",
		"prefix-01ARZ3NDEKTSV4RRFFQ69G5FAW-suffix",
		"root-short",
		"child-short",
	} {
		raw, _ := json.Marshal(map[string]any{"root_key": "root-example", "body": body, "operation_id": "operation-example"})
		if err := Validate("publish_comment", raw, options); err == nil {
			t.Fatalf("local identity body %q was accepted", body)
		}
	}
	for _, body := range []string{"prefix-root-short-suffix", "unrelated-01ARZ3NDEKTSV4RRFFQ69G5FAX-suffix"} {
		raw, _ := json.Marshal(map[string]any{"root_key": "root-example", "body": body, "operation_id": "operation-example"})
		if err := Validate("publish_comment", raw, options); err != nil {
			t.Fatalf("safe body %q rejected: %v", body, err)
		}
	}
}

func TestValidateAllMethodShapesAndExactEOF(t *testing.T) {
	valid := map[string]jsontext.Value{
		"describe":        jsontext.Value(`{}`),
		"resolve_root":    jsontext.Value(`{"locator":"fixture-root"}`),
		"read_root":       jsontext.Value(`{"root_key":"root-example"}`),
		"list_comments":   jsontext.Value(`{"root_key":"root-example"}`),
		"publish_comment": jsontext.Value(`{"root_key":"root-example","body":"Example","operation_id":"operation-example"}`),
		"complete_root":   jsontext.Value(`{"root_key":"root-example"}`),
		"list_fields":     jsontext.Value(`{}`),
		"read_fields":     jsontext.Value(`{"root_key":"root-example","field_ids":["field-example"]}`),
		"write_fields":    jsontext.Value(`{"root_key":"root-example","fields":{"field-example":{"kind":"null"}},"expected":{"field-example":{"kind":"date","value":"2026-08-20"}}}`),
	}
	for method, raw := range valid {
		if err := Validate(method, raw, Options{ExternalRootKey: "root-example"}); err != nil {
			t.Fatalf("%s valid params rejected: %v", method, err)
		}
	}
	err := Validate("read_root", jsontext.Value(`{"root_key":"root-example"} {}`), Options{ExternalRootKey: "root-example"})
	var auditErr *Error
	if !errors.As(err, &auditErr) || auditErr.Code != CodeTrailingJSON {
		t.Fatalf("trailing JSON error = %#v, want trailing-json error", err)
	}
	err = Validate("describe", jsontext.Value(`null`), Options{})
	if !errors.As(err, &auditErr) || auditErr.Code != CodeInvalidJSON {
		t.Fatalf("null parameter error = %#v, want invalid-json error", err)
	}
}
