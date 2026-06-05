package cmdutil

import "github.com/yosuang/clix/internal/iostreams"

type OutputOptions struct {
	JSONFields []string
	JQ         string
}

type Factory struct {
	IO     *iostreams.IOStreams
	Output OutputOptions
}
