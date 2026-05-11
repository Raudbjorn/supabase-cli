// Lightweight stub replacing github.com/containers/common/libnetwork/types
// The full package pulls in 130+ transitive dependencies (Podman ecosystem)
// but supabase-cli only uses ErrNetworkExists for error checking.

package types

import "errors"

// ErrNetworkExists indicates that a network with the given name already exists.
var ErrNetworkExists = errors.New("network already exists")
