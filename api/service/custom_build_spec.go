package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"rustdesk-server/api/config"
	"rustdesk-server/api/utils"
)

// Platform is a platform supported by the Custom Client builder.
type Platform string

const (
	PlatformWindows Platform = "windows"
	PlatformLinux   Platform = "linux"
	PlatformAndroid Platform = "android"
)

const maxBuildAppNameBytes = 128

var androidAppIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// MaxPublicCustomTxtBytes bounds the generated native payload that may be
// parsed while preparing a public download. Larger or malformed payloads are
// omitted by the archive exporter rather than copied as opaque bytes.
const MaxPublicCustomTxtBytes = 128 << 10

// SettingsScope selects the native settings object that consumes option keys.
// The form currently has no manual/raw scope input, so normalization defaults
// to DefaultSettingsScope unless the typed record context selects otherwise.
type SettingsScope string

const (
	DefaultSettingsScope  SettingsScope = "default"
	OverrideSettingsScope SettingsScope = "override"
)

// BuildRecordContext contains values owned by the build record rather than by
// custom_json. These values may be sent to the workflow, but never enter the
// generated custom_.txt mapping.
type BuildRecordContext struct {
	BuildID       uint
	Platform      string
	AppName       string
	Version       string
	SettingsScope SettingsScope
	// AllowMissingAndroidAppID is reserved for explicit legacy-read/migration
	// normalization. Ordinary build and preset writes leave this false, so the
	// typed Android identity is required before persistence or dispatch.
	AllowMissingAndroidAppID bool
}

// BuildSpec is the typed form of user-authored Custom Client settings.
// ServerIP and Key are persisted L1 dispatch fields and are intentionally not
// serialized into custom_.txt; record context is also excluded from that
// payload. Persisted-only fields retain form values for preset/build restore,
// but have no proven native runtime consumer and are never emitted to
// custom_.txt.
type BuildSpec struct {
	BuildID       uint
	Platform      Platform
	AppName       string
	Version       string
	SettingsScope SettingsScope

	ServerIP string
	Key      string

	CompanyName           string
	DownloadURL           string
	RemoveNewVersionNotif *bool
	CycleMonitor          *bool
	XOffline              *bool
	AndroidAppID          string
	AppIconURL            string
	AppLogoURL            string
	PrivacyScreenURL      string

	APIServer           string
	RelayServer         string
	Direction           string
	PermanentPassword   string
	PassApproveMode     string
	Theme               string
	PermissionsType     string
	DenyLAN             *bool
	EnableDirectIP      *bool
	AutoClose           *bool
	HideCM              *bool
	RemoveWallpaper     *bool
	EnableRemoteModi    *bool
	EnableKeyboard      *bool
	EnableClipboard     *bool
	EnableFileTransfer  *bool
	EnableAudio         *bool
	EnableTCP           *bool
	EnableRemoteRestart *bool
	EnableRecording     *bool
	EnableBlockingInput *bool
	EnablePrinter       *bool
	EnableCamera        *bool
	EnableTerminal      *bool

	// stringPresence distinguishes an authored empty string from an omitted
	// field while keeping the presence bookkeeping internal to the typed
	// boundary.
	stringPresence           map[string]struct{}
	allowMissingAndroidAppID bool
}

type buildSpecStringField struct {
	name          string
	persisted     bool
	persistedOnly bool
	get           func(BuildSpec) string
	set           func(*BuildSpec, string)
}

var buildSpecStringFields = []buildSpecStringField{
	{name: "server_ip", persisted: true, get: func(spec BuildSpec) string { return spec.ServerIP }, set: func(spec *BuildSpec, value string) { spec.ServerIP = value }},
	{name: "key", persisted: true, get: func(spec BuildSpec) string { return spec.Key }, set: func(spec *BuildSpec, value string) { spec.Key = value }},
	{name: "api_server", persisted: true, get: func(spec BuildSpec) string { return spec.APIServer }, set: func(spec *BuildSpec, value string) { spec.APIServer = value }},
	{name: "relay_server", persisted: true, get: func(spec BuildSpec) string { return spec.RelayServer }, set: func(spec *BuildSpec, value string) { spec.RelayServer = value }},
	{name: "direction", persisted: true, get: func(spec BuildSpec) string { return spec.Direction }, set: func(spec *BuildSpec, value string) { spec.Direction = value }},
	{name: "permanent_password", persisted: true, get: func(spec BuildSpec) string { return spec.PermanentPassword }, set: func(spec *BuildSpec, value string) { spec.PermanentPassword = value }},
	{name: "pass_approve_mode", persisted: true, get: func(spec BuildSpec) string { return spec.PassApproveMode }, set: func(spec *BuildSpec, value string) { spec.PassApproveMode = value }},
	{name: "theme", persisted: true, get: func(spec BuildSpec) string { return spec.Theme }, set: func(spec *BuildSpec, value string) { spec.Theme = value }},
	{name: "permissions_type", persisted: true, get: func(spec BuildSpec) string { return spec.PermissionsType }, set: func(spec *BuildSpec, value string) { spec.PermissionsType = value }},
	{name: "company_name", persistedOnly: true, get: func(spec BuildSpec) string { return spec.CompanyName }, set: func(spec *BuildSpec, value string) { spec.CompanyName = value }},
	{name: "download_url", persistedOnly: true, get: func(spec BuildSpec) string { return spec.DownloadURL }, set: func(spec *BuildSpec, value string) { spec.DownloadURL = value }},
	{name: "android_app_id", persistedOnly: true, get: func(spec BuildSpec) string { return spec.AndroidAppID }, set: func(spec *BuildSpec, value string) { spec.AndroidAppID = value }},
	{name: "app_icon_url", persistedOnly: true, get: func(spec BuildSpec) string { return spec.AppIconURL }, set: func(spec *BuildSpec, value string) { spec.AppIconURL = value }},
	{name: "app_logo_url", persistedOnly: true, get: func(spec BuildSpec) string { return spec.AppLogoURL }, set: func(spec *BuildSpec, value string) { spec.AppLogoURL = value }},
	{name: "privacy_screen_url", persistedOnly: true, get: func(spec BuildSpec) string { return spec.PrivacyScreenURL }, set: func(spec *BuildSpec, value string) { spec.PrivacyScreenURL = value }},
}

