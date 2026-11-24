package cli_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/cli"
	"github.com/pablodiegoo/datafolio/internal/skill"
)

type cliCommandSpec struct {
	LongFlags  map[string]bool
	ShortFlags map[string]bool
	HasFlagSet bool
}

func TestSkillBundleMatchesCLI(t *testing.T) {
	t.Parallel()

	cliDir := filepath.Join(repoRoot(t), "internal", "cli")
	realCLI := extractRealCLISpec(t, cliDir)

	files, err := skill.BundleFiles()
	if err != nil {
		t.Fatalf("skill.BundleFiles() error: %v", err)
	}

	var skillMD string
	refDocs := make(map[string]string)
	for _, f := range files {
		switch {
		case f.RelPath == "SKILL.md":
			skillMD = string(f.Content)
		case strings.HasPrefix(f.RelPath, "references/") && strings.HasSuffix(f.RelPath, ".md"):
			refDocs[f.RelPath] = string(f.Content)
		}
	}
	if skillMD == "" {
		t.Fatal("Skill Bundle missing SKILL.md")
	}

	docSynopsis, docReferences := parseSkillCLICommands(t, skillMD)
	if diffs := compareCLIAndSkillSpecs(realCLI, docSynopsis, docReferences); len(diffs) > 0 {
		t.Fatalf("Skill Bundle SKILL.md is out of sync with CLI:\n  - %s", strings.Join(diffs, "\n  - "))
	}

	// Verify every documented command and long flag is accepted by real cli.Run at runtime.
	for cmd, spec := range docSynopsis {
		cmdParts := strings.Fields(cmd)
		helpArgs := append(slices.Clone(cmdParts), "--help")
		code, out, errOut := runCLI(t, t.TempDir(), helpArgs...)
		if code != cli.ExitSuccess || strings.TrimSpace(out) == "" {
			t.Errorf("documented command %q --help failed: code=%d stdout=%q stderr=%q", cmd, code, out, errOut)
		}
		if !realCLI[cmd].HasFlagSet {
			continue
		}
		for flagName := range spec.LongFlags {
			probeArgs := append(slices.Clone(cmdParts), "--"+flagName+"=true", "--help")
			pCode, _, pErr := runCLI(t, t.TempDir(), probeArgs...)
			if pCode != cli.ExitSuccess {
				t.Errorf("documented flag --%s on %q rejected by cli.Run: code=%d stderr=%q", flagName, cmd, pCode, pErr)
			}
		}
		for shortName := range spec.ShortFlags {
			probeArgs := append(slices.Clone(cmdParts), "-"+shortName+"=true", "--help")
			pCode, _, pErr := runCLI(t, t.TempDir(), probeArgs...)
			if pCode != cli.ExitSuccess {
				t.Errorf("documented short flag -%s on %q rejected by cli.Run: code=%d stderr=%q", shortName, cmd, pCode, pErr)
			}
		}
	}

	// Also verify any explicit `datafolio <cmd> ...` invocations in references/*.md only cite real commands/flags.
	for relPath, content := range refDocs {
		for _, inv := range extractInlineDatafolioInvocations(content, realCLI) {
			spec, ok := realCLI[inv.Command]
			if !ok {
				t.Errorf("%s references unknown CLI command %q", relPath, inv.Command)
				continue
			}
			for _, flagName := range inv.LongFlags {
				if !spec.LongFlags[flagName] {
					t.Errorf("%s references unknown flag --%s on %q", relPath, flagName, inv.Command)
				}
			}
		}
	}
}

func TestSkillConsistencyDetectorCatchesMissingAndExtraFlags(t *testing.T) {
	t.Parallel()

	realSpec := map[string]cliCommandSpec{
		"build": {
			HasFlagSet: true,
			LongFlags:  map[string]bool{"to": true, "header": true},
			ShortFlags: map[string]bool{"o": true},
		},
	}
	missingFlagDoc := map[string]cliCommandSpec{
		"build": {
			LongFlags:  map[string]bool{"to": true},
			ShortFlags: map[string]bool{"o": true},
		},
	}
	if diffs := compareCLIAndSkillSpecs(realSpec, missingFlagDoc, nil); !containsSubstring(diffs, "--header") {
		t.Fatalf("expected detector to catch missing --header flag, got %v", diffs)
	}

	extraFlagDoc := map[string]cliCommandSpec{
		"build": {
			LongFlags:  map[string]bool{"to": true, "header": true, "phantom": true},
			ShortFlags: map[string]bool{"o": true},
		},
	}
	if diffs := compareCLIAndSkillSpecs(realSpec, extraFlagDoc, nil); !containsSubstring(diffs, "--phantom") {
		t.Fatalf("expected detector to catch bogus --phantom flag, got %v", diffs)
	}

	bogusCmdDoc := map[string]cliCommandSpec{
		"build":  realSpec["build"],
		"deploy": {LongFlags: map[string]bool{}},
	}
	if diffs := compareCLIAndSkillSpecs(realSpec, bogusCmdDoc, nil); !containsSubstring(diffs, "deploy") {
		t.Fatalf("expected detector to catch bogus command, got %v", diffs)
	}
}

