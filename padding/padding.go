package padding

import (
	"bytes"
	"io"
	"sync"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/ansi"
)

type PaddingFunc func(w io.Writer)

type Writer struct {
	Padding uint
	PadFunc PaddingFunc

	ansiWriter *ansi.Writer
	buf        bytes.Buffer
	cache      bytes.Buffer
	lineLen    int
	ansi       bool
}

func NewWriter(width uint, paddingFunc PaddingFunc) *Writer {
	w := &Writer{
		Padding: width,
		PadFunc: paddingFunc,
	}
	w.ansiWriter = &ansi.Writer{
		Forward: &w.buf,
	}
	return w
}

func NewWriterPipe(forward io.Writer, width uint, paddingFunc PaddingFunc) *Writer {
	return &Writer{
		Padding: width,
		PadFunc: paddingFunc,
		ansiWriter: &ansi.Writer{
			Forward: forward,
		},
	}
}

// Bytes is shorthand for declaring a new default padding-writer instance,
// used to immediately pad a byte slice.
func Bytes(b []byte, width uint) []byte {
	f := acquireWriter(width)
	defer wp.Put(f)

	_, _ = f.Write(b)

	if f.lineLen != 0 {
		_ = f.pad()
	}

	return f.buf.Bytes()
}

// String is shorthand for declaring a new default padding-writer instance,
// used to immediately pad a string.
func String(s string, width uint) string {
	return string(Bytes([]byte(s), width))
}

// Write is used to write content to the padding buffer.
func (w *Writer) Write(b []byte) (int, error) {
	for _, c := range string(b) {
		if c == '\x1B' {
			// ANSI escape sequence
			w.ansi = true
		} else if w.ansi {
			if (c >= 0x41 && c <= 0x5a) || (c >= 0x61 && c <= 0x7a) {
				// ANSI sequence terminated
				w.ansi = false
			}
		} else {
			w.lineLen += runewidth.RuneWidth(c)

			if c == '\n' {
				// end of current line
				err := w.pad()
				if err != nil {
					return 0, err
				}
				w.ansiWriter.ResetAnsi()
				w.lineLen = 0
			}
		}

		_, err := w.writeRune(c)
		if err != nil {
			return 0, err
		}
	}

	return len(b), nil
}

func (w *Writer) pad() error {
	if w.Padding > 0 && uint(w.lineLen) < w.Padding {
		if w.PadFunc != nil {
			for i := 0; i < int(w.Padding)-w.lineLen; i++ {
				w.PadFunc(w.ansiWriter)
			}
		} else {
			_, err := w.ansiWriter.Write(bytes.Repeat([]byte(" "), int(w.Padding)-w.lineLen))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (w *Writer) writeRune(r rune) (int, error) {
	bb := make([]byte, utf8.UTFMax)
	n := utf8.EncodeRune(bb, r)
	return w.ansiWriter.Write(bb[:n])
}

// Close will finish the padding operation.
func (w *Writer) Close() (err error) {
	return w.Flush()
}

// Bytes returns the padded result as a byte slice.
func (w *Writer) Bytes() []byte {
	return w.cache.Bytes()
}

// String returns the padded result as a string.
func (w *Writer) String() string {
	return w.cache.String()
}

// Flush will finish the padding operation. Always call it before trying to
// retrieve the final result.
func (w *Writer) Flush() (err error) {
	if w.lineLen != 0 {
		if err = w.pad(); err != nil {
			return
		}
	}

	w.cache.Reset()
	_, err = w.buf.WriteTo(&w.cache)
	w.lineLen = 0
	w.ansi = false

	return
}

var wp = sync.Pool{
	New: func() interface{} {
		w := &Writer{}
		w.ansiWriter = &ansi.Writer{
			Forward: &w.buf,
		}
		return w
	},
}

func acquireWriter(width uint) *Writer {
	w := wp.Get().(*Writer)
	w.Padding = width
	w.lineLen = 0
	w.ansi = false
	w.buf.Reset()

	return w
}
