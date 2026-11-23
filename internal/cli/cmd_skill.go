package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/skill"
)

func runSkill(ctx context.Context, args []string, opts Options) int {
	if len(args) == 0 {
		printSkillUsage(opts.Stderr)
		return ExitUsage
	}

	switch args[0] {
	case "help", "--help", "-h":
		printSkillUsage(opts.Stdout)
		return ExitSuccess
	case "install":
		return runSkillInstall(ctx, args[1:], opts)
	case "status":
		return runSkillStatus(ctx, args[1:], opts)
	default:
		fmt.Fprintf(opts.Stderr, "skill: unknown subcommand %q\n\n", args[0])
		printSkillUsage(opts.Stderr)
		return ExitUsage
	}
}

func runSkillInstall(_ context.Context, args []string, opts Options) int {
	fs := flag.NewFlagSet("skill install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		targetFlag string
		globalFlag bool
		schoolFlag string
		forceFlag  bool
	)
	fs.StringVar(&targetFlag, "target", skill.TargetAgents, "installation target: agents, claude, gemini, or all")
	fs.BoolVar(&globalFlag, "global", false, "install globally under home directory")
	fs.StringVar(&schoolFlag, "school", "tufte", "visual School recorded in manifest")
	fs.BoolVar(&forceFlag, "force", false, "overwrite pre-existing or modified Skill Bundle files")

	pos, err := parsePositionalFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printSkillInstallUsage(opts.Stdout)
			return ExitSuccess
		}
		fmt.Fprintf(opts.Stderr, "skill install: %v\n", err)
		return ExitUsage
	}
	if len(pos) > 0 {
		fmt.Fprintf(opts.Stderr, "skill install: unexpected argument %q\n", pos[0])
		return ExitUsage
	}

	res, err := skill.Install(skill.InstallOptions{
		WorkDir: opts.WorkDir,
		HomeDir: opts.HomeDir,
		Version: opts.Version,
		Target:  targetFlag,
		Global:  globalFlag,
		School:  schoolFlag,
		Force:   forceFlag,
	})
	if err != nil {
		fmt.Fprintf(opts.Stderr, "skill install: %v\n", err)
		if skill.IsUsageError(err) {
			return ExitUsage
		}
		return ExitError
	}

	for _, inst := range res.Installations {
		fmt.Fprintf(opts.Stdout, "installed %s Skill Bundle (%s, school=%s, version=%s) -> %s\n",
			inst.Target, formatFileCount(inst.FilesWritten), inst.School, inst.Version, inst.Path)
	}
	return ExitSuccess
}

func runSkillStatus(_ context.Context, args []string, opts Options) int {
	fs := flag.NewFlagSet("skill status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		targetFlag string
		globalFlag bool
		asJSON     bool
	)
	fs.StringVar(&targetFlag, "target", "", "target to inspect: agents, claude, gemini, or all")
	fs.BoolVar(&globalFlag, "global", false, "inspect global installations under home directory")
	fs.BoolVar(&asJSON, "json", false, "emit machine-readable JSON status")

	pos, err := parsePositionalFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printSkillStatusUsage(opts.Stdout)
			return ExitSuccess
		}
		fmt.Fprintf(opts.Stderr, "skill status: %v\n", err)
		return ExitUsage
	}
	if len(pos) > 0 {
		fmt.Fprintf(opts.Stderr, "skill status: unexpected argument %q\n", pos[0])
		return ExitUsage
	}

	report, err := skill.Status(skill.StatusOptions{
		WorkDir: opts.WorkDir,
		HomeDir: opts.HomeDir,
		Version: opts.Version,
		Target:  targetFlag,
		Global:  globalFlag,
	})
	if err != nil {
		fmt.Fprintf(opts.Stderr, "skill status: %v\n", err)
		if skill.IsUsageError(err) {
			return ExitUsage
		}
		return ExitError
	}

	if asJSON {
		enc := json.NewEncoder(opts.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(opts.Stderr, "skill status: encode json: %v\n", err)
			return ExitError
		}
	} else {
		printSkillStatusReport(opts.Stdout, report)
	}

	if report.Healthy {
		return ExitSuccess
	}
	writeSkillStatusDiagnostic(opts.Stderr, report)
	return ExitError
}

func printSkillStatusReport(w io.Writer, r skill.StatusReport) {
	fmt.Fprintf(w, "DataFolio Skill Bundle Status (current binary: %s)\n", r.CurrentVersion)
	for _, t := range r.Targets {
		state := "ok"
		if !t.Installed {
			state = "not installed"
		} else if !t.ManifestValid {
			state = "untracked"
		} else if !t.Healthy {
			state = "drift"
		}

		fmt.Fprintf(w, "  %-6s  [%s]  %s\n", t.Target, state, t.Path)
		if t.School != "" {
			fmt.Fprintf(w, "          school: %s\n", t.School)
		}
		if t.Installed {
			instVer := t.InstalledVersion
			if instVer == "" {
				instVer = "unknown"
			}
			fmt.Fprintf(w, "          version: installed %s vs current %s\n", instVer, t.CurrentVersion)
		}
		if len(t.ModifiedFiles) > 0 {
			fmt.Fprintf(w, "          modified: %s\n", strings.Join(t.ModifiedFiles, ", "))
		}
		if len(t.MissingFiles) > 0 {
			fmt.Fprintf(w, "          missing: %s\n", strings.Join(t.MissingFiles, ", "))
		}
		if t.Error != "" && t.Installed {
			fmt.Fprintf(w, "          detail: %s\n", t.Error)
		}
	}
	if r.Hint != "" {
		fmt.Fprintf(w, "Hint: %s\n", r.Hint)
	}
}

func writeSkillStatusDiagnostic(w io.Writer, r skill.StatusReport) {
	var firstErr string
	for _, t := range r.Targets {
		if !t.Healthy && t.Error != "" {
			firstErr = fmt.Sprintf("%s (%s): %s", t.Target, t.Path, t.Error)
			break
		}
	}
	if firstErr == "" {
		firstErr = "skill bundle drift detected"
	}
	if r.Hint != "" {
		fmt.Fprintf(w, "skill status: %s — %s\n", firstErr, r.Hint)
		return
	}
	fmt.Fprintf(w, "skill status: %s\n", firstErr)
}

func formatFileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

func printSkillUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: datafolio skill <subcommand> [flags]

Subcommands:
  install   Install the embedded Skill Bundle into a project or global agent directory
  status    Inspect installed Skill Bundle paths, active School, and version drift

Run "datafolio skill install --help" or "datafolio skill status --help" for flags.`)
}

func printSkillInstallUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: datafolio skill install [flags]

Flags:
  --target string   Target harness: agents, claude, gemini, or all (default "agents")
  --global          Install globally (~/.agents/skills, ~/.claude/skills, ~/.gemini/config/skills)
  --school string   Visual School to record in manifest (default "tufte")
  --force           Overwrite pre-existing or locally modified files`)
}

func printSkillStatusUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: datafolio skill status [flags]

Flags:
  --target string   Target harness to inspect: agents, claude, gemini, or all
  --global          Inspect global installations under home directory
  --json            Emit machine-readable JSON diagnostics`)
}
