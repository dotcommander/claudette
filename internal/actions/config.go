package actions

import (
	"fmt"
	"io"

	"github.com/dotcommander/claudette/internal/config"
)

// ShowConfig writes the current claudette config to w as pretty-printed JSON
// with a trailing newline. The output format is identical to what SaveConfig
// writes to disk — the shared formatter is config.MarshalIndent.
//
// A missing config file is rendered as the zero-value Config ("{}"), matching
// LoadConfig's behavior. This lets users run `claudette config show` before
// `claudette install` without surprise.
func ShowConfig(w io.Writer) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	data, err := config.MarshalIndent(cfg)
	if err != nil {
		return fmt.Errorf("formatting config: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = fmt.Fprintln(w)
	return err
}

// ShowConfigPath writes the absolute path to claudette's config file to w,
// followed by a newline. Useful in shell pipelines:
//
//	cat "$(claudette config path)"
func ShowConfigPath(w io.Writer) error {
	path, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}
	_, err = fmt.Fprintln(w, path)
	return err
}
