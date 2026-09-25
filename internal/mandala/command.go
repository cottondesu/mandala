package mandala

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
)

const rootHelp = `Usage: mandala [--project DIR] <command> [flags] [arguments]

Commands: init, add, mark, done, status, gaps, show, clean
Use "mandala <command> --help" for command syntax.
`

func Run(args []string, start string, stdout, stderr io.Writer) int {
	rootFlags := flag.NewFlagSet("mandala", flag.ContinueOnError)
	rootFlags.SetOutput(io.Discard)
	project := rootFlags.String("project", "", "project root directory")
	if err := rootFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return writeResult(stdout, stderr, rootHelp, 0)
		}
		return writeError(stderr, problem("E_USAGE", "%v", err))
	}
	projectSpecified := false
	rootFlags.Visit(func(f *flag.Flag) { projectSpecified = f.Name == "project" })
	if projectSpecified && *project == "" {
		return writeError(stderr, problem("E_USAGE", "--project requires a directory"))
	}
	remaining := rootFlags.Args()
	if len(remaining) == 0 {
		return writeError(stderr, problem("E_USAGE", "missing command; use --help"))
	}
	output, code, err := execute(remaining[0], remaining[1:], start, *project)
	if err != nil {
		return writeError(stderr, err)
	}
	return writeResult(stdout, stderr, output, code)
}

func execute(name string, args []string, start, project string) (string, int, error) {
	switch name {
	case "init", "add", "mark", "done", "status", "gaps", "show", "clean":
	default:
		return "", 0, problem("E_USAGE", "unknown command %q", name)
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var optional, requiredOnly, jsonOutput *bool
	switch name {
	case "add":
		optional = flags.Bool("optional", false, "make this cell optional")
	case "gaps":
		requiredOnly = flags.Bool("required", false, "show only required gaps")
		jsonOutput = flags.Bool("json", false, "write JSON")
	case "show":
		jsonOutput = flags.Bool("json", false, "write JSON")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return commandHelp(name), 0, nil
		}
		return "", 0, problem("E_USAGE", "%v", err)
	}
	positional := flags.Args()
	want := 0
	switch name {
	case "init", "add", "done":
		want = 1
	case "mark":
		want = 2
	}
	if len(positional) != want {
		return "", 0, problem("E_USAGE", "%s expects %d argument(s); use --help", name, want)
	}
	if name == "init" {
		s, err := NewState(positional[0])
		if err != nil {
			return "", 0, err
		}
		root := start
		if project != "" {
			root = project
			if !filepath.IsAbs(root) {
				root = filepath.Join(start, root)
			}
		}
		return "", 0, Init(root, s)
	}
	if name == "clean" {
		return "", 0, Clean(start, project)
	}
	root, err := ProjectRoot(start, project)
	if err != nil {
		return "", 0, err
	}
	s, err := Load(root)
	if err != nil {
		return "", 0, err
	}
	switch name {
	case "add":
		if err := s.Add(positional[0], *optional); err != nil {
			return "", 0, err
		}
		return "", 0, Save(root, s)
	case "mark":
		if err := s.Mark(positional[0], Status(positional[1])); err != nil {
			return "", 0, err
		}
		return "", 0, Save(root, s)
	case "done":
		if err := s.Mark(positional[0], Done); err != nil {
			return "", 0, err
		}
		return "", 0, Save(root, s)
	case "status":
		code := 0
		if s.Counts().RequiredOpen > 0 {
			code = 1
		}
		return renderStatus(s), code, nil
	case "gaps":
		code := 0
		if s.Counts().RequiredOpen > 0 {
			code = 1
		}
		output, err := renderGaps(s, *requiredOnly, *jsonOutput)
		return output, code, err
	case "show":
		output, err := renderShow(s, *jsonOutput)
		return output, 0, err
	}
	return "", 0, problem("E_USAGE", "unknown command %q", name)
}

func commandHelp(name string) string {
	switch name {
	case "init":
		return "Usage: mandala [--project DIR] init <goal>\n"
	case "add":
		return "Usage: mandala [--project DIR] add [--optional] <id>\n"
	case "mark":
		return "Usage: mandala [--project DIR] mark <id> <open|done|na>\n"
	case "done":
		return "Usage: mandala [--project DIR] done <id>\n"
	case "gaps":
		return "Usage: mandala [--project DIR] gaps [--required] [--json]\n"
	case "show":
		return "Usage: mandala [--project DIR] show [--json]\n"
	default:
		return fmt.Sprintf("Usage: mandala [--project DIR] %s\n", name)
	}
}

func writeResult(stdout, stderr io.Writer, output string, code int) int {
	if _, err := io.WriteString(stdout, output); err != nil {
		return writeError(stderr, fmt.Errorf("write stdout: %w", err))
	}
	return code
}

func writeError(stderr io.Writer, err error) int {
	code := "E_IO"
	var p *Problem
	if errors.As(err, &p) {
		code = p.Code
	}
	fmt.Fprintf(stderr, "mandala: %s: %s\n", code, err)
	return 2
}