type buildSpecBoolField struct {
	name          string
	persisted     bool
	persistedOnly bool
	get           func(BuildSpec) *bool
	set           func(*BuildSpec, *bool)
}

var buildSpecBoolFields = []buildSpecBoolField{
	{name: "deny_lan", persisted: true, get: func(spec BuildSpec) *bool { return spec.DenyLAN }, set: func(spec *BuildSpec, value *bool) { spec.DenyLAN = value }},
	{name: "enable_direct_ip", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableDirectIP }, set: func(spec *BuildSpec, value *bool) { spec.EnableDirectIP = value }},
	{name: "auto_close", persisted: true, get: func(spec BuildSpec) *bool { return spec.AutoClose }, set: func(spec *BuildSpec, value *bool) { spec.AutoClose = value }},
	{name: "hide_cm", persisted: true, get: func(spec BuildSpec) *bool { return spec.HideCM }, set: func(spec *BuildSpec, value *bool) { spec.HideCM = value }},
	{name: "remove_wallpaper", persisted: true, get: func(spec BuildSpec) *bool { return spec.RemoveWallpaper }, set: func(spec *BuildSpec, value *bool) { spec.RemoveWallpaper = value }},
	{name: "remove_new_version_notif", persistedOnly: true, get: func(spec BuildSpec) *bool { return spec.RemoveNewVersionNotif }, set: func(spec *BuildSpec, value *bool) { spec.RemoveNewVersionNotif = value }},
	{name: "cycle_monitor", persistedOnly: true, get: func(spec BuildSpec) *bool { return spec.CycleMonitor }, set: func(spec *BuildSpec, value *bool) { spec.CycleMonitor = value }},
	{name: "x_offline", persistedOnly: true, get: func(spec BuildSpec) *bool { return spec.XOffline }, set: func(spec *BuildSpec, value *bool) { spec.XOffline = value }},
	{name: "enable_remote_modi", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableRemoteModi }, set: func(spec *BuildSpec, value *bool) { spec.EnableRemoteModi = value }},
	{name: "enable_keyboard", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableKeyboard }, set: func(spec *BuildSpec, value *bool) { spec.EnableKeyboard = value }},
	{name: "enable_clipboard", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableClipboard }, set: func(spec *BuildSpec, value *bool) { spec.EnableClipboard = value }},
	{name: "enable_file_transfer", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableFileTransfer }, set: func(spec *BuildSpec, value *bool) { spec.EnableFileTransfer = value }},
	{name: "enable_audio", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableAudio }, set: func(spec *BuildSpec, value *bool) { spec.EnableAudio = value }},
	{name: "enable_tcp", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableTCP }, set: func(spec *BuildSpec, value *bool) { spec.EnableTCP = value }},
	{name: "enable_remote_restart", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableRemoteRestart }, set: func(spec *BuildSpec, value *bool) { spec.EnableRemoteRestart = value }},
	{name: "enable_recording", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableRecording }, set: func(spec *BuildSpec, value *bool) { spec.EnableRecording = value }},
	{name: "enable_blocking_input", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableBlockingInput }, set: func(spec *BuildSpec, value *bool) { spec.EnableBlockingInput = value }},
	{name: "enable_printer", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnablePrinter }, set: func(spec *BuildSpec, value *bool) { spec.EnablePrinter = value }},
	{name: "enable_camera", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableCamera }, set: func(spec *BuildSpec, value *bool) { spec.EnableCamera = value }},
	{name: "enable_terminal", persisted: true, get: func(spec BuildSpec) *bool { return spec.EnableTerminal }, set: func(spec *BuildSpec, value *bool) { spec.EnableTerminal = value }},
}

// NormalizedBuild is the service-owned result consumed by persistence and
// dispatch code. DispatchParams contains workflow/L1 values and the generated
// native custom_txt payload; PersistedJSON is the canonical form-field JSON
// stored in custom_json. CustomTxt is kept separately for direct callers/tests.
type NormalizedBuild struct {
	Spec           BuildSpec
	DispatchParams map[string]any
	CustomTxt      string
	PersistedJSON  string
}

// NormalizedCustomTxt is the only representation of custom_txt accepted by
// the shared workflow-dispatch boundary. It can only be produced by the typed
// BuildSpec encoder; a plain string from a caller is rejected there so raw
// native payloads cannot bypass BuildSpec validation.
type NormalizedCustomTxt struct {
	value string
}

func newNormalizedCustomTxt(value string) NormalizedCustomTxt {
	return NormalizedCustomTxt{value: value}
}

// Value returns the generated native payload for diagnostics/tests. Callers
// cannot construct a non-empty normalized value because its storage is private.
func (v NormalizedCustomTxt) Value() string { return v.value }

