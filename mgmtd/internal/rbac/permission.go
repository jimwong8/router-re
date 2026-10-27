package rbac

import (
	"net/http"
	"strings"
)

// Can answers whether a role may perform method+path.
//   - admin:    full access (every method, every path).
//   - operator: all methods on all paths EXCEPT user-management mutations
//     (POST/DELETE under /api/v1/users).
//   - viewer:  read-only — only GET.
//
// Unknown roles deny by default (fail-closed).
func Can(role Role, method, path string) bool {
	switch role {
	case RoleAdmin:
		return true
	case RoleOperator:
		return !isUserManagement(method, path)
	case RoleViewer:
		return strings.EqualFold(method, http.MethodGet)
	default:
		return false
	}
}

// isUserManagement matches POST/DELETE against the /api/v1/users surface,
// which only admins may invoke.
func isUserManagement(method, path string) bool {
	if !strings.HasPrefix(path, "/api/v1/users") {
		return false
	}
	m := strings.ToUpper(strings.TrimSpace(method))
	return m == http.MethodPost || m == http.MethodDelete
}
