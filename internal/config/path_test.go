package config

import "testing"

func TestParsePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Path
		wantErr bool
	}{
		{
			name:  "service section key",
			input: "api.variables.PORT",
			want:  Path{Service: "api", Section: "variables", Key: "PORT"},
		},
		{
			name:  "service section",
			input: "api.variables",
			want:  Path{Service: "api", Section: "variables"},
		},
		{
			name:  "service only",
			input: "api",
			want:  Path{Service: "api"},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePath(tt.input)
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