func (v NormalizedCustomTxt) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

// ClientValidationError marks input errors that must be returned as a client
// error by HTTP handlers. Storage errors remain untyped and keep the existing
// legacy error response path.
type ClientValidationError struct {
	Err error
}

func (e *ClientValidationError) Error() string {
	return e.Err.Error()
}

func (e *ClientValidationError) Unwrap() error {
	return e.Err
}

// IsClientValidationError reports whether err originated from user input.
func IsClientValidationError(err error) bool {
	if err == nil {
		return false
	}
	var validationErr *ClientValidationError
	return errors.As(err, &validationErr)
}

// ParsePlatform validates the closed platform domain used by BuildSpec.
func ParsePlatform(value string) (Platform, error) {
	switch platform := Platform(value); platform {
	case PlatformWindows, PlatformLinux, PlatformAndroid:
		return platform, nil
	default:
		return "", fmt.Errorf("platform has unsupported value %q", value)
	}
}

// ValidateCustomPlatform is the shared persistence boundary for custom build
// and preset records. Existing rows are still readable; only new writes and
// updates are rejected when their platform is outside the supported domain.
func ValidateCustomPlatform(value string) error {
	if _, err := ParsePlatform(value); err != nil {
		return &ClientValidationError{Err: err}
	}
	return nil
}

// ValidateCustomBuildInput validates the normal typed request path without
// changing the persisted custom_json representation. Empty custom_json keeps
// the existing optional-payload behavior; a non-empty value must pass the
// authoritative BuildSpec normalizer.
func ValidateCustomBuildInput(platform, customJSON, appName, version string) error {
	if err := ValidateBuildRecordContext(BuildRecordContext{
		Platform: platform,
		AppName:  appName,
		Version:  version,
	}); err != nil {
		return &ClientValidationError{Err: err}
	}
	if err := ValidateCustomPlatform(platform); err != nil {
		return err
	}
	if customJSON == "" && platform != string(PlatformAndroid) {
		return nil
	}
	if _, err := NormalizeCustomBuildJSON(customJSON, BuildRecordContext{
		Platform: platform,
		AppName:  appName,
		Version:  version,
	}); err != nil {
		return &ClientValidationError{Err: err}
	}
	return nil
}

// ValidateDirectCustomBuilderJSON applies the shared exact storage validator
// before a build or preset service writes a caller-supplied custom_json. The
// normalizer still owns semantic validation; this boundary prevents direct
// saves from accepting compact aliases or record/internal fields.
func ValidateDirectCustomBuilderJSON(customJSON string) error {
	if err := utils.ValidateCustomBuilderJSONFields(customJSON); err != nil {
		return &ClientValidationError{Err: err}
	}
	return nil
}

// CanonicalizeCustomBuildJSON applies the typed parse/validate/normalize path
// and returns the form-field JSON used for persistence. An empty payload keeps
// the existing optional-payload representation; non-empty payloads are always
// rewritten from BuildSpec before a caller writes them.
func CanonicalizeCustomBuildJSON(customJSON string, context BuildRecordContext) (string, error) {
	normalized, err := NormalizeCustomBuildJSON(customJSON, context)
	if err != nil {
		return "", &ClientValidationError{Err: err}
	}
	return normalized.PersistedJSON, nil
}

// ParseSettingsScope validates the closed native settings placement domain.
func ParseSettingsScope(value SettingsScope) (SettingsScope, error) {
	if value == "" {
		return DefaultSettingsScope, nil
	}
	switch value {
	case DefaultSettingsScope, OverrideSettingsScope:
		return value, nil
	default:
		return "", fmt.Errorf("settings scope has unsupported value %q", value)
	}
}

// ValidateBuildRecordContext validates record-owned values before a caller
// constructs workflow parameters, even when custom_json is empty.
func ValidateBuildRecordContext(context BuildRecordContext) error {
	if _, err := ParsePlatform(context.Platform); err != nil {
		return err
	}
	if err := ValidateOutputAppName(context.AppName); err != nil {
		return err
	}
	if err := validateWorkflowValue("version", context.Version); err != nil {
		return err
	}
	_, err := ParseSettingsScope(context.SettingsScope)
	return err
}

// NormalizeCustomBuildJSON parses and normalizes the persisted custom_json
// boundary once. The controller passes record context only; it does not parse
// or map raw fields itself.
func NormalizeCustomBuildJSON(rawJSON string, context BuildRecordContext) (NormalizedBuild, error) {
	if rawJSON == "" {
		normalized, err := NormalizeCustomBuild(map[string]any{}, context)
		if err != nil {
			return NormalizedBuild{}, err
		}
		normalized.PersistedJSON = ""
		return normalized, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return NormalizedBuild{}, fmt.Errorf("invalid custom JSON: %w", err)
	}
	if raw == nil {
		return NormalizedBuild{}, fmt.Errorf("custom JSON must be an object")
	}
	return NormalizeCustomBuild(raw, context)
}

