package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCustomBuildPublicViewOmitsCapabilityKey(t *testing.T) {
	build := &CustomBuild{
		IdModel:              IdModel{Id: 7},
		UserId:               9,
		Name:                 "public-build",
		Platform:             "windows",
		Status:               CustomBuildStatusDone,
		AppName:              "rustqs",
		Version:              "1.2.3",
		DownloadKey:          "capability-secret",
		DownloadKeyExpiresAt: 1234,
		FileSize:             456,
		CustomJson:           `{"enable_audio":true,"permanent_password":"build-secret"}`,
		BuildLog:             "private build log",
		GithubRunId:          55,
		GithubRepo:           "owner/private-repo",
		GithubWorkflow:       "private-workflow.yml",
		GithubRef:            "private-ref",
		GithubArtifactName:   "private-artifact",
		GithubArtifactID:     66,
	}

	encoded, err := json.Marshal(build.Public())
	if err != nil {
		t.Fatalf("marshal public build: %v", err)
	}
	response := string(encoded)
	for _, forbidden := range []string{
		"capability-secret", `"download_key"`, "build-secret", "permanent_password",
		"private build log", `"github_run_id"`, `"github_repo"`, `"github_workflow"`,
		`"github_ref"`, `"github_artifact_name"`, `"github_artifact_id"`, `"user_id"`,
	} {
		if strings.Contains(response, forbidden) {
			t.Fatalf("public build exposed %q: %s", forbidden, response)
		}
	}
	for _, expected := range []string{
		`"id":7`, `"name":"public-build"`, `"platform":"windows"`,
		`"version":"1.2.3"`, `"status":"done"`, `"app_name":"rustqs"`,
		`"file_size":456`, `"download_key_expires_at":1234`,
	} {
		if !strings.Contains(response, expected) {
			t.Fatalf("public build omitted %q: %s", expected, response)
		}
	}
}

func TestCustomBuildSafeViewOmitsCapabilityKey(t *testing.T) {
	build := &CustomBuild{
		IdModel:              IdModel{Id: 8},
		DownloadKey:          "admin-dto-capability-secret",
		DownloadKeyExpiresAt: 1234,
		CustomJson:           `{"enable_audio":true}`,
	}

	encoded, err := json.Marshal(build.Safe())
	if err != nil {
		t.Fatalf("marshal safe build: %v", err)
	}
	response := string(encoded)
	for _, forbidden := range []string{"admin-dto-capability-secret", `"download_key"`} {
		if strings.Contains(response, forbidden) {
			t.Fatalf("safe build exposed %q: %s", forbidden, response)
		}
	}
	if !strings.Contains(response, `"download_key_expires_at":1234`) {
		t.Fatalf("safe build omitted capability expiry metadata: %s", response)
	}
}

func TestRawCustomBuildModelAndListOmitDownloadKey(t *testing.T) {
	build := &CustomBuild{
		Name:        "raw-build",
		DownloadKey: "raw-download-key",
	}
	values := []struct {
		name  string
		value any
	}{
		{name: "model", value: build},
		{name: "list", value: &CustomBuildList{CustomBuilds: []*CustomBuild{build}}},
	}
	for _, test := range values {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatalf("marshal raw custom build %s: %v", test.name, err)
			}
			response := string(encoded)
			if strings.Contains(response, "raw-download-key") || strings.Contains(response, `"download_key"`) {
				t.Fatalf("raw custom build %s exposed download key: %s", test.name, response)
			}
			if !strings.Contains(response, `"name":"raw-build"`) {
				t.Fatalf("raw custom build %s omitted public field: %s", test.name, response)
			}
		})
	}
}

func TestShareRecordSafeListOmitsCredentials(t *testing.T) {
	record := &ShareRecord{
		IdModel:      IdModel{Id: 3},
		UserId:       8,
		PeerId:       "peer-1",
		ShareToken:   "share-token-secret",
		PasswordType: "fixed",
		Password:     "password-secret",
		Expire:       1800,
	}
	encoded, err := json.Marshal((&ShareRecordList{
		ShareRecords: []*ShareRecord{record},
		Pagination:   Pagination{Page: 1, PageSize: 10, Total: 1},
	}).Safe())
	if err != nil {
		t.Fatalf("marshal safe share records: %v", err)
	}
	response := string(encoded)
	for _, forbidden := range []string{"share-token-secret", "password-secret", `"share_token"`, `"password_type"`, `"password"`} {
		if strings.Contains(response, forbidden) {
			t.Fatalf("safe share record exposed %q: %s", forbidden, response)
		}
	}
	for _, expected := range []string{`"id":3`, `"user_id":8`, `"peer_id":"peer-1"`, `"expire":1800`, `"list"`} {
		if !strings.Contains(response, expected) {
			t.Fatalf("safe share record omitted %q: %s", expected, response)
		}
	}
}

