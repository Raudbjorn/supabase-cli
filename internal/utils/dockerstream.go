package utils

import (
	"io"
	"os"

	"golang.org/x/term"
)

// OutputStream wraps an io.Writer and implements the jsonmessage.Stream interface.
// Replaces github.com/docker/cli/cli/streams.NewOut.
type OutputStream struct {
	io.Writer
	fd       uintptr
	isTerm   bool
}

// NewOutputStream creates a new OutputStream from an io.Writer.
func NewOutputStream(w io.Writer) *OutputStream {
	out := &OutputStream{Writer: w}
	if f, ok := w.(*os.File); ok {
		out.fd = f.Fd()
		out.isTerm = term.IsTerminal(int(f.Fd()))
	}
	return out
}

// FD returns the file descriptor of the output stream.
func (o *OutputStream) FD() uintptr {
	return o.fd
}

// IsTerminal returns true if the stream is a terminal.
func (o *OutputStream) IsTerminal() bool {
	return o.isTerm
}
