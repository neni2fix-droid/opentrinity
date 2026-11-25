package routingconfig

import (
	"strings"
	"testing"

	"github.com/nenifix/opentrinity/internal/database"
)

func testSnapshot() Snapshot {
	return Snapshot{
		Providers:   []database.Provider{{ID: 1, Name: "fake", APIBase: "http://127.0.0.1:10001/v1", Enabled: true}},
		Credentials: []database.Credential{{ID: 1, ProviderID: 1, SecretRef: "OT_TEST_KEY", Enabled: true}},
		Models:      []database.Model{{ID: 1, ProviderID: 1, ModelID: "fake-model", Enabled: true}},
		Deployments: []database.Deployment{{Slot: "A", ModelID: 1, Enabled: false}, {Slot: "B", ModelID: 1, Enabled: false}, {Slot: "C", ModelID: 1, Enabled: false}},
	}
}

func TestGenerateContainsOnlyEnvironmentReferences(t *testing.T) {
	s := testSnapshot()
	s.Deployments[0].Enabled = true
	s.Deployments[1].Enabled = true
	out, err := Generate(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"model_name: nenifix-coder",
		"os.environ/OT_TEST_KEY",
		"num_retries: 0",
		"    - nenifix-coder: [nenifix-coder-b]",
		"master_key: os.environ/OT_LITELLM_KEY",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("generated config missing %q", want)
		}
	}
	if strings.Contains(out, "super-secret-value") {
		t.Fatal("generated config contains a secret value")
	}
}

func TestGenerateRejectsNonContiguousSlots(t *testing.T) {
	s := testSnapshot()
	s.Deployments[1].Enabled = true
	if _, err := Generate(s); err == nil {
		t.Fatal("expected non-contiguous slot error")
	}
}

func TestValidateAPIBase(t *testing.T) {
	cases := map[string]bool{
		"http://127.0.0.1:10001/v1": true,
		"https://api.example.com/v1": true,
		"http://api.example.com/v1": false,
		"https://user:pass@example.com/v1": false,
		"https://example.com/v1?x=1": false,
		"ftp://example.com": false,
	}
	for raw, wantOK := range cases {
		err := ValidateAPIBase(raw)
		if (err == nil) != wantOK {
			t.Errorf("ValidateAPIBase(%q) err=%v wantOK=%t", raw, err, wantOK)
		}
	}
}
