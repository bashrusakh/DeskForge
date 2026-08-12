package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rustdesk-server/api/model"
)

func TestParseProducerManifestExactSchemaAndBuildIdentity(t *testing.T) {
	build := producerManifestBuild()
	manifest := producerManifestForBuild(build, map[string]string{"rustqs.exe": "exe"})
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProducerManifest(encoded)
	if err != nil {
		t.Fatalf("ParseProducerManifest() error = %v", err)
	}
	if err := ValidateProducerManifestForBuild(parsed, build); err != nil {
		t.Fatalf("ValidateProducerManifestForBuild() error = %v", err)
	}
	if parsed.DigestScope != ProducerManifestDigestScope || len(parsed.Files) != 1 || parsed.Files[0].Name != "rustqs.exe" {
		t.Fatalf("parsed manifest = %#v", parsed)
	}
	linux := producerManifestBuild()
	linux.Platform = "linux"
	linuxManifest := producerManifestForBuild(linux, map[string]string{
		"rustqs-1.2.3-0.x86_64.rpm": "rpm",
		"rustqs-1.2.3.deb":          "deb",
	})
	if encoded, err := json.Marshal(linuxManifest); err != nil {
		t.Fatal(err)
	} else if parsedLinux, err := ParseProducerManifest(encoded); err != nil {
		t.Fatalf("ParseProducerManifest() Linux error = %v", err)
	} else if err := ValidateProducerManifestForBuild(parsedLinux, linux); err != nil {
		t.Fatalf("ValidateProducerManifestForBuild() Linux error = %v", err)
	}
}

func TestParseProducerManifestRejectsSchemaTamperMissingExtraAndDuplicateKeys(t *testing.T) {
	valid := `{"schema":"deskforge.client-artifact","schema_version":1,"platform":"windows","app_name":"rustqs","output_filenames":["rustqs.exe"],"source_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","workflow_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","workflow_ref":"rustqs/min-test","version":"1.2.3","digest_scope":"sha256 covers delivered output files; manifest.txt is excluded","files":[{"name":"rustqs.exe","sha256":"` + strings.Repeat("c", 64) + `"}]}`
	cases := map[string]string{
		"missing field":  strings.Replace(valid, `"workflow_ref":"rustqs/min-test",`, "", 1),
		"unknown field":  strings.Replace(valid, `"schema_version":1,`, `"schema_version":1,"extra":"unsafe",`, 1),
		"duplicate key":  strings.Replace(valid, `"schema_version":1,`, `"schema_version":1,"schema_version":1,`, 1),
		"wrong hash":     strings.Replace(valid, strings.Repeat("c", 64), strings.Repeat("g", 64), 1),
		"case collision": strings.Replace(valid, `"files":[{"name":"rustqs.exe"`, `"files":[{"name":"rustqs.exe"`, 1),
	}
	// The case-collision case is exercised by the output validator below; this
	// table keeps parser failures focused on the JSON/schema boundary.
	delete(cases, "case collision")
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseProducerManifest([]byte(input)); err == nil {
				t.Fatal("ParseProducerManifest() error = nil, want fail-closed schema rejection")
			}
		})
	}
}

func TestValidateProducerManifestOutputRejectsMissingTamperExtraAndCaseCollision(t *testing.T) {
	build := producerManifestBuild()
	contents := map[string]string{"rustqs.exe": "exe"}
	manifest := producerManifestForBuild(build, contents)
	for name, mutate := range map[string]func(string){
		"missing": func(dir string) {
			_ = os.Remove(filepath.Join(dir, "rustqs.exe"))
		},
		"tamper": func(dir string) {
			_ = os.WriteFile(filepath.Join(dir, "rustqs.exe"), []byte("tampered"), 0600)
		},
		"extra": func(dir string) {
			_ = os.WriteFile(filepath.Join(dir, "helper.dll"), []byte("extra"), 0600)
		},
		"case collision": func(dir string) {
			_ = os.WriteFile(filepath.Join(dir, "RUSTQS.EXE"), []byte("collision"), 0600)
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "rustqs.exe"), []byte(contents["rustqs.exe"]), 0600); err != nil {
				t.Fatal(err)
			}
			mutate(dir)
			if _, err := ValidateProducerManifestOutput(manifest, dir); err == nil {
				t.Fatal("ValidateProducerManifestOutput() error = nil, want fail-closed output rejection")
			}
		})
	}
}

