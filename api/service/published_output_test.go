package service

import (
	"os"
	"path/filepath"
	"testing"

	"rustdesk-server/api/model"
)

func TestValidatePublishedOutputRejectsCaseInsensitiveWindowsNames(t *testing.T) {
	output := t.TempDir()
	for name, contents := range map[string]string{
		"rustqs.exe": "exe",
		"helper.dll": "lower",
		"HELPER.DLL": "upper",
	} {
		if err := os.WriteFile(filepath.Join(output, name), []byte(contents), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	build := &model.CustomBuild{Platform: "windows", AppName: "rustqs"}
	if _, err := ValidatePublishedOutput(output, build); err == nil {
		t.Fatal("ValidatePublishedOutput() accepted case-insensitive Windows artifact collision")
	}
}

func TestValidatePublishedOutputPreservesDistinctPOSIXNames(t *testing.T) {
	output := t.TempDir()
	for name, contents := range map[string]string{
		"helper.dll": "lower",
		"HELPER.DLL": "upper",
	} {
		if err := os.WriteFile(filepath.Join(output, name), []byte(contents), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	build := &model.CustomBuild{Platform: "linux", AppName: "rustqs"}
	if _, err := ValidatePublishedOutput(output, build); err != nil {
		t.Fatalf("ValidatePublishedOutput() rejected distinct POSIX names: %v", err)
	}
}
