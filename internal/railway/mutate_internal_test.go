package railway

import (
	"context"
	"testing"
)

func TestParseBuilder(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"nixpacks", "NIXPACKS", false},
		{"NIXPACKS", "NIXPACKS", false},
		{"Railpack", "RAILPACK", false},
		{"PAKETO", "PAKETO", false},
		{"heroku", "HEROKU", false},
		{"UNKNOWN", "", true},
	}
	for _, tt := range tests {
		b, err := parseBuilder(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseBuilder(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && string(b) != tt.want {
			t.Errorf("parseBuilder(%q) = %q, want %q", tt.input, b, tt.want)
		}
	}
}

func TestParseRestartPolicy(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"ALWAYS", "ALWAYS", false},
		{"never", "NEVER", false},
		{"ON_FAILURE", "ON_FAILURE", false},
		{"on_failure", "ON_FAILURE", false},
		{"INVALID", "", true},
	}
	for _, tt := range tests {
		rp, err := parseRestartPolicy(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseRestartPolicy(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && string(rp) != tt.want {
			t.Errorf("parseRestartPolicy(%q) = %q, want %q", tt.input, rp, tt.want)
		}
	}
}

func TestUpdateServiceSettings_NilDeploy(t *testing.T) {
	// Should be a no-op, no API call made.
	err := UpdateServiceSettings(context.Background(), nil, "svc-1", nil)
	if err != nil {
		t.Fatalf("UpdateServiceSettings(nil) error: %v", err)
	}
}
