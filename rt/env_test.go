package rt

import (
	"os"
	"testing"
)

func TestEnv(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envValue *string // nil = unset, empty string = set but empty, non-nil = set with value
		def      []Value
		want     Value
	}{
		{
			name:     "returns set value",
			key:      "TEST_KEY",
			envValue: strPtr("production"),
			want:     "production",
		},
		{
			name:     "returns empty string when set but empty",
			key:      "TEST_KEY",
			envValue: strPtr(""),
			want:     "",
		},
		{
			name:     "returns default when unset",
			key:      "TEST_KEY",
			envValue: nil,
			def:      []Value{"fallback"},
			want:     "fallback",
		},
		{
			name:     "returns nil when unset with no default",
			key:      "TEST_KEY",
			envValue: nil,
			want:     nil,
		},
		{
			name:     "returns default even when empty string is set",
			key:      "TEST_KEY",
			envValue: strPtr(""),
			def:      []Value{"fallback"},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.envValue != nil {
				os.Setenv(tt.key, *tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			// Act
			got := Env(tt.key, tt.def...)

			// Assert
			if got != tt.want {
				t.Errorf("Env() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnvRequire(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		envValue  *string // nil = unset, empty string = set but empty, non-nil = set with value
		wantPanic bool
		wantValue string
	}{
		{
			name:      "returns non-empty value",
			key:       "REQUIRED_KEY",
			envValue:  strPtr("secret123"),
			wantPanic: false,
			wantValue: "secret123",
		},
		{
			name:      "panics when unset",
			key:       "REQUIRED_KEY",
			envValue:  nil,
			wantPanic: true,
		},
		{
			name:      "panics when empty string",
			key:       "REQUIRED_KEY",
			envValue:  strPtr(""),
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.envValue != nil {
				os.Setenv(tt.key, *tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			// Act & Assert
			if tt.wantPanic {
				defer func() {
					r := recover()
					if r == nil {
						t.Fatal("EnvRequire() did not panic")
					}
					e, ok := r.(*Error)
					if !ok {
						t.Fatalf("EnvRequire() panic was not an *Error: %T", r)
					}
					if e.Kind != KindUndefined {
						t.Errorf("EnvRequire() panic Kind = %v, want %v", e.Kind, KindUndefined)
					}
					if e.Message == "" {
						t.Error("EnvRequire() panic Message is empty")
					}
				}()
				EnvRequire(tt.key)
			} else {
				got := EnvRequire(tt.key)
				if got != tt.wantValue {
					t.Errorf("EnvRequire() = %v, want %v", got, tt.wantValue)
				}
			}
		})
	}
}

func strPtr(s string) *string { return &s }
