package upload

import "testing"

func TestValidateOSSPublicKeyURLAllowsOfficialEndpoint(t *testing.T) {
	validURLs := []string{
		"https://gosspublic.alicdn.com/callback_pub_key_v1.pem",
		"http://gosspublic.alicdn.com/callback_pub_key_v1.pem",
		"https://GOSSPUBLIC.ALICDN.COM/callback_pub_key_v1.pem",
	}

	for _, publicKeyURL := range validURLs {
		t.Run(publicKeyURL, func(t *testing.T) {
			if err := validateOSSPublicKeyURL(publicKeyURL); err != nil {
				t.Fatalf("expected URL to be accepted: %v", err)
			}
		})
	}
}

func TestValidateOSSPublicKeyURLRejectsUntrustedTargets(t *testing.T) {
	invalidURLs := []string{
		"https://example.com/callback_pub_key_v1.pem",
		"https://evil.aliyuncs.com/callback_pub_key_v1.pem",
		"https://gosspublic.alicdn.com.evil.test/callback_pub_key_v1.pem",
		"https://gosspublic.alicdn.com:8443/callback_pub_key_v1.pem",
		"https://gosspublic.alicdn.com/other.pem",
		"https://gosspublic.alicdn.com/callback_pub_key_v1.pem?next=http://127.0.0.1/",
		"https://gosspublic.alicdn.com/callback_pub_key_v1.pem#fragment",
		"https://user@gosspublic.alicdn.com/callback_pub_key_v1.pem",
		"file:///callback_pub_key_v1.pem",
	}

	for _, publicKeyURL := range invalidURLs {
		t.Run(publicKeyURL, func(t *testing.T) {
			if err := validateOSSPublicKeyURL(publicKeyURL); err == nil {
				t.Fatal("expected URL to be rejected")
			}
		})
	}
}