// NormalizeCustomBuild converts form fields and record context into the
// single typed BuildSpec owner and workflow parameters. It deliberately keeps
// L1 dispatch values out of custom_.txt and puts every native option in the
// selected settings object.
func NormalizeCustomBuild(raw map[string]any, context BuildRecordContext) (NormalizedBuild, error) {
	if err := ValidateBuildRecordContext(context); err != nil {
		return NormalizedBuild{}, err
	}
	platform, _ := ParsePlatform(context.Platform)
	scope, err := ParseSettingsScope(context.SettingsScope)
	if err != nil {
		return NormalizedBuild{}, err
	}

	spec, err := parseBuildSpec(raw)
	if err != nil {
		return NormalizedBuild{}, err
	}
	spec.BuildID = context.BuildID
	spec.Platform = platform
	spec.AppName = context.AppName
	spec.Version = context.Version
	spec.SettingsScope = scope
	spec.allowMissingAndroidAppID = context.AllowMissingAndroidAppID

	customTxt, err := EncodeCustomTxt(spec)
	if err != nil {
		return NormalizedBuild{}, err
	}
	params := map[string]any{
		"app_name": spec.AppName,
		"version":  spec.Version,
	}
	if spec.ServerIP != "" {
		params["server"] = spec.ServerIP
	}
	if spec.Key != "" {
		params["key"] = spec.Key
	}
	if platform == PlatformAndroid {
		if spec.AndroidAppID != "" {
			params["android_app_id"] = spec.AndroidAppID
		}
	}
	params["custom_txt"] = newNormalizedCustomTxt(customTxt)

	persistedJSON, err := spec.PersistedJSON()
	if err != nil {
		return NormalizedBuild{}, err
	}
	return NormalizedBuild{Spec: spec, DispatchParams: params, CustomTxt: customTxt, PersistedJSON: persistedJSON}, nil
}

// PersistedJSON serializes the typed fields used by custom_json, including L1
// dispatch values needed for preset restore/retry and persisted-only form
// values without a proven native runtime consumer. Record context, unsupported
// fields, and raw custom_txt are not included. The custom_.txt encoder has its
// own explicit native allowlist, so none of the persisted-only fields enter it.
// Non-nil bool pointers are intentionally emitted even when false so authored
// false remains distinct from omission.
func (spec BuildSpec) PersistedJSON() (string, error) {
	if err := validateBuildSpec(spec); err != nil {
		return "", err
	}
	persisted := make(map[string]any)
	for _, field := range buildSpecStringFields {
		if field.persisted || field.persistedOnly {
			if value := field.get(spec); value != "" || spec.hasString(field.name) {
				persisted[field.name] = value
			}
		}
	}
	for _, field := range buildSpecBoolFields {
		if !field.persisted && !field.persistedOnly {
			continue
		}
		if value := field.get(spec); value != nil {
			persisted[field.name] = *value
		}
	}
	encoded, err := json.Marshal(persisted)
	if err != nil {
		return "", fmt.Errorf("marshal persisted custom JSON: %w", err)
	}
	return string(encoded), nil
}

// ParseBuildSpec validates the typed form boundary and converts it into BuildSpec.
// Raw workflow payloads such as custom_txt are deliberately not accepted here.
func ParseBuildSpec(raw map[string]any) (BuildSpec, error) {
	return parseBuildSpec(raw)
}

// BuildCustomTxtFromForm returns base64(JSON) after parsing and validating the
// ordinary typed form path. Raw custom_txt is rejected rather than forwarded.
func BuildCustomTxtFromForm(raw map[string]any) (string, error) {
	spec, err := ParseBuildSpec(raw)
	if err != nil {
		return "", err
	}
	return EncodeCustomTxt(spec)
}

func parseBuildSpec(raw map[string]any) (BuildSpec, error) {
	var spec BuildSpec
	if err := validateKnownBuildFields(raw); err != nil {
		return BuildSpec{}, err
	}

	for _, field := range buildSpecStringFields {
		value, fieldErr := optionalString(raw, field.name)
		if fieldErr != nil {
			return BuildSpec{}, fieldErr
		}
		if field.name == "key" {
			value = config.NormalizePublicKey(value)
		}
		field.set(&spec, value)
		if _, ok := raw[field.name]; ok {
			spec.markString(field.name)
		}
	}

	for _, field := range buildSpecBoolFields {
		value, fieldErr := optionalBool(raw, field.name)
		if fieldErr != nil {
			return BuildSpec{}, fieldErr
		}
		field.set(&spec, value)
	}
	if spec.AndroidAppID != "" {
		if err := ValidateAndroidAppID(spec.AndroidAppID); err != nil {
			return BuildSpec{}, err
		}
	}
	if err := validateBuildSpecTransportFields(spec); err != nil {
		return BuildSpec{}, err
	}
	for _, validation := range []struct {
		name    string
		value   string
		allowed []string
	}{
		{"direction", spec.Direction, []string{"both", "incoming", "outgoing"}},
		{"permissions_type", spec.PermissionsType, []string{"custom", "full", "view"}},
		{"theme", spec.Theme, []string{"light", "dark", "system"}},
		{"pass_approve_mode", spec.PassApproveMode, []string{"password", "click", "password-click", "both"}},
	} {
		if err := validateEnum(validation.name, validation.value, validation.allowed...); err != nil {
			return BuildSpec{}, err
		}
	}
	for _, endpoint := range []struct {
		name  string
		value string
	}{
		{"server_ip", spec.ServerIP},
		{"relay_server", spec.RelayServer},
	} {
		if err := validateEndpoint(endpoint.name, endpoint.value); err != nil {
			return BuildSpec{}, err
		}
	}
	if err := validateAPIURL("api_server", spec.APIServer); err != nil {
		return BuildSpec{}, err
	}
	if spec.HideCM != nil && *spec.HideCM && spec.PermanentPassword == "" {
		return BuildSpec{}, fmt.Errorf("permanent_password is required when hide_cm is true")
	}

	return spec, nil
}

