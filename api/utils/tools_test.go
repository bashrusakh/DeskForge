package utils

import "testing"

func TestValidateBuildVersionStrictSemver(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "release", value: "1.4.8", valid: true},
		{name: "prerelease", value: "1.4.8-rc.1", valid: true},
		{name: "numeric prerelease zero", value: "1.4.8-0", valid: true},
		{name: "zero release", value: "0.0.0", valid: true},
		{name: "traversal", value: "../../etc", valid: false},
		{name: "backslash", value: `1.2.3\\out`, valid: false},
		{name: "control", value: "1.2.3\n", valid: false},
		{name: "missing patch", value: "1.2", valid: false},
		{name: "leading zero", value: "01.2.3", valid: false},
		{name: "leading zero prerelease", value: "1.2.3-01", valid: false},
		{name: "empty prerelease identifier", value: "1.2.3-rc.", valid: false},
		{name: "build metadata rejected", value: "1.2.3+build.7", valid: false},
		{name: "too long for storage", value: "1.2.3-" + "a" + "bcdefghijklmnopqrstuvwxyz012345", valid: false},
		{name: "shell syntax", value: "1.2.3$(id)", valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidateBuildVersion(test.value); got != test.valid {
				t.Fatalf("ValidateBuildVersion(%q) = %v, want %v", test.value, got, test.valid)
			}
		})
	}
}
