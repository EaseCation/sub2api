//go:build unit

package adminaccess

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepositoryUsageRequiresGlobalUsageRead(t *testing.T) {
	path := "/api/v1/admin/usage/repositories"
	require.False(t, RouteAllowed(Policy{}, "GET", path))
	require.False(t, RouteAllowed(Policy{GlobalPermissions: []string{"users.read"}}, "GET", path))
	require.False(t, RouteAllowed(Policy{Rules: []Rule{{Permissions: []string{"accounts.read"}, GroupIDs: []int64{1}}}}, "GET", path))
	require.True(t, RouteAllowed(Policy{GlobalPermissions: []string{"usage.read"}}, "GET", path))
}