func TestProducerManifestOutputHashesAndCompatibilityNames(t *testing.T) {
	for _, tc := range []struct {
		platform string
		app      string
		version  string
		want     []string
	}{
		{platform: "windows", app: "rustqs", version: "1.2.3", want: []string{"rustqs.exe"}},
		{platform: "linux", app: "rustqs", version: "1.2.3", want: []string{"rustqs-1.2.3-0.x86_64.rpm", "rustqs-1.2.3.deb"}},
		{platform: "android", app: "rustqs", version: "1.2.3", want: []string{"rustqs.apk"}},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			got, err := ExpectedProducerOutputFilenames(tc.platform, tc.app, tc.version)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("expected output names = %v, want %v", got, tc.want)
			}
		})
	}
	build := producerManifestBuild()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rustqs.exe"), []byte("exe"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := producerManifestForBuild(build, map[string]string{"rustqs.exe": "exe"})
	total, err := ValidateProducerManifestOutput(manifest, dir)
	if err != nil || total != 3 {
		t.Fatalf("ValidateProducerManifestOutput() = %d, %v; want 3 bytes", total, err)
	}
	wantHash := sha256.Sum256([]byte("exe"))
	if manifest.Files[0].SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("manifest hash = %q, want %x", manifest.Files[0].SHA256, wantHash)
	}
}

