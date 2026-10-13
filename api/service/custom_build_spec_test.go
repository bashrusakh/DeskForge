package service

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"rustdesk-server/api/config"
)

func TestParsePlatform(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   string
		want    Platform
		wantErr bool
	}{
		{name: "windows", value: "windows", want: PlatformWindows},
		{name: "linux", value: "linux", want: PlatformLinux},
		{name: "android", value: "android", want: PlatformAndroid},
		{name: "macos", value: "macos", wantErr: true},
		{name: "unknown", value: "plan9", wantErr: true},
		{name: "empty", value: "", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParsePlatform(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatalf("ParsePlatform(%q) error = nil, want error", test.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePlatform(%q) error = %v", test.value, err)
			}
			if got != test.want {
				t.Errorf("ParsePlatform(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestPublicCustomTxtRedactsPrivateNativeSettings(t *testing.T) {
	raw := `{"app-name":"Acme Support","conn-type":"incoming","password":"password-secret","default-settings":{"allow-hide-cm":"Y","verification-method":"use-permanent-password","enable-audio":"Y","theme":"dark"}}`
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	got, err := PublicCustomTxt(encoded)
	if err != nil {
		t.Fatalf("PublicCustomTxt() error = %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("public custom_.txt is not standard base64: %v", err)
	}
	var public map[string]any
	if err := json.Unmarshal(decoded, &public); err != nil {
		t.Fatalf("public custom_.txt is not client-compatible JSON: %v", err)
	}
	if public["app-name"] != "Acme Support" {
		t.Fatalf("public app-name = %#v, want preserved runtime application identity", public["app-name"])
	}
	if public["conn-type"] != "incoming" {
		t.Fatalf("public conn-type = %#v, want incoming", public["conn-type"])
	}
	settings, ok := public["default-settings"].(map[string]any)
	if !ok {
		t.Fatalf("public default-settings = %T, want object", public["default-settings"])
	}
	if settings["enable-audio"] != "Y" || settings["theme"] != "dark" {
		t.Fatalf("safe L2 settings = %#v, want preserved settings", settings)
	}
	for _, forbidden := range []string{"password", "allow-hide-cm", "verification-method", "password-secret", "use-permanent-password"} {
		if strings.Contains(string(decoded), forbidden) {
			t.Fatalf("public custom_.txt contains forbidden value %q: %s", forbidden, decoded)
		}
	}
	repeated, err := PublicCustomTxt(encoded)
	if err != nil {
		t.Fatalf("repeated PublicCustomTxt() error = %v", err)
	}
	if repeated != got {
		t.Fatalf("public custom_.txt is not deterministic: first=%q second=%q", got, repeated)
	}
}

func TestPublicCustomTxtFailsClosedForMalformedOrUnsafePayloads(t *testing.T) {
	for _, test := range []struct {
		name    string
		encoded string
	}{
		{name: "invalid base64", encoded: "%%%"},
		{name: "invalid JSON", encoded: base64.StdEncoding.EncodeToString([]byte("not-json"))},
		{name: "duplicate JSON key", encoded: base64.StdEncoding.EncodeToString([]byte(`{"conn-type":"incoming","conn-type":"outgoing"}`))},
		{name: "unknown secret field", encoded: base64.StdEncoding.EncodeToString([]byte(`{"token":"token-secret"}`))},
		{name: "unsafe nested field", encoded: base64.StdEncoding.EncodeToString([]byte(`{"default-settings":{"payload-key":"payload-secret"}}`))},
		{name: "null nested setting", encoded: base64.StdEncoding.EncodeToString([]byte(`{"default-settings":{"enable-audio":null}}`))},
		{name: "unknown persisted-only branding field", encoded: base64.StdEncoding.EncodeToString([]byte(`{"company_name":"Acme Ltd"}`))},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := PublicCustomTxt(test.encoded)
			if err == nil {
				t.Fatalf("PublicCustomTxt() output = %q, want fail-closed error", got)
			}
			if strings.Contains(got, "secret") {
				t.Fatalf("failed public transform returned secret-bearing output: %q", got)
			}
		})
	}
}

func TestValidateOutputAppName(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "ascii name", value: "rustqs"},
		{name: "spaces and unicode", value: "My RustDesk 客户端"},
		{name: "empty", value: "", wantErr: true},
		{name: "dot", value: ".", wantErr: true},
		{name: "dot dot", value: "..", wantErr: true},
		{name: "slash traversal", value: "../rustqs", wantErr: true},
		{name: "backslash traversal", value: `..\rustqs`, wantErr: true},
		{name: "control newline", value: "rustqs\nnext", wantErr: true},
		{name: "control carriage return", value: "rustqs\rnext", wantErr: true},
		{name: "control nul", value: "rustqs\x00next", wantErr: true},
		{name: "excessive", value: strings.Repeat("a", maxBuildAppNameBytes+1), wantErr: true},
		{name: "windows reserved punctuation", value: "rust|qs", wantErr: true},
		{name: "reserved CON", value: "CON", wantErr: true},
		{name: "reserved extension", value: "con.txt", wantErr: true},
		{name: "reserved spaced extension", value: "LPT1 .exe", wantErr: true},
		{name: "reserved case insensitive", value: "aUx", wantErr: true},
		{name: "safe reserved prefix", value: "CONSOLE"},
		{name: "safe COM10", value: "COM10"},
		{name: "trailing dot", value: "rustqs.", wantErr: true},
		{name: "trailing space", value: "rustqs ", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateOutputAppName(test.value)
			if test.wantErr && err == nil {
				t.Fatalf("ValidateOutputAppName(%q) error = nil, want error", test.value)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("ValidateOutputAppName(%q) error = %v", test.value, err)
			}
		})
	}
}

func TestValidateWindowsArtifactFilename(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "legitimate exe", value: "rustqs.exe"},
		{name: "legitimate dll", value: "helper.DLL"},
		{name: "legitimate zip", value: "driver_bundle.zip"},
		{name: "config txt", value: "custom_.txt"},
		{name: "traversal", value: "../rustqs.exe", wantErr: true},
		{name: "separator", value: `bin\\helper.dll`, wantErr: true},
		{name: "control", value: "helper\n.dll", wantErr: true},
		{name: "reserved device", value: "CON.exe", wantErr: true},
		{name: "unsupported extension", value: "helper.bat", wantErr: true},
		{name: "trailing space", value: "helper.dll ", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateWindowsArtifactFilename(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateWindowsArtifactFilename(%q) error = %v, wantErr %v", test.value, err, test.wantErr)
			}
		})
	}
	if got := WindowsArtifactNameKey("helper.DLL"); got != "helper.dll" {
		t.Fatalf("WindowsArtifactNameKey() = %q, want case-insensitive identity", got)
	}
}

func TestNormalizeCustomBuildRejectsUnsafeWorkflowValues(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  map[string]any
		ctx  BuildRecordContext
	}{
		{name: "key newline", raw: map[string]any{"key": "public\nkey"}, ctx: BuildRecordContext{Platform: "windows", AppName: "rustqs"}},
		{name: "key carriage return", raw: map[string]any{"key": "public\rkey"}, ctx: BuildRecordContext{Platform: "windows", AppName: "rustqs"}},
		{name: "key nul", raw: map[string]any{"key": "public\x00key"}, ctx: BuildRecordContext{Platform: "windows", AppName: "rustqs"}},
		{name: "server tab", raw: map[string]any{"server_ip": "id.example\t:21116"}, ctx: BuildRecordContext{Platform: "windows", AppName: "rustqs"}},
		{name: "app newline", ctx: BuildRecordContext{Platform: "windows", AppName: "rustqs\nnext"}},
		{name: "version carriage return", ctx: BuildRecordContext{Platform: "windows", AppName: "rustqs", Version: "1.2.3\r"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeCustomBuild(test.raw, test.ctx); err == nil {
				t.Fatal("NormalizeCustomBuild() error = nil, want unsafe workflow value rejection")
			}
		})
	}
	for _, value := range []string{"public\nkey", "public\rkey", "public\x00key"} {
		if _, err := ParseBuildSpec(map[string]any{"key": value}); err == nil {
			t.Fatalf("ParseBuildSpec() accepted unsafe key %q", value)
		}
	}
}

func TestNormalizeWorkflowDispatchParamsRequiresTypedCustomTxt(t *testing.T) {
	if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"custom_txt": "raw-native-payload"}); err == nil {
		t.Fatal("NormalizeWorkflowDispatchParams() accepted raw custom_txt")
	}
	normalized, err := NormalizeCustomBuild(map[string]any{"enable_audio": true}, BuildRecordContext{
		Platform: string(PlatformWindows),
		AppName:  "rustqs",
		Version:  "1.2.3",
	})
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() error = %v", err)
	}
	params, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), normalized.DispatchParams)
	if err != nil {
		t.Fatalf("NormalizeWorkflowDispatchParams() rejected typed BuildSpec output: %v", err)
	}
	if _, ok := params["custom_txt"].(string); !ok {
		t.Fatalf("normalized custom_txt type = %T, want string for provider JSON", params["custom_txt"])
	}
	keyParams, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{
		"key": validRustDeskPublicKey + "\r\n",
	})
	if err != nil {
		t.Fatalf("NormalizeWorkflowDispatchParams() rejected trailing key line endings: %v", err)
	}
	if keyParams["key"] != validRustDeskPublicKey {
		t.Fatalf("normalized public key = %q, want trailing CR/LF removed", keyParams["key"])
	}
}

func TestNormalizeWorkflowDispatchParamsRejectsCallerWorkflowSHA(t *testing.T) {
	_, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{
		"workflow_sha": strings.Repeat("a", 40),
	})
	if err == nil {
		t.Fatal("NormalizeWorkflowDispatchParams() accepted caller-authored workflow_sha")
	}
	if got, want := err.Error(), `unsupported workflow dispatch field "workflow_sha"`; got != want {
		t.Fatalf("NormalizeWorkflowDispatchParams() error = %q, want %q", got, want)
	}
}

func TestNormalizeCustomBuildFullNativeMapping(t *testing.T) {
	raw := map[string]any{
		"server_ip":             "id.example:21116",
		"key":                   validRustDeskPublicKey,
		"app_name":              "ignored-app-name",
		"version":               "ignored-version",
		"direction":             "incoming",
		"permanent_password":    "secret",
		"pass_approve_mode":     "password-click",
		"permissions_type":      "custom",
		"theme":                 "dark",
		"deny_lan":              true,
		"enable_direct_ip":      false,
		"auto_close":            true,
		"hide_cm":               false,
		"remove_wallpaper":      true,
		"enable_remote_modi":    false,
		"enable_keyboard":       false,
		"enable_clipboard":      false,
		"enable_file_transfer":  false,
		"enable_audio":          true,
		"enable_tcp":            true,
		"enable_remote_restart": true,
		"enable_recording":      false,
		"enable_blocking_input": true,
		"enable_printer":        false,
		"enable_camera":         true,
		"enable_terminal":       false,
		"api_server":            "https://api.example",
		"relay_server":          "relay.example:21117",
	}

	got, err := NormalizeCustomBuild(raw, BuildRecordContext{
		BuildID:  42,
		Platform: "windows",
		AppName:  "record-app",
		Version:  "1.2.3",
	})
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() error = %v", err)
	}
	decoded := decodeCustomTxt(t, got.CustomTxt)
	want := map[string]any{
		"app-name":  "record-app",
		"conn-type": "incoming",
		"password":  "secret",
		"default-settings": map[string]any{
			"access-mode":                      "custom",
			"approve-mode":                     "password-click",
			"verification-method":              "use-both-passwords",
			"allow-hide-cm":                    "N",
			"enable-keyboard":                  "N",
			"enable-clipboard":                 "N",
			"enable-file-transfer":             "N",
			"enable-audio":                     "Y",
			"enable-tunnel":                    "Y",
			"enable-remote-restart":            "Y",
			"enable-record-session":            "N",
			"enable-block-input":               "Y",
			"enable-camera":                    "Y",
			"enable-terminal":                  "N",
			"enable-remote-printer":            "N",
			"allow-remote-config-modification": "N",
			"enable-lan-discovery":             "N",
			"allow-auto-disconnect":            "Y",
			"allow-remove-wallpaper":           "Y",
			"api-server":                       "https://api.example",
			"relay-server":                     "relay.example:21117",
			"direct-server":                    "N",
			"theme":                            "dark",
		},
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("decoded custom JSON = %#v, want %#v", decoded, want)
	}
	if got.DispatchParams["server"] != "id.example:21116" {
		t.Errorf("dispatch server = %v, want literal endpoint", got.DispatchParams["server"])
	}
	if got.DispatchParams["key"] != validRustDeskPublicKey {
		t.Errorf("dispatch key = %v, want canonical public key", got.DispatchParams["key"])
	}
	if got.DispatchParams["app_name"] != "record-app" || got.DispatchParams["version"] != "1.2.3" {
		t.Errorf("record context leaked or was lost in dispatch params: %#v", got.DispatchParams)
	}
	if got.Spec.BuildID != 42 || got.Spec.Platform != PlatformWindows {
		t.Errorf("typed record context = %#v", got.Spec)
	}
	assertNoLegacyAliases(t, decoded)
}

