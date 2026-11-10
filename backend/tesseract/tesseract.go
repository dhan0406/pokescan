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
	// Box in the image that was read, in pixels.
	Left, Top, Width, Height int
}

// Engine runs the tesseract binary at Path. A zero Timeout means 5s.
type Engine struct {
	Path    string
	Timeout time.Duration
}

// Options choose how Tesseract reads an image.
type Options struct {
	PSM       int    // page segmentation mode; 0 means 7 (a single text line)
	Whitelist string // if set, only these characters are recognized
}

// NameOptions reads the card name band: one line of ordinary text.
var NameOptions = Options{}

// NameFallbackOptions reads the name band as a block of text. Single-line mode
// sometimes finds nothing at all on a small crop that block mode reads fine, so
// the name is retried with this when the first read gives no usable name. Block
// mode adds more stray marks, which the caller filters by confidence.
var NameFallbackOptions = Options{PSM: 6}

// NumberOptions reads the collector number band. The band holds the number
// among other small text (illustrator, set symbol), so it is read as a block,
// and only characters a number can contain are allowed. That makes digits far
// more reliable, but Tesseract then reports a confidence of 0 for them, so the
// caller must validate the result another way.
var NumberOptions = Options{PSM: 6, Whitelist: "0123456789/ABCDEFGHIJKLMNOPQRSTUVWXYZ"}

// Read runs OCR on a PNG image containing a single line of text. The process
// is killed when ctx ends or the timeout passes; a timeout or deadline returns
// an error matching apierr.ErrTimeout.
func (e Engine) Read(ctx context.Context, png []byte, opts Options) (Result, error) {
	timeout := e.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	psm := opts.PSM
	if psm == 0 {
		psm = 7 // a single text line
	}
	// tsv: per-word output with confidences.
	args := []string{"stdin", "stdout", "--psm", strconv.Itoa(psm)}
	if opts.Whitelist != "" {
		args = append(args, "-c", "tessedit_char_whitelist="+opts.Whitelist)
	}
	args = append(args, "tsv")
	cmd := exec.CommandContext(ctx, e.Path, args...)
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
		w := Word{Text: text, Confidence: conf}
		w.Left, _ = strconv.Atoi(cols[6])
		w.Top, _ = strconv.Atoi(cols[7])
		w.Width, _ = strconv.Atoi(cols[8])
		w.Height, _ = strconv.Atoi(cols[9])
		r.Words = append(r.Words, w)
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
