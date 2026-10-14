// Command examsim runs practice exams from VCE-derived content.
//
// Usage:
//
//	examsim probe <file.vce>                       inspect a VCE binary
//	examsim import <file.txt> [-o exam.json]        convert a VCE text export
//	examsim validate <exam.json>                   check an exam file
//	examsim run <exam.json> [flags]                start a practice session
//	examsim version
//
// Run flags:
//
//	--mode study|exam     study gives immediate feedback (default), exam is timed & scored
//	--time N              time limit in minutes (exam mode)
//	--shuffle             shuffle question order
//	--shuffle-options     shuffle answer option order
//	--resume <session>    resume a saved session
//	--no-save             don't auto-save the session
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/montagoc/examsim/internal/exam"
	"github.com/montagoc/examsim/internal/importer"
	"github.com/montagoc/examsim/internal/player"
	"github.com/montagoc/examsim/internal/vce"
)

const version = "0.1.0"

// splitArgs separates flag arguments from positional arguments so flags may
// appear before or after the positional. valueFlags names flags that consume
// the following argument (both -name and --name spellings).
func splitArgs(args []string, valueFlags map[string]bool) (flagArgs, positional []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		name := a
		if k := strings.Index(a, "="); k >= 0 {
			name = a[:k]
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			flagArgs = append(flagArgs, a)
			if valueFlags[name] && !strings.Contains(a, "=") && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		positional = append(positional, a)
	}
	return flagArgs, positional
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "probe":
		err = cmdProbe(os.Args[2:])
	case "import":
		err = cmdImport(os.Args[2:])
	case "validate":
		err = cmdValidate(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("examsim", version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`examsim — practice exams from VCE-derived content

Commands:
  probe <file.vce>                 Inspect a VCE binary (format/encryption check)
  import <file.txt> [-o exam.json] Convert a VCE text export to examsim JSON
  validate <exam.json>             Validate an examsim exam file
  run <exam.json> [flags]          Start an interactive practice session
  version                          Print version

Run flags:
  --mode study|exam     "study" gives immediate feedback (default); "exam" is scored at the end
  --time N              Time limit in minutes
  --shuffle             Shuffle question order
  --shuffle-options     Shuffle answer option order
  --resume <session>    Resume a previously saved session
  --no-save             Don't auto-save the session

Workflow for examcollection.com VCE files:
  1. Open the .vce in VCE Exam Simulator and export it as .txt
     (current VCE files are encrypted; nothing third-party can read them directly)
  2. examsim import exam.txt -o exam.json
  3. examsim run exam.json
`)
}

func cmdProbe(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: examsim probe <file.vce>")
	}
	r, err := vce.Probe(args[0])
	if err != nil {
		return err
	}
	fmt.Print(r.String())
	return nil
}

func cmdImport(args []string) error {
	// Accept flags before or after the input file.
	var positional []string
	out := ""
	for i := 0; i < len(args); i++ {
		if (args[i] == "-o" || args[i] == "--o") && i+1 < len(args) {
			out = args[i+1]
			i++
			continue
		}
		positional = append(positional, args[i])
	}
	if len(positional) != 1 {
		return fmt.Errorf("usage: examsim import <file.txt> [-o exam.json]")
	}
	in := positional[0]
	e, err := importer.FromTextFile(in)
	if err != nil {
		return err
	}
	dest := out
	if dest == "" {
		ext := filepath.Ext(in)
		dest = in[:len(in)-len(ext)] + ".json"
	}
	if err := e.Save(dest); err != nil {
		return err
	}
	fmt.Printf("Imported %d questions → %s\n", len(e.Questions), dest)
	return nil
}

func cmdValidate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: examsim validate <exam.json>")
	}
	e, err := exam.Load(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("OK: %q — %d questions\n", e.Title, len(e.Questions))
	return nil
}

func cmdRun(args []string) error {
	// Accept flags before or after the exam file.
	flagArgs, positional := splitArgs(args, map[string]bool{
		"-mode": true, "--mode": true,
		"-time": true, "--time": true,
		"-resume": true, "--resume": true,
	})
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	mode := fs.String("mode", "study", "study or exam")
	timeLimit := fs.Int("time", 0, "time limit in minutes")
	shuffle := fs.Bool("shuffle", false, "shuffle question order")
	shuffleOpts := fs.Bool("shuffle-options", false, "shuffle answer option order")
	resume := fs.String("resume", "", "resume a saved session file")
	noSave := fs.Bool("no-save", false, "don't auto-save the session")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if len(positional) != 1 {
		return fmt.Errorf("usage: examsim run <exam.json> [flags]")
	}

	e, err := exam.Load(positional[0])
	if err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	sessionDir := ""
	if home != "" {
		sessionDir = filepath.Join(home, ".examsim", "sessions")
	}

	var s *player.Session
	if *resume != "" {
		s, err = player.LoadSession(*resume, e)
		if err != nil {
			return fmt.Errorf("resume: %w", err)
		}
		fmt.Printf("Resumed session: %d/%d answered\n", len(s.Answers), len(s.Order))
	} else {
		var m player.Mode
		switch *mode {
		case "study":
			m = player.Study
		case "exam":
			m = player.ExamMode
		default:
			return fmt.Errorf("mode must be study or exam")
		}
		limit := *timeLimit
		if limit == 0 {
			limit = e.TimeLimitMinutes
		}
		s = player.NewSession(e, player.Options{
			Mode:             m,
			TimeLimitMinutes: limit,
			ShuffleQuestions: *shuffle,
			ShuffleOptions:   *shuffleOpts,
			SessionDir:       sessionDir,
			NoSave:           *noSave,
		})
	}

	p := player.New(e, s, player.Options{SessionDir: sessionDir, NoSave: *noSave}, os.Stdin, os.Stdout)
	p.Run()
	return nil
}