func TestEncodeCustomTxtSettingsScope(t *testing.T) {
	for _, test := range []struct {
		name  string
		scope SettingsScope
		key   string
	}{
		{name: "default", scope: DefaultSettingsScope, key: "default-settings"},
		{name: "override", scope: OverrideSettingsScope, key: "override-settings"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeCustomBuild(map[string]any{
				"permissions_type": "view",
				"api_server":       "https://api.example",
			}, BuildRecordContext{Platform: "linux", AppName: "rustqs", SettingsScope: test.scope})
			if err != nil {
				t.Fatalf("NormalizeCustomBuild() error = %v", err)
			}
			decoded := decodeCustomTxt(t, got.CustomTxt)
			settings := nestedSettings(t, decoded, test.key)
			if settings["access-mode"] != "view" || settings["api-server"] != "https://api.example" {
				t.Errorf("%s = %#v", test.key, settings)
			}
			other := "default-settings"
			if test.key == other {
				other = "override-settings"
			}
			if _, ok := decoded[other]; ok {
				t.Errorf("unexpected %s object", other)
			}
		})
	}

	// Ordinary form parsing has no raw/manual scope input and defaults to default.
	encoded, err := BuildCustomTxtFromForm(map[string]any{"permissions_type": "custom"})
	if err != nil {
		t.Fatalf("BuildCustomTxtFromForm() error = %v", err)
	}
	if _, ok := decodeCustomTxt(t, encoded)["default-settings"]; !ok {
		t.Error("ordinary form must default to default-settings")
	}
	if _, ok := decodeCustomTxt(t, encoded)["app-name"]; ok {
		t.Error("no record context means no top-level app-name")
	}
}

func TestBuildCustomTxtBooleanSemantics(t *testing.T) {
	for _, test := range []struct {
		name    string
		raw     map[string]any
		key     string
		want    string
		wantErr bool
	}{
		{name: "true becomes Y", raw: map[string]any{"enable_keyboard": true}, key: "enable-keyboard", want: "Y"},
		{name: "false becomes N", raw: map[string]any{"enable_keyboard": false}, key: "enable-keyboard", want: "N"},
		{name: "absent remains absent", raw: map[string]any{"permissions_type": "custom"}, key: "enable-keyboard"},
		{name: "wrong type rejected", raw: map[string]any{"enable_keyboard": "false"}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := BuildCustomTxtFromForm(test.raw)
			if test.wantErr {
				if err == nil {
					t.Fatal("BuildCustomTxtFromForm() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildCustomTxtFromForm() error = %v", err)
			}
			settings := nestedSettings(t, decodeCustomTxt(t, encoded), "default-settings")
			got, ok := settings[test.key]
			if test.want == "" {
				if ok {
					t.Errorf("default-settings.%s = %v, want absent", test.key, got)
				}
				return
			}
			if !ok || got != test.want {
				t.Errorf("default-settings.%s = %v, want %q", test.key, got, test.want)
			}
		})
	}
}

func TestBuildCustomTxtOverrideBooleanMatrix(t *testing.T) {
	fields := []struct {
		input string
		key   string
	}{
		{"deny_lan", "enable-lan-discovery"},
		{"enable_direct_ip", "direct-server"},
		{"auto_close", "allow-auto-disconnect"},
		{"hide_cm", "allow-hide-cm"},
		{"remove_wallpaper", "allow-remove-wallpaper"},
		{"enable_remote_modi", "allow-remote-config-modification"},
		{"enable_keyboard", "enable-keyboard"},
		{"enable_clipboard", "enable-clipboard"},
		{"enable_file_transfer", "enable-file-transfer"},
		{"enable_audio", "enable-audio"},
		{"enable_tcp", "enable-tunnel"},
		{"enable_remote_restart", "enable-remote-restart"},
		{"enable_recording", "enable-record-session"},
		{"enable_blocking_input", "enable-block-input"},
		{"enable_printer", "enable-remote-printer"},
		{"enable_camera", "enable-camera"},
		{"enable_terminal", "enable-terminal"},
	}
	for _, value := range []bool{true, false} {
		raw := map[string]any{"permanent_password": "secret", "pass_approve_mode": "password"}
		for _, field := range fields {
			raw[field.input] = value
		}
		got, err := NormalizeCustomBuildJSON(mustJSON(t, raw), BuildRecordContext{
			Platform:      "windows",
			AppName:       "rustqs",
			SettingsScope: OverrideSettingsScope,
		})
		if err != nil {
			t.Fatalf("NormalizeCustomBuildJSON(%v) error = %v", value, err)
		}
		settings := nestedSettings(t, decodeCustomTxt(t, got.CustomTxt), "override-settings")
		for _, field := range fields {
			want := yn(value)
			if field.input == "deny_lan" {
				want = yn(!value)
			}
			if settings[field.key] != want {
				t.Errorf("override-settings.%s = %v, want %q for %s", field.key, settings[field.key], want, field.input)
			}
		}
	}
}

func TestBuildCustomTxtAbsentBooleanIsSystemDerived(t *testing.T) {
	encoded, err := BuildCustomTxtFromForm(map[string]any{"permissions_type": "custom"})
	if err != nil {
		t.Fatalf("BuildCustomTxtFromForm() error = %v", err)
	}
	settings := nestedSettings(t, decodeCustomTxt(t, encoded), "default-settings")
	if _, ok := settings["enable-lan-discovery"]; ok {
		t.Fatal("absent deny_lan must omit enable-lan-discovery; system defaults are not user-authored false")
	}
	if _, ok := settings["enable-keyboard"]; ok {
		t.Fatal("absent enable_keyboard must omit enable-keyboard; downstream derives its default")
	}
}

func TestBuildCustomTxtHideConnectionManagement(t *testing.T) {
	for _, test := range []struct {
		name    string
		raw     map[string]any
		want    map[string]any
		wantErr bool
	}{
		{
			name: "false uses both passwords",
			raw:  map[string]any{"hide_cm": false},
			want: map[string]any{"allow-hide-cm": "N", "verification-method": "use-both-passwords"},
		},
		{
			name: "true uses permanent password",
			raw:  map[string]any{"hide_cm": true, "permanent_password": "secret", "pass_approve_mode": "password"},
			want: map[string]any{"allow-hide-cm": "Y", "verification-method": "use-permanent-password", "approve-mode": "password"},
		},
		{
			name:    "true with password-click approve mode rejected",
			raw:     map[string]any{"hide_cm": true, "permanent_password": "secret", "pass_approve_mode": "password-click"},
			wantErr: true,
		},
		{
			name:    "true with click approve mode rejected",
			raw:     map[string]any{"hide_cm": true, "permanent_password": "secret", "pass_approve_mode": "click"},
			wantErr: true,
		},
		{
			name:    "true with both approve mode rejected",
			raw:     map[string]any{"hide_cm": true, "permanent_password": "secret", "pass_approve_mode": "both"},
			wantErr: true,
		},
		{
			name:    "true with absent approve mode rejected",
			raw:     map[string]any{"hide_cm": true, "permanent_password": "secret"},
			wantErr: true,
		},
		{
			name: "absent emits neither key",
			raw:  map[string]any{"permissions_type": "custom"},
			want: map[string]any{},
		},
		{
			name:    "true without password rejected",
			raw:     map[string]any{"hide_cm": true},
			wantErr: true,
		},
		{
			name:    "whitespace-only password rejected",
			raw:     map[string]any{"hide_cm": true, "permanent_password": " \t\n "},
			wantErr: true,
		},
		{
			name:    "wrong type rejected",
			raw:     map[string]any{"hide_cm": "false"},
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := BuildCustomTxtFromForm(test.raw)
			if test.wantErr {
				if err == nil {
					t.Fatal("BuildCustomTxtFromForm() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildCustomTxtFromForm() error = %v", err)
			}
			settings := nestedSettings(t, decodeCustomTxt(t, encoded), "default-settings")
			for key, want := range test.want {
				if got := settings[key]; got != want {
					t.Errorf("settings[%q] = %v, want %v", key, got, want)
				}
			}
			if len(test.want) == 0 {
				if _, ok := settings["allow-hide-cm"]; ok {
					t.Error("allow-hide-cm must be absent when hide_cm is absent")
				}
				if _, ok := settings["verification-method"]; ok {
					t.Error("verification-method must be absent when hide_cm is absent")
				}
			}
		})
	}
}

func TestNormalizeCustomBuildPreservesNonEmptyPermanentPassword(t *testing.T) {
	const password = " authored password "
	normalized, err := NormalizeCustomBuild(map[string]any{
		"hide_cm":            true,
		"permanent_password": password,
		"pass_approve_mode":  "password",
	}, BuildRecordContext{Platform: "linux", AppName: "rustqs", Version: "1.2.3"})
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() error = %v", err)
	}
	if normalized.Spec.PermanentPassword != password {
		t.Fatalf("normalized password = %q, want authored content %q", normalized.Spec.PermanentPassword, password)
	}
	persisted := decodeJSON(t, normalized.PersistedJSON)
	if persisted["permanent_password"] != password {
		t.Fatalf("persisted password = %#v, want authored content %q", persisted["permanent_password"], password)
	}
}

func TestNormalizeCustomBuildEndpointPreservation(t *testing.T) {
	for _, endpoint := range []string{
		"relay.example",
		"relay.example:21117",
		"192.0.2.10",
		"192.0.2.10:21117",
		"2001:db8::10",
		"[2001:db8::10]:21117",
	} {
		t.Run(endpoint, func(t *testing.T) {
			got, err := NormalizeCustomBuild(map[string]any{
				"server_ip":      endpoint,
				"relay_server":   endpoint,
				"android_app_id": "com.example.rustqs",
			}, BuildRecordContext{Platform: "android", AppName: "rustqs"})
			if err != nil {
				t.Fatalf("NormalizeCustomBuild() error = %v", err)
			}
			if got.DispatchParams["server"] != endpoint {
				t.Errorf("server = %v, want literal %q", got.DispatchParams["server"], endpoint)
			}
			settings := nestedSettings(t, decodeCustomTxt(t, got.CustomTxt), "default-settings")
			if settings["relay-server"] != endpoint {
				t.Errorf("relay-server = %v, want literal %q", settings["relay-server"], endpoint)
			}
		})
	}
}

