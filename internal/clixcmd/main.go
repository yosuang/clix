package clixcmd

import (
	"os"
	"strings"

	"github.com/yosuang/clix/internal/adapter"
	"github.com/yosuang/clix/internal/catalog"
	"github.com/yosuang/clix/internal/cmd"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/iostreams"
	"github.com/yosuang/clix/internal/paths"
	"github.com/yosuang/clix/internal/protocol"
	"github.com/yosuang/clix/internal/runservice"
	"github.com/yosuang/clix/internal/store"
)

func Main() {
	os.Exit(Run(iostreams.System(), os.Args[1:]))
}

func Run(io *iostreams.IOStreams, args []string) int {
	f, cleanup, err := newFactory(io)
	if err != nil {
		writeRunError(io, args, err)
		return protocol.ExitCode(err)
	}
	defer cleanup()

	root := cmd.NewRoot(f)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		writeRunError(io, args, err)
		return protocol.ExitCode(err)
	}
	return 0
}

func newFactory(io *iostreams.IOStreams) (*cmdutil.Factory, func(), error) {
	layout, err := paths.Resolve()
	if err != nil {
		return nil, func() {}, err
	}
	loadedCatalog, err := catalog.Load(catalog.Options{ToolsDir: layout.ToolsDir})
	if err != nil {
		return nil, func() {}, err
	}
	runStore, err := store.Open(layout.DatabasePath)
	if err != nil {
		return nil, func() {}, err
	}
	adapters := adapter.NewRegistry(adapter.WithSecrets(environmentSecrets()))
	runService := runservice.New(runservice.ServiceOptions{
		Store:    runStore,
		Catalog:  loadedCatalog,
		Adapters: adapters,
	})
	cleanup := func() {
		_ = runStore.Close()
	}
	return &cmdutil.Factory{
		IO:            io,
		CatalogLoader: loadedCatalogLoader{catalog: loadedCatalog},
		RunStore:      runStore,
		RunService:    runService,
	}, cleanup, nil
}

func writeRunError(io *iostreams.IOStreams, args []string, err error) {
	if jsonFlagRequested(args) {
		_ = protocol.WriteJSONError(io.ErrOut, err)
	} else {
		_ = protocol.WriteTextError(io.ErrOut, err)
	}
}

func jsonFlagRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || strings.HasPrefix(arg, "--json=") {
			return true
		}
	}
	return false
}

func environmentSecrets() map[string]string {
	secrets := map[string]string{}
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			secrets[name] = value
		}
	}
	return secrets
}

type loadedCatalogLoader struct {
	catalog catalog.Catalog
}

func (l loadedCatalogLoader) Load() (catalog.Catalog, error) {
	return l.catalog, nil
}
