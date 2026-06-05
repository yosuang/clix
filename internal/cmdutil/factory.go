package cmdutil

import "github.com/yosuang/clix/internal/iostreams"

type OutputOptions struct {
	JSONFields []string
	JSONSet    bool
	JQ         string
	JQSet      bool
}

type Factory struct {
	IO     *iostreams.IOStreams
	Output OutputOptions
}