func TestAndroidAppIDIsTypedAndroidOnlyAndRequiredBeforeDispatch(t *testing.T) {
	normalized, err := NormalizeCustomBuild(map[string]any{
		"android_app_id": "com.example.rustqs",
		"enable_audio":   true,
	}, BuildRecordContext{Platform: string(PlatformAndroid), AppName: "rustqs", Version: "1.2.3"})
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() error = %v", err)
	}
	if got := normalized.DispatchParams["android_app_id"]; got != "com.example.rustqs" {
		t.Fatalf("android dispatch identity = %#v, want typed package identifier", got)
	}
	if _, ok := normalized.DispatchParams["custom_txt"].(NormalizedCustomTxt); !ok {
		t.Fatal("generated custom_txt did not retain typed dispatch boundary")
	}
	encrypted, err := (&GithubBuildConfigService{}).EncryptPayload("android-contract-key", normalized.DispatchParams)
	if err != nil {
		t.Fatalf("EncryptPayload() error = %v", err)
	}
	decrypted, err := (&GithubBuildConfigService{}).DecryptPayload("android-contract-key", encrypted)
	if err != nil {
		t.Fatalf("DecryptPayload() error = %v", err)
	}
	if decrypted["android_app_id"] != "com.example.rustqs" {
		t.Fatalf("DFP1 android_app_id = %#v, want typed package identifier", decrypted["android_app_id"])
	}

	for _, tc := range []struct {
		name string
		id   string
	}{
		{name: "missing", id: ""},
		{name: "single segment", id: "com"},
		{name: "uppercase", id: "Com.Example.App"},
		{name: "traversal", id: "com.example/escape"},
		{name: "empty segment", id: "com.example..app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.id == "" {
				if err := ValidateAndroidAppID(tc.id); err == nil {
					t.Fatal("ValidateAndroidAppID() accepted a missing package identifier")
				}
				return
			}
			_, err := NormalizeCustomBuild(map[string]any{"android_app_id": tc.id}, BuildRecordContext{
				Platform: string(PlatformAndroid), AppName: "rustqs", Version: "1.2.3",
			})
			if err == nil {
				t.Fatalf("NormalizeCustomBuild() accepted invalid android_app_id %q", tc.id)
			}
		})
	}

	if _, err := NormalizeWorkflowDispatchParams(string(PlatformAndroid), map[string]any{}); err == nil {
		t.Fatal("NormalizeWorkflowDispatchParams() accepted Android dispatch without identity")
	}
	noL2, err := NormalizeWorkflowDispatchParams(string(PlatformAndroid), map[string]any{
		"android_app_id": "com.example.rustqs",
	})
	if err != nil {
		t.Fatalf("NormalizeWorkflowDispatchParams() rejected valid no-L2 Android dispatch: %v", err)
	}
	if _, ok := noL2["custom_txt"]; ok {
		t.Fatal("no-L2 Android dispatch manufactured a raw custom_txt field")
	}
	if _, err := NormalizeWorkflowDispatchParams(string(PlatformLinux), map[string]any{
		"android_app_id": "com.example.rustqs",
	}); err == nil {
		t.Fatal("non-Android dispatch accepted android_app_id")
	}
}

func TestNormalizeCustomBuildRequiresAndroidAppIDBeforePersistence(t *testing.T) {
	for _, platform := range []Platform{PlatformWindows, PlatformLinux} {
		t.Run(string(platform), func(t *testing.T) {
			if _, err := NormalizeCustomBuild(map[string]any{}, BuildRecordContext{
				Platform: string(platform), AppName: "rustqs", Version: "1.2.3",
			}); err != nil {
				t.Fatalf("NormalizeCustomBuild() error = %v, want platform without Android identity to remain valid", err)
			}
		})
	}

	_, err := NormalizeCustomBuild(map[string]any{}, BuildRecordContext{
		Platform: string(PlatformAndroid), AppName: "rustqs", Version: "1.2.3",
	})
	if err == nil || err.Error() != "android_app_id is required for Android builds" {
		t.Fatalf("NormalizeCustomBuild() error = %v, want required Android identity validation", err)
	}
}

func TestNormalizeCustomBuildAPIURLValidation(t *testing.T) {
	for _, value := range []string{"https://api.example", "https://api.example:21114/api"} {
		t.Run(value, func(t *testing.T) {
			got, err := NormalizeCustomBuild(map[string]any{"api_server": value}, BuildRecordContext{Platform: "linux", AppName: "rustqs"})
			if err != nil {
				t.Fatalf("NormalizeCustomBuild() error = %v", err)
			}
			settings := nestedSettings(t, decodeCustomTxt(t, got.CustomTxt), "default-settings")
			if settings["api-server"] != value {
				t.Errorf("api-server = %v, want literal %q", settings["api-server"], value)
			}
		})
	}
	for _, value := range []string{"api.example", "api://example", "https://"} {
		t.Run("invalid-"+value, func(t *testing.T) {
			if _, err := NormalizeCustomBuild(map[string]any{"api_server": value}, BuildRecordContext{Platform: "linux", AppName: "rustqs"}); err == nil {
				t.Errorf("NormalizeCustomBuild(%q) error = nil, want error", value)
			}
		})
	}
}

func TestNormalizeCustomBuildValidation(t *testing.T) {
	for _, test := range []struct {
		name    string
		context BuildRecordContext
		raw     map[string]any
	}{
		{name: "unknown platform", context: BuildRecordContext{Platform: "macos"}},
		{name: "missing platform", context: BuildRecordContext{}},
		{name: "invalid hostname", context: BuildRecordContext{Platform: "windows"}, raw: map[string]any{"server_ip": "bad host"}},
		{name: "missing hostname", context: BuildRecordContext{Platform: "windows"}, raw: map[string]any{"server_ip": ":21116"}},
		{name: "invalid port", context: BuildRecordContext{Platform: "linux"}, raw: map[string]any{"relay_server": "relay.example:not-a-port"}},
		{name: "zero port", context: BuildRecordContext{Platform: "linux"}, raw: map[string]any{"relay_server": "relay.example:0"}},
		{name: "endpoint URL", context: BuildRecordContext{Platform: "linux"}, raw: map[string]any{"relay_server": "https://relay.example"}},
		{name: "bracketed hostname", context: BuildRecordContext{Platform: "linux"}, raw: map[string]any{"relay_server": "[relay.example]:21117"}},
		{name: "invalid direction", context: BuildRecordContext{Platform: "linux"}, raw: map[string]any{"direction": "sideways"}},
		{name: "invalid permissions type", context: BuildRecordContext{Platform: "linux"}, raw: map[string]any{"permissions_type": "admin"}},
		{name: "invalid theme", context: BuildRecordContext{Platform: "linux"}, raw: map[string]any{"theme": "blue"}},
		{name: "invalid approve mode", context: BuildRecordContext{Platform: "linux"}, raw: map[string]any{"pass_approve_mode": "auto"}},
		{name: "wrong endpoint type", context: BuildRecordContext{Platform: "linux"}, raw: map[string]any{"server_ip": 21116}},
		{name: "wrong URL type", context: BuildRecordContext{Platform: "linux"}, raw: map[string]any{"api_server": false}},
		{name: "invalid scope", context: BuildRecordContext{Platform: "linux", SettingsScope: "manual"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeCustomBuild(test.raw, test.context); err == nil {
				t.Fatal("NormalizeCustomBuild() error = nil, want error")
			}
		})
	}
}

func TestNormalizeCustomBuildRejectsRawCustomTxt(t *testing.T) {
	if _, err := NormalizeCustomBuildJSON(`{"custom_txt":"not-forwarded"}`, BuildRecordContext{Platform: "windows"}); err == nil {
		t.Fatal("NormalizeCustomBuildJSON() error = nil, want raw custom_txt rejection")
	}
	if _, err := BuildCustomTxtFromForm(map[string]any{"custom_txt": "not-forwarded"}); err == nil {
		t.Fatal("BuildCustomTxtFromForm() error = nil, want raw custom_txt rejection")
	}
}

func TestNormalizeCustomBuildRecordContextAndUnsupportedFields(t *testing.T) {
	got, err := NormalizeCustomBuildJSON(`{"server_ip":"id.example:21116","app_name":"raw-app","version":"raw-version","permissions_type":"custom"}`, BuildRecordContext{
		BuildID:  7,
		Platform: "windows",
		AppName:  "record-app",
		Version:  "record-version",
	})
	if err != nil {
		t.Fatalf("NormalizeCustomBuildJSON() error = %v", err)
	}
	decoded := decodeCustomTxt(t, got.CustomTxt)
	if got.DispatchParams["server"] != "id.example:21116" || got.DispatchParams["app_name"] != "record-app" || got.DispatchParams["version"] != "record-version" {
		t.Errorf("dispatch params = %#v", got.DispatchParams)
	}
	for _, key := range []string{"app_name", "version", "server_ip", "key"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("%s leaked into custom JSON", key)
		}
	}
}

