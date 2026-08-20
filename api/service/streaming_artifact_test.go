package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func serviceArtifactZip(t *testing.T, contents string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("artifact.bin")
	if err != nil {
		t.Fatalf("create artifact entry: %v", err)
	}
	if _, err := entry.Write([]byte(contents)); err != nil {
		t.Fatalf("write artifact entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close artifact ZIP: %v", err)
	}
	return buffer.Bytes()
}

type failingArtifactReader struct {
	remaining int
}

func (r *failingArtifactReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, errors.New("simulated interrupted download")
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	return n, nil
}

func artifactTempFiles(t *testing.T, dir string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "deskforge-artifact-*"))
	if err != nil {
		t.Fatalf("glob artifact temp files: %v", err)
	}
	return paths
}

func TestDownloadArtifactStreamsToBoundedTemporaryArchive(t *testing.T) {
	previousLimit := githubArtifactBodyLimit
	previousTempDir := githubArtifactTempDir
	payload := serviceArtifactZip(t, strings.Repeat("z", 32))
	githubArtifactBodyLimit = int64(len(payload))
	githubArtifactTempDir = t.TempDir()
	t.Cleanup(func() {
		githubArtifactBodyLimit = previousLimit
		githubArtifactTempDir = previousTempDir
	})
	before := artifactTempFiles(t, githubArtifactTempDir)
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/repos/owner/repo/actions/runs/7/artifacts":
			return githubResponse(http.StatusOK, `{"artifacts":[{"id":42,"name":"artifact"}]}`, nil), nil
		case "/repos/owner/repo/actions/artifacts/42/zip":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(payload)),
			}, nil
		default:
			return githubResponse(http.StatusNotFound, `{"message":"unexpected endpoint"}`, nil), nil
		}
	}))

	result, err := (&GithubBuildConfigService{}).DownloadArtifact(context.Background(), githubConfig(), 7, 42, "artifact")
	if err != nil {
		t.Fatalf("DownloadArtifact() error = %v", err)
	}
	if result.ArtifactID != 42 || result.ArtifactName != "artifact" || result.Size != int64(len(payload)) {
		t.Fatalf("DownloadArtifact() result = %#v, want exact identity and size", result)
	}
	data, err := os.ReadFile(result.ArchivePath)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("temporary archive = %d bytes, err=%v", len(data), err)
	}
	if strings.HasSuffix(result.ArchivePath, ".part") {
		t.Fatalf("returned path still has partial suffix: %q", result.ArchivePath)
	}
	if err := os.Remove(result.ArchivePath); err != nil {
		t.Fatalf("remove successful temporary archive: %v", err)
	}
	if after := artifactTempFiles(t, githubArtifactTempDir); len(after) != len(before) {
		t.Fatalf("temporary files after successful cleanup = %v, before=%v", after, before)
	}
}

func TestDownloadArtifactCleansPartialAndOversizedTemporaryFiles(t *testing.T) {
	previousLimit := githubArtifactBodyLimit
	previousTempDir := githubArtifactTempDir
	githubArtifactBodyLimit = 16
	githubArtifactTempDir = t.TempDir()
	t.Cleanup(func() {
		githubArtifactBodyLimit = previousLimit
		githubArtifactTempDir = previousTempDir
	})

	t.Run("partial body", func(t *testing.T) {
		before := artifactTempFiles(t, githubArtifactTempDir)
		withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.HasSuffix(req.URL.Path, "/actions/runs/7/artifacts") {
				return githubResponse(http.StatusOK, `{"artifacts":[{"id":42,"name":"artifact"}]}`, nil), nil
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(&failingArtifactReader{remaining: 4})}, nil
		}))
		_, err := (&GithubBuildConfigService{}).DownloadArtifact(context.Background(), githubConfig(), 7, 42, "artifact")
		var transportErr *GithubTransportError
		if !errors.As(err, &transportErr) {
			t.Fatalf("partial download error = %T %v, want transport error", err, err)
		}
		if after := artifactTempFiles(t, githubArtifactTempDir); len(after) != len(before) {
			t.Fatalf("partial download left temp files = %v, before=%v", after, before)
		}
	})

	t.Run("oversized body", func(t *testing.T) {
		before := artifactTempFiles(t, githubArtifactTempDir)
		withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.HasSuffix(req.URL.Path, "/actions/runs/7/artifacts") {
				return githubResponse(http.StatusOK, `{"artifacts":[{"id":42,"name":"artifact"}]}`, nil), nil
			}
			return &http.Response{
				StatusCode:    http.StatusOK,
				Header:        make(http.Header),
				ContentLength: 17,
				Body:          io.NopCloser(strings.NewReader(strings.Repeat("x", 17))),
			}, nil
		}))
		_, err := (&GithubBuildConfigService{}).DownloadArtifact(context.Background(), githubConfig(), 7, 42, "artifact")
		var contractErr *GithubContractError
		if !errors.As(err, &contractErr) || !strings.Contains(err.Error(), "exceeds limit") {
			t.Fatalf("oversized download error = %T %v, want bounded contract error", err, err)
		}
		if after := artifactTempFiles(t, githubArtifactTempDir); len(after) != len(before) {
			t.Fatalf("oversized download left temp files = %v, before=%v", after, before)
		}
	})
}

