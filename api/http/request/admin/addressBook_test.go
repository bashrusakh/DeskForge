package admin

import "testing"

func TestAddressBookFormPreservesTypedCredentialInput(t *testing.T) {
	form := AddressBookForm{
		Id:       "peer-id",
		Password: "password-secret",
		Hash:     "hash-credential",
	}
	addressBook := form.ToAddressBook()
	if addressBook.Password != form.Password || addressBook.Hash != form.Hash {
		t.Fatalf("typed credentials were not mapped to write model: %#v", addressBook)
	}
}