func TestNormalizeCustomBuildRejectsUnknownPersistedFields(t *testing.T) {
	for _, raw := range []string{
		`{"future_field":"value"}`,
		`{"legacy_variant":{"value":true}}`,
		`{"custom_txt":"raw"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := NormalizeCustomBuildJSON(raw, BuildRecordContext{Platform: "windows"}); err == nil {
				t.Fatal("NormalizeCustomBuildJSON() error = nil, want fail-closed rejection")
			}
		})
	}
}

func TestNormalizedBuildPersistedJSONIsCanonicalFormJSON(t *testing.T) {
	got, err := NormalizeCustomBuildJSON(`{"server_ip":"id.example:21116","key":"`+validRustDeskPublicKey+`","api_server":"https://api.example","enable_audio":false,"enable_terminal":true,"app_name":"raw-app","version":"raw-version","platform":"raw-platform"}`, BuildRecordContext{
		BuildID:  99,
		Platform: "linux",
		AppName:  "record-app",
		Version:  "record-version",
	})
	if err != nil {
		t.Fatalf("NormalizeCustomBuildJSON() error = %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal([]byte(got.PersistedJSON), &persisted); err != nil {
		t.Fatalf("PersistedJSON is not JSON: %v", err)
	}
	if persisted["api_server"] != "https://api.example" {
		t.Fatalf("known values not preserved: %#v", persisted)
	}
	if got.DispatchParams["server"] != "id.example:21116" || got.DispatchParams["key"] != validRustDeskPublicKey {
		t.Fatalf("L1 endpoint/key dispatch values not preserved: %#v", got.DispatchParams)
	}
	if persisted["server_ip"] != "id.example:21116" || persisted["key"] != validRustDeskPublicKey {
		t.Fatalf("L1 endpoint/key persisted values not preserved: %#v", persisted)
	}
	if persisted["enable_audio"] != false || persisted["enable_terminal"] != true {
		t.Fatalf("typed bool values not preserved: %#v", persisted)
	}
	for _, key := range []string{"app_name", "version", "platform", "build_id", "custom_txt"} {
		if _, ok := persisted[key]; ok {
			t.Errorf("excluded field %q present in persisted JSON: %#v", key, persisted)
		}
	}
	if got.PersistedJSON == got.CustomTxt {
		t.Fatal("PersistedJSON must be form-field JSON, not base64 custom_.txt")
	}
}

func TestPersistedJSONPreservesAuthoredEmptyStringsOnly(t *testing.T) {
	got, err := NormalizeCustomBuild(map[string]any{
		"server_ip":    "",
		"key":          "",
		"company_name": "",
		"api_server":   "",
	}, BuildRecordContext{Platform: "windows", AppName: "rustqs"})
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() error = %v", err)
	}
	persisted := decodeJSON(t, got.PersistedJSON)
	for _, key := range []string{"server_ip", "key", "company_name", "api_server"} {
		value, ok := persisted[key]
		if !ok || value != "" {
			t.Errorf("persisted[%q] = %#v, present = %v, want authored empty string", key, value, ok)
		}
	}
	if _, ok := persisted["relay_server"]; ok {
		t.Error("absent relay_server was serialized")
	}
}

func TestCanonicalPersistedPRESETFieldsAndCustomTxtBoundary(t *testing.T) {
	// The operational api-server is an internal address; the public origin is
	// the one used to resolve local /upload/... assets for the build runner.
	setBrandingConfigForTest(t, "http://localhost:21114", "https://deskforge.example")
	raw := map[string]any{
		"server_ip":                "id.example:21116",
		"key":                      validRustDeskPublicKey,
		"api_server":               "https://api.example",
		"relay_server":             "relay.example:21117",
		"company_name":             "DeskForge",
		"download_url":             "https://download.example/client",
		"website_url":              "https://www.example.com",
		"privacy_policy_url":       "https://www.example.com/privacy",
		"direction":                "incoming",
		"pass_approve_mode":        "password-click",
		"permanent_password":       "secret",
		"deny_lan":                 false,
		"enable_direct_ip":         true,
		"auto_close":               false,
		"hide_cm":                  false,
		"theme":                    "dark",
		"remove_wallpaper":         true,
		"remove_new_version_notif": false,
		"permissions_type":         "full",
		"enable_keyboard":          false,
		"enable_clipboard":         true,
		"enable_file_transfer":     false,
		"enable_audio":             true,
		"enable_tcp":               false,
		"enable_remote_restart":    true,
		"enable_recording":         false,
		"enable_blocking_input":    true,
		"enable_remote_modi":       false,
		"enable_printer":           true,
		"enable_camera":            false,
		"enable_terminal":          true,
		"cycle_monitor":            true,
		"x_offline":                false,
		"android_app_id":           "com.example.client",
		"app_icon_url":             "/upload/icon.png",
		"app_logo_url":             "/upload/logo.png",
		"privacy_screen_url":       "/upload/privacy.png",
	}
	got, err := NormalizeCustomBuild(raw, BuildRecordContext{Platform: "windows", AppName: "rustqs"})
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() error = %v", err)
	}

	persisted := decodeJSON(t, got.PersistedJSON)
	for key, want := range raw {
		if got := persisted[key]; !reflect.DeepEqual(got, want) {
			t.Errorf("persisted[%q] = %#v, want %#v", key, got, want)
		}
	}
	for _, key := range []string{
		"server_ip", "key", "company_name", "download_url", "website_url", "privacy_policy_url",
		"remove_new_version_notif",
		"cycle_monitor", "x_offline", "android_app_id", "app_icon_url", "app_logo_url",
		"privacy_screen_url",
	} {
		if _, ok := decodeCustomTxt(t, got.CustomTxt)[key]; ok {
			t.Errorf("L1/persisted-only field %q entered custom_.txt", key)
		}
	}
	assertNoKeys(t, decodeCustomTxt(t, got.CustomTxt), []string{
		"server_ip", "key", "company_name", "download_url", "website_url", "privacy_policy_url", "remove_new_version_notif",
		"cycle_monitor", "x_offline", "android_app_id", "app_icon_url", "app_logo_url",
		"privacy_screen_url",
	})
	// Persisted-only image-URL branding fields now also dispatch to the
	// workflow build steps while staying out of custom_.txt. The dispatch value
	// is the stored relative path resolved against the public origin, never the
	// operational api-server.
	for key, relative := range map[string]string{
		"app_icon_url":       "/upload/icon.png",
		"app_logo_url":       "/upload/logo.png",
		"privacy_screen_url": "/upload/privacy.png",
	} {
		if got := got.DispatchParams[key]; got != "https://deskforge.example"+relative {
			t.Errorf("dispatch[%q] = %v, want %q", key, got, "https://deskforge.example"+relative)
		}
	}
	if got.DispatchParams["server"] != raw["server_ip"] || got.DispatchParams["key"] != raw["key"] {
		t.Fatalf("dispatch params did not use normalized L1 values: %#v", got.DispatchParams)
	}
}

func TestParseBuildSpecRejectsInvalidTypesForEveryTypedField(t *testing.T) {
	for _, field := range buildSpecStringFields {
		t.Run(field.name+" string", func(t *testing.T) {
			if _, err := NormalizeCustomBuild(map[string]any{field.name: 123}, BuildRecordContext{Platform: "linux"}); err == nil {
				t.Fatalf("NormalizeCustomBuild() error = nil for invalid %s type", field.name)
			}
		})
	}
	for _, field := range buildSpecBoolFields {
		t.Run(field.name+" bool", func(t *testing.T) {
			if _, err := NormalizeCustomBuild(map[string]any{field.name: "false"}, BuildRecordContext{Platform: "linux"}); err == nil {
				t.Fatalf("NormalizeCustomBuild() error = nil for invalid %s type", field.name)
			}
		})
	}
}

func TestNormalizeCustomBuildJSONRejectsNonObject(t *testing.T) {
	if _, err := NormalizeCustomBuildJSON(`[]`, BuildRecordContext{Platform: "windows"}); err == nil {
		t.Fatal("NormalizeCustomBuildJSON() error = nil, want error")
	}
}

func decodeCustomTxt(t *testing.T, encoded string) map[string]any {
	t.Helper()
	decodedJSON, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode custom_.txt: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(decodedJSON, &decoded); err != nil {
		t.Fatalf("unmarshal custom_.txt: %v", err)
	}
	return decoded
}

func decodeJSON(t *testing.T, value string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}
	return decoded
}

func assertNoKeys(t *testing.T, value map[string]any, forbidden []string) {
	t.Helper()
	for key, child := range value {
		for _, forbiddenKey := range forbidden {
			if key == forbiddenKey {
				t.Errorf("forbidden key %q emitted", key)
			}
		}
		if nested, ok := child.(map[string]any); ok {
			assertNoKeys(t, nested, forbidden)
		}
	}
}

func mustJSON(t *testing.T, value map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(encoded)
}

func nestedSettings(t *testing.T, decoded map[string]any, name string) map[string]any {
	t.Helper()
	value, ok := decoded[name].(map[string]any)
	if !ok {
		t.Fatalf("%s is %T, want object", name, decoded[name])
	}
	return value
}

func assertNoLegacyAliases(t *testing.T, value any) {
	t.Helper()
	legacy := []string{
		"permissions-mode", "deny-lan-discovery", "enable-printer",
		"remove-wallpaper", "hide-connection-management", "auto-close-incoming-sessions",
		"custom-rendezvous-api-server", "allow_remote_config_modification", "disable_update",
		"direction", "server_ip", "key", "app_name", "version", "allow-darktheme",
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			for _, alias := range legacy {
				if key == alias {
					t.Errorf("legacy/internal alias %q emitted", alias)
				}
			}
			assertNoLegacyAliases(t, child)
		}
	case []any:
		for _, child := range typed {
			assertNoLegacyAliases(t, child)
		}
	case string:
		if strings.Contains(typed, "permissions-mode") {
			t.Errorf("legacy alias leaked into value %q", typed)
		}
	}
}

func TestNormalizeCustomBuildDispatchesBrandingContext(t *testing.T) {
	context := BuildRecordContext{Platform: "windows", AppName: "rustqs", Version: "1.2.3"}

	// The operational api-server is an internal address (the stock deployment
	// hardcodes http://localhost:21114). It must never be used as the asset
	// origin, so with no public origin configured a server-relative asset fails
	// closed at the typed boundary with a field error instead of producing a
	// payload with the loopback base.
	setBrandingConfigForTest(t, "http://127.0.0.1:21114", "")
	for _, field := range []string{"app_icon_url", "app_logo_url", "privacy_screen_url"} {
		_, err := NormalizeCustomBuild(map[string]any{field: "/upload/asset.png"}, context)
		if err == nil {
			t.Fatalf("NormalizeCustomBuild(%s relative, no public origin) error = nil, want error", field)
		}
		f, code, ok := FieldErrorMetadata(err)
		if !ok || f != field || code != FieldCodeInvalidFormat {
			t.Fatalf("field/code = %s/%s ok=%v, want %s/%s (err=%v)", f, code, ok, field, FieldCodeInvalidFormat, err)
		}
	}

	// Empty branding values are omitted so provider-side defaults stay intact.
	empty, err := NormalizeCustomBuild(map[string]any{"enable_audio": true}, context)
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() error = %v", err)
	}
	for _, key := range []string{"company_name", "download_url", "website_url", "privacy_policy_url", "app_icon_url", "app_logo_url", "privacy_screen_url"} {
		if _, ok := empty.DispatchParams[key]; ok {
			t.Errorf("empty branding field %q was emitted to dispatch", key)
		}
	}

	// With the public origin configured, server-relative image values are
	// resolved against it in the dispatch payload so the runner can fetch them;
	// persisted custom_json keeps the stored relative value. The operational
	// api-server stays unused.
	setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
	normalized, err := NormalizeCustomBuild(map[string]any{
		"company_name":       "Acme Ltd",
		"download_url":       "https://download.example/app",
		"website_url":        "https://www.example.com",
		"privacy_policy_url": "https://www.example.com/privacy",
		"app_icon_url":       "/upload/icon.png",
		"app_logo_url":       "/upload/logo.png",
		"privacy_screen_url": "/upload/privacy.png",
		"enable_audio":       true,
	}, context)
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() error = %v", err)
	}
	for key, want := range map[string]any{
		"company_name":       "Acme Ltd",
		"download_url":       "https://download.example/app",
		"website_url":        "https://www.example.com",
		"privacy_policy_url": "https://www.example.com/privacy",
		"app_icon_url":       "https://deskforge.example/upload/icon.png",
		"app_logo_url":       "https://deskforge.example/upload/logo.png",
		"privacy_screen_url": "https://deskforge.example/upload/privacy.png",
	} {
		if got := normalized.DispatchParams[key]; got != want {
			t.Errorf("dispatch[%q] = %v, want %q", key, got, want)
		}
	}
	for key, stored := range map[string]string{
		"app_icon_url":       "/upload/icon.png",
		"app_logo_url":       "/upload/logo.png",
		"privacy_screen_url": "/upload/privacy.png",
	} {
		if got := decodeJSON(t, normalized.PersistedJSON)[key]; got != stored {
			t.Errorf("persisted[%q] = %v, want the stored relative value %q", key, got, stored)
		}
	}

	// Forged caller values cannot bypass the typed boundary.
	if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{
		"company_name": 123,
	}); err == nil {
		t.Fatal("NormalizeWorkflowDispatchParams() accepted non-string company_name")
	}
	if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{
		"website_url": "not a url",
	}); err == nil {
		t.Fatal("NormalizeWorkflowDispatchParams() accepted invalid website_url")
	}
	if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{
		"app_icon_url": "line\nbreak",
	}); err == nil {
		t.Fatal("NormalizeWorkflowDispatchParams() accepted unsafe app_icon_url")
	}

	providerParams, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), normalized.DispatchParams)
	if err != nil {
		t.Fatalf("NormalizeWorkflowDispatchParams() rejected typed branding params: %v", err)
	}
	for key, want := range map[string]any{
		"company_name":       "Acme Ltd",
		"website_url":        "https://www.example.com",
		"privacy_policy_url": "https://www.example.com/privacy",
		"app_icon_url":       "https://deskforge.example/upload/icon.png",
		"app_logo_url":       "https://deskforge.example/upload/logo.png",
		"privacy_screen_url": "https://deskforge.example/upload/privacy.png",
	} {
		if got := providerParams[key]; got != want {
			t.Errorf("provider params[%q] = %v, want %q", key, got, want)
		}
	}

	// Already-absolute public http(s) values stay untouched.
	absolute, err := NormalizeCustomBuild(map[string]any{
		"app_icon_url": "https://cdn.example/i.png",
	}, context)
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() with absolute image URL error = %v", err)
	}
	if got := absolute.DispatchParams["app_icon_url"]; got != "https://cdn.example/i.png" {
		t.Errorf("dispatch[app_icon_url] = %v, want untouched absolute URL", got)
	}

	// The dispatch gate re-resolves forged relative values against the public
	// origin too, and stays idempotent for provider-derived absolute params.
	forged, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{
		"app_icon_url": "/upload/forged.png",
	})
	if err != nil {
		t.Fatalf("NormalizeWorkflowDispatchParams() rejected forged relative image URL: %v", err)
	}
	if got := forged["app_icon_url"]; got != "https://deskforge.example/upload/forged.png" {
		t.Errorf("forged dispatch[app_icon_url] = %v, want absolutized value", got)
	}
	gated, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), normalized.DispatchParams)
	if err != nil {
		t.Fatalf("NormalizeWorkflowDispatchParams() rejected resolved params: %v", err)
	}
	for key, want := range map[string]any{
		"app_icon_url":       "https://deskforge.example/upload/icon.png",
		"app_logo_url":       "https://deskforge.example/upload/logo.png",
		"privacy_screen_url": "https://deskforge.example/upload/privacy.png",
	} {
		if got := gated[key]; got != want {
			t.Errorf("gated params[%q] = %v, want %q", key, got, want)
		}
	}

	// A config change between create and dispatch cannot emit a bad URL: with no
	// public origin the dispatch gate refuses a relative asset too.
	setBrandingConfigForTest(t, "http://127.0.0.1:21114", "")
	if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{
		"app_icon_url": "/upload/relative.png",
	}); err == nil {
		t.Fatal("NormalizeWorkflowDispatchParams() accepted a relative image with no public origin")
	}

	// Existing string safety gates still run before resolution, even with a
	// configured public origin.
	if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{
		"app_icon_url": "line\nbreak",
	}); err == nil {
		t.Fatal("NormalizeWorkflowDispatchParams() accepted unsafe app_icon_url with configured public origin")
	}
}

// setBrandingConfigForTest pins the service package config so branding image
// resolution sees the operational api-server and/or the public origin under
// test; the package global is restored on cleanup.
func setBrandingConfigForTest(t *testing.T, apiServer, publicAPIServer string) {
	t.Helper()
	original := Config
	Config = &config.Config{Rustdesk: config.Rustdesk{ApiServer: apiServer, PublicApiServer: publicAPIServer}}
	t.Cleanup(func() { Config = original })
}

func TestHideCmIncompatibleApproveModeIsExplicitClientError(t *testing.T) {
	for _, approve := range []string{"", "password-click", "click", "both"} {
		t.Run("approve="+approve, func(t *testing.T) {
			raw := `{"hide_cm":true,"permanent_password":"secret"`
			if approve != "" {
				raw += `,"pass_approve_mode":"` + approve + `"`
			}
			raw += `}`
			err := ValidateCustomBuildInput("windows", raw, "rustqs", "1.2.3")
			if !IsClientValidationError(err) {
				t.Fatalf("ValidateCustomBuildInput() error = %v, want ClientValidationError", err)
			}
			field, code, ok := FieldErrorMetadata(err)
			if !ok || field != "pass_approve_mode" || code != FieldCodeIncompatible {
				t.Fatalf("FieldErrorMetadata() = (%q, %q, %v), want pass_approve_mode/%s", field, code, ok, FieldCodeIncompatible)
			}
		})
	}
}

func TestNormalizeCustomBuildBrandFieldsPresetRestoreRoundTrip(t *testing.T) {
	first, err := NormalizeCustomBuild(map[string]any{
		"server_ip":                "id.example:21116",
		"key":                      validRustDeskPublicKey,
		"company_name":             "Acme Ltd",
		"download_url":             "https://download.example/app",
		"website_url":              "https://www.example.com",
		"privacy_policy_url":       "https://www.example.com/privacy",
		"hide_cm":                  true,
		"permanent_password":       "secret",
		"pass_approve_mode":        "password",
		"remove_new_version_notif": true,
	}, BuildRecordContext{Platform: "windows", AppName: "rustqs", Version: "1.2.3"})
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() error = %v", err)
	}

	// Preset restore/rebuild path: the persisted form restores the same spec.
	restored, err := NormalizeCustomBuildJSON(first.PersistedJSON, BuildRecordContext{
		Platform: "windows", AppName: "rustqs", Version: "1.2.3",
	})
	if err != nil {
		t.Fatalf("NormalizeCustomBuildJSON(persisted) error = %v", err)
	}
	if restored.CustomTxt != first.CustomTxt {
		t.Error("restored custom_txt differs from the original build")
	}
	if got, want := restored.DispatchParams, first.DispatchParams; len(got) != len(want) {
		t.Fatalf("restored dispatch params = %#v, want %#v", got, want)
	} else {
		for key, wantValue := range want {
			if gotValue, ok := got[key]; !ok || !reflect.DeepEqual(gotValue, wantValue) {
				t.Errorf("restored dispatch[%q] = %#v, want %#v", key, gotValue, wantValue)
			}
		}
	}

	// New branding fields that were unknown before 284 keep failing closed.
	if _, err := NormalizeCustomBuildJSON(`{"website_url":123}`, BuildRecordContext{Platform: "windows", AppName: "rustqs"}); err == nil {
		t.Fatal("NormalizeCustomBuildJSON() accepted non-string website_url")
	}
}

func TestValidateStoredCustomBrandingField(t *testing.T) {
	// The migration preflight delegates to this single seam so the branding
	// field set and its rules cannot drift from the typed/dispatch boundaries.
	// The seam validates stored data only and never consults a public origin, so
	// no origin is configured here: a server-relative image must still pass.
	wantFields := []string{
		"company_name", "download_url", "website_url", "privacy_policy_url",
		"app_icon_url", "app_logo_url", "privacy_screen_url",
	}
	if got := CustomBrandingFieldNames(); !reflect.DeepEqual(got, wantFields) {
		t.Fatalf("CustomBrandingFieldNames() = %v, want %v", got, wantFields)
	}
	// The returned list must be a copy so a caller cannot mutate the domain.
	fields := CustomBrandingFieldNames()
	fields[0] = "mutated"
	if got := CustomBrandingFieldNames()[0]; got != "company_name" {
		t.Fatalf("CustomBrandingFieldNames() leaked mutable state: got %q", got)
	}

	for _, tc := range []struct {
		name    string
		field   string
		value   any
		wantErr bool
	}{
		{"valid-download", "download_url", "https://download.example/app", false},
		{"valid-website", "website_url", "https://www.example.com", false},
		{"valid-privacy", "privacy_policy_url", "https://www.example.com/privacy", false},
		{"valid-company", "company_name", "Acme Ltd", false},
		{"valid-image-relative", "app_icon_url", "/upload/icon.png", false},
		{"valid-image-absolute", "app_icon_url", "https://cdn.example/i.png", false},
		{"empty-accepted", "company_name", "", false},
		{"relative-download", "download_url", "example.com/app", true},
		{"mixed-case-scheme", "download_url", "HTTP://x", true},
		{"punctuation-host", "privacy_policy_url", "https://.", true},
		{"over-length-url", "website_url", "https://x.example/" + strings.Repeat("a", 2049), true},
		// The seam is the union of the typed-boundary and dispatch-gate rules:
		// a C1 control character (U+0086) is neither whitespace nor rejected by
		// url.ParseRequestURI, so the URL text fields must also carry the
		// dispatch control-character rule or the migration would pass a row the
		// dispatch gate later rejects with a non-field error.
		{"website-url-c1-control", "website_url", "https://example.com/\u0086", true},
		{"download-url-c1-control", "download_url", "https://download.example/\u0086", true},
		{"privacy-url-c1-control", "privacy_policy_url", "https://www.example.com/\u0086", true},
		{"company-control-char", "company_name", "Acme\nLtd", true},
		{"company-over-length", "company_name", strings.Repeat("x", 257), true},
		{"image-control-char", "app_logo_url", "bad\nvalue", true},
		// The image keys carry the same public-URL rule as the producer for a
		// non-relative value: a non-http(s) scheme, a host-less URL, and a
		// literal loopback/private host are refused here so the migration fails
		// closed on a row the producer would refuse. A server-relative value is
		// valid stored data and passes without a configured public origin.
		{"image-file-scheme", "app_icon_url", "file:///etc/passwd", true},
		{"image-missing-host", "app_logo_url", "https:///x.png", true},
		{"image-loopback-host", "privacy_screen_url", "http://127.0.0.1:21114/x.png", true},
		{"non-string", "download_url", 123, true},
		{"explicit-null", "company_name", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStoredCustomBrandingField(tc.field, tc.value)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateStoredCustomBrandingField(%q, %v) = nil, want error", tc.field, tc.value)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateStoredCustomBrandingField(%q, %v) = %v, want accepted", tc.field, tc.value, err)
			}
		})
	}

	// A stored relative image is valid data on its own: the seam must accept it
	// with no public origin configured, because resolving it is the producer's
	// concern. An absolute public image is accepted without an origin too.
	if err := ValidateStoredCustomBrandingField("app_icon_url", "/upload/icon.png"); err != nil {
		t.Fatalf("ValidateStoredCustomBrandingField rejected a stored relative image with no public origin: %v", err)
	}
	if err := ValidateStoredCustomBrandingField("app_icon_url", "https://cdn.example/i.png"); err != nil {
		t.Fatalf("ValidateStoredCustomBrandingField rejected an absolute image without a public origin: %v", err)
	}
	// A stored image that is data-invalid independent of any origin still fails
	// closed with no origin configured: a protocol-relative path, embedded
	// whitespace, and an over-length value are refused.
	if err := ValidateStoredCustomBrandingField("app_icon_url", "//cdn.example/i.png"); err == nil {
		t.Fatal("ValidateStoredCustomBrandingField accepted a protocol-relative image path")
	}
	if err := ValidateStoredCustomBrandingField("app_icon_url", "/upload/a b.png"); err == nil {
		t.Fatal("ValidateStoredCustomBrandingField accepted a relative image with whitespace")
	}
	if err := ValidateStoredCustomBrandingField("app_icon_url", "/upload/"+strings.Repeat("a", 2049)); err == nil {
		t.Fatal("ValidateStoredCustomBrandingField accepted an over-length relative image")
	}

	// A non-branding field is outside this seam's domain; callers must not use
	// it to inspect unrelated legacy fields.
	if err := ValidateStoredCustomBrandingField("server_ip", "not a valid host"); err == nil {
		t.Fatal("ValidateStoredCustomBrandingField accepted a non-branding field")
	}
}

func TestNormalizeCustomBuildDownloadURLValidation(t *testing.T) {
	// download_url reaches the Windows fork build workflow, which requires an
	// absolute http(s) URL. The typed boundary must reject the same values the
	// fork rejects, with the same field-level invalid_format contract as the
	// sibling website_url/privacy_policy_url branding URLs.
	context := BuildRecordContext{Platform: "windows", AppName: "rustqs", Version: "1.2.3"}
	for _, value := range []string{"https://download.example/app", "http://download.example/app"} {
		t.Run("valid-"+value, func(t *testing.T) {
			got, err := NormalizeCustomBuild(map[string]any{"download_url": value}, context)
			if err != nil {
				t.Fatalf("NormalizeCustomBuild() error = %v, want accepted", err)
			}
			if got.DispatchParams["download_url"] != value {
				t.Errorf("dispatch[download_url] = %v, want %q", got.DispatchParams["download_url"], value)
			}
			if persisted := decodeJSON(t, got.PersistedJSON)["download_url"]; persisted != value {
				t.Errorf("persisted[download_url] = %v, want %q", persisted, value)
			}
		})
	}

	for _, value := range []string{"/upload/client.exe", "download.example/app", "ftp://download.example/app"} {
		t.Run("invalid-"+value, func(t *testing.T) {
			custom := mustJSON(t, map[string]any{
				"server_ip":    "id.example:21116",
				"key":          validRustDeskPublicKey,
				"api_server":   "https://api.example",
				"relay_server": "relay.example:21117",
				"download_url": value,
			})
			err := ValidateCustomBuildInput("windows", custom, "rustqs", "1.2.3")
			if err == nil {
				t.Fatalf("ValidateCustomBuildInput() accepted download_url %q", value)
			}
			field, code, ok := FieldErrorMetadata(err)
			if !ok || field != "download_url" || code != FieldCodeInvalidFormat {
				t.Fatalf("field/code = %s/%s ok=%v, want download_url/%s (err=%v)", field, code, ok, FieldCodeInvalidFormat, err)
			}
		})
	}

	// The existing control-character rejection is preserved; the value is now
	// rejected at the typed boundary too, before it reaches the dispatch gate.
	if _, err := NormalizeCustomBuild(map[string]any{"download_url": "https://download.example/a\nb"}, context); err == nil {
		t.Error("NormalizeCustomBuild() accepted a control character in download_url")
	}
	if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"download_url": "https://download.example/a\nb"}); err == nil {
		t.Error("NormalizeWorkflowDispatchParams() accepted a control character in download_url")
	}

	// Empty download_url stays allowed and is omitted from the dispatch payload,
	// exactly like the sibling branding URLs.
	empty, err := NormalizeCustomBuild(map[string]any{"download_url": ""}, context)
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() with empty download_url error = %v", err)
	}
	if _, ok := empty.DispatchParams["download_url"]; ok {
		t.Error("empty download_url was emitted to dispatch")
	}

	// The dispatch primitive keeps the same absolute-URL gate for forged caller
	// values, mirroring website_url.
	if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"download_url": "/upload/client.exe"}); err == nil {
		t.Fatal("NormalizeWorkflowDispatchParams() accepted relative download_url")
	}
}

func TestNormalizeCustomBuildBrandingFieldForkSubset(t *testing.T) {
	// The fork Windows build workflow's validate_branding_url requires a
	// case-sensitive lowercase http:// or https:// prefix, rejects any
	// [[:space:]], and caps URL text at 2048 bytes; company_name is capped at
	// 256 bytes. DeskForge's acceptance domain must be a subset of that at the
	// typed boundary AND the dispatch gate, so an accepted value never fails
	// late at build time with no field-level feedback. api_server keeps its own
	// (looser) validateAPIURL contract and is intentionally not covered here.
	context := BuildRecordContext{Platform: "windows", AppName: "rustqs", Version: "1.2.3"}
	urlFields := []string{"download_url", "website_url", "privacy_policy_url"}
	// Literal fork limits (kept independent of the implementation constants so
	// this test still compiles against the pre-fix source for the falsification
	// probe and fails on the behavior, not on a missing symbol).
	const brandingURLMax = 2048
	const companyNameMax = 256

	accept := func(t *testing.T, field, value string) {
		t.Helper()
		got, err := NormalizeCustomBuild(map[string]any{field: value}, context)
		if err != nil {
			t.Fatalf("NormalizeCustomBuild(%s=%q) error = %v, want accepted", field, value, err)
		}
		if got.DispatchParams[field] != value {
			t.Errorf("dispatch[%s] = %v, want %q", field, got.DispatchParams[field], value)
		}
		if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{field: value}); err != nil {
			t.Fatalf("NormalizeWorkflowDispatchParams(%s=%q) error = %v, want accepted", field, value, err)
		}
	}
	reject := func(t *testing.T, field, value string) {
		t.Helper()
		typedErr := func() error {
			_, err := NormalizeCustomBuild(map[string]any{field: value}, context)
			return err
		}()
		if typedErr == nil {
			t.Fatalf("typed boundary accepted %s=%q", field, value)
		}
		if f, code, ok := FieldErrorMetadata(typedErr); !ok || f != field || code != FieldCodeInvalidFormat {
			t.Fatalf("typed field/code = %s/%s ok=%v, want %s/%s (err=%v)", f, code, ok, field, FieldCodeInvalidFormat, typedErr)
		}
		gateErr := func() error {
			_, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{field: value})
			return err
		}()
		if gateErr == nil {
			t.Fatalf("dispatch gate accepted %s=%q", field, value)
		}
		if f, code, ok := FieldErrorMetadata(gateErr); !ok || f != field || code != FieldCodeInvalidFormat {
			t.Fatalf("dispatch field/code = %s/%s ok=%v, want %s/%s (err=%v)", f, code, ok, field, FieldCodeInvalidFormat, gateErr)
		}
	}
	// A control-character value (e.g. a tab) is whitespace the fork rejects. At
	// the typed boundary it is a field-level invalid_format; at the dispatch
	// gate the pre-existing control-character gate runs first and returns a
	// plain error, which is preserved.
	rejectTyped := func(t *testing.T, field, value string) {
		t.Helper()
		typedErr := func() error {
			_, err := NormalizeCustomBuild(map[string]any{field: value}, context)
			return err
		}()
		if typedErr == nil {
			t.Fatalf("typed boundary accepted %s=%q", field, value)
		}
		if f, code, ok := FieldErrorMetadata(typedErr); !ok || f != field || code != FieldCodeInvalidFormat {
			t.Fatalf("typed field/code = %s/%s ok=%v, want %s/%s (err=%v)", f, code, ok, field, FieldCodeInvalidFormat, typedErr)
		}
		if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{field: value}); err == nil {
			t.Fatalf("dispatch gate accepted %s=%q", field, value)
		}
	}

	for _, field := range urlFields {
		t.Run(field, func(t *testing.T) {
			// Lowercase absolute http(s) stays accepted and reaches the payload
			// unchanged.
			accept(t, field, "https://branding.example/path")
			accept(t, field, "http://branding.example/path")
			// Every host with at least one letter or digit stays accepted: DNS
			// names, single-label hosts, IPv4/IPv6 literals, IDN/punycode, and a
			// host carrying an explicit port.
			accept(t, field, "https://localhost")
			accept(t, field, "https://example.com")
			accept(t, field, "https://192.168.1.1")
			accept(t, field, "https://[::1]")
			accept(t, field, "http://localhost:8080/path")
			accept(t, field, "https://xn--bcher-kva.example")
			accept(t, field, "https://bücher.example")
			// Exactly the 2048-byte cap is accepted.
			base := "https://branding.example/"
			accept(t, field, base+strings.Repeat("a", brandingURLMax-len(base)))
			// Mixed/upper-case scheme: net/url would lowercase it, the fork does not.
			reject(t, field, "HTTP://branding.example/path")
			reject(t, field, "Https://branding.example/path")
			// Interior whitespace the fork's [[:space:]] rejects; net/url accepts a
			// plain interior space, which is the original late-build defect.
			reject(t, field, "https://branding.example/a b")
			rejectTyped(t, field, "https://branding.example/a\tb")
			// One byte over the cap.
			reject(t, field, base+strings.Repeat("a", brandingURLMax-len(base)+1))
			// A host made only of punctuation parses in net/url but has no usable
			// domain, so it must be rejected instead of persisting a link with an
			// empty/nonsense host.
			reject(t, field, "https://.")
			reject(t, field, "https://..")
			reject(t, field, "https://-")
		})
	}

	// Exactly the 256-byte company_name cap is accepted; one over is rejected.
	acceptCompany := func(t *testing.T, value string) {
		t.Helper()
		got, err := NormalizeCustomBuild(map[string]any{"company_name": value}, context)
		if err != nil {
			t.Fatalf("NormalizeCustomBuild(company_name len %d) error = %v, want accepted", len(value), err)
		}
		if got.DispatchParams["company_name"] != value {
			t.Errorf("dispatch[company_name] length = %v, want length %d", got.DispatchParams["company_name"], len(value))
		}
		if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"company_name": value}); err != nil {
			t.Fatalf("NormalizeWorkflowDispatchParams(company_name len %d) error = %v, want accepted", len(value), err)
		}
	}
	t.Run("company_name", func(t *testing.T) {
		acceptCompany(t, "Acme Ltd")
		acceptCompany(t, strings.Repeat("x", companyNameMax))
		reject(t, "company_name", strings.Repeat("x", companyNameMax+1))
		// A control character the fork build rejects is now caught at the typed
		// boundary with a field-level invalid_format (mirroring the URL fields);
		// the pre-existing dispatch control-character gate still rejects it first
		// with a plain error, which is preserved.
		rejectTyped(t, "company_name", "Acme\nLtd")
		rejectTyped(t, "company_name", "Acme\tLtd")
		rejectTyped(t, "company_name", "Acme\x00Ltd")
	})

	// Empty branding values stay allowed and are omitted from dispatch.
	t.Run("empty-omitted", func(t *testing.T) {
		for _, field := range append(append([]string{}, urlFields...), "company_name") {
			got, err := NormalizeCustomBuild(map[string]any{field: ""}, context)
			if err != nil {
				t.Fatalf("NormalizeCustomBuild(%s=\"\") error = %v, want accepted", field, err)
			}
			if _, ok := got.DispatchParams[field]; ok {
				t.Errorf("empty %s was emitted to dispatch", field)
			}
		}
	})

	// The production create boundary returns the same {field, code} the admin UI
	// localizes, wrapped as a client validation error.
	for _, test := range []struct {
		field string
		value string
	}{
		{"website_url", "HTTP://example.com"},
		{"privacy_policy_url", "https://example.com/a b"},
		{"download_url", "https://example.com/" + strings.Repeat("a", brandingURLMax)},
		{"company_name", strings.Repeat("x", companyNameMax+1)},
		// A degenerate punctuation-only host is a field-level invalid_format at
		// the production create boundary too, so the admin UI localizes it inline.
		{"website_url", "https://."},
		{"privacy_policy_url", "https://.."},
		{"download_url", "https://-"},
	} {
		t.Run("create-"+test.field, func(t *testing.T) {
			custom := mustJSON(t, map[string]any{
				"server_ip":    "id.example:21116",
				"key":          validRustDeskPublicKey,
				"api_server":   "https://api.example",
				"relay_server": "relay.example:21117",
				test.field:     test.value,
			})
			err := ValidateCustomBuildInput("windows", custom, "rustqs", "1.2.3")
			if !IsClientValidationError(err) {
				t.Fatalf("ValidateCustomBuildInput() error = %v, want ClientValidationError", err)
			}
			f, code, ok := FieldErrorMetadata(err)
			if !ok || f != test.field || code != FieldCodeInvalidFormat {
				t.Fatalf("field/code = %s/%s ok=%v, want %s/%s (err=%v)", f, code, ok, test.field, FieldCodeInvalidFormat, err)
			}
		})
	}

	// The create boundary rejects a company_name control character with the same
	// field-level invalid_format as the length and URL cases above.
	t.Run("create-company_name-control-character", func(t *testing.T) {
		custom := mustJSON(t, map[string]any{
			"server_ip":    "id.example:21116",
			"key":          validRustDeskPublicKey,
			"api_server":   "https://api.example",
			"relay_server": "relay.example:21117",
			"company_name": "Acme\nLtd",
		})
		err := ValidateCustomBuildInput("windows", custom, "rustqs", "1.2.3")
		if !IsClientValidationError(err) {
			t.Fatalf("ValidateCustomBuildInput() error = %v, want ClientValidationError", err)
		}
		f, code, ok := FieldErrorMetadata(err)
		if !ok || f != "company_name" || code != FieldCodeInvalidFormat {
			t.Fatalf("field/code = %s/%s ok=%v, want company_name/%s (err=%v)", f, code, ok, FieldCodeInvalidFormat, err)
		}
	})
}

func TestBrandingURLTextFieldBoundaryAgreement(t *testing.T) {
	// D-a: the typed boundary (NormalizeCustomBuild), the dispatch gate
	// (NormalizeWorkflowDispatchParams), and the stored-data seam
	// (ValidateStoredCustomBrandingField) must make exactly the same
	// accept/refuse decision for download_url/website_url/privacy_policy_url.
	// They now share one rule because validateBrandingURL applies the
	// control-character safety rule (validateWorkflowValue) before the URL shape
	// rule, so a C1 control character such as U+0086 — which is neither
	// whitespace nor rejected by url.ParseRequestURI — can no longer be written
	// by the producer and then abort the migration preflight.
	context := BuildRecordContext{Platform: "windows", AppName: "rustqs", Version: "1.2.3"}
	urlFields := []string{"download_url", "website_url", "privacy_policy_url"}
	values := []struct {
		name  string
		value string
	}{
		{"accepted-https", "https://branding.example/path"},
		{"accepted-http", "http://branding.example"},
		{"c1-control", "https://branding.example/\u0086"},
		{"nul-control", "https://branding.example/a\x00b"},
		{"tab-control", "https://branding.example/a\tb"},
		{"interior-space", "https://branding.example/a b"},
		{"relative", "branding.example/path"},
		{"punctuation-host", "https://."},
		{"mixed-case-scheme", "HTTP://branding.example/path"},
	}
	for _, field := range urlFields {
		for _, tc := range values {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				typedAccepted := func() bool {
					_, err := NormalizeCustomBuild(map[string]any{field: tc.value}, context)
					return err == nil
				}()
				gateAccepted := func() bool {
					_, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{field: tc.value})
					return err == nil
				}()
				storedAccepted := ValidateStoredCustomBrandingField(field, tc.value) == nil
				if typedAccepted != gateAccepted || gateAccepted != storedAccepted {
					t.Fatalf("boundary disagreement for %s=%q: typed=%v gate=%v stored=%v",
						field, tc.value, typedAccepted, gateAccepted, storedAccepted)
				}
			})
		}
	}

	// A control character in each URL text field is rejected at build creation
	// with the existing structured invalid_format field error, so the admin UI
	// can localize it inline instead of the value aborting the build later.
	for _, field := range urlFields {
		t.Run("create-"+field+"-c1-control", func(t *testing.T) {
			custom := mustJSON(t, map[string]any{
				"server_ip":    "id.example:21116",
				"key":          validRustDeskPublicKey,
				"api_server":   "https://api.example",
				"relay_server": "relay.example:21117",
				field:          "https://branding.example/\u0086",
			})
			err := ValidateCustomBuildInput("windows", custom, "rustqs", "1.2.3")
			if !IsClientValidationError(err) {
				t.Fatalf("ValidateCustomBuildInput() error = %v, want ClientValidationError", err)
			}
			f, code, ok := FieldErrorMetadata(err)
			if !ok || f != field || code != FieldCodeInvalidFormat {
				t.Fatalf("field/code = %s/%s ok=%v, want %s/%s (err=%v)", f, code, ok, field, FieldCodeInvalidFormat, err)
			}
		})
	}
}

func TestBrandingImageURLOriginAndValidation(t *testing.T) {
	context := BuildRecordContext{Platform: "windows", AppName: "rustqs", Version: "1.2.3"}

	// (1) A relative asset resolves against the public origin — the address the
	// build runner can reach — and never against the operational api-server,
	// which the stock deployment hardcodes to a loopback address.
	setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
	resolved, err := NormalizeCustomBuild(map[string]any{"app_icon_url": "/upload/x.png"}, context)
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() error = %v", err)
	}
	resolvedValue, _ := resolved.DispatchParams["app_icon_url"].(string)
	if resolvedValue != "https://deskforge.example/upload/x.png" {
		t.Errorf("dispatch[app_icon_url] = %v, want https://deskforge.example/upload/x.png", resolved.DispatchParams["app_icon_url"])
	} else if strings.Contains(resolvedValue, "127.0.0.1") || strings.Contains(resolvedValue, "localhost") {
		t.Errorf("dispatch[app_icon_url] = %v, must not use the operational api-server", resolvedValue)
	}

	// (3) An already-absolute external URL is preserved unchanged.
	absolute, err := NormalizeCustomBuild(map[string]any{"app_icon_url": "https://cdn.example/i.png"}, context)
	if err != nil {
		t.Fatalf("NormalizeCustomBuild(absolute) error = %v", err)
	}
	if value := absolute.DispatchParams["app_icon_url"]; value != "https://cdn.example/i.png" {
		t.Errorf("dispatch[app_icon_url] = %v, want the absolute URL unchanged", value)
	}

	// (2) A relative asset with no usable public origin fails closed at the typed
	// boundary with a field error; no payload/dispatch is produced.
	setBrandingConfigForTest(t, "http://127.0.0.1:21114", "")
	if _, err := NormalizeCustomBuild(map[string]any{"app_logo_url": "/upload/logo.png"}, context); err == nil {
		t.Fatal("NormalizeCustomBuild() accepted a relative asset with no public origin")
	} else if f, code, ok := FieldErrorMetadata(err); !ok || f != "app_logo_url" || code != FieldCodeInvalidFormat {
		t.Fatalf("field/code = %s/%s ok=%v, want app_logo_url/%s (err=%v)", f, code, ok, FieldCodeInvalidFormat, err)
	}
	// A malformed (loopback) public origin is unusable and also fails closed.
	setBrandingConfigForTest(t, "http://127.0.0.1:21114", "http://127.0.0.1:21114")
	if _, err := NormalizeCustomBuild(map[string]any{"privacy_screen_url": "/upload/privacy.png"}, context); err == nil {
		t.Fatal("NormalizeCustomBuild() accepted a relative asset with a loopback public origin")
	}

	// (4) Non-http(s) schemes, malformed/host-less URLs, loopback/private hosts,
	// and a relative value without a leading slash are rejected at the typed
	// boundary with a field error and at the dispatch gate.
	setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
	for _, tc := range []struct {
		field string
		value string
	}{
		{"app_icon_url", "file:///etc/passwd"},
		{"app_icon_url", "ftp://example.com/i.png"},
		{"app_icon_url", "upload/i.png"},
		{"app_icon_url", "https:///i.png"},
		{"app_icon_url", "https://"},
		{"app_logo_url", "https://exa mple/i.png"},
		{"app_logo_url", "http://127.0.0.1:21114/i.png"},
		{"app_logo_url", "http://localhost/i.png"},
		{"privacy_screen_url", "http://10.0.0.5/i.png"},
		{"privacy_screen_url", "http://169.254.0.1/i.png"},
		{"privacy_screen_url", "http://[::1]/i.png"},
		{"privacy_screen_url", "http://192.168.1.10/i.png"},
	} {
		t.Run("reject-"+tc.value, func(t *testing.T) {
			typedErr := func() error {
				_, err := NormalizeCustomBuild(map[string]any{tc.field: tc.value}, context)
				return err
			}()
			if typedErr == nil {
				t.Fatalf("typed boundary accepted %s=%q", tc.field, tc.value)
			}
			if f, code, ok := FieldErrorMetadata(typedErr); !ok || f != tc.field || code != FieldCodeInvalidFormat {
				t.Fatalf("typed field/code = %s/%s ok=%v, want %s/%s (err=%v)", f, code, ok, tc.field, FieldCodeInvalidFormat, typedErr)
			}
			gateErr := func() error {
				_, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{tc.field: tc.value})
				return err
			}()
			if gateErr == nil {
				t.Fatalf("dispatch gate accepted %s=%q", tc.field, tc.value)
			}
			if f, code, ok := FieldErrorMetadata(gateErr); !ok || f != tc.field || code != FieldCodeInvalidFormat {
				t.Fatalf("dispatch field/code = %s/%s ok=%v, want %s/%s (err=%v)", f, code, ok, tc.field, FieldCodeInvalidFormat, gateErr)
			}
		})
	}

	// The three image fields are part of the structured field-error set the
	// admin UI localizes, so the production create boundary reports them inline.
	custom := mustJSON(t, map[string]any{
		"server_ip":    "id.example:21116",
		"key":          validRustDeskPublicKey,
		"api_server":   "https://api.example",
		"relay_server": "relay.example:21117",
		"app_icon_url": "file:///etc/passwd",
	})
	if err := ValidateCustomBuildInput("windows", custom, "rustqs", "1.2.3"); err == nil {
		t.Fatal("ValidateCustomBuildInput() accepted a file: image URL")
	} else if f, code, ok := FieldErrorMetadata(err); !ok || f != "app_icon_url" || code != FieldCodeInvalidFormat {
		t.Fatalf("create field/code = %s/%s ok=%v, want app_icon_url/%s (err=%v)", f, code, ok, FieldCodeInvalidFormat, err)
	}

	// The cap on the resolved URL is maxBrandingURLBytes (2048 bytes).
	overLong := "https://cdn.example/" + strings.Repeat("a", maxBrandingURLBytes)
	if _, err := NormalizeCustomBuild(map[string]any{"app_icon_url": overLong}, context); err == nil {
		t.Fatal("NormalizeCustomBuild() accepted an over-length image URL")
	}
}

func TestBrandingLiteralHostAliasRejection(t *testing.T) {
	// D-b: net.ParseIP accepts only canonical IPv4/IPv6, so a resolver-only
	// spelling of a loopback/RFC1918 address slips past a naive literal test.
	// The public-host rule must normalize the host and reject the trailing-dot
	// and inet_aton decimal/hex/octal integer forms, and must fail closed on a
	// host that is neither a literal nor a plausible hostname — as a configured
	// public origin and as an absolute branding image value.
	context := BuildRecordContext{Platform: "windows", AppName: "rustqs", Version: "1.2.3"}

	blockedHosts := []struct {
		name string
		host string
	}{
		{"trailing-dot", "127.0.0.1."},
		{"trailing-dots", "127.0.0.1.."},
		{"decimal-integer", "2130706433"},
		{"hex-integer", "0x7f000001"},
		{"octal-octets", "0177.0.0.1"},
		{"two-part-shorthand", "127.1"},
		{"decimal-rfc1918", "167772161"},
		{"ipv6-loopback", "[::1]"},
		{"ipv6-expanded-loopback", "[0:0:0:0:0:0:0:1]"},
		{"implausible-hostname", "foo..bar"},
	}
	for _, tc := range blockedHosts {
		t.Run("origin/"+tc.name, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", "http://"+tc.host)
			if _, err := NormalizeCustomBuild(map[string]any{"app_icon_url": "/upload/x.png"}, context); err == nil {
				t.Fatalf("NormalizeCustomBuild() accepted a relative asset with blocked origin host %q", tc.host)
			}
		})
		t.Run("image/"+tc.name, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
			value := "http://" + tc.host + "/i.png"
			typedErr := func() error {
				_, err := NormalizeCustomBuild(map[string]any{"app_icon_url": value}, context)
				return err
			}()
			if typedErr == nil {
				t.Fatalf("typed boundary accepted an absolute image with blocked host %q", tc.host)
			}
			if f, code, ok := FieldErrorMetadata(typedErr); !ok || f != "app_icon_url" || code != FieldCodeInvalidFormat {
				t.Fatalf("typed field/code = %s/%s ok=%v, want app_icon_url/%s (err=%v)", f, code, ok, FieldCodeInvalidFormat, typedErr)
			}
			if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"app_icon_url": value}); err == nil {
				t.Fatalf("dispatch gate accepted an absolute image with blocked host %q", tc.host)
			}
		})
	}

	// The stored-data seam must refuse a stored absolute image whose host is a
	// normalized loopback form, so the schema-285 preflight fails closed instead
	// of passing a row the producer would reject.
	if err := ValidateStoredCustomBrandingField("app_icon_url", "http://2130706433/i.png"); err == nil {
		t.Fatal("ValidateStoredCustomBrandingField accepted a decimal-integer loopback image host")
	}

	// A genuine public host is still accepted as an origin and as an absolute
	// image value.
	setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
	if _, err := NormalizeCustomBuild(map[string]any{"app_logo_url": "/upload/logo.png"}, context); err != nil {
		t.Fatalf("NormalizeCustomBuild() rejected a public origin: %v", err)
	}
	if _, err := NormalizeCustomBuild(map[string]any{"app_logo_url": "https://cdn.example/logo.png"}, context); err != nil {
		t.Fatalf("NormalizeCustomBuild() rejected a public absolute image: %v", err)
	}
}

func TestBrandingPublicLiteralAddressPolicyBoundaries(t *testing.T) {
	// The public-host rule is one literal public-address policy shared by every
	// boundary. It must refuse the IANA special-purpose ranges the old
	// loopback/private/link-local check accepted (unspecified, resolver aliases,
	// shared/CGNAT, documentation, benchmarking, multicast, reserved, IPv6
	// documentation, the dummy IPv6 prefix, SRv6 SIDs, the deprecated
	// IPv4-compatible form, ...) as a configured public origin, as an absolute
	// branding image value at the typed boundary, at the dispatch gate, and at
	// the stored-data seam — and still accept public literals. The IPv6 half is
	// additionally bounded to 2000::/3, the only block IANA allocates as global
	// unicast, so the IANA-reserved blocks outside that block and the reserved
	// space inside it are refused as well.
	context := BuildRecordContext{Platform: "windows", AppName: "rustqs", Version: "1.2.3"}

	blocked := []string{
		"0.0.0.0", "0.1.2.3", "127.0.0.1", "127.1", "2130706433", "0x7f000001",
		"169.254.1.1", "10.0.0.1", "100.64.0.1", "192.0.2.1", "198.18.0.1",
		"198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "255.255.255.255",
		"[::]", "[::1]", "[fc00::1]", "[fe80::1]", "[2001:db8::1]",
		"[3fff::1]", "[5f00::1]", "[100:0:0:1::1]", "[::127.0.0.1]",
		// IPv6 blocks outside the 2000::/3 global-unicast bound (IPv6 Address
		// Space registry Reserved-by-IETF rows) and the returned 6bone range.
		"[400::1]", "[800::1]", "[1000::1]", "[6000::1]", "[fe00::1]",
		"[fec0::1]", "[3ffe::1]",
		// The reserved staircase inside 2000::/3 itself (IPv6 Global Unicast
		// Address Space registry RESERVED rows 2d00::/8 ... 3ffc::/15).
		"[2d00::1]", "[3f00::1]",
		// The 2001::/23 IETF protocol-assignments block is refused as a whole;
		// only the registry's Globally Reachable more-specific allocations
		// pass through, so unlisted space in it is refused.
		"[2001:1::4]", "[2001:5::1]",
	}
	for _, host := range blocked {
		t.Run("origin/"+host, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", "http://"+host)
			if _, err := NormalizeCustomBuild(map[string]any{"app_icon_url": "/upload/x.png"}, context); err == nil {
				t.Fatalf("NormalizeCustomBuild() resolved a relative asset against non-public origin host %q", host)
			}
		})
		t.Run("typed/"+host, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
			value := "http://" + host + "/i.png"
			_, err := NormalizeCustomBuild(map[string]any{"app_icon_url": value}, context)
			if err == nil {
				t.Fatalf("typed boundary accepted absolute image with non-public host %q", host)
			}
			if f, code, ok := FieldErrorMetadata(err); !ok || f != "app_icon_url" || code != FieldCodeInvalidFormat {
				t.Fatalf("typed field/code = %s/%s ok=%v, want app_icon_url/%s (err=%v)", f, code, ok, FieldCodeInvalidFormat, err)
			}
		})
		t.Run("gate/"+host, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
			value := "http://" + host + "/i.png"
			if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"app_icon_url": value}); err == nil {
				t.Fatalf("dispatch gate accepted absolute image with non-public host %q", host)
			}
		})
		t.Run("stored/"+host, func(t *testing.T) {
			value := "http://" + host + "/i.png"
			if err := ValidateStoredCustomBrandingField("app_icon_url", value); err == nil {
				t.Fatalf("stored-data seam accepted absolute image with non-public host %q", host)
			}
		})
	}

	// A configured public origin still resolves a server-relative asset, both
	// as a DNS name and as a public literal address.
	for _, origin := range []string{"https://deskforge.example", "http://8.8.8.8:21114"} {
		t.Run("origin-accepted/"+origin, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", origin)
			normalized, err := NormalizeCustomBuild(map[string]any{"app_icon_url": "/upload/x.png"}, context)
			if err != nil {
				t.Fatalf("NormalizeCustomBuild() rejected a relative asset with public origin %q: %v", origin, err)
			}
			if got, want := normalized.DispatchParams["app_icon_url"], origin+"/upload/x.png"; got != want {
				t.Fatalf("dispatch[app_icon_url] = %v, want %q", got, want)
			}
		})
	}

	// Public absolute image URLs (public literals and DNS names) stay accepted
	// at every boundary, including the registry Globally Reachable blocks
	// adjacent to the refused special-purpose ranges and the more-specific
	// allocations inside 2001::/23: the DRIP prefix (2001:30::/28), AMT
	// (2001:3::/32), the AS112 blocks (2001:4:112::/48, 2620:4f:8000::/48), the
	// DNS-SD SRP anycast address (2001:1::3), and an ordinary RIR allocation
	// (2001:4860:4860::8888).
	for _, value := range []string{
		"http://8.8.8.8/i.png",
		"http://1.1.1.1/i.png",
		"http://[2606:4700:4700::1111]/i.png",
		"http://[2001:30::1]/i.png",
		"http://[2001:3::1]/i.png",
		"http://[2620:4f:8000::1]/i.png",
		"http://[2001:4:112::1]/i.png",
		"http://[2001:1::3]/i.png",
		"http://[2001:4860:4860::8888]/i.png",
		"https://cdn.example/path.png",
	} {
		t.Run("accepted/"+value, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
			normalized, err := NormalizeCustomBuild(map[string]any{"app_icon_url": value}, context)
			if err != nil {
				t.Fatalf("typed boundary rejected public image %q: %v", value, err)
			}
			if got := normalized.DispatchParams["app_icon_url"]; got != value {
				t.Fatalf("dispatch[app_icon_url] = %v, want %q", got, value)
			}
			if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"app_icon_url": value}); err != nil {
				t.Fatalf("dispatch gate rejected public image %q: %v", value, err)
			}
			if err := ValidateStoredCustomBrandingField("app_icon_url", value); err != nil {
				t.Fatalf("stored-data seam rejected public image %q: %v", value, err)
			}
		})
	}

	// Policy edges: an IPv4-mapped IPv6 literal is classified as its IPv4
	// address (the mapped loopback form is refused), a zoned literal is refused
	// without being resolved, the shared-address block boundary is exact, and a
	// mapped public address stays public.
	for _, tc := range []struct {
		name   string
		value  string
		accept bool
	}{
		{"mapped-loopback-refused", "http://[::ffff:127.0.0.1]/i.png", false},
		{"zoned-link-local-refused", "http://[fe80::1%25eth0]/i.png", false},
		{"shared-range-last-refused", "http://100.127.255.255/i.png", false},
		{"shared-range-next-accepted", "http://100.128.0.0/i.png", true},
		{"mapped-public-accepted", "http://[::ffff:8.8.8.8]/i.png", true},
	} {
		t.Run("edge/"+tc.name, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
			_, err := NormalizeCustomBuild(map[string]any{"app_icon_url": tc.value}, context)
			if tc.accept && err != nil {
				t.Fatalf("typed boundary rejected %q: %v", tc.value, err)
			}
			if !tc.accept && err == nil {
				t.Fatalf("typed boundary accepted %q", tc.value)
			}
		})
	}

	// A stored server-relative image stays valid stored data with no public
	// origin configured — resolving it is the producer's concern, so the
	// migration preflight must not abort on it — while the typed boundary still
	// fails closed on the same value.
	previousConfig := Config
	Config = nil
	t.Cleanup(func() { Config = previousConfig })
	if err := ValidateStoredCustomBrandingField("app_icon_url", "/upload/x.png"); err != nil {
		t.Fatalf("stored-data seam rejected a stored relative image with no configured origin: %v", err)
	}
	if _, err := NormalizeCustomBuild(map[string]any{"app_icon_url": "/upload/x.png"}, context); err == nil {
		t.Fatal("typed boundary accepted a relative asset with no configured public origin")
	}
}

func TestBrandingFetchURLWireFormBoundaries(t *testing.T) {
	// The producer must accept only fetch URLs the workflow's HTTP fetcher
	// (fetch_branding_image.py: Python http.client plus getaddrinfo) can send.
	// canonicalizeHost strips at most one trailing dot — the DNS root label —
	// so a host with a second trailing dot has an empty label and is refused
	// instead of being dispatched as its single-dot spelling, which the
	// fetcher would then fail to resolve late in getaddrinfo. A raw non-ASCII
	// host, path, or query is accepted only when it can be normalized to the
	// ASCII wire form; the dispatched value is asserted to be ASCII (IDNA host,
	// percent-encoded path and query) while the stored value keeps the spelling
	// the operator authored.
	context := BuildRecordContext{Platform: "windows", AppName: "rustqs", Version: "1.2.3"}

	// A second trailing dot is an empty label, not a root label: refused as a
	// configured public origin, as an absolute branding image at the typed
	// boundary and the dispatch gate, and at the stored-data seam.
	for _, host := range []string{"example.com..", "cdn.example.."} {
		t.Run("double-dot-origin/"+host, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://"+host)
			if _, err := NormalizeCustomBuild(map[string]any{"app_icon_url": "/upload/x.png"}, context); err == nil {
				t.Fatalf("NormalizeCustomBuild() resolved a relative asset against double-dot origin host %q", host)
			}
		})
		t.Run("double-dot-typed/"+host, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
			value := "https://" + host + "/i.png"
			_, err := NormalizeCustomBuild(map[string]any{"app_icon_url": value}, context)
			if err == nil {
				t.Fatalf("typed boundary accepted an absolute image with double-dot host %q", host)
			}
			if f, code, ok := FieldErrorMetadata(err); !ok || f != "app_icon_url" || code != FieldCodeInvalidFormat {
				t.Fatalf("typed field/code = %s/%s ok=%v, want app_icon_url/%s (err=%v)", f, code, ok, FieldCodeInvalidFormat, err)
			}
			if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"app_icon_url": value}); err == nil {
				t.Fatalf("dispatch gate accepted an absolute image with double-dot host %q", host)
			}
			if err := ValidateStoredCustomBrandingField("app_icon_url", value); err == nil {
				t.Fatalf("stored-data seam accepted an absolute image with double-dot host %q", host)
			}
		})
	}

	// A single trailing dot is the root label and stays accepted, unchanged, as
	// a configured origin and as an absolute image host.
	setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example.")
	normalized, err := NormalizeCustomBuild(map[string]any{"app_icon_url": "/upload/x.png"}, context)
	if err != nil {
		t.Fatalf("NormalizeCustomBuild() rejected a single-trailing-dot public origin: %v", err)
	}
	if got, want := normalized.DispatchParams["app_icon_url"], "https://deskforge.example./upload/x.png"; got != want {
		t.Fatalf("dispatch[app_icon_url] = %v, want %q", got, want)
	}
	if _, err := NormalizeCustomBuild(map[string]any{"app_logo_url": "https://deskforge.example./logo.png"}, context); err != nil {
		t.Fatalf("typed boundary rejected an absolute image with a single-trailing-dot host: %v", err)
	}

	// An already-ASCII URL — including a punycode host and already
	// percent-encoded escapes — is dispatched byte-for-byte unchanged.
	for _, value := range []string{
		"https://xn--bcher-kva.example/i.png",
		"https://cdn.example/%D0%B8.png?q=%D0%B8",
	} {
		normalized, err := NormalizeCustomBuild(map[string]any{"app_icon_url": value}, context)
		if err != nil {
			t.Fatalf("typed boundary rejected ASCII fetch URL %q: %v", value, err)
		}
		if got := normalized.DispatchParams["app_icon_url"]; got != value {
			t.Fatalf("dispatch[app_icon_url] = %v, want the accepted value %q", got, value)
		}
	}

	// A raw-Unicode host, path, and query is accepted and dispatched in the
	// ASCII wire form: IDNA (punycode) host plus percent-encoded path/query.
	t.Run("unicode-wire-form", func(t *testing.T) {
		const unicodeURL = "https://bücher.example/иконка.png?q=и"
		const unicodeURLWire = "https://xn--bcher-kva.example/%D0%B8%D0%BA%D0%BE%D0%BD%D0%BA%D0%B0.png?q=%D0%B8"
		normalized, err := NormalizeCustomBuild(map[string]any{"app_icon_url": unicodeURL}, context)
		if err != nil {
			t.Fatalf("typed boundary rejected a representable Unicode fetch URL %q: %v", unicodeURL, err)
		}
		if got := normalized.DispatchParams["app_icon_url"]; got != unicodeURLWire {
			t.Fatalf("dispatch[app_icon_url] = %v, want wire form %q", got, unicodeURLWire)
		}
		if !strings.Contains(normalized.PersistedJSON, "bücher.example") {
			t.Fatalf("persisted custom_json = %s, want the stored Unicode spelling preserved", normalized.PersistedJSON)
		}
		gateParams, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"app_icon_url": unicodeURL})
		if err != nil {
			t.Fatalf("dispatch gate rejected a representable Unicode fetch URL %q: %v", unicodeURL, err)
		}
		if got := gateParams["app_icon_url"]; got != unicodeURLWire {
			t.Fatalf("gate[app_icon_url] = %v, want wire form %q", got, unicodeURLWire)
		}
		if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"app_icon_url": unicodeURLWire}); err != nil {
			t.Fatalf("dispatch gate rejected its own ASCII wire form %q: %v", unicodeURLWire, err)
		}
		if err := ValidateStoredCustomBrandingField("app_icon_url", unicodeURL); err != nil {
			t.Fatalf("stored-data seam rejected a representable Unicode fetch URL %q: %v", unicodeURL, err)
		}
	})

	// A raw-Unicode path with an ASCII host normalizes the request target only,
	// preserving a literal IPv6 host's brackets.
	for _, tc := range []struct {
		value string
		want  string
	}{
		{"https://cdn.example/иконка.png", "https://cdn.example/%D0%B8%D0%BA%D0%BE%D0%BD%D0%BA%D0%B0.png"},
		{"https://[2001:30::1]/и.png", "https://[2001:30::1]/%D0%B8.png"},
	} {
		t.Run("ascii-host/"+tc.value, func(t *testing.T) {
			setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
			normalized, err := NormalizeCustomBuild(map[string]any{"app_icon_url": tc.value}, context)
			if err != nil {
				t.Fatalf("typed boundary rejected %q: %v", tc.value, err)
			}
			if got := normalized.DispatchParams["app_icon_url"]; got != tc.want {
				t.Fatalf("dispatch[app_icon_url] = %v, want %q", got, tc.want)
			}
		})
	}

	// A non-ASCII public origin resolves a server-relative asset to the ASCII
	// wire form as well.
	t.Run("idn-origin", func(t *testing.T) {
		setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://пример.рф")
		normalized, err := NormalizeCustomBuild(map[string]any{"app_icon_url": "/upload/x.png"}, context)
		if err != nil {
			t.Fatalf("NormalizeCustomBuild() rejected a relative asset with an IDN public origin: %v", err)
		}
		if got, want := normalized.DispatchParams["app_icon_url"], "https://xn--e1afmkfd.xn--p1ai/upload/x.png"; got != want {
			t.Fatalf("dispatch[app_icon_url] = %v, want %q", got, want)
		}
	})

	// A Unicode spelling that IDNA maps to a non-public literal is refused
	// fail-closed: the full-width digit host "１２７.０.０.１" maps to 127.0.0.1,
	// which the fetcher would resolve to loopback and refuse only late.
	t.Run("mapped-non-public-literal", func(t *testing.T) {
		setBrandingConfigForTest(t, "http://127.0.0.1:21114", "https://deskforge.example")
		const mappedLoopback = "https://１２７.０.０.１/i.png"
		_, err := NormalizeCustomBuild(map[string]any{"app_icon_url": mappedLoopback}, context)
		if err == nil {
			t.Fatalf("typed boundary accepted Unicode host %q that maps to a non-public literal", mappedLoopback)
		}
		if f, code, ok := FieldErrorMetadata(err); !ok || f != "app_icon_url" || code != FieldCodeInvalidFormat {
			t.Fatalf("typed field/code = %s/%s ok=%v, want app_icon_url/%s (err=%v)", f, code, ok, FieldCodeInvalidFormat, err)
		}
		if _, err := NormalizeWorkflowDispatchParams(string(PlatformWindows), map[string]any{"app_icon_url": mappedLoopback}); err == nil {
			t.Fatalf("dispatch gate accepted Unicode host %q that maps to a non-public literal", mappedLoopback)
		}
		if err := ValidateStoredCustomBrandingField("app_icon_url", mappedLoopback); err == nil {
			t.Fatalf("stored-data seam accepted Unicode host %q that maps to a non-public literal", mappedLoopback)
		}
	})
}