func TestDownloadArtifactCancellationCleansTemporaryFile(t *testing.T) {
	previousTempDir := githubArtifactTempDir
	githubArtifactTempDir = t.TempDir()
	t.Cleanup(func() { githubArtifactTempDir = previousTempDir })
	before := artifactTempFiles(t, githubArtifactTempDir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/actions/runs/7/artifacts") {
			return githubResponse(http.StatusOK, `{"artifacts":[{"id":42,"name":"artifact"}]}`, nil), nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("cancelled"))}, nil
	}))
	_, err := (&GithubBuildConfigService{}).DownloadArtifact(ctx, githubConfig(), 7, 42, "artifact")
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("cancelled download error = %v, want context cancellation", err)
	}
	if after := artifactTempFiles(t, githubArtifactTempDir); len(after) != len(before) {
		t.Fatalf("cancelled download left temp files = %v, before=%v", after, before)
	}
}

func TestDownloadArtifactRejectsStoredIDNotBelongingToRun(t *testing.T) {
	zipRequests := 0
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/actions/runs/7/artifacts") {
			return githubResponse(http.StatusOK, `{"artifacts":[{"id":99,"name":"artifact"}]}`, nil), nil
		}
		if strings.HasSuffix(req.URL.Path, "/actions/artifacts/42/zip") {
			zipRequests++
		}
		return githubResponse(http.StatusNotFound, `{"message":"unexpected ZIP request"}`, nil), nil
	}))
	_, err := (&GithubBuildConfigService{}).DownloadArtifact(context.Background(), githubConfig(), 7, 42, "artifact")
	if err == nil || !strings.Contains(err.Error(), "does not match run artifact id") {
		t.Fatalf("DownloadArtifact() error = %v, want wrong-run identity rejection", err)
	}
	if zipRequests != 0 {
		t.Fatal("DownloadArtifact() requested ZIP for an artifact absent from the stored run")
	}
}

func TestDownloadArtifactRejectsExpiredStoredArtifact(t *testing.T) {
	withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/actions/runs/7/artifacts") {
			return githubResponse(http.StatusOK, `{"artifacts":[{"id":42,"name":"artifact","expired":true}]}`, nil), nil
		}
		return githubResponse(http.StatusNotFound, `{"message":"ZIP must not be requested"}`, nil), nil
	}))
	_, err := (&GithubBuildConfigService{}).DownloadArtifact(context.Background(), githubConfig(), 7, 42, "artifact")
	var unavailable *GithubArtifactUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("DownloadArtifact() error = %T %v, want expired artifact rejection", err, err)
	}
}

func TestDownloadArtifactRejectsFixedLengthAndChunkedShortBodies(t *testing.T) {
	payload := serviceArtifactZip(t, "short body test")
	previousTempDir := githubArtifactTempDir
	githubArtifactTempDir = t.TempDir()
	t.Cleanup(func() { githubArtifactTempDir = previousTempDir })
	for _, tc := range []struct {
		name   string
		header bool
	}{
		{name: "fixed length", header: true},
		{name: "chunked", header: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withGithubTransport(t, githubRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/actions/runs/7/artifacts") {
					return githubResponse(http.StatusOK, `{"artifacts":[{"id":42,"name":"artifact"}]}`, nil), nil
				}
				body := payload[:len(payload)-1]
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}
				if tc.header {
					resp.Header.Set("Content-Length", strconv.Itoa(len(payload)))
				}
				return resp, nil
			}))
			_, err := (&GithubBuildConfigService{}).DownloadArtifact(context.Background(), githubConfig(), 7, 42, "artifact")
			if err == nil || !strings.Contains(err.Error(), "invalid ZIP archive") && !strings.Contains(err.Error(), "does not match Content-Length") {
				t.Fatalf("DownloadArtifact() error = %v, want short-body rejection", err)
			}
			if files := artifactTempFiles(t, githubArtifactTempDir); len(files) != 0 {
				t.Fatalf("short body left temporary files: %v", files)
			}
		})
	}
}
