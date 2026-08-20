package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCustomBuildSafeResponsesRedactSecretCustomJSON(t *testing.T) {
	build := &CustomBuild{
		IdModel:    IdModel{Id: 7},
		Name:       "safe-build",
		Platform:   "windows",
		Version:    "1.2.3",
		CustomJson: `{"enable_audio":true,"key":"public-key","permanent_password":"builder-secret","nested":{"token":"nested-secret"}}`,
	}

	views := []struct {
		name string
		view any
	}{
		{name: "list", view: (&CustomBuildList{CustomBuilds: []*CustomBuild{build}}).Safe()},
		{name: "detail", view: build.Safe()},
		{name: "create", view: build.Safe()},
		{name: "update", view: build.Safe()},
	}
	for _, test := range views {
		t.Run(test.name, func(t *testing.T) {
			assertSafeCustomResponse(t, test.view)
		})
	}
}

func TestCustomPresetSafeResponsesRedactSecretCustomJSON(t *testing.T) {
	preset := &CustomPreset{
		IdModel:    IdModel{Id: 8},
		Name:       "safe-preset",
		Platform:   "windows",
		Version:    "1.2.3",
		CustomJson: `{"enable_audio":true,"key":"public-key","permanent_password":"preset-secret","nested":{"secret":"nested-secret"}}`,
	}

	views := []struct {
		name string
		view any
	}{
		{name: "list", view: (&CustomPresetList{CustomPresets: []*CustomPreset{preset}}).Safe()},
		{name: "detail", view: preset.Safe()},
		{name: "create", view: preset.Safe()},
		{name: "update", view: preset.Safe()},
	}
	for _, test := range views {
		t.Run(test.name, func(t *testing.T) {
			assertSafeCustomResponse(t, test.view)
		})
	}
}

func TestCustomPresetSafeReportsPermanentPasswordPresenceWithoutSecret(t *testing.T) {
	for _, test := range []struct {
		name       string
		customJSON string
		want       bool
	}{
		{name: "present", customJSON: `{"permanent_password":"preset-secret"}`, want: true},
		{name: "blank", customJSON: `{"permanent_password":"  "}`, want: false},
		{name: "absent", customJSON: `{"enable_audio":true}`, want: false},
		{name: "ciphertext", customJSON: "enc:v1:not-ciphertext", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := (&CustomPreset{CustomJson: test.customJSON}).Safe()
			if view.HasPermanentPassword != test.want {
				t.Fatalf("has_permanent_password = %v, want %v", view.HasPermanentPassword, test.want)
			}
			encoded, err := json.Marshal(view)
			if err != nil {
				t.Fatalf("marshal safe preset: %v", err)
			}
			if strings.Contains(string(encoded), "preset-secret") {
				t.Fatalf("safe preset exposed password: %s", encoded)
			}
		})
	}
}

func TestCustomSafeViewNeverSerializesCiphertext(t *testing.T) {
	build := &CustomBuild{CustomJson: "enc:v1:not-ciphertext"}
	preset := &CustomPreset{CustomJson: "enc:v1:not-ciphertext"}
	for _, view := range []any{build.Safe(), preset.Safe()} {
		encoded, err := json.Marshal(view)
		if err != nil {
			t.Fatalf("marshal safe view: %v", err)
		}
		if strings.Contains(string(encoded), "enc:v1:") {
			t.Fatalf("safe view exposed ciphertext: %s", encoded)
		}
	}
}

func TestRawSecretBearingModelsNeverSerializeStorageFields(t *testing.T) {
	models := []any{
		&GithubBuildConfig{Repo: "owner/repo", WorkflowRefApproved: true, Token: "github_pat_secret", PayloadKey: "payload-secret"},
		&CustomBuild{CustomJson: `{"permanent_password":"build-secret","enable_audio":true}`},
		&CustomPreset{CustomJson: `{"permanent_password":"preset-secret"}`},
	}
	for _, value := range models {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal raw model: %v", err)
		}
		response := string(encoded)
		for _, forbidden := range []string{"github_pat_secret", "payload-secret", "build-secret", "preset-secret", "custom_json", "payload_key", `"token"`, `"workflow_ref_approved"`} {
			if strings.Contains(response, forbidden) {
				t.Fatalf("raw model serialized forbidden field/value %q: %s", forbidden, response)
			}
		}
	}
}