var buildSpecRecordFields = map[string]struct{}{
	"app_name": {},
	"build_id": {},
	"platform": {},
	"version":  {},
}

func validateKnownBuildFields(raw map[string]any) error {
	for name := range raw {
		if name == "custom_txt" {
			return fmt.Errorf("custom_txt is not accepted by the typed build path")
		}
		if _, ok := buildSpecRecordFields[name]; ok {
			continue
		}
		known := false
		for _, field := range buildSpecStringFields {
			if field.name == name {
				known = true
				break
			}
		}
		if !known {
			for _, field := range buildSpecBoolFields {
				if field.name == name {
					known = true
					break
				}
			}
		}
		if !known {
			return fmt.Errorf("unsupported custom field %q", name)
		}
	}
	return nil
}

func (spec *BuildSpec) markString(name string) {
	if spec.stringPresence == nil {
		spec.stringPresence = make(map[string]struct{})
	}
	spec.stringPresence[name] = struct{}{}
}

func (spec BuildSpec) hasString(name string) bool {
	_, ok := spec.stringPresence[name]
	return ok
}

// EncodeCustomTxt serializes the exact native contract. Password and
// conn-type are flat; all other options are consumed from one selected
// default-settings or override-settings object.
func EncodeCustomTxt(spec BuildSpec) (string, error) {
	if err := validateBuildSpec(spec); err != nil {
		return "", err
	}

	config := make(map[string]any)
	if spec.Direction != "" && spec.Direction != "both" {
		config["conn-type"] = spec.Direction
	}
	if spec.PermanentPassword != "" {
		config["password"] = spec.PermanentPassword
	}

	settings := make(map[string]string)
	// nil means the user did not author a value; omission preserves the
	// downstream/system-derived default and is not a user-authored false.
	if spec.DenyLAN != nil {
		settings["enable-lan-discovery"] = yn(!*spec.DenyLAN)
	}
	if spec.AutoClose != nil {
		settings["allow-auto-disconnect"] = yn(*spec.AutoClose)
	}
	if spec.APIServer != "" {
		settings["api-server"] = spec.APIServer
	}
	if spec.RelayServer != "" {
		settings["relay-server"] = spec.RelayServer
	}
	if spec.PermissionsType != "" {
		settings["access-mode"] = spec.PermissionsType
	}
	if spec.PassApproveMode != "" {
		settings["approve-mode"] = spec.PassApproveMode
	}
	// Native theme is platform-independent here. Do not invent the legacy
	// allow-darktheme key without a proven local runtime consumer.
	if spec.Theme != "" && spec.Theme != "system" {
		settings["theme"] = spec.Theme
	}
	if spec.EnableDirectIP != nil {
		settings["direct-server"] = yn(*spec.EnableDirectIP)
	}
	if spec.HideCM != nil {
		settings["allow-hide-cm"] = yn(*spec.HideCM)
		if *spec.HideCM {
			settings["verification-method"] = "use-permanent-password"
		} else {
			settings["verification-method"] = "use-both-passwords"
		}
	}
	if spec.RemoveWallpaper != nil {
		settings["allow-remove-wallpaper"] = yn(*spec.RemoveWallpaper)
	}

	for _, field := range []struct {
		value *bool
		key   string
	}{
		{spec.EnableRemoteModi, "allow-remote-config-modification"},
		{spec.EnableKeyboard, "enable-keyboard"},
		{spec.EnableClipboard, "enable-clipboard"},
		{spec.EnableFileTransfer, "enable-file-transfer"},
		{spec.EnableAudio, "enable-audio"},
		{spec.EnableTCP, "enable-tunnel"},
		{spec.EnableRemoteRestart, "enable-remote-restart"},
		{spec.EnableRecording, "enable-record-session"},
		{spec.EnableBlockingInput, "enable-block-input"},
		{spec.EnablePrinter, "enable-remote-printer"},
		{spec.EnableCamera, "enable-camera"},
		{spec.EnableTerminal, "enable-terminal"},
	} {
		if field.value != nil {
			settings[field.key] = yn(*field.value)
		}
	}

	if len(settings) > 0 {
		scope := spec.SettingsScope
		if scope == "" {
			scope = DefaultSettingsScope
		}
		config[string(scope)+"-settings"] = settings
	}
	if len(config) == 0 {
		return "", nil
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal custom client config: %w", err)
	}
	return base64.StdEncoding.EncodeToString(encoded), nil
}

// nativeCustomTxt is the typed/native schema boundary for generated
// custom_.txt. Password is accepted only so it can be removed; it is never
// included in the public representation. The settings type has its own
// allowlist so unknown or secret-like fields cannot pass through as raw JSON.
type nativeCustomTxt struct {
	ConnType         string                  `json:"conn-type,omitempty"`
	Password         string                  `json:"password,omitempty"`
	DefaultSettings  nativeCustomTxtSettings `json:"default-settings,omitempty"`
	OverrideSettings nativeCustomTxtSettings `json:"override-settings,omitempty"`
}

type nativeCustomTxtSettings map[string]string

var nativeCustomTxtSafeSettings = map[string]struct{}{
	"enable-lan-discovery":             {},
	"allow-auto-disconnect":            {},
	"api-server":                       {},
	"relay-server":                     {},
	"access-mode":                      {},
	"approve-mode":                     {},
	"theme":                            {},
	"direct-server":                    {},
	"allow-remove-wallpaper":           {},
	"allow-remote-config-modification": {},
	"enable-keyboard":                  {},
	"enable-clipboard":                 {},
	"enable-file-transfer":             {},
	"enable-audio":                     {},
	"enable-tunnel":                    {},
	"enable-remote-restart":            {},
	"enable-record-session":            {},
	"enable-block-input":               {},
	"enable-remote-printer":            {},
	"enable-camera":                    {},
	"enable-terminal":                  {},
}

