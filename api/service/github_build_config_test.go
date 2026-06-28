package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestCompareSemver проверяет упорядочивание compareSemver для стабильных
// и pre-release версий: numeric > numeric-equal-non-numeric ("1.4.8" > "1.4.8-beta"),
// trailing-segments-равны только если non-zero, pre-release между равными numeric
// сегментами ("1.4.8-beta" < "1.4.8-beta.1").
func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.4.8", "1.4.7", 1},
		{"1.4.7", "1.4.8", -1},
		{"1.4.8", "1.4.8", 0},
		{"1.4.8-beta", "1.4.8", -1},
		{"1.4.8", "1.4.8-beta", 1},
		{"1.4.8-beta", "1.4.8-beta.1", -1},
		{"1.4.8-beta.1", "1.4.8-beta", 1},
		{"1.4.8-beta", "1.4.8-beta", 0},
	}
	for _, c := range cases {
		got := compareSemver(c.a, c.b)
		if got != c.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestGetAvailableVersionsCtxCancel подтверждает, что отменённый/протухший
// caller-контекст возвращается сразу через select на ctx.Done(), не дожидаясь
// detached shared refresh. Используем уже отменённый контекст, чтобы select
// немедленно выбрал ветку ctx.Done().
//
// Тест НЕ мокает GitHub API: первый запуск инициирует реальный fetchReleases
// в shared goroutine, но caller возвращается мгновенно через ctx.Err().
// Чтобы не оставлять в кеше результат, блокирующий последующие тесты —
// сбрасываем releasesCache после.
func TestGetAvailableVersionsCtxCancel(t *testing.T) {
	s := &GithubBuildConfigService{}

	// Сбросить кеш, чтобы попасть в singleflight-ветку.
	releasesCache.mu.Lock()
	releasesCache.versions = nil
	releasesCache.cachedAt = time.Time{}
	releasesCache.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменяем заранее

	start := time.Now()
	versions, err := s.GetAvailableVersions(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got err=%v versions=%v", err, versions)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("GetAvailableVersions did not honor ctx cancellation: took %v", elapsed)
	}

	// Сбрасываем кеш, чтобы фоновый fetch не оставил в нём результат
	// (тест не делает assertion на сеть).
	releasesCache.mu.Lock()
	releasesCache.versions = nil
	releasesCache.cachedAt = time.Time{}
	releasesCache.mu.Unlock()
}
