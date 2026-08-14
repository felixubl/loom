// Command loom is a Loom read-eval-print loop and script runner.
//
//	loom              start the REPL
//	loom program.loom run a program, then exit
//	loom -i prog.loom run a program, then start the REPL with its definitions
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/felixubl/loom"
)

func main() {
	interactive := false
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "-i" {
		interactive, args = true, args[1:]
	}

	session := loom.NewSession(loom.New())
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	for _, path := range args {
		if err := runFile(session, out, path); err != nil {
			out.Flush()
			fmt.Fprintln(os.Stderr, "loom:", err)
			os.Exit(1)
		}
	}
	if len(args) > 0 && !interactive {
		return
	}
	repl(session, out)
}

func runFile(s *loom.Session, out *bufio.Writer, path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	program, err := s.Store().ParseProgram(string(src))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	results, err := s.RunProgram(program)
	for _, r := range results {
		if line := render(s.Store(), r); line != "" {
			fmt.Fprintln(out, line)
		}
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func repl(s *loom.Session, out *bufio.Writer) {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1<<20)
	prompting := isTerminal(os.Stdin)

	var pending strings.Builder
	for {
		if prompting {
			if pending.Len() == 0 {
				fmt.Fprint(out, "loom> ")
			} else {
				fmt.Fprint(out, "  ... ")
			}
			out.Flush()
		}
		if !in.Scan() {
			break
		}
		pending.WriteString(in.Text())
		pending.WriteByte('\n')

		// A transaction block spans lines, so keep reading until its braces
		// balance.
		if unbalanced(pending.String()) {
			continue
		}
		source := strings.TrimSpace(pending.String())
		pending.Reset()
		if source == "" {
			continue
		}

		command, err := s.Store().ParseCommand(source)
		if err != nil {
			fmt.Fprintln(out, "error:", err)
			continue
		}
		result, err := s.Run(command)
		if err != nil {
			fmt.Fprintln(out, "error:", err)
			continue
		}
		if line := render(s.Store(), result); line != "" {
			fmt.Fprintln(out, line)
		}
		out.Flush()
	}
	if prompting {
		fmt.Fprintln(out)
	}
}

// unbalanced reports whether an open brace is still waiting to be closed,
// ignoring braces inside string literals.
func unbalanced(src string) bool {
	depth, inText := 0, false
	for i := 0; i < len(src); i++ {
		switch {
		case inText && src[i] == '\\':
			i++
		case src[i] == '"':
			inText = !inText
		case inText:
		case src[i] == '{':
			depth++
		case src[i] == '}':
			depth--
		}
	}
	return depth > 0
}

// render turns a result into the one line the REPL prints for it. Definitions
// and retractions print nothing.
func render(store *loom.Store, r loom.Result) string {
	switch r.Command.(type) {
	case loom.Evaluate:
		return store.Display(r.Value)
	case loom.Assert, loom.Transaction:
		ids := make([]string, 0, len(r.Claims))
		for _, id := range r.Claims {
			ids = append(ids, fmt.Sprintf("claim:%d", id))
		}
		return strings.Join(ids, "\n")
	case loom.Query:
		if len(r.Rows) == 0 {
			return "(no matches)"
		}
		lines := make([]string, 0, len(r.Rows))
		for _, row := range r.Rows {
			lines = append(lines, renderRow(store, row))
		}
		sort.Strings(lines)
		return strings.Join(lines, "\n")
	}
	return ""
}

func renderRow(store *loom.Store, row loom.Bindings) string {
	if len(row) == 0 {
		return "(match)"
	}
	names := make([]string, 0, len(row))
	for name := range row {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+" = "+store.Source(row[name]))
	}
	return strings.Join(parts, ", ")
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