var nativeCustomTxtPasswordDependentSettings = map[string]struct{}{
	"allow-hide-cm":       {},
	"verification-method": {},
}

func (s *nativeCustomTxtSettings) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("native settings must be an object: %w", err)
	}
	if raw == nil {
		return errors.New("native settings must be an object")
	}
	settings := make(nativeCustomTxtSettings, len(raw))
	for key, value := range raw {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("native setting %q must not be null", key)
		}
		var stringValue string
		if err := json.Unmarshal(value, &stringValue); err != nil {
			return fmt.Errorf("native setting %q must be a string", key)
		}
		if err := validateWorkflowValue("native setting", stringValue); err != nil {
			return err
		}
		if _, passwordDependent := nativeCustomTxtPasswordDependentSettings[key]; passwordDependent {
			continue
		}
		if _, safe := nativeCustomTxtSafeSettings[key]; !safe {
			return fmt.Errorf("native setting %q is not safe for public export", key)
		}
		settings[key] = stringValue
	}
	*s = settings
	return nil
}

// PublicCustomTxt parses generated base64(JSON) custom_.txt content and
// returns a canonical public variant. It preserves conn-type and allowlisted
// non-secret L2 settings, while removing password, allow-hide-cm, and
// verification-method together. Unknown fields fail closed so callers can
// omit the file instead of exposing opaque private bytes.
func PublicCustomTxt(encoded string) (string, error) {
	if encoded == "" {
		return "", errors.New("custom_.txt payload is empty")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode custom_.txt payload: %w", err)
	}
	if len(decoded) == 0 || len(decoded) > MaxPublicCustomTxtBytes {
		return "", errors.New("custom_.txt payload exceeds public parsing bounds")
	}
	if err := rejectDuplicateJSONKeys(decoded); err != nil {
		return "", fmt.Errorf("custom_.txt payload is not canonical JSON: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(decoded, &raw); err != nil {
		return "", fmt.Errorf("decode custom_.txt JSON: %w", err)
	}
	if raw == nil {
		return "", errors.New("custom_.txt JSON must be an object")
	}
	for key, value := range raw {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return "", fmt.Errorf("custom_.txt field %q must not be null", key)
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var native nativeCustomTxt
	if err := decoder.Decode(&native); err != nil {
		return "", fmt.Errorf("decode typed custom_.txt JSON: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return "", errors.New("custom_.txt JSON contains trailing data")
		}
		return "", fmt.Errorf("decode custom_.txt trailing data: %w", err)
	}
	if native.ConnType != "" {
		if err := validateEnum("conn-type", native.ConnType, "both", "incoming", "outgoing"); err != nil {
			return "", err
		}
	}
	if native.Password != "" {
		if err := validateWorkflowValue("password", native.Password); err != nil {
			return "", err
		}
	}

	public := make(map[string]any, 3)
	if native.ConnType != "" {
		public["conn-type"] = native.ConnType
	}
	if len(native.DefaultSettings) > 0 {
		public["default-settings"] = native.DefaultSettings
	}
	if len(native.OverrideSettings) > 0 {
		public["override-settings"] = native.OverrideSettings
	}
	canonical, err := json.Marshal(public)
	if err != nil {
		return "", fmt.Errorf("marshal public custom_.txt JSON: %w", err)
	}
	return base64.StdEncoding.EncodeToString(canonical), nil
}

func validateBuildSpec(spec BuildSpec) error {
	if spec.Platform != "" {
		if _, err := ParsePlatform(string(spec.Platform)); err != nil {
			return err
		}
		if spec.Platform == PlatformAndroid && !spec.allowMissingAndroidAppID {
			if spec.AndroidAppID == "" {
				return fmt.Errorf("android_app_id is required for Android builds")
			}
			if err := ValidateAndroidAppID(spec.AndroidAppID); err != nil {
				return err
			}
		}
	}
	if _, err := ParseSettingsScope(spec.SettingsScope); err != nil {
		return err
	}
	for _, validation := range []struct {
		name    string
		value   string
		allowed []string
	}{
		{"direction", spec.Direction, []string{"both", "incoming", "outgoing"}},
		{"permissions_type", spec.PermissionsType, []string{"custom", "full", "view"}},
		{"theme", spec.Theme, []string{"light", "dark", "system"}},
		{"pass_approve_mode", spec.PassApproveMode, []string{"password", "click", "password-click", "both"}},
	} {
		if err := validateEnum(validation.name, validation.value, validation.allowed...); err != nil {
			return err
		}
	}
	if spec.HideCM != nil && *spec.HideCM && spec.PermanentPassword == "" {
		return fmt.Errorf("permanent_password is required when hide_cm is true")
	}
	if err := validateBuildSpecTransportFields(spec); err != nil {
		return err
	}
	for _, endpoint := range []struct {
		name  string
		value string
	}{
		{"server_ip", spec.ServerIP},
		{"relay_server", spec.RelayServer},
	} {
		if err := validateEndpoint(endpoint.name, endpoint.value); err != nil {
			return err
		}
	}
	if err := validateAPIURL("api_server", spec.APIServer); err != nil {
		return err
	}
	return nil
}

func validateBuildSpecTransportFields(spec BuildSpec) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{"server_ip", spec.ServerIP},
		{"key", spec.Key},
	} {
		if err := validateWorkflowValue(field.name, field.value); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkflowValue(name, value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s contains invalid UTF-8", name)
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return fmt.Errorf("%s contains unsafe control characters", name)
		}
	}
	return nil
}

