package traffic

import (
	"errors"
	"fmt"

	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
	"github.com/cxxzsh/oh-my-toys/internal/platform/sysproxy"
	"github.com/cxxzsh/oh-my-toys/internal/platform/trust"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// ErrNotRunning is what an action that needs a capture in progress returns
// when none runs.
var ErrNotRunning = errors.New("no capture is running")

// ErrBadConfig is what SetConfig returns when the settings cannot work;
// the detail says which field is wrong, in the words of a log.
var ErrBadConfig = errors.New("capture settings are not usable")

// systemError wraps what the system proxy or the trust store returned, so
// that the UI can tell a refusal (a password is needed) from a system that
// cannot do it at all. The original error stays in the chain for the log.
func systemError(what string, err error) error {
	switch {
	case errors.Is(err, sysproxy.ErrPermission), errors.Is(err, trust.ErrPermission):
		return fmt.Errorf("%s: %w: %w", what, tool.ErrPermission, err)
	case errors.Is(err, sysproxy.ErrUnsupported), errors.Is(err, trust.ErrUnsupported):
		return fmt.Errorf("%s: %w: %w", what, tool.ErrUnsupportedHere, err)
	default:
		return fmt.Errorf("%s: %w", what, err)
	}
}

// packetError wraps what the packet source returned. what names the step,
// such as "open en0", and iface the interface it was about.
func packetError(what, iface string, err error) error {
	where := what
	if iface != "" {
		where = what + " " + iface
	}
	switch {
	case errors.Is(err, pcap.ErrPermission):
		return fmt.Errorf("%s: %w: %w", where, tool.ErrPermission, err)
	case errors.Is(err, pcap.ErrDriverMissing):
		return fmt.Errorf("%s: %w: %w", where, tool.ErrDriverMissing, err)
	case errors.Is(err, pcap.ErrUnsupported):
		return fmt.Errorf("%s: %w: %w", where, tool.ErrUnsupportedHere, err)
	default:
		return fmt.Errorf("%s: %w", where, err)
	}
}
