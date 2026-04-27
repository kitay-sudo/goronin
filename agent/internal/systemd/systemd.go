// Package systemd writes the goronin.service unit file and wraps systemctl
// for the start/stop/restart/status/logs CLI commands.
//
// The agent itself is a single binary at /usr/local/bin/goronin running as
// root (it needs to bind low-trap ports, manage iptables, and watch /root
// for canary access). Service file is minimal — no PrivateTmp etc., to
// avoid breaking iptables and journalctl access on older systemd versions.
package systemd

import (
	"fmt"
	"os"
	"os/exec"
)

// UnitPath is the canonical location for the systemd unit file.
const UnitPath = "/etc/systemd/system/goronin.service"

// ServiceName is the systemd unit name.
const ServiceName = "goronin.service"

// unitTemplate is the rendered service file. ExecStart points at the
// installed binary's `daemon` subcommand so re-running `goronin` from
// the shell doesn't accidentally double-fork.
const unitTemplate = `[Unit]
Description=GORONIN — honeypot guard
Documentation=https://github.com/kitay-sudo/goronin
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s daemon
Restart=on-failure
RestartSec=5s
User=root
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`

// Install writes the unit file and runs `systemctl daemon-reload`.
// binaryPath is where the agent binary actually lives (usually
// /usr/local/bin/goronin).
func Install(binaryPath string) error {
	content := fmt.Sprintf(unitTemplate, binaryPath)
	if err := os.WriteFile(UnitPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}
	if err := run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	return nil
}

// Enable marks the service to start on boot.
func Enable() error { return run("systemctl", "enable", ServiceName) }

// Start launches the service now.
func Start() error { return run("systemctl", "start", ServiceName) }

// Stop halts the service.
func Stop() error { return run("systemctl", "stop", ServiceName) }

// Restart bounces the service.
func Restart() error { return run("systemctl", "restart", ServiceName) }

// Status streams `systemctl status` output to stdout.
func Status() error { return runInteractive("systemctl", "status", ServiceName, "--no-pager") }

// Logs streams `journalctl -u goronin -f` to stdout. Caller can Ctrl-C.
// follow=false runs without -f and shows the last 200 lines.
func Logs(follow bool) error {
	args := []string{"-u", ServiceName, "-n", "200"}
	if follow {
		args = append(args, "-f")
	}
	return runInteractive("journalctl", args...)
}

// run executes a command and returns combined output as part of the error
// message when it fails. Used for non-interactive wrapper calls.
func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %v: %s", name, args, err, string(out))
	}
	return nil
}

// runInteractive wires the child process directly to the parent's
// stdout/stderr/stdin so the user sees colors, can scroll, and Ctrl-C
// works (for `journalctl -f`).
func runInteractive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
