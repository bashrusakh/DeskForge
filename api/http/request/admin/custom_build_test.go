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