// NormalizeWorkflowDispatchParams validates the typed values that may enter a
// workflow payload. Provider-derived identity fields are still overwritten by
// DispatchBuild, but caller-supplied duplicates are type-checked before that
// overwrite so an arbitrary map cannot bypass the normal BuildSpec boundary.
func NormalizeWorkflowDispatchParams(platform string, raw map[string]any) (map[string]any, error) {
	if _, err := ParsePlatform(platform); err != nil {
		return nil, err
	}

	normalized := make(map[string]any, len(raw))
	specRaw := make(map[string]any)
	context := BuildRecordContext{
		Platform: platform,
		AppName:  "dispatch-output",
		Version:  "1.0.0",
	}
	for name, value := range raw {
		switch name {
		case "app_name":
			stringValue, err := dispatchStringValue(name, value)
			if err != nil {
				return nil, err
			}
			if err := ValidateOutputAppName(stringValue); err != nil {
				return nil, err
			}
			context.AppName = stringValue
			normalized[name] = stringValue
		case "server":
			stringValue, err := dispatchStringValue(name, value)
			if err != nil {
				return nil, err
			}
			specRaw["server_ip"] = stringValue
			normalized[name] = stringValue
		case "key":
			stringValue, err := dispatchStringValue(name, value)
			if err != nil {
				return nil, err
			}
			stringValue = config.NormalizePublicKey(stringValue)
			specRaw[name] = stringValue
			normalized[name] = stringValue
		case "custom_txt":
			normalizedValue, ok := value.(NormalizedCustomTxt)
			if !ok {
				return nil, fmt.Errorf("workflow dispatch field %q must be generated from typed BuildSpec", name)
			}
			stringValue := normalizedValue.value
			if err := validateWorkflowValue(name, stringValue); err != nil {
				return nil, err
			}
			normalized[name] = stringValue
		case "android_app_id":
			if platform != string(PlatformAndroid) {
				return nil, fmt.Errorf("workflow dispatch field %q is only valid for Android", name)
			}
			stringValue, err := dispatchStringValue(name, value)
			if err != nil {
				return nil, err
			}
			if err := ValidateAndroidAppID(stringValue); err != nil {
				return nil, err
			}
			specRaw[name] = stringValue
			normalized[name] = stringValue
		case "version":
			stringValue, err := dispatchStringValue(name, value)
			if err != nil {
				return nil, err
			}
			if !utils.ValidateBuildVersion(stringValue) {
				return nil, fmt.Errorf("version has invalid or unsafe format")
			}
			normalized[name] = stringValue
		case "source_sha":
			stringValue, err := dispatchStringValue(name, value)
			if err != nil {
				return nil, err
			}
			if !validGithubSourceSHA(stringValue) {
				return nil, fmt.Errorf("source_sha must be a 40-64 character hexadecimal SHA")
			}
			normalized[name] = stringValue
		case "workflow_repo":
			stringValue, err := dispatchStringValue(name, value)
			if err != nil {
				return nil, err
			}
			if err := validateGithubRepo(stringValue); err != nil {
				return nil, fmt.Errorf("workflow_repo is invalid: %w", err)
			}
			normalized[name] = stringValue
		case "release_repo":
			stringValue, err := dispatchStringValue(name, value)
			if err != nil {
				return nil, err
			}
			if err := validateGithubRepo(stringValue); err != nil {
				return nil, fmt.Errorf("release_repo is invalid: %w", err)
			}
			normalized[name] = stringValue
		case "assets_release_tag":
			stringValue, err := dispatchStringValue(name, value)
			if err != nil {
				return nil, err
			}
			if err := validateWorkflowValue(name, stringValue); err != nil {
				return nil, err
			}
			normalized[name] = stringValue
		case "assets_release_id":
			integer, err := dispatchIntegerValue(name, value)
			if err != nil {
				return nil, err
			}
			normalized[name] = integer
		case "release_assets":
			assets, ok := value.([]ReleaseAsset)
			if !ok {
				return nil, fmt.Errorf("workflow dispatch field %q must use typed release assets", name)
			}
			// This caller value is deliberately retained only until DispatchBuild
			// overwrites it with the resolved catalog identity. Preserve the
			// existing overwrite rule; forged asset metadata must never be used,
			// but it also must not replace the provider-derived payload contract.
			normalized[name] = assets
		default:
			return nil, fmt.Errorf("unsupported workflow dispatch field %q", name)
		}
	}
	if len(specRaw) > 0 {
		if _, err := NormalizeCustomBuild(specRaw, context); err != nil {
			return nil, err
		}
	}
	if platform == string(PlatformAndroid) {
		if _, ok := normalized["android_app_id"]; !ok {
			return nil, fmt.Errorf("android_app_id is required for Android workflow dispatch")
		}
	}
	return normalized, nil
}

func dispatchStringValue(name string, value any) (string, error) {
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("workflow dispatch field %q must be a string", name)
	}
	return stringValue, nil
}

