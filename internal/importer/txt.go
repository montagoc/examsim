// Package importer converts external exam text formats into examsim's
// native exam format.
//
// Supported input: the plain-text layout produced by exporting an exam from
// VCE software ("Export to TXT"), e.g.:
//
//	QUESTION 1
//	Topic: Networking
//
//	Which command displays the routing table on Linux?
//
//	A. ifconfig
//	B. ip route
//	C. netstat -i
//	D. ss -tuln
//
//	Correct Answer: B
//	Explanation: ip route shows the kernel routing table; ...
//
// Rules are intentionally tolerant: "QUESTION" headers are case-insensitive,
// options may use "A." or "A)", answers accept "B", "B,C" or "B C", and
// explanation blocks run until the next question header or end of file.
package importer

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/montagoc/examsim/internal/exam"
)

var (
	questionRe = regexp.MustCompile(`(?i)^question\s+(\d+)\s*$`)
	optionRe   = regexp.MustCompile(`^([A-Za-z])[.)]\s+(.*\S)\s*$`)
	answerRe   = regexp.MustCompile(`(?i)^(?:correct\s+)?answers?\s*:\s*([A-Za-z][A-Za-z,\s]*)$`)
	topicRe    = regexp.MustCompile(`(?i)^topic\s*:\s*(.+?)\s*$`)
	exhibitRe  = regexp.MustCompile(`(?i)^exhibit\s*:\s*(.*)$`)
	explRe     = regexp.MustCompile(`(?i)^explanation\s*:\s*(.*)$`)
)

// FromTextFile parses a VCE-style text export into an Exam.
func FromTextFile(path string) (*exam.Exam, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open text file: %w", err)
	}
	defer f.Close()

	e := &exam.Exam{PassingScore: 70}
	var cur *exam.Question
	var textLines []string
	var explLines []string
	inExplanation := false
	currentOption := -1 // index into cur.Options being continued, -1 = question text

	flush := func() {
		if cur == nil {
			return
		}
		cur.Text = strings.TrimSpace(strings.Join(textLines, "\n"))
		cur.Explanation = strings.TrimSpace(strings.Join(explLines, "\n"))
		if len(cur.Correct) > 1 {
			cur.Type = exam.Multiple
		} else {
			cur.Type = exam.Single
		}
		e.Questions = append(e.Questions, *cur)
		cur = nil
		textLines = nil
		explLines = nil
		inExplanation = false
		currentOption = -1
	}

	lineNo := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		lineNo++
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		if m := questionRe.FindStringSubmatch(line); m != nil {
			flush()
			var id int
			fmt.Sscanf(m[1], "%d", &id)
			cur = &exam.Question{ID: id, Type: exam.Single}
			continue
		}
		if cur == nil {
			// Preamble before the first QUESTION header: use as exam title.
			if e.Title == "" {
				e.Title = line
			} else {
				e.Description += line + "\n"
			}
			e.Description = strings.TrimSpace(e.Description)
			continue
		}
		if m := topicRe.FindStringSubmatch(line); m != nil {
			cur.Topic = strings.TrimSpace(m[1])
			continue
		}
		if m := exhibitRe.FindStringSubmatch(line); m != nil {
			if cur.Exhibit != "" {
				cur.Exhibit += "\n"
			}
			cur.Exhibit += strings.TrimSpace(m[1])
			continue
		}
		if m := explRe.FindStringSubmatch(line); m != nil {
			inExplanation = true
			currentOption = -1
			if strings.TrimSpace(m[1]) != "" {
				explLines = append(explLines, strings.TrimSpace(m[1]))
			}
			continue
		}
		if m := answerRe.FindStringSubmatch(line); m != nil {
			letters, lerr := parseLetters(m[1], len(cur.Options), lineNo)
			if lerr != nil {
				return nil, lerr
			}
			cur.Correct = letters
			inExplanation = false
			currentOption = -1
			continue
		}
		if m := optionRe.FindStringSubmatch(line); m != nil {
			inExplanation = false
			want := len(cur.Options) // next option must be the next letter
			got := int(strings.ToUpper(m[1])[0] - 'A')
			if got != want {
				return nil, fmt.Errorf("line %d: expected option %c, got %c", lineNo, 'A'+want, 'A'+got)
			}
			cur.Options = append(cur.Options, strings.TrimSpace(m[2]))
			currentOption = len(cur.Options) - 1
			continue
		}
		// Continuation line.
		if inExplanation {
			explLines = append(explLines, line)
		} else if currentOption >= 0 {
			cur.Options[currentOption] += " " + line
		} else {
			textLines = append(textLines, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read text file: %w", err)
	}
	flush()

	if strings.TrimSpace(e.Title) == "" {
		e.Title = "Imported exam"
	}
	if verr := e.Validate(); verr != nil {
		return nil, fmt.Errorf("imported exam failed validation: %w", verr)
	}
	return e, nil
}

// parseLetters converts "B", "A,C" or "A C" into 0-based option indices.
func parseLetters(s string, nOptions, lineNo int) ([]int, error) {
	clean := strings.ToUpper(strings.ReplaceAll(s, ",", " "))
	var out []int
	for _, tok := range strings.Fields(clean) {
		if len(tok) != 1 || tok[0] < 'A' || tok[0] > 'Z' {
			return nil, fmt.Errorf("line %d: bad answer letter %q", lineNo, tok)
		}
		idx := int(tok[0] - 'A')
		if idx >= nOptions {
			return nil, fmt.Errorf("line %d: answer %q beyond %d options", lineNo, tok, nOptions)
		}
		out = append(out, idx)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("line %d: no answer letters found", lineNo)
	}
	return out, nil
}
