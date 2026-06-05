package clixcmd

import (
	"os"
	"strings"

	"github.com/yosuang/clix/internal/cmd"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/iostreams"
	"github.com/yosuang/clix/internal/protocol"
)

func Main() {
	os.Exit(Run(iostreams.System(), os.Args[1:]))
}

func Run(io *iostreams.IOStreams, args []string) int {
	f := &cmdutil.Factory{IO: io}
	root := cmd.NewRoot(f)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		if f.Output.JSONSet || jsonFlagRequested(args) {
			_ = protocol.WriteJSONError(io.ErrOut, err)
		} else {
			_ = protocol.WriteTextError(io.ErrOut, err)
		}
		return protocol.ExitCode(err)
	}
	return 0
}

func jsonFlagRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || strings.HasPrefix(arg, "--json=") {
			return true
		}
	}
	return false
}
