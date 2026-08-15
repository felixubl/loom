// Command loom is a Loom read-eval-print loop and script runner.
//
//	loom              start the REPL
//	loom program.loom run a program, then exit
//	loom -i prog.loom run a program, then start the REPL with its definitions
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
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
		for _, line := range loom.Render(s.Store(), r) {
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

		source := strings.TrimSpace(pending.String())
		if source == "" {
			pending.Reset()
			continue
		}

		command, err := s.Store().ParseCommand(source)
		if incomplete(err) {
			// An open bracket, or a statement that cannot end yet. Keep reading
			// rather than reporting an error the user is still in the middle of.
			continue
		}
		pending.Reset()
		if err != nil {
			fmt.Fprintln(out, "error:", err)
			continue
		}
		result, err := s.Run(command)
		if err != nil {
			fmt.Fprintln(out, "error:", err)
			continue
		}
		for _, line := range loom.Render(s.Store(), result) {
			fmt.Fprintln(out, line)
		}
		out.Flush()
	}
	if prompting {
		fmt.Fprintln(out)
	}
}

// incomplete reports whether the input merely ran out while the grammar still
// wanted something, as opposed to being wrong.
func incomplete(err error) bool {
	var syntax *loom.SyntaxError
	return errors.As(err, &syntax) && syntax.Incomplete
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
