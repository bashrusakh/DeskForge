package api

import (
	"encoding/json"
	"testing"
)

func TestAddressBookRequestFormsPreserveTypedCredentials(t *testing.T) {
	const payload = `{"data":"{\"peers\":[{\"id\":\"peer-id\",\"password\":\"password-secret\",\"hash\":\"hash-secret\"}]}"}`
	var form AddressBookForm
	if err := json.Unmarshal([]byte(payload), &form); err != nil {
		t.Fatalf("decode address book form failed: %v", err)
	}
	var data AddressBookFormData
	if err := json.Unmarshal([]byte(form.Data), &data); err != nil {
		t.Fatalf("decode address book form data failed: %v", err)
	}
	if len(data.Peers) != 1 {
		t.Fatalf("decoded peer count = %d, want 1", len(data.Peers))
	}
	peer := data.Peers[0].ToAddressBook()
	if peer.Password != "password-secret" || peer.Hash != "hash-secret" {
		t.Fatal("typed address book credentials were not preserved")
	}
}

func TestPersonalAddressBookFormPreservesTypedCredentials(t *testing.T) {
	var form PersonalAddressBookForm
	if err := json.Unmarshal([]byte(`{"id":"peer-id","password":"password-secret","hash":"hash-secret"}`), &form); err != nil {
		t.Fatalf("decode personal address book form failed: %v", err)
	}
	peer := form.ToAddressBook()
	if peer.Password != "password-secret" || peer.Hash != "hash-secret" {
		t.Fatal("typed personal address book credentials were not preserved")
	}
}
