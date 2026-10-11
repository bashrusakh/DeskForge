package admin

import "testing"

func TestCustomBuildFormToCustomBuildPreservesUserAuthoredAppName(t *testing.T) {
	tests := []struct {
		name        string
		form        CustomBuildForm
		wantAppName string
	}{
		{
			name: "preserves empty app name",
			form: CustomBuildForm{
				Name: "windows-1.2.3",
			},
			wantAppName: "",
		},
		{
			name: "preserves explicit app name",
			form: CustomBuildForm{
				Name:    "windows-1.2.3",
				AppName: "DeskForge",
			},
			wantAppName: "DeskForge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := tt.form.ToCustomBuild()
			if build.AppName != tt.wantAppName {
				t.Fatalf("AppName = %q, want %q", build.AppName, tt.wantAppName)
			}
		})
	}
}

func TestCustomBuildFormToCustomBuildDoesNotCopyRequestedPresetID(t *testing.T) {
	// preset_id is request-only input: provenance is attached later from the
	// resolved owned preset row (AttachPresetProvenance), so the raw request id
	// must never reach the persisted model with an empty name.
	presetID := uint(42)
	form := CustomBuildForm{Name: "windows-1.2.3", PresetId: &presetID}
	build := form.ToCustomBuild()
	if build.PresetId != 0 || build.PresetName != "" {
		t.Fatalf("ToCustomBuild() copied raw preset provenance: preset_id=%d preset_name=%q", build.PresetId, build.PresetName)
	}
}
