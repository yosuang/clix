package clixcmd

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/yosuang/clix/internal/adapter"
	"github.com/yosuang/clix/internal/catalog"
	"github.com/yosuang/clix/internal/cmd"
	"github.com/yosuang/clix/internal/cmdutil"
	"github.com/yosuang/clix/internal/domain"
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
	catalogLoader := catalog.NewLoader(catalog.Options{ToolsDir: layout.ToolsDir})
	runStore := &lazyRunStore{path: layout.DatabasePath}
	runService := &lazyRunService{
		catalogLoader: catalogLoader,
		store:         runStore,
		secrets:       environmentSecrets(),
	}
	cleanup := func() {
		_ = runStore.Close()
	}
	return &cmdutil.Factory{
		IO:            io,
		CatalogLoader: catalogLoader,
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

type lazyRunStore struct {
	path string
	db   *store.SQLite
}

func (s *lazyRunStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *lazyRunStore) open() (*store.SQLite, error) {
	if s.db != nil {
		return s.db, nil
	}
	db, err := store.Open(s.path)
	if err != nil {
		return nil, err
	}
	s.db = db
	return db, nil
}

func (s *lazyRunStore) InsertRun(ctx context.Context, run domain.Run) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	return db.InsertRun(ctx, run)
}

func (s *lazyRunStore) GetRun(ctx context.Context, id string) (domain.Run, error) {
	db, err := s.open()
	if err != nil {
		return domain.Run{}, err
	}
	return db.GetRun(ctx, id)
}

func (s *lazyRunStore) ListRuns(ctx context.Context, status *domain.Status) ([]domain.Run, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	return db.ListRuns(ctx, status)
}

func (s *lazyRunStore) ClaimPendingRun(ctx context.Context, id string, startedAt time.Time) (domain.Run, error) {
	db, err := s.open()
	if err != nil {
		return domain.Run{}, err
	}
	return db.ClaimPendingRun(ctx, id, startedAt)
}

func (s *lazyRunStore) MarkSucceeded(ctx context.Context, id string, finishedAt time.Time) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	return db.MarkSucceeded(ctx, id, finishedAt)
}

func (s *lazyRunStore) MarkFailed(ctx context.Context, id string, finishedAt time.Time, code protocol.Code, message string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	return db.MarkFailed(ctx, id, finishedAt, code, message)
}

func (s *lazyRunStore) MarkRejected(ctx context.Context, id string, finishedAt time.Time) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	return db.MarkRejected(ctx, id, finishedAt)
}

type lazyRunService struct {
	catalogLoader cmdutil.CatalogLoader
	store         *lazyRunStore
	secrets       map[string]string
	service       *runservice.Service
}

func (s *lazyRunService) serviceInstance() (*runservice.Service, error) {
	if s.service != nil {
		return s.service, nil
	}
	loadedCatalog, err := s.catalogLoader.Load()
	if err != nil {
		return nil, err
	}
	s.service = runservice.New(runservice.ServiceOptions{
		Store:    s.store,
		Catalog:  loadedCatalog,
		Adapters: adapter.NewRegistry(adapter.WithSecrets(s.secrets)),
	})
	return s.service, nil
}

func (s *lazyRunService) Run(ctx context.Context, toolName string, input json.RawMessage) (runservice.Result, error) {
	service, err := s.serviceInstance()
	if err != nil {
		return runservice.Result{}, err
	}
	return service.Run(ctx, toolName, input)
}

func (s *lazyRunService) Approve(ctx context.Context, runID string) (runservice.Result, error) {
	service, err := s.serviceInstance()
	if err != nil {
		return runservice.Result{}, err
	}
	return service.Approve(ctx, runID)
}

func (s *lazyRunService) Reject(ctx context.Context, runID string) (domain.Run, error) {
	service, err := s.serviceInstance()
	if err != nil {
		return domain.Run{}, err
	}
	return service.Reject(ctx, runID)
}
