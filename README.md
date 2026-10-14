# examsim

A terminal-based practice exam runner, written in Go. Built for running
certification practice exams from VCE-derived content.

## The honest VCE situation

`.vce` files from examcollection.com are **AES-256 encrypted** (VCE Exam
Simulator 2.x format). No third-party tool can parse them directly — anyone
claiming otherwise is selling something. examsim doesn't pretend:

1. `examsim probe exam.vce` — inspects the binary and tells you whether its
   content is recoverable or encrypted.
2. For encrypted files: open the `.vce` in VCE Exam Simulator (or VCE
   Designer), use **File → Export** to save it as `.txt`.
3. `examsim import exam.txt -o exam.json` — converts the text export into
   examsim's native JSON format.
4. `examsim run exam.json` — take the practice exam.

## Install

```bash
go build -o examsim ./cmd/examsim
# or: go install github.com/montagoc/examsim/cmd/examsim@latest
```

Requires Go 1.25+. No external dependencies.

## Usage

```bash
# Inspect a VCE binary
examsim probe exam.vce

# Convert a VCE text export to examsim JSON
examsim import exam.txt -o exam.json

# Validate an exam file
examsim validate exam.json

# Study mode: immediate feedback + explanations (default)
examsim run exam.json

# Exam mode: timed, scored at the end, no peeking
examsim run exam.json --mode exam --time 90

# Shuffle questions and/or options
examsim run exam.json --shuffle --shuffle-options

# Sessions auto-save to ~/.examsim/sessions/; resume with:
examsim run exam.json --resume ~/.examsim/sessions/<session>.json
```

### In-session commands

| Input      | Action                          |
|------------|---------------------------------|
| `b` / `a,c`| Answer with letter(s)           |
| `n` / `p`  | Next / previous question        |
| `j 12`     | Jump to question 12             |
| `m`        | Mark / unmark for review        |
| `s`        | Show progress                   |
| `e`        | End exam and score              |
| `q`        | Quit (session stays saved)      |

## Text import format

`examsim import` reads the plain-text layout from a VCE "Export to TXT":

```
Exam Title Here

QUESTION 1
Topic: Networking

Which command displays the routing table?

A. ifconfig
B. ip route
C. netstat -i

Correct Answer: B
Explanation: ip route shows the kernel routing table.
```

- `QUESTION N` headers (case-insensitive) start each question.
- `Topic:` is optional. `Exhibit:` lines are shown as exhibits.
- Options use `A.` or `A)` style; letters must run A, B, C… in order.
- Answers: `Correct Answer: B`, `Answer: A, C`, or `Answers: A C`.
- `Explanation:` runs until the next question or end of file.
- More than one correct letter ⇒ multiple-choice ("select all that apply").
- A shipped example lives in `exams/sample-linux-basics.txt`.

The native JSON schema is documented by the types in
`internal/exam/exam.go` — hand-write exams directly if you prefer.

## Project layout

```
cmd/examsim/          CLI entrypoint (probe/import/validate/run)
internal/exam/        Exam + Question types, JSON load/save/validate
internal/importer/    VCE text-export parser → Exam
internal/vce/         .vce binary prober (encryption/format detection)
internal/player/      Interactive session engine, scoring, save/resume
exams/                Sample exams
```

## Roadmap

- PDF import (examcollection also ships PDFs)
- TUI with colors (bubbletea)
- Spaced-repetition / weak-area drilling from session history
- Per-topic score breakdowns
- Export results to Markdown/PDF

## Note on content

This tool is for practicing with exam content you have the right to use.
Many VCE files circulate as unauthorized "braindumps" of live exams — using
those to pass a certification can get the certification revoked. Practice
ethically.
