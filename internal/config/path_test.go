package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestParsePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    config.Path
		wantErr bool
	}{
		{
			name:  "service section key",
			input: "api.variables.PORT",
			want:  config.Path{Service: "api", Section: "variables", Key: "PORT"},
		},
		{
			name:  "service section",
			input: "api.variables",
			want:  config.Path{Service: "api", Section: "variables"},
		},
		{
			name:  "service only",
			input: "api",
			want:  config.Path{Service: "api"},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "too many segments",
			input:   "a.b.c.d",
			wantErr: true,
		},
		{
			name:    "leading dot",
			input:   ".api",
			wantErr: true,
		},
		{
			name:    "trailing dot",
			input:   "api.",
			wantErr: true,
		},
		{
			name:    "consecutive dots",
			input:   "api..variables",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "  ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ParsePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePath() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Fatalf("ParsePath() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
