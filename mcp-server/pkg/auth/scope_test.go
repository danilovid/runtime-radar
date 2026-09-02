package auth

import (
	"context"
	"testing"
)

func TestAllowsScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scopes []string
		scope  Scope
		want   bool
	}{
		{
			// Every key issued before scopes existed has none, and a session
			// opened with the product's own JWT never has any.
			name:  "a session without scopes reaches everything",
			scope: ScopeAdmission,
			want:  true,
		},
		{
			name:   "a scoped key reaches its own half",
			scopes: []string{string(ScopeAdmission)},
			scope:  ScopeAdmission,
			want:   true,
		},
		{
			name:   "a scoped key does not reach the other half",
			scopes: []string{string(ScopeAdmission)},
			scope:  ScopeRuntimeMonitor,
			want:   false,
		},
		{
			name:   "a key may carry both",
			scopes: []string{string(ScopeAdmission), string(ScopeRuntimeMonitor)},
			scope:  ScopeRuntimeMonitor,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := WithScopes(context.Background(), tt.scopes)

			if got := AllowsScope(ctx, tt.scope); got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestIsScope(t *testing.T) {
	t.Parallel()

	for _, scope := range Scopes {
		if !IsScope(string(scope)) {
			t.Errorf("expected %s to be a known scope", scope)
		}
	}

	if IsScope("system_settings") {
		t.Error("expected a role permission not to pass as a scope")
	}
}
