package cli

import (
	"context"
	"fmt"
)

func runDoctor(_ context.Context, _ []string, opts Options) int {
	fmt.Fprintln(opts.Stderr, "doctor: command is not available in this build")
	return ExitUsage
}
