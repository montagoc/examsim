// Package player runs interactive practice exam sessions in the terminal.
package player

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/montagoc/examsim/internal/exam"
)

// Mode selects the session behavior.
type Mode int

const (
	// Study shows immediate feedback and explanations after each answer.
	Study Mode = iota
	// ExamMode hides feedback until the end and optionally enforces a timer.
	ExamMode
)

// Options configures a session.
type Options struct {
	Mode             Mode
	TimeLimitMinutes int
	ShuffleQuestions bool
	ShuffleOptions   bool
	SessionDir       string // where auto-saves go; "" disables auto-save
	NoSave           bool
}

// Session is the serializable state of an in-progress exam.
type Session struct {
	ExamTitle        string        `json:"exam_title"`
	Mode             Mode          `json:"mode"`
	StartedAt        time.Time     `json:"started_at"`
	TimeLimitMinutes int           `json:"time_limit_minutes"`
	ElapsedSeconds   int64         `json:"elapsed_seconds"`
	Order            []int         `json:"order"`   // question IDs in presentation order
	OptMap           map[int][]int `json:"opt_map"` // qid -> display position -> original option index
	Answers          map[int][]int `json:"answers"` // qid -> chosen original option indices
	Marked           map[int]bool  `json:"marked"`
	Current          int           `json:"current"` // index into Order
	Finished         bool          `json:"finished"`
	questionByID     map[int]*exam.Question
	deadline         time.Time
}

// NewSession builds a session for the exam.
func NewSession(e *exam.Exam, opts Options) *Session {
	s := &Session{
		ExamTitle:        e.Title,
		Mode:             opts.Mode,
		StartedAt:        time.Now(),
		TimeLimitMinutes: opts.TimeLimitMinutes,
		Answers:          map[int][]int{},
		Marked:           map[int]bool{},
		OptMap:           map[int][]int{},
		questionByID:     map[int]*exam.Question{},
	}
	for i := range e.Questions {
		q := &e.Questions[i]
		s.questionByID[q.ID] = q
		s.Order = append(s.Order, q.ID)
		// Default option order = original order.
		perm := make([]int, len(q.Options))
		for j := range perm {
			perm[j] = j
		}
		if opts.ShuffleOptions {
			rand.Shuffle(len(perm), func(a, b int) { perm[a], perm[b] = perm[b], perm[a] })
		}
		s.OptMap[q.ID] = perm
	}
	if opts.ShuffleQuestions {
		rand.Shuffle(len(s.Order), func(a, b int) { s.Order[a], s.Order[b] = s.Order[b], s.Order[a] })
	}
	if s.TimeLimitMinutes > 0 {
		s.deadline = time.Now().Add(time.Duration(s.TimeLimitMinutes) * time.Minute)
	}
	return s
}

// attach re-links questions after loading a session from disk.
func (s *Session) attach(e *exam.Exam) {
	s.questionByID = map[int]*exam.Question{}
	for i := range e.Questions {
		q := &e.Questions[i]
		s.questionByID[q.ID] = q
	}
	if s.TimeLimitMinutes > 0 {
		remaining := time.Duration(s.TimeLimitMinutes)*time.Minute - time.Duration(s.ElapsedSeconds)*time.Second
		if remaining < 0 {
			remaining = 0
		}
		s.deadline = time.Now().Add(remaining)
	}
}

// Save writes the session to path.
func (s *Session) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// LoadSession reads a session from path and links it to the exam.
func LoadSession(path string, e *exam.Exam) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	s.attach(e)
	return &s, nil
}

// Player drives one interactive session.
type Player struct {
	exam    *exam.Exam
	session *Session
	opts    Options
	in      *bufio.Reader
	out     io.Writer
	saveTo  string
}

// New creates a player. in/out allow testing; pass os.Stdin/os.Stdout normally.
func New(e *exam.Exam, s *Session, opts Options, in io.Reader, out io.Writer) *Player {
	return &Player{exam: e, session: s, opts: opts, in: bufio.NewReader(in), out: out}
}

func (p *Player) printf(format string, a ...any) { fmt.Fprintf(p.out, format, a...) }

func (p *Player) readLine() string {
	line, _ := p.in.ReadString('\n')
	return strings.TrimSpace(line)
}

