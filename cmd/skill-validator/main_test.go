package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSkill(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "valid",
			content: "---\nname: sample-skill\ndescription: A useful sample.\n---\n",
		},
		{
			name:    "missing description",
			content: "---\nname: sample-skill\n---\n",
			wantErr: "description",
		},
		{
			name:    "invalid name",
			content: "---\nname: Sample_Skill\ndescription: A useful sample.\n---\n",
			wantErr: "hyphen-case",
		},
		{
			name:    "unexpected key",
			content: "---\nname: sample-skill\ndescription: A useful sample.\ncommands: all\n---\n",
			wantErr: "unexpected frontmatter key",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			err := validateSkill(root)
			if test.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}
