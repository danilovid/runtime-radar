package assistant

import (
	"crypto/tls"
	"testing"
)

func TestNormalizeMCPEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		endpoint   string
		tlsEnabled bool
		want       string
	}{
		{
			name:       "upgrades http when TLS is on",
			endpoint:   "http://mcp-server:9000/mcp",
			tlsEnabled: true,
			want:       "https://mcp-server:9000/mcp",
		},
		{
			name:       "keeps https when TLS is on",
			endpoint:   "https://mcp-server:9000/mcp",
			tlsEnabled: true,
			want:       "https://mcp-server:9000/mcp",
		},
		{
			name:       "keeps http when TLS is off",
			endpoint:   "http://mcp-server:9000/mcp",
			tlsEnabled: false,
			want:       "http://mcp-server:9000/mcp",
		},
		{
			name:       "downgrades https when TLS is off",
			endpoint:   "https://mcp-server:9000/mcp",
			tlsEnabled: false,
			want:       "http://mcp-server:9000/mcp",
		},
		{
			name:       "leaves opaque values alone",
			endpoint:   "not a url",
			tlsEnabled: true,
			want:       "not a url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NormalizeMCPEndpoint(tt.endpoint, tt.tlsEnabled)
			if got != tt.want {
				t.Fatalf("NormalizeMCPEndpoint(%q, %v) = %q, want %q", tt.endpoint, tt.tlsEnabled, got, tt.want)
			}
		})
	}
}

func TestNewMCPToolBoxFactoryRewritesHTTPWhenTLSConfigIsSet(t *testing.T) {
	t.Parallel()

	factory := NewMCPToolBoxFactory("http://mcp-server:9000/mcp", &tls.Config{})
	if factory.endpoint != "https://mcp-server:9000/mcp" {
		t.Fatalf("endpoint = %q, want https://mcp-server:9000/mcp", factory.endpoint)
	}
}
