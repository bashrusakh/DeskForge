package admin

import "testing"

func TestCustomPresetFormMapsRedactedPasswordIntent(t *testing.T) {
	preset := (&CustomPresetForm{
		Name:                      "preset",
		Platform:                  "windows",
		Version:                   "1.2.3",
		PreservePermanentPassword: true,
		CustomJson:                `{"permanent_password":""}`,
	}).ToCustomPreset()

	if !preset.PreservePermanentPassword {
		t.Fatal("PreservePermanentPassword = false, want true")
	}
	if preset.CustomJson != `{"permanent_password":""}` {
		t.Fatalf("CustomJson = %q, want redacted payload unchanged", preset.CustomJson)
	}
}
