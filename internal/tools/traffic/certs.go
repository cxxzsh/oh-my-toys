package traffic

import (
	"context"
	"errors"
	"fmt"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/ca"
)

// Cert is the state of the app's root certificate: whether it exists,
// whether the system trusts it, and where a person can hand it to another
// program.
func (t *Tool) Cert() (tool.CertState, error) {
	a, err := ca.Load(t.caDir)
	if errors.Is(err, ca.ErrNotFound) {
		return tool.CertState{}, nil
	}
	if err != nil {
		return tool.CertState{}, fmt.Errorf("read the root certificate: %w", err)
	}
	state := tool.CertState{
		Exists:      true,
		Fingerprint: a.Fingerprint(),
		Path:        a.CertPath(),
		NotAfter:    a.NotAfter(),
	}
	trusted, err := t.sys.trust.trusted(a.CertPEM())
	if err != nil {
		return state, systemError("read whether the root certificate is trusted", err)
	}
	state.Trusted = trusted
	return state, nil
}

// InstallCert makes the root certificate when there is none, and makes the
// system trust it. The system asks the person for a password itself; a
// refusal comes back as tool.ErrPermission. It holds the tool's actions
// back while it works, because making the root is not safe beside a
// capture that makes one too.
func (t *Tool) InstallCert(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err //nolint:wrapcheck // the caller's own cancellation.
	}
	t.opMu.Lock()
	defer t.opMu.Unlock()
	a, err := t.root(rootMake)
	if err != nil {
		return err
	}
	if err := t.sys.trust.install(a.CertPEM()); err != nil {
		return systemError("trust the root certificate", err)
	}
	return nil
}

// RemoveCert makes the system stop trusting the root certificate and
// deletes it with its key. A trust store that refuses keeps the file, so
// that the root can still be reported and removed later. A running capture
// that opens HTTPS is restarted without making a new root, so that it stops
// using the deleted one and no new secret is made behind the person's
// back.
func (t *Tool) RemoveCert(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err //nolint:wrapcheck // the caller's own cancellation.
	}
	t.opMu.Lock()
	defer t.opMu.Unlock()
	if err := t.removeTrust(); err != nil {
		return err
	}
	return t.restartTunnellingLocked(ctx)
}

// removeTrust takes the certificate out of the trust store and deletes it.
// A trust store that refuses keeps the file: without the PEM the app could
// neither report the trusted root nor remove it.
func (t *Tool) removeTrust() error {
	a, err := ca.Load(t.caDir)
	switch {
	case err == nil:
		if err := t.sys.trust.remove(a.CertPEM()); err != nil {
			return systemError("stop trusting the root certificate", err)
		}
	case errors.Is(err, ca.ErrNotFound):
	default:
		return fmt.Errorf("read the root certificate: %w", err)
	}
	if err := ca.Remove(t.caDir); err != nil {
		return fmt.Errorf("delete the root certificate: %w", err)
	}
	return nil
}

// restartTunnellingLocked restarts a capture that opens HTTPS without a
// root certificate, which is only possible once the root is gone. The
// caller holds opMu.
func (t *Tool) restartTunnellingLocked(ctx context.Context) error {
	t.mu.Lock()
	decrypting := t.proxy != nil && t.proxy.decrypting
	t.mu.Unlock()
	if !decrypting {
		return nil
	}
	if err := t.stopCapture(); err != nil {
		return err
	}
	return t.startCapture(ctx, t.Config(), rootRefuse)
}
