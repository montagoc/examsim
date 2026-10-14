// Package exam defines examsim's native exam format: the Exam and Question
// types, JSON loading/saving, and validation.
package exam

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// QuestionType is the kind of question.
type QuestionType string

const (
	// Single means exactly one correct option.
	Single QuestionType = "single"
	// Multiple means one or more correct options ("select all that apply").
	Multiple QuestionType = "multiple"
)

// Question is a single exam question.
type Question struct {
	ID          int          `json:"id"`
	Type        QuestionType `json:"type"`
	Topic       string       `json:"topic,omitempty"`
	Text        string       `json:"text"`
	Exhibit     string       `json:"exhibit,omitempty"`
	Options     []string     `json:"options"`
	Correct     []int        `json:"correct"` // 0-based indices into Options
	Explanation string       `json:"explanation,omitempty"`
}

// CorrectLetters returns the correct answers as letters, e.g. "B" or "A,C".
func (q Question) CorrectLetters() string {
	letters := make([]string, 0, len(q.Correct))
	for _, i := range q.Correct {
		letters = append(letters, string(rune('A'+i)))
	}
	return strings.Join(letters, ",")
}

// Exam is a complete practice exam.
type Exam struct {
	Title            string     `json:"title"`
	Description      string     `json:"description,omitempty"`
	Author           string     `json:"author,omitempty"`
	Version          string     `json:"version,omitempty"`
	PassingScore     int        `json:"passing_score"`      // percent, 0-100
	TimeLimitMinutes int        `json:"time_limit_minutes"` // 0 = no limit
	Questions        []Question `json:"questions"`
}

// Load reads an exam from a JSON file.
func Load(path string) (*Exam, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read exam file: %w", err)
	}
	var e Exam
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parse exam JSON: %w", err)
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return &e, nil
}

// Save writes the exam as pretty-printed JSON.
func (e *Exam) Save(path string) error {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("encode exam JSON: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write exam file: %w", err)
	}
	return nil
}

// Validate checks the exam for structural problems.
func (e *Exam) Validate() error {
	var problems []string
	if strings.TrimSpace(e.Title) == "" {
		problems = append(problems, "title is empty")
	}
	if e.PassingScore < 0 || e.PassingScore > 100 {
		problems = append(problems, fmt.Sprintf("passing_score %d out of range 0-100", e.PassingScore))
	}
	if len(e.Questions) == 0 {
		problems = append(problems, "exam has no questions")
	}
	seen := map[int]bool{}
	for i := range e.Questions {
		q := &e.Questions[i]
		prefix := fmt.Sprintf("question %d", q.ID)
		if q.ID <= 0 {
			prefix = fmt.Sprintf("question at index %d", i)
		} else if seen[q.ID] {
			problems = append(problems, fmt.Sprintf("%s: duplicate id", prefix))
		}
		seen[q.ID] = true
		if q.Type != Single && q.Type != Multiple {
			problems = append(problems, fmt.Sprintf("%s: unknown type %q", prefix, q.Type))
		}
		if strings.TrimSpace(q.Text) == "" {
			problems = append(problems, fmt.Sprintf("%s: empty text", prefix))
		}
		if len(q.Options) < 2 {
			problems = append(problems, fmt.Sprintf("%s: fewer than 2 options", prefix))
		}
		if len(q.Correct) == 0 {
			problems = append(problems, fmt.Sprintf("%s: no correct answer", prefix))
		}
		for _, c := range q.Correct {
			if c < 0 || c >= len(q.Options) {
				problems = append(problems, fmt.Sprintf("%s: correct index %d out of range", prefix, c))
			}
		}
		if q.Type == Single && len(q.Correct) != 1 {
			problems = append(problems, fmt.Sprintf("%s: single-choice with %d correct answers", prefix, len(q.Correct)))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid exam:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// IsCorrect reports whether the given 0-based option indices exactly match
// the correct answers (order-independent).
func (q Question) IsCorrect(chosen []int) bool {
	if len(chosen) != len(q.Correct) {
		return false
	}
	want := map[int]bool{}
	for _, c := range q.Correct {
		want[c] = true
	}
	for _, c := range chosen {
		if !want[c] {
			return false
		}
	}
	return true
}
