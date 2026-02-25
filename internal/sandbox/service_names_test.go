package sandbox

import (
	"testing"
)

func TestResolveProcessName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "db maps to postgres", input: "db", want: "postgres"},
		{name: "auth maps to gotrue", input: "auth", want: "gotrue"},
		{name: "rest maps to postgrest", input: "rest", want: "postgrest"},
		{name: "api maps to proxy", input: "api", want: "proxy"},
		{name: "unknown service", input: "storage", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "postgres is not user-facing", input: "postgres", wantErr: true},
		{name: "gotrue is not user-facing", input: "gotrue", wantErr: true},
		{name: "one-shot postgres-init", input: "postgres-init", wantErr: true},
		{name: "one-shot gotrue-migrate", input: "gotrue-migrate", wantErr: true},
		{name: "case sensitive", input: "DB", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveProcessName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveProcessName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ResolveProcessName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidServiceNames(t *testing.T) {
	names := ValidServiceNames()

	// Should contain exactly 4 services
	if len(names) != 4 {
		t.Fatalf("ValidServiceNames() returned %d names, want 4", len(names))
	}

	// Should be sorted
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("ValidServiceNames() not sorted: %v", names)
			break
		}
	}

	// Should contain expected names
	expected := map[string]bool{"api": true, "auth": true, "db": true, "rest": true}
	for _, name := range names {
		if !expected[name] {
			t.Errorf("ValidServiceNames() contains unexpected name %q", name)
		}
	}
}
