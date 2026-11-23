package cli

import (
	"context"
	"fmt"
)

func runSkill(_ context.Context, _ []string, opts Options) int {
	fmt.Fprintln(opts.Stderr, "skill: command is not available in this build")
	return ExitUsage
}
