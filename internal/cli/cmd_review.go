package cli

import (
	"context"
	"fmt"
)

func runReview(_ context.Context, _ []string, opts Options) int {
	fmt.Fprintln(opts.Stderr, "review: command is not available in this build")
	return ExitUsage
}
