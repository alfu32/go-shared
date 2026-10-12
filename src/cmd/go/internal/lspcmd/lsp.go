// Package lspcmd provides the go lsp entry point for the GoX language server.
package lspcmd

import (
	"cmd/go/internal/base"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var Cmd = &base.Command{
	UsageLine:   "go lsp [goxpls arguments]",
	Short:       "run the GoX language server",
	Long:        "Run goxpls with the supplied arguments. goxpls is the language server for Go and GoX.",
	CustomFlags: true,
	Run:         run,
}

func run(_ context.Context, _ *base.Command, args []string) {
	path, err := serverPath()
	if err != nil {
		base.Fatal(err)
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		base.Fatal(err)
	}
}

func serverPath() (string, error) {
	if exe, err := os.Executable(); err == nil {
		name := "goxpls"
		if filepath.Ext(exe) == ".exe" {
			name += ".exe"
		}
		path := filepath.Join(filepath.Dir(exe), name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	if path, err := exec.LookPath("goxpls"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("goxpls not found next to go or on PATH; install the GoX toolchain to use 'go lsp'")
}
