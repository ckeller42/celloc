package uciconf

import (
	"context"
	"os/exec"
)

// uciShow runs `uci -N show geolocd` and returns its stdout.
type uciShow func(ctx context.Context) (string, error)

func defaultUciShow(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "uci", "-N", "show", "geolocd").Output()
	return string(out), err
}

// Load reads the geolocd config from uci. On any error it returns Defaults()
// alongside the error so the caller can decide whether the missing key is fatal.
func Load(ctx context.Context) (Config, error) { return loadWith(ctx, defaultUciShow) }

func loadWith(ctx context.Context, run uciShow) (Config, error) {
	out, err := run(ctx)
	if err != nil {
		return Defaults(), err
	}
	return ParseUciShow(out), nil
}