func TestProducerManifestPrivateCustomFileIsOptionalAndPlatformCompatible(t *testing.T) {
	platforms := []struct {
		platform string
		app      string
		public   map[string]string
	}{
		{platform: "windows", app: "rustqs", public: map[string]string{"rustqs.exe": "exe"}},
		{platform: "linux", app: "rustqs", public: map[string]string{
			"rustqs-1.2.3-0.x86_64.rpm": "rpm", "rustqs-1.2.3.deb": "deb",
		}},
		{platform: "android", app: "rustqs", public: map[string]string{"rustqs.apk": "apk"}},
		{platform: "bridge", app: "rustdesk-bridge", public: map[string]string{
			"flutter/ios/Runner/bridge_generated.h":     "ios-header",
			"flutter/lib/generated_bridge.dart":         "dart",
			"flutter/lib/generated_bridge.freezed.dart": "freezed",
			"flutter/macos/Runner/bridge_generated.h":   "mac-header",
			"src/bridge_generated.io.rs":                "io-rs",
			"src/bridge_generated.rs":                   "rs",
		}},
	}
	for _, tc := range platforms {
		t.Run(tc.platform, func(t *testing.T) {
			build := producerManifestBuild()
			build.Platform = tc.platform
			build.AppName = tc.app
			manifest := producerManifestForBuild(build, tc.public)
			for _, withPrivate := range []bool{false, true} {
				t.Run(map[bool]string{false: "absent", true: "present"}[withPrivate], func(t *testing.T) {
					dir := t.TempDir()
					for name, contents := range tc.public {
						full := filepath.Join(dir, filepath.FromSlash(name))
						if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(full, []byte(contents), 0600); err != nil {
							t.Fatal(err)
						}
					}
					candidate := manifest
					if withPrivate {
						candidate.PrivateFilenames = []string{"custom_.txt"}
						if err := os.WriteFile(filepath.Join(dir, "custom_.txt"), []byte("private settings"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					if total, err := ValidateProducerManifestOutput(candidate, dir); err != nil {
						t.Fatalf("ValidateProducerManifestOutput() error = %v", err)
					} else if withPrivate && total != int64(len("exe"+"private settings")) && tc.platform == "windows" {
						t.Fatalf("output total = %d, want private bytes included in bounded total", total)
					}
				})
			}
			encoded, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			withoutOptionalField := strings.Replace(string(encoded), `,"private_filenames":null`, "", 1)
			if _, err := ParseProducerManifest([]byte(withoutOptionalField)); err != nil {
				t.Fatalf("ParseProducerManifest() rejected absent optional private_filenames: %v", err)
			}
		})
	}
}

func TestProducerManifestPrivateFileDeclarationRejectsMisdeclaredOrSecretFiles(t *testing.T) {
	build := producerManifestBuild()
	manifest := producerManifestForBuild(build, map[string]string{"rustqs.exe": "exe"})
	for _, privateFilenames := range [][]string{{"secret.txt"}, {"custom_.txt", "custom_.txt"}} {
		candidate := manifest
		candidate.PrivateFilenames = privateFilenames
		encoded, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseProducerManifest(encoded); err == nil {
			t.Fatalf("ParseProducerManifest() accepted private_filenames=%v", privateFilenames)
		}
	}

	publicMisdeclared := manifest
	publicMisdeclared.Files = append([]ProducerManifestFile(nil), manifest.Files...)
	publicMisdeclared.Files[0].Name = "custom_.txt"
	encoded, err := json.Marshal(publicMisdeclared)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseProducerManifest(encoded); err == nil {
		t.Fatal("ParseProducerManifest() accepted custom_.txt in public files")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rustqs.exe"), []byte("exe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "custom_.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateProducerManifestOutput(manifest, dir); err == nil {
		t.Fatal("ValidateProducerManifestOutput() accepted undeclared custom_.txt")
	}
	manifest.PrivateFilenames = []string{"custom_.txt"}
	if err := os.WriteFile(filepath.Join(dir, "other-secret.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateProducerManifestOutput(manifest, dir); err == nil {
		t.Fatal("ValidateProducerManifestOutput() accepted an unlisted secret file")
	}
}

func TestProducerManifestBridgeAllowsNestedFilesAndRejectsUnsafeOrExtraOutputs(t *testing.T) {
	build := &model.CustomBuild{
		Platform:         "bridge",
		AppName:          "rustdesk-bridge",
		Version:          "1.2.3",
		BuildRef:         strings.Repeat("a", 40),
		GithubRef:        strings.Repeat("b", 40),
		WorkflowSelector: "bridge",
	}
	names, err := ExpectedProducerOutputFilenames(build.Platform, build.AppName, build.Version)
	if err != nil {
		t.Fatal(err)
	}
	contents := make(map[string]string, len(names))
	manifest := producerManifestForBuild(build, contents)
	dir := t.TempDir()
	for _, name := range names {
		contents[name] = name
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatalf("create bridge output parent for %q: %v", name, err)
		}
		if err := os.WriteFile(full, []byte(name), 0600); err != nil {
			t.Fatalf("write bridge output %q: %v", name, err)
		}
	}
	manifest = producerManifestForBuild(build, contents)
	if total, err := ValidateProducerManifestOutput(manifest, dir); err != nil {
		t.Fatalf("ValidateProducerManifestOutput() bridge error = %v", err)
	} else if total == 0 {
		t.Fatal("ValidateProducerManifestOutput() bridge total = 0, want nested files")
	}

	for _, unsafeName := range []string{"../escape", "/absolute", "C:/absolute", "flutter/../escape", "flutter\\escape"} {
		t.Run("unsafe "+unsafeName, func(t *testing.T) {
			candidate := manifest
			candidate.OutputFilenames = append([]string(nil), manifest.OutputFilenames...)
			candidate.Files = append([]ProducerManifestFile(nil), manifest.Files...)
			candidate.OutputFilenames[0] = unsafeName
			candidate.Files[0].Name = unsafeName
			encoded, marshalErr := json.Marshal(candidate)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, parseErr := ParseProducerManifest(encoded); parseErr == nil {
				t.Fatalf("ParseProducerManifest() accepted unsafe bridge path %q", unsafeName)
			}
		})
	}

	t.Run("bridge symlink", func(t *testing.T) {
		linkTarget := filepath.Join(dir, "outside")
		if err := os.WriteFile(linkTarget, []byte("outside"), 0600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, filepath.FromSlash(names[0]))
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(linkTarget, link); err != nil {
			t.Fatal(err)
		}
		if _, err := ValidateProducerManifestOutput(manifest, dir); err == nil {
			t.Fatal("ValidateProducerManifestOutput() accepted a bridge symlink")
		}
	})

	t.Run("bridge extra file", func(t *testing.T) {
		extra := filepath.Join(dir, "flutter", "extra.dart")
		if err := os.WriteFile(extra, []byte("extra"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ValidateProducerManifestOutput(manifest, dir); err == nil {
			t.Fatal("ValidateProducerManifestOutput() accepted an extra bridge output")
		}
	})
}

func TestValidateProducerManifestOutputKeepsFinalPlatformsFlat(t *testing.T) {
	build := producerManifestBuild()
	manifest := producerManifestForBuild(build, map[string]string{"rustqs.exe": "exe"})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rustqs.exe"), []byte("exe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "extra.bin"), []byte("extra"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateProducerManifestOutput(manifest, dir); err == nil {
		t.Fatal("ValidateProducerManifestOutput() accepted nested final-platform output")
	}
}

func TestProducerManifestProvenanceRejectsMissingMismatchAndTamperedFields(t *testing.T) {
	build := producerManifestBuild()
	manifest := producerManifestForBuild(build, map[string]string{"rustqs.exe": "exe"})
	valid, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*ProducerManifest)
		parse  bool
	}{
		{name: "missing source tree", parse: true, mutate: func(value *ProducerManifest) { value.SourceTreeSHA = "" }},
		{name: "mismatched source", mutate: func(value *ProducerManifest) { value.SourceSHA = strings.Repeat("d", 40) }},
		{name: "tampered submodule", mutate: func(value *ProducerManifest) {
			value.Submodules = []ProducerManifestSubmodule{{Path: "libs/example", CommitSHA: strings.Repeat("d", 40)}}
		}},
		{name: "missing verification", parse: true, mutate: func(value *ProducerManifest) { value.VerificationScope = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := manifest
			tc.mutate(&candidate)
			encoded, marshalErr := json.Marshal(candidate)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			parsed, parseErr := ParseProducerManifest(encoded)
			if tc.parse {
				if parseErr == nil {
					t.Fatal("ParseProducerManifest() error = nil, want fail-closed shape rejection")
				}
				return
			}
			if parseErr != nil {
				t.Fatalf("ParseProducerManifest() error = %v, want shape-valid candidate", parseErr)
			}
			storedJSON, storedErr := manifest.StoredJSON()
			if storedErr != nil {
				t.Fatal(storedErr)
			}
			build.ProducerManifestJSON = storedJSON
			if err := ValidateProducerManifestForBuild(parsed, build); err == nil {
				t.Fatal("ValidateProducerManifestForBuild() error = nil, want stored provenance mismatch rejection")
			}
		})
	}
	if _, err := ParseProducerManifest(valid); err != nil {
		t.Fatalf("valid provenance manifest rejected: %v", err)
	}
}

func TestProducerManifestV2VerifiedSelfReportIsNormalizedToReported(t *testing.T) {
	manifest := producerManifestForBuild(producerManifestBuild(), map[string]string{"rustqs.exe": "exe"})
	manifest.VerificationScope = producerManifestLegacyScope
	manifest.VerificationResult = "verified"
	stored, err := manifest.StoredJSON()
	if err != nil {
		t.Fatalf("StoredJSON() rejected compatible v2 self-report: %v", err)
	}
	if strings.Contains(stored, `"verification_result":"verified"`) || !strings.Contains(stored, `"verification_result":"reported"`) {
		t.Fatalf("stored producer manifest retained verified self-report: %s", stored)
	}
}

func producerManifestBuild() *model.CustomBuild {
	return &model.CustomBuild{
		Platform:         "windows",
		AppName:          "rustqs",
		Version:          "1.2.3",
		BuildRef:         strings.Repeat("a", 40),
		WorkflowSelector: "rustqs/min-test",
		GithubRef:        strings.Repeat("b", 40),
		GithubSourceSha:  strings.Repeat("b", 40),
		GithubRunId:      77,
	}
}

func producerManifestForBuild(build *model.CustomBuild, contents map[string]string) ProducerManifest {
	names, err := ExpectedProducerOutputFilenames(build.Platform, build.AppName, build.Version)
	if err != nil {
		panic(err)
	}
	files := make([]ProducerManifestFile, 0, len(names))
	for _, name := range names {
		digest := sha256.Sum256([]byte(contents[name]))
		files = append(files, ProducerManifestFile{Name: name, Size: int64(len(contents[name])), SHA256: hex.EncodeToString(digest[:])})
	}
	return ProducerManifest{
		Schema:               ProducerManifestSchema,
		ManifestSchema:       ProducerManifestSchema,
		SchemaVersion:        ProducerManifestVersion,
		Platform:             build.Platform,
		AppName:              build.AppName,
		OutputFilenames:      append([]string(nil), names...),
		SourceSHA:            build.BuildRef,
		WorkflowSHA:          build.GithubRef,
		WorkflowRef:          build.WorkflowSelector,
		Version:              build.Version,
		SourceTreeSHA:        strings.Repeat("c", 40),
		Submodules:           []ProducerManifestSubmodule{{Path: "libs/hbb_common", CommitSHA: strings.Repeat("d", 40)}},
		DigestScope:          ProducerManifestDigestScope,
		VerificationScope:    ProducerManifestVerificationScope,
		VerificationResult:   ProducerManifestVerificationResult,
		PublicationTimestamp: 1700000000,
		HandoffContract:      ProducerManifestHandoffContract,
		Files:                files,
	}
}
