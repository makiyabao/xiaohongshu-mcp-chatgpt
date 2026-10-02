// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package configs

import (
	"os"
	"strings"
)

// ProtectedUserIDs reads an optional comma-separated allowlist of users that
// cannot be unfollowed unless the caller explicitly sets override=true.
func ProtectedUserIDs() map[string]struct{} {
	users := make(map[string]struct{})
	for _, id := range strings.Split(os.Getenv("XHS_PROTECTED_USER_IDS"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			users[id] = struct{}{}
		}
	}
	return users
}
