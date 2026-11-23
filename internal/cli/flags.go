package cli

import (
	"flag"
	"path/filepath"
	"strings"
)

type boolFlagger interface {
	IsBoolFlag() bool
}

// parsePositionalFlags reorders flags placed after positional arguments so
// invocations like `build report.md --to html` and `build --to html report.md`
// behave identically under Go's flag package.
func parsePositionalFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	isBool := make(map[string]bool)
	fs.VisitAll(func(f *flag.Flag) {
		if bf, ok := f.Value.(boolFlagger); ok && bf.IsBoolFlag() {
			isBool[f.Name] = true
		}
	})

	var flagArgs, positionals []string
	sawDoubleDash := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			sawDoubleDash = true
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
			if eqIdx := strings.IndexByte(name, '='); eqIdx >= 0 {
				flagArgs = append(flagArgs, arg)
				continue
			}
			flagArgs = append(flagArgs, arg)
			if !isBool[name] && fs.Lookup(name) != nil && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		positionals = append(positionals, arg)
	}

	ordered := make([]string, 0, len(flagArgs)+len(positionals)+1)
	ordered = append(ordered, flagArgs...)
	if sawDoubleDash {
		ordered = append(ordered, "--")
	}
	ordered = append(ordered, positionals...)

	if err := fs.Parse(ordered); err != nil {
		return nil, err
	}
	return fs.Args(), nil
}

func resolveWorkPath(workDir, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(workDir, p)
}
