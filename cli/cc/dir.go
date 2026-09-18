package cc

import (
	"os"
	"path/filepath"

	"github.com/threehook/eamerald/cli/x"
	"github.com/threehook/eamerald/internal/fs"
	"github.com/threehook/eamerald/internal/xdg"
)

// Common eamerald directory paths and operations.

// GetEameraldDir returns the eamerald root directory ($HOME/.config/eamerald).
func GetEameraldDir() string {
	if eameraldDir := os.Getenv(x.EnvEameraldDir); eameraldDir != "" {
		return eameraldDir
	}

	return filepath.Clean(filepath.Join(xdg.ConfigHome, "eamerald"))
}

// GetEameraldCfgDir returns the eamerald config directory ($XDG_CONFIG_HOME/eamerald/cfg).
func GetEameraldCfgDir() string {
	if cfgDir := os.Getenv(x.EnvEameraldCfgDir); cfgDir != "" {
		return cfgDir
	}

	return filepath.Clean(filepath.Join(xdg.ConfigHome, "eamerald", "cfg"))
}

// GetEameraldCertsDir returns the eamerald certs directory ($XDG_DATA_HOME/eamerald/certs).
func GetEameraldCertsDir() string {
	if certsDir := os.Getenv(x.EnvEameraldCertsDir); certsDir != "" {
		return certsDir
	}

	return filepath.Clean(filepath.Join(xdg.DataHome, "eamerald", "certs"))
}

// GetEameraldDataDir returns the eamerald db directory ($XDG_DATA_HOME/eamerald/db).
func GetEameraldDataDir() string {
	if dataDir := os.Getenv(x.EnvEameraldDBDir); dataDir != "" {
		return dataDir
	}

	return filepath.Clean(filepath.Join(xdg.DataHome, "eamerald", "db"))
}

// GetEameraldDecisionsDir returns the eamerald decisions log directory ($XDG_DATA_HOME/eamerald/decisions).
func GetEameraldDecisionsDir() string {
	if dataDir := os.Getenv(x.EnvEameraldDecisionsDir); dataDir != "" {
		return dataDir
	}

	return filepath.Clean(filepath.Join(xdg.DataHome, "eamerald", "decisions"))
}

// GetEameraldTemplateDir returns the templates installation directory ($XDG_DATA_HOME/eamerald/tmpl).
func GetEameraldTemplateDir() string {
	if tmplDir := os.Getenv(x.EnvEameraldTmplDir); tmplDir != "" {
		return tmplDir
	}

	return filepath.Clean(filepath.Join(xdg.DataHome, "eamerald", "tmpl"))
}

// GetEameraldTemplateURL returns the URL to the templates container, can be local or remote.
func GetEameraldTemplateURL() string {
	if tmplURL := os.Getenv(x.EnvEameraldTmplURL); tmplURL != "" {
		return tmplURL
	}

	return x.DefEameraldTmplURL
}

func EnsureDirs() error {
	for _, f := range []func() error{
		EnsureEameraldDir,
		EnsureEameraldCfgDir,
		EnsureEameraldCertsDir,
		EnsureEameraldDataDir,
		EnsureEameraldDecisionsDir,
		EnsureEameraldTemplateDir,
	} {
		if err := f(); err != nil {
			return err
		}
	}

	return nil
}

func EnsureEameraldDir() error {
	dir := GetEameraldDir()
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return nil
	}

	return os.MkdirAll(dir, fs.FileModeOwnerRWX)
}

func EnsureEameraldCfgDir() error {
	dir := GetEameraldCfgDir()
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return nil
	}

	return os.MkdirAll(dir, fs.FileModeOwnerRWX)
}

func EnsureEameraldCertsDir() error {
	dir := GetEameraldCertsDir()
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return nil
	}

	return os.MkdirAll(dir, fs.FileModeOwnerRWX)
}

func EnsureEameraldDataDir() error {
	dir := GetEameraldDataDir()
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return nil
	}

	return os.MkdirAll(dir, fs.FileModeOwnerRWX)
}

func EnsureEameraldDecisionsDir() error {
	dir := GetEameraldDecisionsDir()
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return nil
	}

	return os.MkdirAll(dir, fs.FileModeOwnerRWX)
}

func EnsureEameraldTemplateDir() error {
	dir := GetEameraldTemplateDir()
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return nil
	}

	return os.MkdirAll(dir, fs.FileModeOwnerRWX)
}
