package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadRepositoryAutoDetect(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Gateway.RepositoryAutoDetect)
	t.Setenv("GATEWAY_REPOSITORY_AUTO_DETECT", "false")
	cfg, err = Load()
	require.NoError(t, err)
	require.False(t, cfg.Gateway.RepositoryAutoDetect)
}
