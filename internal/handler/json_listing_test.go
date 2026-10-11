package handler

import (
	"bytes"
	"encoding/json"
	"testing"
)

const emptyListing = "{\n  \"data\": []\n}\n"

func TestEncodeListing_EmptyAndNilAreAnEmptyArray(t *testing.T) {
	for name, entries := range map[string][]string{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			doc, err := encodeListing(entries)
			if err != nil {
				t.Fatalf("encodeListing: %v", err)
			}
			if string(doc) != emptyListing {
				t.Errorf("encodeListing = %q, want %q", doc, emptyListing)
			}
		})
	}
}

func TestEncodeListing_EscapesOnlyWhatJSONRequires(t *testing.T) {
	value := `Media & Co <ops> "quoted" back\slash é`

	doc, err := encodeListing([]string{value})
	if err != nil {
		t.Fatalf("encodeListing: %v", err)
	}

	var decoded jsonListing[string]
	if err := json.Unmarshal(doc, &decoded); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, doc)
	}
	if len(decoded.Data) != 1 || decoded.Data[0] != value {
		t.Errorf("round trip = %q, want [%q]", decoded.Data, value)
	}
	if !bytes.Contains(doc, []byte("Media & Co <ops>")) {
		t.Errorf("expected &, < and > written literally:\n%s", doc)
	}
}