func dispatchIntegerValue(name string, value any) (int64, error) {
	var integer int64
	switch typed := value.(type) {
	case int:
		integer = int64(typed)
	case int64:
		integer = typed
	case float64:
		if typed != float64(int64(typed)) {
			return 0, fmt.Errorf("workflow dispatch field %q must be an integer", name)
		}
		integer = int64(typed)
	default:
		return 0, fmt.Errorf("workflow dispatch field %q must be an integer", name)
	}
	if integer <= 0 {
		return 0, fmt.Errorf("workflow dispatch field %q must be positive", name)
	}
	return integer, nil
}

// ValidateOutputAppName validates the user-authored name used in artifact and
// response filenames. It accepts ordinary Unicode names and spaces, but never
// permits an empty, traversing, or control-bearing path component.
func ValidateOutputAppName(value string) error {
	if err := validateWorkflowValue("app_name", value); err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("app_name is required")
	}
	if len(value) > maxBuildAppNameBytes {
		return fmt.Errorf("app_name exceeds %d bytes", maxBuildAppNameBytes)
	}
	if value == "." || value == ".." || strings.ContainsAny(value, `/\\<>:"|?*`) ||
		strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return fmt.Errorf("app_name must be a single safe filename component")
	}
	if isWindowsReservedDeviceName(value) {
		return fmt.Errorf("app_name uses a reserved Windows device name")
	}
	return nil
}

// ValidateAndroidAppID validates the user-authored Android application/package
// identifier before it can reach Gradle or the Android manifest. It is a
// platform-specific typed field and is never accepted as a generic workflow
// or raw native parameter.
func ValidateAndroidAppID(value string) error {
	if err := validateWorkflowValue("android_app_id", value); err != nil {
		return err
	}
	if len(value) == 0 || len(value) > 255 || !androidAppIDPattern.MatchString(value) {
		return fmt.Errorf("android_app_id must be a lowercase Java package identifier")
	}
	return nil
}

// ValidateWindowsArtifactFilename validates a filename that may cross the
// provider ZIP/publication boundary. It is stricter than app-name validation:
// Windows output is flat, must have a known executable/archive/config
// extension, and may not contain path syntax or device names.
func ValidateWindowsArtifactFilename(value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("artifact filename contains invalid UTF-8")
	}
	if value == "" || len(value) > 255 || value == "." || value == ".." {
		return fmt.Errorf("artifact filename is empty or too long")
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return fmt.Errorf("artifact filename contains unsafe control characters")
		}
	}
	if strings.ContainsAny(value, `/\\<>:"|?*`) || strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return fmt.Errorf("artifact filename contains unsafe Windows path characters")
	}
	if isWindowsReservedDeviceName(value) {
		return fmt.Errorf("artifact filename uses a reserved Windows device name")
	}
	dot := strings.LastIndexByte(value, '.')
	if dot <= 0 {
		return fmt.Errorf("artifact filename must have a supported extension")
	}
	switch strings.ToLower(value[dot:]) {
	case ".dll", ".exe", ".zip", ".txt":
		return nil
	default:
		return fmt.Errorf("artifact filename uses an unsupported extension")
	}
}

// WindowsArtifactNameKey returns the publication identity used for Windows
// artifact names. Windows-delivered output is case-insensitive even when it is
// staged on a case-sensitive filesystem.
func WindowsArtifactNameKey(value string) string {
	return strings.ToLower(value)
}

func isWindowsReservedDeviceName(value string) bool {
	base := value
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	base = strings.TrimRight(base, " .")
	upper := strings.ToUpper(base)
	if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" {
		return true
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		return upper[3] >= '1' && upper[3] <= '9'
	}
	return false
}

func validateEndpoint(name, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	if net.ParseIP(value) != nil {
		return nil
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		if port == "" {
			return fmt.Errorf("%s has invalid endpoint %q", name, value)
		}
		portNumber, err := strconv.ParseUint(port, 10, 16)
		if err != nil || portNumber == 0 {
			return fmt.Errorf("%s has invalid endpoint %q", name, value)
		}
		if strings.HasPrefix(value, "[") && net.ParseIP(host) == nil {
			return fmt.Errorf("%s has invalid endpoint %q", name, value)
		}
		if net.ParseIP(host) == nil {
			if err := validateHost(host); err != nil {
				return fmt.Errorf("%s has invalid endpoint %q: %w", name, value, err)
			}
		}
		return nil
	}
	if strings.Contains(value, ":") {
		return fmt.Errorf("%s has invalid endpoint %q", name, value)
	}
	if err := validateHost(value); err != nil {
		return fmt.Errorf("%s has invalid endpoint %q: %w", name, value, err)
	}
	return nil
}

func validateAPIURL(name, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("%s has invalid URL %q", name, value)
	}
	return nil
}

func validateHost(value string) error {
	if value == "" {
		return fmt.Errorf("invalid host")
	}
	parsed, err := url.Parse("//" + value)
	if err != nil || parsed.Host != value || parsed.Hostname() != value || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("invalid host")
	}
	return nil
}

func optionalString(raw map[string]any, name string) (string, error) {
	value, ok := raw[name]
	if !ok {
		return "", nil
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return stringValue, nil
}

func optionalBool(raw map[string]any, name string) (*bool, error) {
	value, ok := raw[name]
	if !ok {
		return nil, nil
	}
	boolValue, ok := value.(bool)
	if !ok {
		return nil, fmt.Errorf("%s must be a boolean", name)
	}
	return &boolValue, nil
}

func validateEnum(name, value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("%s has unsupported value %q", name, value)
}

func yn(value bool) string {
	if value {
		return "Y"
	}
	return "N"
}
