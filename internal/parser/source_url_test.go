package parser

import (
	"testing"
)

func TestExtractRefName(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"push main", "refs/heads/main", "main"},
		{"push feature branch", "refs/heads/feature/add-auth", "feature/add-auth"},
		{"push with slash", "refs/heads/bugfix/JIRA-123-fix", "bugfix/JIRA-123-fix"},
		{"tag simple", "refs/tags/v1.0.0", "v1.0.0"},
		{"tag with dots", "refs/tags/release-2024.06.15", "release-2024.06.15"},
		{"plain ref", "abc123", "abc123"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractRefName(tt.ref)
			if got != tt.want {
				t.Errorf("ExtractRefName(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestBuildSourceUrl_GitLab(t *testing.T) {
	const codebaseUrl = "https://gitlab.example.com"

	tests := []struct {
		name    string
		trigger string
		repoUrl string
		ref     string
		codeSha string
		mrIid   int
		want    string
	}{
		{
			name:    "GitLab MR",
			trigger: "MR",
			repoUrl: "git@gitlab.example.com:group/my-project.git",
			ref:     "refs/merge-requests/42/head",
			codeSha: "abc123def456",
			mrIid:   42,
			want:    "https://gitlab.example.com/group/my-project/-/merge_requests/42",
		},
		{
			name:    "GitLab MR with nested group",
			trigger: "MR",
			repoUrl: "git@gitlab.example.com:org/subgroup/project.git",
			ref:     "refs/merge-requests/7/head",
			codeSha: "def789",
			mrIid:   7,
			want:    "https://gitlab.example.com/org/subgroup/project/-/merge_requests/7",
		},
		{
			name:    "GitLab PUSH main",
			trigger: "PUSH",
			repoUrl: "git@gitlab.example.com:group/my-project.git",
			ref:     "refs/heads/main",
			codeSha: "abc123",
			mrIid:   0,
			want:    "https://gitlab.example.com/group/my-project/-/tree/main",
		},
		{
			name:    "GitLab PUSH feature branch",
			trigger: "PUSH",
			repoUrl: "git@gitlab.example.com:group/my-project.git",
			ref:     "refs/heads/feature/oauth-login",
			codeSha: "abc123",
			mrIid:   0,
			want:    "https://gitlab.example.com/group/my-project/-/tree/feature/oauth-login",
		},
		{
			name:    "GitLab TAG",
			trigger: "TAG",
			repoUrl: "git@gitlab.example.com:group/my-project.git",
			ref:     "refs/tags/v2.0.0",
			codeSha: "abc123",
			mrIid:   0,
			want:    "https://gitlab.example.com/group/my-project/-/tags/v2.0.0",
		},
		{
			name:    "GitLab TAG release",
			trigger: "TAG",
			repoUrl: "git@gitlab.example.com:group/my-project.git",
			ref:     "refs/tags/release-2024.06.15",
			codeSha: "abc123",
			mrIid:   0,
			want:    "https://gitlab.example.com/group/my-project/-/tags/release-2024.06.15",
		},
		{
			name:    "GitLab unsupported trigger (API)",
			trigger: "API",
			repoUrl: "git@gitlab.example.com:group/my-project.git",
			ref:     "refs/heads/main",
			codeSha: "abc123",
			mrIid:   0,
			want:    "",
		},
		{
			name:    "GitLab empty repo URL",
			trigger: "PUSH",
			repoUrl: "",
			ref:     "refs/heads/main",
			codeSha: "abc123",
			mrIid:   0,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSourceUrl("GitLab", tt.trigger, codebaseUrl, tt.repoUrl, tt.ref, tt.codeSha, tt.mrIid)
			if got != tt.want {
				t.Errorf("BuildSourceUrl() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSourceUrl_Codeup(t *testing.T) {
	const codebaseUrl = "https://codeup.devops.csdc.com"

	tests := []struct {
		name    string
		trigger string
		repoUrl string
		ref     string
		codeSha string
		mrIid   int
		want    string
	}{
		{
			name:    "Codeup MR",
			trigger: "MR",
			repoUrl: "ssh://git@codeup.devops.csdc.com:9022/org123/SZ/PublicService/my-service.git",
			ref:     "refs/merge-requests/15/head",
			codeSha: "abc123def456",
			mrIid:   15,
			want:    "https://codeup.devops.csdc.com/codeup/org123/SZ/PublicService/my-service/change/15",
		},
		{
			name:    "Codeup PUSH main",
			trigger: "PUSH",
			repoUrl: "ssh://git@codeup.devops.csdc.com:9022/org123/SZ/PublicService/my-service.git",
			ref:     "refs/heads/main",
			codeSha: "abc123",
			mrIid:   0,
			want:    "https://codeup.devops.csdc.com/codeup/org123/SZ/PublicService/my-service/commit/abc123?branch=main",
		},
		{
			name:    "Codeup PUSH feature branch with special chars",
			trigger: "PUSH",
			repoUrl: "ssh://git@codeup.devops.csdc.com:9022/org123/SZ/PublicService/my-service.git",
			ref:     "refs/heads/feature/test & build",
			codeSha: "abc123",
			mrIid:   0,
			want:    "https://codeup.devops.csdc.com/codeup/org123/SZ/PublicService/my-service/commit/abc123?branch=feature%2Ftest+%26+build",
		},
		{
			name:    "Codeup TAG",
			trigger: "TAG",
			repoUrl: "ssh://git@codeup.devops.csdc.com:9022/org123/SZ/PublicService/my-service.git",
			ref:     "refs/tags/v1.0.0",
			codeSha: "abc123",
			mrIid:   0,
			want:    "https://codeup.devops.csdc.com/codeup/org123/SZ/PublicService/my-service/tree/v1.0.0",
		},
		{
			name:    "Codeup TAG release",
			trigger: "TAG",
			repoUrl: "ssh://git@codeup.devops.csdc.com:9022/org123/SZ/PublicService/my-service.git",
			ref:     "refs/tags/release-2024.06.15",
			codeSha: "abc123",
			mrIid:   0,
			want:    "https://codeup.devops.csdc.com/codeup/org123/SZ/PublicService/my-service/tree/release-2024.06.15",
		},
		{
			name:    "Codeup unsupported trigger",
			trigger: "API",
			repoUrl: "ssh://git@codeup.devops.csdc.com:9022/org123/SZ/PublicService/my-service.git",
			ref:     "refs/heads/main",
			codeSha: "abc123",
			mrIid:   0,
			want:    "",
		},
		{
			name:    "Codeup unsupported platform",
			trigger: "PUSH",
			repoUrl: "ssh://git@codeup.devops.csdc.com:9022/org123/SZ/PublicService/my-service.git",
			ref:     "refs/heads/main",
			codeSha: "abc123",
			mrIid:   0,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform := "Codeup"
			if tt.name == "Codeup unsupported platform" {
				platform = "Unknown"
			}
			got := BuildSourceUrl(platform, tt.trigger, codebaseUrl, tt.repoUrl, tt.ref, tt.codeSha, tt.mrIid)
			if got != tt.want {
				t.Errorf("BuildSourceUrl() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSourceUrl_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		platform  string
		trigger   string
		codebase  string
		repoUrl   string
		ref       string
		codeSha   string
		mrIid     int
		want      string
	}{
		{
			name:     "empty repo URL returns empty",
			platform: "GitLab",
			trigger:  "PUSH",
			codebase: "https://gitlab.example.com",
			repoUrl:  "",
			ref:      "refs/heads/main",
			codeSha:  "abc123",
			mrIid:    0,
			want:     "",
		},
		{
			name:     "empty trigger returns empty",
			platform: "GitLab",
			trigger:  "",
			codebase: "https://gitlab.example.com",
			repoUrl:  "git@gitlab.example.com:group/project.git",
			ref:      "refs/heads/main",
			codeSha:  "abc123",
			mrIid:    0,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSourceUrl(tt.platform, tt.trigger, tt.codebase, tt.repoUrl, tt.ref, tt.codeSha, tt.mrIid)
			if got != tt.want {
				t.Errorf("BuildSourceUrl() = %q, want %q", got, tt.want)
			}
		})
	}
}
