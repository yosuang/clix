package iostreams

import (
	"io"
	"os"
)

type IOStreams struct {
	In       io.Reader
	Out      io.Writer
	ErrOut   io.Writer
	StdinTTY bool
}

func System() *IOStreams {
	return &IOStreams{
		In:       os.Stdin,
		Out:      os.Stdout,
		ErrOut:   os.Stderr,
		StdinTTY: isTerminal(os.Stdin),
	}
}

func TestIO(in io.Reader, out io.Writer, errOut io.Writer, stdinTTY bool) *IOStreams {
	return &IOStreams{In: in, Out: out, ErrOut: errOut, StdinTTY: stdinTTY}
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