// Run executes the interactive loop, then prints the final report.
func (p *Player) Run() {
	s := p.session
	if p.opts.SessionDir != "" && !p.opts.NoSave && p.saveTo == "" {
		slug := strings.Map(func(r rune) rune {
			if r >= 'A' && r <= 'Z' {
				return r + ('a' - 'A')
			}
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return '-'
		}, s.ExamTitle)
		p.saveTo = filepath.Join(p.opts.SessionDir, fmt.Sprintf("%s-%d.json", slug, time.Now().Unix()))
	}
	p.autosave()

	modeName := "STUDY"
	if s.Mode == ExamMode {
		modeName = "EXAM"
	}
	p.printf("\n=== %s  [%s mode] ===\n", s.ExamTitle, modeName)
	p.printf("%d questions", len(s.Order))
	if s.TimeLimitMinutes > 0 {
		p.printf("  |  time limit: %d min", s.TimeLimitMinutes)
	}
	if s.Mode == Study {
		p.printf("  |  immediate feedback on")
	}
	p.printf("\nCommands: <answer> | n next | p prev | j <num> jump | m mark | s status | e end | q quit\n\n")

	for {
		if s.TimeLimitMinutes > 0 && time.Now().After(s.deadline) {
			p.printf("\n*** Time is up! ***\n")
			break
		}
		if s.Current < 0 {
			s.Current = 0
		}
		if s.Current >= len(s.Order) {
			break
		}
		if !p.askQuestion() {
			return // user quit
		}
	}
	s.Finished = true
	p.autosave()
	p.report()
}

func (p *Player) askQuestion() bool {
	s := p.session
	qid := s.Order[s.Current]
	q := s.questionByID[qid]
	perm := s.OptMap[qid]
	total := len(s.Order)
	answered := len(s.Answers)

	p.printf("--- Question %d of %d  (answered %d/%d)", s.Current+1, total, answered, total)
	if s.Marked[qid] {
		p.printf("  [MARKED]")
	}
	if s.TimeLimitMinutes > 0 {
		rem := time.Until(s.deadline).Round(time.Second)
		if rem < 0 {
			rem = 0
		}
		p.printf("  [time left %s]", rem)
	}
	p.printf("\n")
	if q.Topic != "" {
		p.printf("Topic: %s\n", q.Topic)
	}
	p.printf("\n%s\n\n", q.Text)
	if q.Exhibit != "" {
		p.printf("[Exhibit]\n%s\n\n", q.Exhibit)
	}
	for disp, orig := range perm {
		p.printf("  %c. %s\n", 'A'+disp, q.Options[orig])
	}
	if prev, ok := s.Answers[qid]; ok {
		p.printf("\nYour current answer: %s\n", lettersOf(prev, perm))
	}
	p.printf("\n> ")

	for {
		line := p.readLine()
		lower := strings.ToLower(line)
		switch {
		case lower == "q":
			p.printf("Session saved. Resume later with: examsim run --resume %s\n", p.saveTo)
			return false
		case lower == "n":
			s.Current++
			p.autosave()
			return true
		case lower == "p":
			if s.Current > 0 {
				s.Current--
			}
			p.autosave()
			return true
		case lower == "m":
			s.Marked[qid] = !s.Marked[qid]
			if s.Marked[qid] {
				p.printf("Marked for review.\n")
			} else {
				p.printf("Unmarked.\n")
			}
			p.autosave()
			p.printf("\n> ")
		case lower == "s":
			p.status()
			p.printf("\n> ")
		case lower == "e":
			s.Current = len(s.Order)
			return true
		case strings.HasPrefix(lower, "j "):
			var n int
			if _, err := fmt.Sscanf(lower[2:], "%d", &n); err != nil || n < 1 || n > total {
				p.printf("Usage: j <1-%d>\n> ", total)
				continue
			}
			s.Current = n - 1
			p.autosave()
			return true
		default:
			chosen, err := parseAnswerInput(line, perm)
			if err != nil {
				p.printf("Enter answer letter(s) like \"b\" or \"a,c\", or a command.\n> ")
				continue
			}
			if q.Type == exam.Single && len(chosen) != 1 {
				p.printf("Single-choice: pick exactly one letter.\n> ")
				continue
			}
			s.Answers[qid] = chosen
			p.autosave()
			if s.Mode == Study {
				p.feedback(q, chosen)
				p.printf("\nPress Enter for next, or type a command > ")
				line := p.readLine()
				if strings.ToLower(line) == "q" {
					return false
				}
				s.Current++
				p.autosave()
				return true
			}
			// Exam mode: advance silently.
			s.Current++
			p.autosave()
			return true
		}
	}
}

