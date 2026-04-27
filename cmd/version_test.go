package cmd

import "testing"

func TestFormatVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		sha     string
		want    string
	}{
		{
			name:    "dev version with unknown commit",
			version: "dev",
			sha:     "unknown",
			want:    "dandori-cli dev\n  commit:     unknown\n",
		},
		{
			name:    "release version with short sha",
			version: "v1.2.3",
			sha:     "abc1234",
			want:    "dandori-cli v1.2.3\n  commit:     abc1234\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatVersion(tt.version, tt.sha)
			if got != tt.want {
				t.Errorf("formatVersion(%q, %q) = %q, want %q", tt.version, tt.sha, got, tt.want)
			}
		})
	}
}
