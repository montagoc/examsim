// Package vce inspects .vce binaries and reports what examsim can do with
// them.
//
// The VCE binary format is proprietary. Older files stored content in
// recoverable form; current files (VCE Exam Simulator 2.x, the kind served by
// examcollection.com) are AES-256 encrypted and cannot be parsed by any
// third-party tool. Probe does not pretend otherwise: it measures the file's
// structure and entropy, then tells the user the honest next step.
package vce

import (
	"fmt"
	"math"
	"os"
	"strings"
)

// Report is the result of probing a .vce file.
type Report struct {
	Path           string
	Size           int64
	Entropy        float64 // Shannon entropy of the sampled bytes (0-8)
	ReadableRuns   int     // number of long printable runs found
	LooksEncrypted bool
	Summary        string
	NextStep       string
}

// Probe inspects a .vce file and reports whether its content is recoverable.
func Probe(path string) (*Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open vce file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat vce file: %w", err)
	}

	const sampleSize = 65536
	buf := make([]byte, sampleSize)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil, fmt.Errorf("read vce file: %w", err)
	}
	buf = buf[:n]

	r := &Report{Path: path, Size: info.Size(), Entropy: shannon(buf)}
	minRun := 24
	if len(buf) < 4096 {
		minRun = 10
	}
	r.ReadableRuns = countRuns(buf, minRun)
	r.LooksEncrypted = r.Entropy > 7.5 && r.ReadableRuns == 0

	if r.LooksEncrypted {
		r.Summary = "binary content is encrypted or compressed; no recoverable text found"
		r.NextStep = "open the file in VCE Exam Simulator (or VCE Designer), use " +
			"File > Export to save it as .txt, then run: examsim import <file>.txt"
	} else if r.ReadableRuns > 0 {
		r.Summary = fmt.Sprintf("contains %d readable text sections (older/unprotected format)", r.ReadableRuns)
		r.NextStep = "try extracting text with `strings`, or export to .txt from VCE " +
			"software for a clean import: examsim import <file>.txt"
	} else {
		r.Summary = "no readable text found; format unrecognized"
		r.NextStep = "export the exam to .txt from VCE Exam Simulator / VCE Designer, " +
			"then run: examsim import <file>.txt"
	}
	return r, nil
}

// String renders the report for the terminal.
func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "File:      %s\n", r.Path)
	fmt.Fprintf(&b, "Size:      %d bytes\n", r.Size)
	fmt.Fprintf(&b, "Entropy:   %.2f / 8.00\n", r.Entropy)
	fmt.Fprintf(&b, "Readable:  %s\n", r.Summary)
	fmt.Fprintf(&b, "Next step: %s\n", r.NextStep)
	return b.String()
}

// shannon computes Shannon entropy over the byte distribution.
func shannon(b []byte) float64 {
	if len(b) == 0 {
		return 0
	}
	var freq [256]int
	for _, c := range b {
		freq[c]++
	}
	var h float64
	n := float64(len(b))
	for _, f := range freq {
		if f == 0 {
			continue
		}
		p := float64(f) / n
		h -= p * math.Log2(p)
	}
	return h
}

// countRuns counts runs of printable ASCII (or UTF-16LE text) of at least
// minLen characters, a rough signal for unencrypted content.
func countRuns(b []byte, minLen int) int {
	runs := 0
	// ASCII runs.
	run := 0
	for _, c := range b {
		if c >= 32 && c < 127 {
			run++
		} else {
			if run >= minLen {
				runs++
			}
			run = 0
		}
	}
	if run >= minLen {
		runs++
	}
	// UTF-16LE runs: printable char followed by 0x00.
	run = 0
	for i := 0; i+1 < len(b); i += 2 {
		c := b[i]
		if b[i+1] == 0 && c >= 32 && c < 127 {
			run++
		} else {
			if run >= minLen {
				runs++
			}
			run = 0
			if b[i+1] != 0 {
				i-- // re-examine odd offsets cheaply
			}
		}
	}
	if run >= minLen {
		runs++
	}
	return runs
}