func containsSubstring(items []string, sub string) bool {
	for _, item := range items {
		if strings.Contains(item, sub) {
			return true
		}
	}
	return false
}

func compareCLIAndSkillSpecs(realCLI, synopsis, extraRefs map[string]cliCommandSpec) []string {
	var diffs []string

	for cmd, realSpec := range realCLI {
		docSpec, ok := synopsis[cmd]
		if !ok {
			diffs = append(diffs, "CLI command "+strconv.Quote("datafolio "+cmd)+" missing from SKILL.md ## CLI Commands")
			continue
		}
		for _, flagName := range sortedKeys(realSpec.LongFlags) {
			if !docSpec.LongFlags[flagName] {
				diffs = append(diffs, "CLI flag --"+flagName+" on "+strconv.Quote("datafolio "+cmd)+" missing from SKILL.md ## CLI Commands")
			}
		}
		for _, shortName := range sortedKeys(realSpec.ShortFlags) {
			if !docSpec.ShortFlags[shortName] {
				diffs = append(diffs, "CLI short flag -"+shortName+" on "+strconv.Quote("datafolio "+cmd)+" missing from SKILL.md ## CLI Commands")
			}
		}
	}

	for cmd, docSpec := range synopsis {
		realSpec, ok := realCLI[cmd]
		if !ok {
			diffs = append(diffs, "SKILL.md documents unknown command "+strconv.Quote("datafolio "+cmd))
			continue
		}
		for _, flagName := range sortedKeys(docSpec.LongFlags) {
			if !realSpec.LongFlags[flagName] {
				diffs = append(diffs, "SKILL.md documents unknown flag --"+flagName+" on "+strconv.Quote("datafolio "+cmd))
			}
		}
		for _, shortName := range sortedKeys(docSpec.ShortFlags) {
			if !realSpec.ShortFlags[shortName] {
				diffs = append(diffs, "SKILL.md documents unknown short flag -"+shortName+" on "+strconv.Quote("datafolio "+cmd))
			}
		}
	}

	for cmd, refSpec := range extraRefs {
		realSpec, ok := realCLI[cmd]
		if !ok {
			diffs = append(diffs, "SKILL.md references unknown command "+strconv.Quote("datafolio "+cmd))
			continue
		}
		for _, flagName := range sortedKeys(refSpec.LongFlags) {
			if !realSpec.LongFlags[flagName] {
				diffs = append(diffs, "SKILL.md references unknown flag --"+flagName+" on "+strconv.Quote("datafolio "+cmd))
			}
		}
	}

	slices.Sort(diffs)
	return diffs
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// extractRealCLISpec parses internal/cli/run.go and cmd_*.go ASTs to extract every
// real operational command/subcommand and its registered FlagSet flags.
func extractRealCLISpec(t *testing.T, cliDir string) map[string]cliCommandSpec {
	t.Helper()

	fset := token.NewFileSet()
	funcs := make(map[string]*ast.FuncDecl)

	entries, err := os.ReadDir(cliDir)
	if err != nil {
		t.Fatalf("read internal/cli: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if name != "run.go" && !strings.HasPrefix(name, "cmd_") {
			continue
		}
		fileAST, err := parser.ParseFile(fset, filepath.Join(cliDir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range fileAST.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name != nil {
				funcs[fn.Name.Name] = fn
			}
		}
	}

	runFn, ok := funcs["Run"]
	if !ok {
		t.Fatal("Run function not found in internal/cli/run.go")
	}

	out := make(map[string]cliCommandSpec)
	topCases := extractFirstArgSwitchCases(runFn)
	for _, tc := range topCases {
		primary := primaryCommandAlias(tc.Names)
		if primary == "" || primary == "help" {
			continue
		}
		if tc.Handler == "" {
			out[primary] = cliCommandSpec{
				LongFlags:  map[string]bool{},
				ShortFlags: map[string]bool{},
			}
			continue
		}
		handlerFn, ok := funcs[tc.Handler]
		if !ok {
			t.Fatalf("handler %s for command %q not found", tc.Handler, primary)
		}
		subCases := extractFirstArgSwitchCases(handlerFn)
		if len(subCases) > 0 {
			for _, sc := range subCases {
				subPrimary := primaryCommandAlias(sc.Names)
				if subPrimary == "" || subPrimary == "help" {
					continue
				}
				fullCmd := primary + " " + subPrimary
				subHandler, ok := funcs[sc.Handler]
				if !ok {
					t.Fatalf("subcommand handler %s for %q not found", sc.Handler, fullCmd)
				}
				out[fullCmd] = extractFuncFlags(subHandler)
			}
			continue
		}
		out[primary] = extractFuncFlags(handlerFn)
	}
	return out
}

type switchDispatchCase struct {
	Names   []string
	Handler string
}

func extractFirstArgSwitchCases(fn *ast.FuncDecl) []switchDispatchCase {
	if fn == nil || fn.Body == nil {
		return nil
	}
	var cases []switchDispatchCase
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok || !isArgsZeroIndexExpr(sw.Tag) {
			return true
		}
		for _, stmt := range sw.Body.List {
			cc, ok := stmt.(*ast.CaseClause)
			if !ok || len(cc.List) == 0 {
				continue
			}
			var names []string
			for _, expr := range cc.List {
				if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if val, err := strconv.Unquote(lit.Value); err == nil {
						names = append(names, val)
					}
				}
			}
			handler := ""
			for _, bodyStmt := range cc.Body {
				if ret, ok := bodyStmt.(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
					if call, ok := ret.Results[0].(*ast.CallExpr); ok {
						if id, ok := call.Fun.(*ast.Ident); ok {
							handler = id.Name
						}
					}
				}
			}
			cases = append(cases, switchDispatchCase{Names: names, Handler: handler})
		}
		return false
	})
	return cases
}

