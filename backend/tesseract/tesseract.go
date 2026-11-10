// Package tesseract runs the Tesseract OCR command-line tool.
package tesseract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/dhan0406/pokescan/backend/apierr"
)

const defaultTimeout = 5 * time.Second

// Result is the text Tesseract read and its mean word confidence (0-100).
type Result struct {
	Text       string
	Confidence float64
	Words      []Word
}

// Word is one word Tesseract read, with its confidence (0-100).
type Word struct {
	Text       string
	Confidence float64
}

// Engine runs the tesseract binary at Path. A zero Timeout means 5s.
type Engine struct {
	Path    string
	Timeout time.Duration
}

// Read runs OCR on a PNG image containing a single line of text. The process
// is killed when ctx ends or the timeout passes; a timeout or deadline returns
// an error matching apierr.ErrTimeout.
func (e Engine) Read(ctx context.Context, png []byte) (Result, error) {
	timeout := e.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// --psm 7: treat the image as a single text line. tsv: per-word output
	// with confidences.
	cmd := exec.CommandContext(ctx, e.Path, "stdin", "stdout", "--psm", "7", "tsv")
	cmd.Stdin = bytes.NewReader(png)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// If the process is killed, don't wait for child processes still holding
	// its output pipes open.
	cmd.WaitDelay = 200 * time.Millisecond

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{}, fmt.Errorf("%w: tesseract: %v", apierr.ErrTimeout, err)
		}
		if ctx.Err() != nil {
			return Result{}, fmt.Errorf("tesseract: %w", ctx.Err())
		}
		return Result{}, fmt.Errorf("tesseract: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseTSV(stdout.String()), nil
}

// parseTSV joins the word rows (level 5) of Tesseract's TSV output, keeping
// each word's confidence and their mean. Rows with confidence -1 are not words.
func parseTSV(tsv string) Result {
	var r Result
	var texts []string
	for i, line := range strings.Split(tsv, "\n") {
		if i == 0 || line == "" {
			continue // header
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 12 || cols[0] != "5" {
			continue
		}
		conf, err := strconv.ParseFloat(cols[10], 64)
		text := strings.TrimSpace(cols[11])
		if err != nil || conf < 0 || text == "" {
			continue
		}
		r.Words = append(r.Words, Word{Text: text, Confidence: conf})
		texts = append(texts, text)
		r.Confidence += conf
	}
	if len(r.Words) == 0 {
		return Result{}
	}
	r.Text = strings.Join(texts, " ")
	r.Confidence /= float64(len(r.Words))
	return r
}
