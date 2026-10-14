package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/montagoc/examsim/internal/exam"
)

const sampleDoc = `My Sample Exam

QUESTION 1
Topic: Networking

Which command shows the routing table?

A. ifconfig
B. ip route
C. netstat -i

Correct Answer: B
Explanation: ip route shows the kernel routing table.

QUESTION 2

Select the colors of the French flag. (Select all that apply)

A) Red
B) Green
C) White
D) Blue

Answer: A, C, D
Explanation: Blue, white, red — vertical bands.
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "exam.txt")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFromTextFile(t *testing.T) {
	e, err := FromTextFile(writeTemp(t, sampleDoc))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if e.Title != "My Sample Exam" {
		t.Errorf("title = %q", e.Title)
	}
	if len(e.Questions) != 2 {
		t.Fatalf("got %d questions, want 2", len(e.Questions))
	}
	q1 := e.Questions[0]
	if q1.Type != exam.Single {
		t.Errorf("q1 type = %q, want single", q1.Type)
	}
	if q1.Topic != "Networking" {
		t.Errorf("q1 topic = %q", q1.Topic)
	}
	if len(q1.Options) != 3 || q1.Options[1] != "ip route" {
		t.Errorf("q1 options = %v", q1.Options)
	}
	if len(q1.Correct) != 1 || q1.Correct[0] != 1 {
		t.Errorf("q1 correct = %v, want [1]", q1.Correct)
	}
	if q1.Explanation == "" {
		t.Error("q1 explanation empty")
	}
	q2 := e.Questions[1]
	if q2.Type != exam.Multiple {
		t.Errorf("q2 type = %q, want multiple", q2.Type)
	}
	if len(q2.Correct) != 3 {
		t.Errorf("q2 correct = %v, want 3 answers", q2.Correct)
	}
}

func TestFromTextFileBadAnswer(t *testing.T) {
	doc := "QUESTION 1\n\nPick one.\n\nA. x\nB. y\n\nCorrect Answer: Z\n"
	_, err := FromTextFile(writeTemp(t, doc))
	if err == nil {
		t.Error("expected error for out-of-range answer, got nil")
	}
}

func TestFromTextFileNoQuestions(t *testing.T) {
	_, err := FromTextFile(writeTemp(t, "just some text\n"))
	if err == nil {
		t.Error("expected validation error for zero questions, got nil")
	}
}

func TestSampleExamFile(t *testing.T) {
	// The shipped sample exam must import cleanly.
	e, err := FromTextFile("../../exams/sample-linux-basics.txt")
	if err != nil {
		t.Fatalf("sample exam import failed: %v", err)
	}
	if len(e.Questions) != 6 {
		t.Errorf("got %d questions, want 6", len(e.Questions))
	}
	multi := 0
	for _, q := range e.Questions {
		if q.Type == exam.Multiple {
			multi++
		}
	}
	if multi != 1 {
		t.Errorf("got %d multiple-choice questions, want 1", multi)
	}
}
