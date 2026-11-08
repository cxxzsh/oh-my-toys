//go:build linux

package sysproxy

import (
	"fmt"
	"os/exec"
	"strings"
)

// The gsettings keys and the two modes live with the shared command
// builders in sysproxy.go, so the sequence is tested on any machine.

// noSchema is what gsettings prints when the GNOME schema is absent, which
// means this desktop has no settings the package can change.
const noSchema = "No such schema"

// runner runs one system command and returns everything it printed, also
// when it fails. A fake replaces it in tests.
type runner func(name string, args ...string) (string, error)

// execRunner runs the command for real. The frozen API of this package
// takes no context, so there is none to stop it with.
func execRunner(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput() //nolint:noctx,gosec // the frozen API takes no context, and the program is a package constant
	return string(out), err
}

func current() (Snapshot, error) { return readGNOME(execRunner) }

func enable(hostport string) (Snapshot, error) { return enableGNOME(execRunner, hostport) }

func restore(s Snapshot) error { return restoreGNOME(execRunner, s) }

func points(s Snapshot, hostport string) bool { return gnomePoints(s, hostport) }

// readGNOME reads the mode and the two schemes of the GNOME proxy.
func readGNOME(r runner) (Snapshot, error) {
	if err := checkGSettings(); err != nil {
		return Snapshot{}, err
	}
	mode, err := gsettingsString(r, gnomeSchema, settingMode)
	if err != nil {
		return Snapshot{}, err
	}
	http, err := readGNOMEProxy(r, gnomeSchema+".http")
	if err != nil {
		return Snapshot{}, err
	}
	https, err := readGNOMEProxy(r, gnomeSchema+".https")
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{GNOME: &GNOMESettings{Mode: mode, HTTP: http, HTTPS: https}}, nil
}

func readGNOMEProxy(r runner, schema string) (GNOMEProxy, error) {
	host, err := gsettingsString(r, schema, settingHost)
	if err != nil {
		return GNOMEProxy{}, err
	}
	out, err := queryGSettings(r, schema, settingPort)
	if err != nil {
		return GNOMEProxy{}, err
	}
	port, err := parseGSettingsInt(out)
	if err != nil {
		return GNOMEProxy{}, fmt.Errorf("%s port: %w", schema, err)
	}
	return GNOMEProxy{Host: host, Port: port}, nil
}

// enableGNOME snapshots the settings, points both schemes at host:port and
// puts the snapshot back if any command fails.
func enableGNOME(r runner, hostport string) (Snapshot, error) {
	host, port, err := parseHostPort(hostport)
	if err != nil {
		return Snapshot{}, err
	}
	snap, err := readGNOME(r)
	if err != nil {
		return Snapshot{}, err
	}
	if err := runCommands(r, gnomeEnableCommands(host, port)); err != nil {
		return snap, rollback(err, restoreGNOME(r, snap))
	}
	return snap, nil
}

func restoreGNOME(r runner, s Snapshot) error {
	if s.GNOME == nil {
		return nil
	}
	if err := checkGSettings(); err != nil {
		return err
	}
	return runCommands(r, gnomeRestoreCommands(*s.GNOME))
}

// gnomePoints reports whether the snapshot is manual mode and both schemes
// point at hostport.
func gnomePoints(s Snapshot, hostport string) bool {
	host, port, err := parseHostPort(hostport)
	if err != nil || s.GNOME == nil {
		return false
	}
	return gnomeSettingsPoint(*s.GNOME, host, port)
}

// restorePointing restores the saved settings only while GNOME still points
// at hostport, because anything else is the person's own change.
func restorePointing(s Snapshot, hostport string) error {
	host, port, err := parseHostPort(hostport)
	if err != nil || s.GNOME == nil {
		return err
	}
	if err := checkGSettings(); err != nil {
		return err
	}
	current, err := readGNOME(execRunner)
	if err != nil {
		return err
	}
	if !gnomeSettingsPoint(*current.GNOME, host, port) {
		return nil
	}
	return runCommands(execRunner, gnomeRestoreCommands(*s.GNOME))
}

// checkGSettings reports ErrUnsupported when the desktop has no gsettings
// to drive, which is how every desktop that is not GNOME-based behaves.
func checkGSettings() error {
	if _, err := exec.LookPath(gsettingsProgram); err != nil {
		return fmt.Errorf("gsettings: %w", ErrUnsupported)
	}
	return nil
}

func gsettingsString(r runner, schema, key string) (string, error) {
	out, err := queryGSettings(r, schema, key)
	if err != nil {
		return "", err
	}
	return parseGSettingsString(out), nil
}

// queryGSettings reads one setting and turns a missing schema into
// ErrUnsupported, since the schema is the whole desktop's proxy support.
func queryGSettings(r runner, schema, key string) (string, error) {
	out, err := r(gsettingsProgram, gsettingsGet, schema, key)
	if err == nil {
		return out, nil
	}
	if strings.Contains(out, noSchema) {
		return "", fmt.Errorf("gsettings get %s %s: %w", schema, key, ErrUnsupported)
	}
	return "", commandFailed("gsettings get "+schema+" "+key, out, err)
}

// runCommands runs the planned gsettings writes in order.
func runCommands(r runner, commands [][]string) error {
	for _, cmd := range commands {
		if len(cmd) == 0 {
			continue
		}
		out, err := r(cmd[0], cmd[1:]...)
		if err == nil {
			continue
		}
		if strings.Contains(out, noSchema) {
			return fmt.Errorf("%s: %w", strings.Join(cmd[1:], " "), ErrUnsupported)
		}
		return commandFailed(strings.Join(cmd, " "), out, err)
	}
	return nil
}