func TestUserTokenSafeListMasksBearerSecretAndPreservesSchema(t *testing.T) {
	secret := "0123456789abcdef-secret-token"
	encoded, err := json.Marshal((&UserTokenList{
		UserTokens: []UserToken{{
			IdModel:   IdModel{Id: 3},
			UserId:    8,
			Token:     secret,
			ExpiredAt: 99,
		}},
		Pagination: Pagination{Page: 1, PageSize: 10, Total: 1},
	}).Safe())
	if err != nil {
		t.Fatalf("marshal safe token list: %v", err)
	}
	response := string(encoded)
	if strings.Contains(response, secret) {
		t.Fatalf("safe token list exposed bearer secret: %s", response)
	}
	for _, expected := range []string{`"id":3`, `"user_id":8`, `"token":"0123****oken"`, `"expired_at":99`, `"list"`} {
		if !strings.Contains(response, expected) {
			t.Fatalf("safe token list omitted %q: %s", expected, response)
		}
	}
}

func TestRawUserTokenModelDoesNotSerializeBearerSecret(t *testing.T) {
	encoded, err := json.Marshal(&UserToken{Token: "raw-token-secret"})
	if err != nil {
		t.Fatalf("marshal raw user token: %v", err)
	}
	response := string(encoded)
	if strings.Contains(response, "raw-token-secret") || strings.Contains(response, `"token"`) {
		t.Fatalf("raw user token serialized bearer secret: %s", response)
	}
}

func TestAddressBookSafeViewRedactsCredentialsAndPreservesPublicFields(t *testing.T) {
	addressBook := &AddressBook{
		RowId:            4,
		Id:               "peer-id",
		Username:         "alice",
		Password:         "password-secret",
		Hash:             "hash-credential",
		Hostname:         "host",
		Alias:            "friendly",
		Platform:         "Windows",
		UserId:           8,
		ForceAlwaysRelay: true,
		RdpPort:          "3389",
		RdpUsername:      "rdp-user",
		LoginName:        "login",
	}

	encoded, err := json.Marshal((&AddressBookList{AddressBooks: []*AddressBook{addressBook}}).Safe())
	if err != nil {
		t.Fatalf("marshal safe address book: %v", err)
	}
	response := string(encoded)
	for _, forbidden := range []string{"password-secret", "hash-credential", `"password"`, `"hash"`} {
		if strings.Contains(response, forbidden) {
			t.Fatalf("safe address book exposed %q: %s", forbidden, response)
		}
	}
	for _, expected := range []string{`"id":"peer-id"`, `"hostname":"host"`, `"alias":"friendly"`, `"rdpPort":"3389"`, `"loginName":"login"`} {
		if !strings.Contains(response, expected) {
			t.Fatalf("safe address book omitted %q: %s", expected, response)
		}
	}
}

func TestRawAddressBookModelAndListOmitCredentials(t *testing.T) {
	addressBook := &AddressBook{
		Id:       "raw-peer",
		Password: "raw-password-secret",
		Hash:     "raw-hash-secret",
		Hostname: "host",
	}
	values := []struct {
		name  string
		value any
	}{
		{name: "model", value: addressBook},
		{name: "list", value: &AddressBookList{AddressBooks: []*AddressBook{addressBook}}},
	}
	for _, test := range values {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatalf("marshal raw address book %s failed", test.name)
			}
			response := string(encoded)
			if strings.Contains(response, "raw-password-secret") || strings.Contains(response, "raw-hash-secret") || strings.Contains(response, `"password"`) || strings.Contains(response, `"hash"`) {
				t.Fatalf("raw address book %s serialized credential fields", test.name)
			}
			if !strings.Contains(response, `"id":"raw-peer"`) || !strings.Contains(response, `"hostname":"host"`) {
				t.Fatalf("raw address book %s omitted public fields", test.name)
			}
		})
	}
}

func TestRawShareRecordModelAndListOmitCredentials(t *testing.T) {
	record := &ShareRecord{
		IdModel:      IdModel{Id: 5},
		PeerId:       "raw-peer",
		ShareToken:   "raw-share-token-secret",
		PasswordType: "fixed",
		Password:     "raw-password-secret",
		Expire:       1800,
	}
	values := []struct {
		name  string
		value any
	}{
		{name: "model", value: record},
		{name: "list", value: &ShareRecordList{ShareRecords: []*ShareRecord{record}}},
	}
	for _, test := range values {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatalf("marshal raw share record %s failed", test.name)
			}
			response := string(encoded)
			if strings.Contains(response, "raw-share-token-secret") || strings.Contains(response, "raw-password-secret") || strings.Contains(response, `"share_token"`) || strings.Contains(response, `"password"`) {
				t.Fatalf("raw share record %s serialized credential fields", test.name)
			}
			if !strings.Contains(response, `"id":5`) || !strings.Contains(response, `"peer_id":"raw-peer"`) {
				t.Fatalf("raw share record %s omitted public fields", test.name)
			}
		})
	}
}
