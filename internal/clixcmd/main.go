package clixcmd

import (
	"fmt"
	"os"

	"github.com/yosuang/clix/internal/cmd"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/iostreams"
	"github.com/yosuang/clix/internal/protocol"
)

func Main() {
	io := iostreams.System()
	f := &cmdutil.Factory{IO: io}
	root := cmd.NewRoot(f)
	if err := root.Execute(); err != nil {
		perr := protocol.AsError(err)
		_, _ = fmt.Fprintln(io.ErrOut, perr.Error())
		os.Exit(protocol.ExitCode(perr))
	}
}
