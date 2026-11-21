package service

import (
	"context"
	"net/url"
	"strings"
)

// UsageRepositoryHeader is client-supplied attribution, never an authorization
// or pricing input. Automatic Codex discovery is handled separately and only
// consumes scoped markers in structured tool results, never free-form prompts.
const UsageRepositoryHeader = "X-Sub2api-Repository"

type usageRepositoryContextKey struct{}

// NormalizeUsageRepository stores a host/path identity, discarding URL credentials,
// query strings and fragments. Worktrees and SSH/HTTPS remotes share an identity.
func NormalizeUsageRepository(raw string) string {
	if len(raw) > 2048 {
		return ""
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		if at := strings.IndexByte(raw, '@'); at >= 0 {
			colon := strings.IndexByte(raw[at+1:], ':')
			if colon < 0 {
				return ""
			}
			colon += at + 1
			raw = "ssh://" + raw[:colon] + "/" + raw[colon+1:]
		} else {
			raw = "https://" + raw
		}
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh") || u.Hostname() == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	defaultPort := (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") || (u.Scheme == "ssh" && port == "22")
	if port != "" && !defaultPort {
		host += ":" + port
	}
	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return ""
		}
	}
	if host == "github.com" {
		path = strings.ToLower(path)
	}
	identity := host + "/" + path
	if len(identity) > 512 {
		return ""
	}
	for _, c := range identity {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._/:", c)
		if !valid {
			return ""
		}
	}
	return identity
}

// WithUsageRepository does no I/O and leaves the context untouched when absent.
func WithUsageRepository(ctx context.Context, raw string) context.Context {
	if repository := NormalizeUsageRepository(raw); repository != "" {
		return context.WithValue(ctx, usageRepositoryContextKey{}, repository)
	}
	return ctx
}

// CopyUsageRepositoryContext copies only the small immutable identifier into a
// worker context, without retaining the original request or its body.
func CopyUsageRepositoryContext(parent, base context.Context) context.Context {
	if parent != nil {
		if repository, _ := parent.Value(usageRepositoryContextKey{}).(string); repository != "" {
			return context.WithValue(base, usageRepositoryContextKey{}, repository)
		}
	}
	return base
}

// UsageRepositoryFromContext returns immutable request attribution, or nil.
func UsageRepositoryFromContext(ctx context.Context) *string {
	if ctx != nil {
		if repository, _ := ctx.Value(usageRepositoryContextKey{}).(string); repository != "" {
			return &repository
		}
	}
	return nil
}