func (p *Player) feedback(q *exam.Question, chosen []int) {
	if q.IsCorrect(chosen) {
		p.printf("\n  ✓ Correct (%s)\n", q.CorrectLetters())
	} else {
		p.printf("\n  ✗ Incorrect. Correct answer: %s\n", q.CorrectLetters())
	}
	if q.Explanation != "" {
		p.printf("\n  %s\n", q.Explanation)
	}
}

func (p *Player) status() {
	s := p.session
	p.printf("Answered %d of %d", len(s.Answers), len(s.Order))
	var marked []int
	for i, qid := range s.Order {
		if s.Marked[qid] {
			marked = append(marked, i+1)
		}
	}
	if len(marked) > 0 {
		p.printf("  |  marked: %v", marked)
	}
	p.printf("\n")
}

// Score tallies correct answers.
func (s *Session) Score() (correct, total int) {
	total = len(s.Order)
	for _, qid := range s.Order {
		q := s.questionByID[qid]
		if q == nil {
			continue
		}
		if q.IsCorrect(s.Answers[qid]) {
			correct++
		}
	}
	return correct, total
}

func (p *Player) report() {
	s := p.session
	correct, total := s.Score()
	pct := 0
	if total > 0 {
		pct = correct * 100 / total
	}
	elapsed := time.Since(s.StartedAt).Round(time.Second) + time.Duration(s.ElapsedSeconds)*time.Second

	p.printf("\n========== RESULTS ==========\n")
	p.printf("Score: %d/%d (%d%%)\n", correct, total, pct)
	passLine := p.exam.PassingScore
	if passLine <= 0 {
		passLine = 70
	}
	if pct >= passLine {
		p.printf("Result: PASS  (passing score %d%%)  🎉\n", passLine)
	} else {
		p.printf("Result: FAIL  (passing score %d%%)\n", passLine)
	}
	p.printf("Time: %s\n", elapsed)

	if correct < total {
		p.printf("\n----- Review: missed questions -----\n")
		for i, qid := range s.Order {
			q := s.questionByID[qid]
			chosen := s.Answers[qid]
			if q.IsCorrect(chosen) {
				continue
			}
			perm := s.OptMap[qid]
			p.printf("\nQ%d: %s\n", i+1, firstLine(q.Text, 100))
			p.printf("  You answered: %s   |   Correct: %s\n", lettersOf(chosen, perm), q.CorrectLetters())
			if q.Explanation != "" {
				p.printf("  %s\n", firstLine(q.Explanation, 220))
			}
		}
	}
	p.printf("\n")
}

func (p *Player) autosave() {
	if p.saveTo == "" || p.opts.NoSave {
		return
	}
	_ = p.session.Save(p.saveTo)
}

// lettersOf renders chosen original indices as display letters.
func lettersOf(chosen, perm []int) string {
	if len(chosen) == 0 {
		return "—"
	}
	inv := map[int]int{}
	for disp, orig := range perm {
		inv[orig] = disp
	}
	disp := make([]int, 0, len(chosen))
	for _, c := range chosen {
		if d, ok := inv[c]; ok {
			disp = append(disp, d)
		}
	}
	sort.Ints(disp)
	letters := make([]string, 0, len(disp))
	for _, d := range disp {
		letters = append(letters, string(rune('A'+d)))
	}
	return strings.Join(letters, ",")
}

// parseAnswerInput turns "b" or "a,c" into original option indices.
func parseAnswerInput(line string, perm []int) ([]int, error) {
	clean := strings.ToUpper(strings.ReplaceAll(line, ",", " "))
	var out []int
	seen := map[int]bool{}
	for _, tok := range strings.Fields(clean) {
		if len(tok) != 1 || tok[0] < 'A' || tok[0] > 'Z' {
			return nil, fmt.Errorf("bad input")
		}
		disp := int(tok[0] - 'A')
		if disp < 0 || disp >= len(perm) {
			return nil, fmt.Errorf("out of range")
		}
		orig := perm[disp]
		if !seen[orig] {
			seen[orig] = true
			out = append(out, orig)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty")
	}
	return out, nil
}

func firstLine(s string, max int) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
