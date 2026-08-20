package api

import (
	"encoding/json"
	"strings"
	"testing"

	"rustdesk-server/api/model"
)

func TestWebClientAddressBookPayloadDoesNotExposeStoredCredentialHash(t *testing.T) {
	payload := &WebClientPeerPayload{}
	payload.FromAddressBook(&model.AddressBook{
		Username: "alice",
		Hostname: "host",
		Platform: "Windows",
		Hash:     "hash-credential",
		Password: "password-secret",
	})
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal web client payload: %v", err)
	}
	response := string(encoded)
	for _, forbidden := range []string{"hash-credential", "password-secret", `"hash"`, `"password"`} {
		if strings.Contains(response, forbidden) {
			t.Fatalf("web client payload exposed %q: %s", forbidden, response)
		}
	}
	for _, expected := range []string{`"username":"alice"`, `"hostname":"host"`, `"platform":"Windows"`} {
		if !strings.Contains(response, expected) {
			t.Fatalf("web client payload omitted %q: %s", expected, response)
		}
	}
}
