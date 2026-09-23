package contributionhttp

import "testing"

func TestChangeStrictShapes(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"tag_ids":[]}`, `{"source":{"url":"https://example.invalid","rights_status":"confirmed"}}`,
		`{"source":{"url":"https://example.invalid","availability_state":"active"}}`,
		`{"translation":{"locale":"ja","name":null}}`, `{"translation":{"locale":"ja","exists":true}}`,
		`{"source_id":"bad"}`, `{"relation":{"other_resource_id":"bad","relation_type":"part_of","direction":"outgoing"}}`,
		`{"source":{"url":"https://example.invalid"},"translation":{"locale":"ja"}}`,
	} {
		if _, err := ParseChange([]byte(raw), false); err == nil {
			t.Fatal("malformed/forbidden shape accepted")
		}
	}
	c, err := ParseChange([]byte(`{"translation":{"locale":"ja","summary":null}}`), false)
	if err != nil || !c.Translation.Summary.Set || c.Translation.Summary.Value != nil || c.Translation.Description.Set {
		t.Fatal("null/omission erased")
	}
	if _, err = ParseChange([]byte(`{"source":{"url":"https://example.invalid"}}`), true); err == nil {
		t.Fatal("review did not require explicit availability")
	}
}