func TestGithubBuildConfigSafeViewPreservesPresenceFlagsOnly(t *testing.T) {
	encoded, err := json.Marshal((&GithubBuildConfig{
		Repo:       "owner/repo",
		Token:      "github_pat_secret",
		PayloadKey: "payload-secret",
	}).Safe())
	if err != nil {
		t.Fatalf("marshal safe config: %v", err)
	}
	response := string(encoded)
	for _, forbidden := range []string{"github_pat_secret", "payload-secret", `"token"`, `"payload_key"`} {
		if strings.Contains(response, forbidden) {
			t.Fatalf("safe config serialized forbidden value/field %q: %s", forbidden, response)
		}
	}
	for _, expected := range []string{`"has_token":true`, `"has_payload_key":true`} {
		if !strings.Contains(response, expected) {
			t.Fatalf("safe config omitted %q: %s", expected, response)
		}
	}
}

func TestGithubBuildConfigSafeViewDistinguishesProviderPolicyStatus(t *testing.T) {
	for _, test := range []struct {
		name       string
		approved   bool
		verified   bool
		sha        string
		branch     string
		wantStatus string
	}{
		{name: "approval required", wantStatus: WorkflowRefStatusApprovalRequired},
		{name: "legacy attestation", approved: true, wantStatus: WorkflowRefStatusProviderPolicyUnverified},
		{name: "provider-backed approval", approved: true, verified: true, wantStatus: WorkflowRefStatusApproved},
		{name: "malformed historical SHA", approved: true, verified: true, sha: "historical-sha", wantStatus: WorkflowRefStatusProviderPolicyUnverified},
		{name: "mutable historical selector", approved: true, verified: true, branch: "refs/heads/main", wantStatus: WorkflowRefStatusProviderPolicyUnverified},
	} {
		t.Run(test.name, func(t *testing.T) {
			sha := test.sha
			if sha == "" {
				sha = strings.Repeat("a", 40)
			}
			branch := test.branch
			if branch == "" {
				branch = "refs/tags/workflow-v1"
			}
			view := (&GithubBuildConfig{
				Branch:                      branch,
				WorkflowRefApproved:         test.approved,
				WorkflowRefProviderVerified: test.verified,
				WorkflowRefApprovalSHA:      sha,
			}).Safe()
			if view.WorkflowRefStatus != test.wantStatus {
				t.Fatalf("workflow_ref_status = %q, want %q", view.WorkflowRefStatus, test.wantStatus)
			}
			if test.wantStatus == WorkflowRefStatusApproved && view.WorkflowRefTrustStatus != WorkflowRefTrustProviderReported {
				t.Fatalf("workflow_ref_trust_status = %q, want provider-reported", view.WorkflowRefTrustStatus)
			}
			if test.wantStatus != WorkflowRefStatusApproved && view.WorkflowRefApproved {
				t.Fatal("malformed or stale approval was exposed as effective approved")
			}
		})
	}
}

func assertSafeCustomResponse(t *testing.T, view any) {
	t.Helper()
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal safe response: %v", err)
	}
	response := string(encoded)
	for _, secret := range []string{"builder-secret", "preset-secret", "nested-secret", `"permanent_password"`, `"token"`, `"secret"`} {
		if strings.Contains(response, secret) {
			t.Fatalf("safe response exposed %q: %s", secret, response)
		}
	}
	for _, nonSecret := range []string{"enable_audio", "public-key"} {
		if !strings.Contains(response, nonSecret) {
			t.Fatalf("safe response omitted non-secret %q: %s", nonSecret, response)
		}
	}
}
