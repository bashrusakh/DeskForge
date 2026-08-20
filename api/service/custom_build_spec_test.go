package service

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
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
	raw := `{"conn-type":"incoming","password":"password-secret","default-settings":{"allow-hide-cm":"Y","verification-method":"use-permanent-password","enable-audio":"Y","theme":"dark"}}`
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
		"key":                   "public-key",
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
		"hide_cm":               true,
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
		"conn-type": "incoming",
		"password":  "secret",
		"default-settings": map[string]any{
			"access-mode":                      "custom",
			"approve-mode":                     "password-click",
			"verification-method":              "use-permanent-password",
			"allow-hide-cm":                    "Y",
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
	if got.DispatchParams["key"] != "public-key" {
		t.Errorf("dispatch key = %v, want public-key", got.DispatchParams["key"])
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
		raw := map[string]any{"permanent_password": "secret"}
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
			raw:  map[string]any{"hide_cm": true, "permanent_password": "secret"},
			want: map[string]any{"allow-hide-cm": "Y", "verification-method": "use-permanent-password"},
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
	got, err := NormalizeCustomBuildJSON(`{"server_ip":"id.example:21116","key":"public-key","api_server":"https://api.example","enable_audio":false,"enable_terminal":true,"app_name":"raw-app","version":"raw-version","platform":"raw-platform"}`, BuildRecordContext{
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
	if got.DispatchParams["server"] != "id.example:21116" || got.DispatchParams["key"] != "public-key" {
		t.Fatalf("L1 endpoint/key dispatch values not preserved: %#v", got.DispatchParams)
	}
	if persisted["server_ip"] != "id.example:21116" || persisted["key"] != "public-key" {
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
	raw := map[string]any{
		"server_ip":                "id.example:21116",
		"key":                      "public-key",
		"api_server":               "https://api.example",
		"relay_server":             "relay.example:21117",
		"company_name":             "DeskForge",
		"download_url":             "https://download.example/client",
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
		"server_ip", "key", "company_name", "download_url", "remove_new_version_notif",
		"cycle_monitor", "x_offline", "android_app_id", "app_icon_url", "app_logo_url",
		"privacy_screen_url",
	} {
		if _, ok := decodeCustomTxt(t, got.CustomTxt)[key]; ok {
			t.Errorf("L1/persisted-only field %q entered custom_.txt", key)
		}
	}
	assertNoKeys(t, decodeCustomTxt(t, got.CustomTxt), []string{
		"server_ip", "key", "company_name", "download_url", "remove_new_version_notif",
		"cycle_monitor", "x_offline", "android_app_id", "app_icon_url", "app_logo_url",
		"privacy_screen_url",
	})
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
