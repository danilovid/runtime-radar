package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactEventJSON(t *testing.T) {
	t.Parallel()

	const (
		base64Blob = "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNkZWZnaGlqa2xtbm9wcXJzdHV2d3h5ejAxMjM0NTY3ODk="
		pemBlock   = "-----BEGIN RSA PRIVATE KEY-----\\nMIIBOgIBAAJBAKj34GkxFhD90vcNLYLI\\n-----END RSA PRIVATE KEY-----"
	)

	cases := []struct {
		name      string
		eventJSON string
		leaked    []string // must not survive redaction
		kept      []string // must survive, so that the event stays explicable
	}{
		{
			name:      "bearer token in an authorization header",
			eventJSON: `{"arguments":"curl -H \"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.body.sig\" https://api.test"}`,
			leaked:    []string{"eyJhbGciOiJIUzI1NiJ9"},
			kept:      []string{"curl", "Authorization", "https://api.test"},
		},
		{
			name:      "bare bearer scheme",
			eventJSON: `{"arguments":"http --auth bearer s3cr3ttokenvalue123"}`,
			leaked:    []string{"s3cr3ttokenvalue123"},
			kept:      []string{"http"},
		},
		{
			name:      "basic scheme",
			eventJSON: `{"arguments":"fetch --header Basic dXNlcjpwYXNzd29yZA=="}`,
			leaked:    []string{"dXNlcjpwYXNzd29yZA"},
			kept:      []string{"fetch"},
		},
		{
			name:      "password argument",
			eventJSON: `{"arguments":"mysql --password=hunter2 -u root"}`,
			leaked:    []string{"hunter2"},
			kept:      []string{"mysql", "password", "-u root"},
		},
		{
			name:      "passwd and pwd arguments",
			eventJSON: `{"arguments":"tool --passwd topsecret1 --pwd:topsecret2"}`,
			leaked:    []string{"topsecret1", "topsecret2"},
			kept:      []string{"tool"},
		},
		{
			name:      "token and api_key arguments",
			eventJSON: `{"arguments":"app --token=abc.def.ghi --api_key='AKIAIOSFODNN7EXAMPLE'"}`,
			leaked:    []string{"abc.def.ghi", "AKIAIOSFODNN7EXAMPLE"},
			kept:      []string{"app", "token", "api_key"},
		},
		{
			name:      "api-key spelled with a dash",
			eventJSON: `{"arguments":"app --api-key myrealkeyvalue"}`,
			leaked:    []string{"myrealkeyvalue"},
		},
		{
			name:      "pem block",
			eventJSON: `{"arguments":"echo \"` + pemBlock + `\" > id_rsa"}`,
			leaked:    []string{"MIIBOgIBAAJBAKj34GkxFhD90vcNLYLI", "BEGIN RSA PRIVATE KEY"},
			kept:      []string{"id_rsa"},
		},
		{
			name:      "long base64 blob",
			eventJSON: `{"arguments":"echo ` + base64Blob + ` | base64 -d"}`,
			leaked:    []string{base64Blob[:40]},
			kept:      []string{"base64 -d"},
		},
		{
			name:      "value of a secret-named key",
			eventJSON: `{"env":{"DB_PASSWORD":"plain-but-secret","PATH":"/usr/bin"}}`,
			leaked:    []string{"plain-but-secret"},
			kept:      []string{"/usr/bin", "DB_PASSWORD"},
		},
		{
			name:      "secrets nested in arrays",
			eventJSON: `{"process":{"args":["--password=hunter2","--verbose"]}}`,
			leaked:    []string{"hunter2"},
			kept:      []string{"--verbose"},
		},
		{
			name: "invalid json is still redacted",
			// A truncated dump can carry exactly the same credentials, so it
			// must not be passed through untouched.
			eventJSON: `{"arguments":"psql --password=hunter2`,
			leaked:    []string{"hunter2"},
			kept:      []string{"psql"},
		},
		{
			name:      "identifiers under the base64 bound are kept",
			eventJSON: `{"process":{"exec_id":"a3Jvbm9zLWRlc2t0b3A6MTM1MTgyMDAwMDAwMDoyMjgwOA==","pid":42}}`,
			kept:      []string{"a3Jvbm9zLWRlc2t0b3A6MTM1MTgyMDAwMDAwMDoyMjgwOA=="},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := RedactEventJSON(tc.eventJSON)

			for _, secret := range tc.leaked {
				if strings.Contains(got, secret) {
					t.Fatalf("secret %q survived redaction: %s", secret, got)
				}
			}
			for _, keep := range tc.kept {
				if !strings.Contains(got, keep) {
					t.Fatalf("expected %q to survive redaction, got: %s", keep, got)
				}
			}
			if len(tc.leaked) > 0 && !strings.Contains(got, redactedMarker) {
				t.Fatalf("nothing was marked as redacted: %s", got)
			}
		})
	}
}

func TestRedactEventJSONKeepsDocumentShape(t *testing.T) {
	t.Parallel()

	const eventJSON = `{"id":"e-1","process":{"pid":42,"privileged":true,"parent":null,"args":["--token=abc.def.ghi"]}}`

	var got map[string]any
	if err := json.Unmarshal([]byte(RedactEventJSON(eventJSON)), &got); err != nil {
		t.Fatalf("redacted event is not valid json: %v", err)
	}

	process, ok := got["process"].(map[string]any)
	if !ok {
		t.Fatalf("process object is missing: %#v", got)
	}
	// Non-string values can't carry a credential, and dropping or rewriting
	// them would cost the model the context it explains the event from.
	if process["pid"] != float64(42) || process["privileged"] != true {
		t.Fatalf("scalar fields were altered: %#v", process)
	}
	if _, hasParent := process["parent"]; !hasParent {
		t.Fatal("null field was dropped")
	}

	args, ok := process["args"].([]any)
	if !ok || len(args) != 1 {
		t.Fatalf("unexpected args: %#v", process["args"])
	}
	if args[0] != "--token="+redactedMarker {
		t.Fatalf("unexpected redacted argument: %#v", args[0])
	}
}
