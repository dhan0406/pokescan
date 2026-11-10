package tesseract

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dhan0406/pokescan/backend/apierr"
)

const header = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"

func fakeEngine(t *testing.T) Engine {
	t.Helper()
	p, err := filepath.Abs("testdata/fake-tesseract.sh")
	if err != nil {
		t.Fatal(err)
	}
	return Engine{Path: p}
}

// withTSV makes the fake print tsv.
func withTSV(t *testing.T, tsv string) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "out.tsv")
	if err := os.WriteFile(f, []byte(tsv), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_TSV", f)
}

func TestRead_argsAndStdin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FAKE_ARGS_OUT", filepath.Join(dir, "args"))
	t.Setenv("FAKE_STDIN_OUT", filepath.Join(dir, "stdin"))
	withTSV(t, header)

	in := []byte("fake png bytes")
	if _, err := fakeEngine(t).Read(context.Background(), in, NameOptions); err != nil {
		t.Fatalf("Read: %v", err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if got := strings.Fields(string(args)); strings.Join(got, " ") != "stdin stdout --psm 7 tsv" {
		t.Errorf("args = %q, want stdin stdout --psm 7 tsv", got)
	}
	stdin, _ := os.ReadFile(filepath.Join(dir, "stdin"))
	if !bytes.Equal(stdin, in) {
		t.Errorf("stdin = %q, want %q", stdin, in)
	}
}

func TestRead_optionsBecomeArguments(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{"name options", NameOptions, "stdin stdout --psm 7 tsv"},
		{"name fallback: block mode", NameFallbackOptions, "stdin stdout --psm 6 tsv"},
		{"custom page mode, no whitelist", Options{PSM: 11}, "stdin stdout --psm 11 tsv"},
		{"zero value defaults to single line", Options{}, "stdin stdout --psm 7 tsv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := filepath.Join(t.TempDir(), "args")
			t.Setenv("FAKE_ARGS_OUT", args)
			withTSV(t, header)
			if _, err := fakeEngine(t).Read(context.Background(), []byte("x"), tt.opts); err != nil {
				t.Fatalf("Read: %v", err)
			}
			got, _ := os.ReadFile(args)
			if strings.Join(strings.Fields(string(got)), " ") != tt.want {
				t.Errorf("args = %q, want %q", strings.Fields(string(got)), tt.want)
			}
		})
	}
}

func TestRead_parsesWordsAndConfidence(t *testing.T) {
	withTSV(t, header+
		"1\t1\t0\t0\t0\t0\t0\t0\t400\t60\t-1\t\n"+
		"5\t1\t1\t1\t1\t1\t10\t5\t120\t40\t90.000000\tPikachu\n"+
		"5\t1\t1\t1\t1\t2\t140\t5\t20\t40\t70.000000\tV\n")
	got, err := fakeEngine(t).Read(context.Background(), []byte("x"), NameOptions)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Text != "Pikachu V" || got.Confidence != 80 {
		t.Errorf("Read = %+v, want {Pikachu V 80}", got)
	}
	want := []Word{{"Pikachu", 90, 10, 5, 120, 40}, {"V", 70, 140, 5, 20, 40}}
	if !reflect.DeepEqual(got.Words, want) {
		t.Errorf("Words = %+v, want %+v", got.Words, want)
	}
}

func TestRead_ignoresMinusOneRows(t *testing.T) {
	withTSV(t, header+
		"2\t1\t1\t0\t0\t0\t0\t0\t400\t60\t-1\t\n"+
		"4\t1\t1\t1\t1\t0\t0\t0\t400\t60\t-1\t\n"+
		"5\t1\t1\t1\t1\t1\t10\t5\t120\t40\t88.5\tSnorlax\n")
	got, err := fakeEngine(t).Read(context.Background(), []byte("x"), NameOptions)
	if err != nil || got.Text != "Snorlax" || got.Confidence != 88.5 || len(got.Words) != 1 {
		t.Errorf("Read = %+v, %v; want {Snorlax 88.5}", got, err)
	}
}

func TestRead_noWords(t *testing.T) {
	withTSV(t, header+"1\t1\t0\t0\t0\t0\t0\t0\t400\t60\t-1\t\n")
	got, err := fakeEngine(t).Read(context.Background(), []byte("x"), NameOptions)
	if err != nil || got.Text != "" || got.Confidence != 0 || len(got.Words) != 0 {
		t.Errorf("Read = %+v, %v; want zero Result, nil", got, err)
	}
}

func TestRead_nonZeroExit(t *testing.T) {
	t.Setenv("FAKE_EXIT", "1")
	_, err := fakeEngine(t).Read(context.Background(), []byte("x"), NameOptions)
	if err == nil || errors.Is(err, apierr.ErrTimeout) {
		t.Errorf("err = %v, want a non-timeout error", err)
	}
}

func TestRead_missingBinary(t *testing.T) {
	_, err := Engine{Path: filepath.Join(t.TempDir(), "no-such-tesseract")}.Read(context.Background(), []byte("x"), NameOptions)
	if err == nil {
		t.Error("err = nil, want an error")
	}
}

func TestRead_timeout(t *testing.T) {
	t.Setenv("FAKE_SLEEP", "2")
	e := fakeEngine(t)
	e.Timeout = 100 * time.Millisecond
	start := time.Now()
	_, err := e.Read(context.Background(), []byte("x"), NameOptions)
	if !errors.Is(err, apierr.ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Read took %v, want under 1s", d)
	}
}

func TestRead_cancelledContextKillsProcess(t *testing.T) {
	t.Setenv("FAKE_SLEEP", "2")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	_, err := fakeEngine(t).Read(ctx, []byte("x"), NameOptions)
	if err == nil {
		t.Error("err = nil, want an error")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Read took %v, want under 1s", d)
	}
}

func TestRead_realBinary(t *testing.T) {
	path, err := exec.LookPath("tesseract")
	if err != nil {
		t.Skip("tesseract not installed")
	}
	img := image.NewGray(image.Rect(0, 0, 400, 60))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	got, err := Engine{Path: path}.Read(context.Background(), buf.Bytes(), NameOptions)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Text != "" {
		t.Errorf("Text = %q, want empty for a blank image", got.Text)
	}
}
