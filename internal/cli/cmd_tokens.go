package cli

import (
	"context"
	"fmt"
)

func runTokens(_ context.Context, _ []string, opts Options) int {
	fmt.Fprintln(opts.Stderr, "tokens: command is not available in this build")
	return ExitUsage
}
