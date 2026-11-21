package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeUsageRepository(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/Owner/Repo.git",
		"git@github.com:Owner/Repo.git",
		"ssh://git@github.com/Owner/Repo.git",
		"github.com/owner/repo/",
		"https://user:secret@github.com/Owner/Repo.git?token=secret#branch",
	} {
		t.Run(raw, func(t *testing.T) { require.Equal(t, "github.com/owner/repo", NormalizeUsageRepository(raw)) })
	}
	require.Equal(t, "git.example.com/team/subgroup/Repo", NormalizeUsageRepository("https://git.example.com/team/subgroup/Repo.git"))
	for _, raw := range []string{"", "file:///home/user/project", "/local/project", "https://github.com/owner", "https://github.com/owner/../repo", "https://github.com/a/%0Asecret", strings.Repeat("a", 2049), "github.com/team/" + strings.Repeat("a", 512)} {
		t.Run("invalid/"+raw[:min(30, len(raw))], func(t *testing.T) { require.Empty(t, NormalizeUsageRepository(raw)) })
	}
}

func TestUsageRepositoryContextAbsentAndDetached(t *testing.T) {
	base := context.Background()
	require.Equal(t, base, WithUsageRepository(base, ""))
	require.Nil(t, UsageRepositoryFromContext(base))
	request, cancel := context.WithCancel(WithUsageRepository(base, "git@github.com:owner/repo.git"))
	worker := CopyUsageRepositoryContext(request, base)
	cancel()
	require.NoError(t, worker.Err())
	require.Equal(t, "github.com/owner/repo", *UsageRepositoryFromContext(worker))
}

func BenchmarkUsageRepositoryCapture(b *testing.B) {
	for name, raw := range map[string]string{"absent": "", "canonical": "github.com/owner/repo", "ssh": "git@github.com:owner/repo.git"} {
		b.Run(name, func(b *testing.B) {
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				_ = WithUsageRepository(ctx, raw)
			}
		})
	}
}
