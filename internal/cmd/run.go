package cmd

import (
	"bytes"
	"io"
	"strings"

	"github.com/yosuang/clix/internal/protocol"
)

type RunOptions struct {
	InputFlag string
	StdinTTY  bool
}

func (o RunOptions) InputReader(stdin io.Reader) (io.Reader, error) {
	if o.InputFlag != "" {
		if !o.StdinTTY {
			piped, err := io.ReadAll(stdin)
			if err != nil {
				return nil, protocol.NewError(protocol.ValidationError, "stdin could not be read")
			}
			if len(bytes.TrimSpace(piped)) > 0 {
				return nil, protocol.NewError(protocol.UsageError, "--input cannot be combined with non-empty stdin")
			}
		}
		return strings.NewReader(o.InputFlag), nil
	}
	if o.StdinTTY {
		return nil, protocol.NewError(protocol.ValidationError, "input is required")
	}
	return stdin, nil
}