func isArgsZeroIndexExpr(expr ast.Expr) bool {
	idx, ok := expr.(*ast.IndexExpr)
	if !ok {
		return false
	}
	ident, ok := idx.X.(*ast.Ident)
	if !ok || ident.Name != "args" {
		return false
	}
	lit, ok := idx.Index.(*ast.BasicLit)
	return ok && lit.Kind == token.INT && lit.Value == "0"
}

func primaryCommandAlias(names []string) string {
	for _, n := range names {
		if !strings.HasPrefix(n, "-") {
			return n
		}
	}
	return ""
}

func extractFuncFlags(fn *ast.FuncDecl) cliCommandSpec {
	spec := cliCommandSpec{
		LongFlags:  make(map[string]bool),
		ShortFlags: make(map[string]bool),
	}
	if fn == nil || fn.Body == nil {
		return spec
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkgIdent, ok := sel.X.(*ast.Ident); ok && pkgIdent.Name == "flag" && sel.Sel.Name == "NewFlagSet" {
			spec.HasFlagSet = true
			return true
		}
		var nameArgIdx int
		switch sel.Sel.Name {
		case "BoolVar", "StringVar", "IntVar", "Int64Var", "UintVar", "Uint64Var", "Float64Var", "DurationVar", "Var":
			nameArgIdx = 1
		case "Bool", "String", "Int", "Int64", "Uint", "Uint64", "Float64", "Duration":
			nameArgIdx = 0
		default:
			return true
		}
		if len(call.Args) <= nameArgIdx {
			return true
		}
		lit, ok := call.Args[nameArgIdx].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		flagName, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if len(flagName) == 1 {
			spec.ShortFlags[flagName] = true
		} else {
			spec.LongFlags[flagName] = true
		}
		return true
	})
	return spec
}

type datafolioInvocation struct {
	Command   string
	LongFlags []string
}

func parseSkillCLICommands(t *testing.T, skillMD string) (map[string]cliCommandSpec, map[string]cliCommandSpec) {
	t.Helper()

	section := extractMarkdownSection(skillMD, "## CLI Commands")
	if section == "" {
		t.Fatal("SKILL.md missing `## CLI Commands` section")
	}

	synopsis := make(map[string]cliCommandSpec)
	for _, block := range extractFencedCodeBlocks(section) {
		for _, cmdLine := range joinContinuations(block) {
			fields := tokenizeShellLine(cmdLine)
			if len(fields) < 2 || fields[0] != "datafolio" {
				continue
			}
			cmd, rest := splitCommandAndArgs(fields[1:])
			entry, exists := synopsis[cmd]
			if !exists {
				entry = cliCommandSpec{
					LongFlags:  make(map[string]bool),
					ShortFlags: make(map[string]bool),
				}
			}
			for _, tok := range rest {
				clean := strings.Trim(tok, "[]")
				if strings.HasPrefix(clean, "--") {
					name := strings.TrimPrefix(clean, "--")
					if idx := strings.IndexByte(name, '='); idx >= 0 {
						name = name[:idx]
					}
					if name != "" {
						entry.LongFlags[name] = true
					}
				} else if strings.HasPrefix(clean, "-") && len(clean) == 2 {
					entry.ShortFlags[strings.TrimPrefix(clean, "-")] = true
				}
			}
			synopsis[cmd] = entry
		}
	}

	restContent := strings.Replace(skillMD, section, "", 1)
	extraRefs := make(map[string]cliCommandSpec)
	for _, inv := range extractInlineDatafolioInvocations(restContent, nil) {
		entry, exists := extraRefs[inv.Command]
		if !exists {
			entry = cliCommandSpec{
				LongFlags:  make(map[string]bool),
				ShortFlags: make(map[string]bool),
			}
		}
		for _, fl := range inv.LongFlags {
			entry.LongFlags[fl] = true
		}
		extraRefs[inv.Command] = entry
	}
	return synopsis, extraRefs
}

var inlineDatafolioRe = regexp.MustCompile("(?m)(?:^|[` \\t(])(datafolio[ \\t]+[^`\\n)\\r]+)")

func extractInlineDatafolioInvocations(md string, knownCommands map[string]cliCommandSpec) []datafolioInvocation {
	var out []datafolioInvocation
	matches := inlineDatafolioRe.FindAllStringSubmatch(md, -1)
	for _, m := range matches {
		fields := tokenizeShellLine(m[1])
		if len(fields) < 2 || fields[0] != "datafolio" {
			continue
		}
		// Skip prose like "datafolio binary" or "datafolio <command>".
		if strings.HasPrefix(fields[1], "<") || fields[1] == "binary" || fields[1] == "Go" {
			continue
		}
		cmd, rest := splitCommandAndArgs(fields[1:])
		if knownCommands != nil {
			// If a prose line says "datafolio build or datafolio review", stop before "or".
			if parts := strings.Fields(cmd); len(parts) == 2 {
				if _, ok := knownCommands[cmd]; !ok {
					if _, topOK := knownCommands[parts[0]]; topOK && (parts[1] == "or" || parts[1] == "and" || parts[1] == "in" || parts[1] == "to") {
						cmd = parts[0]
					}
				}
			}
		}
		var flags []string
		for _, tok := range rest {
			clean := strings.Trim(tok, "[](),.;:")
			if strings.HasPrefix(clean, "--") && len(clean) > 2 {
				name := strings.TrimPrefix(clean, "--")
				if idx := strings.IndexByte(name, '='); idx >= 0 {
					name = name[:idx]
				}
				flags = append(flags, name)
			}
		}
		out = append(out, datafolioInvocation{Command: cmd, LongFlags: flags})
	}
	return out
}

func extractMarkdownSection(md, heading string) string {
	lines := strings.Split(normalizeLF(md), "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i
			break
		}
	}
	if start == -1 {
		return ""
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func extractFencedCodeBlocks(md string) []string {
	var blocks []string
	var cur bytes.Buffer
	inBlock := false
	for _, line := range strings.Split(normalizeLF(md), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inBlock {
				inBlock = true
				cur.Reset()
			} else {
				inBlock = false
				blocks = append(blocks, cur.String())
			}
			continue
		}
		if inBlock {
			cur.WriteString(line)
			cur.WriteByte('\n')
		}
	}
	return blocks
}

func joinContinuations(block string) []string {
	var lines []string
	var cur strings.Builder
	for _, raw := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasSuffix(trimmed, "\\") {
			cur.WriteString(strings.TrimSpace(strings.TrimSuffix(trimmed, "\\")))
			cur.WriteByte(' ')
			continue
		}
		cur.WriteString(trimmed)
		lines = append(lines, cur.String())
		cur.Reset()
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

func tokenizeShellLine(line string) []string {
	return strings.Fields(line)
}

func splitCommandAndArgs(tokens []string) (string, []string) {
	if len(tokens) == 0 {
		return "", nil
	}
	first := strings.Trim(tokens[0], "`.,;:")
	if len(tokens) >= 2 {
		second := strings.Trim(tokens[1], "`.,;:")
		if isBareSubcommandWord(second) && (first == "tokens" || first == "skill") {
			return first + " " + second, tokens[2:]
		}
		// Catch any unexpected subcommand word documented on a single-level command.
		if isBareSubcommandWord(second) && !strings.ContainsAny(second, "./") && !strings.HasSuffix(second, ".md") {
			return first + " " + second, tokens[2:]
		}
	}
	return first, tokens[1:]
}

func isBareSubcommandWord(tok string) bool {
	if tok == "" || strings.HasPrefix(tok, "-") || strings.HasPrefix(tok, "<") || strings.HasPrefix(tok, "[") {
		return false
	}
	for _, r := range tok {
		if !((r >= 'a' && r <= 'z') || r == '-') {
			return false
		}
	}
	return true
}
