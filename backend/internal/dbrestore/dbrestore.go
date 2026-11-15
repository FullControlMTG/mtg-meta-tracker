// Package dbrestore runs store.RestoreDump on behalf of the admin API: it picks
// dumps out of the dump directory and runs one restore at a time in the
// background, with the job worker paused, while the API reports maintenance.
package dbrestore

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

var (
	ErrBusy    = errors.New("a restore is already running")
	ErrBadName = errors.New("dump name must be a plain *.dump file name")
)

// A bare file name, so a request cannot reach outside the dump directory. The
// leading character rules out the .<name>.partial files a dump in flight writes.
var dumpName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.dump$`)

type Dump struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// List returns the dumps in dir, newest first. The Jenkins names embed a UTC
// timestamp, so name order is time order; mtimes on the NAS are not trusted.
func List(dir string) ([]Dump, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	dumps := []Dump{}
	for _, e := range entries {
		if e.IsDir() || !dumpName.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		dumps = append(dumps, Dump{Name: e.Name(), Size: info.Size(), ModTime: info.ModTime().UTC()})
	}
	sort.Slice(dumps, func(i, j int) bool { return dumps[i].Name > dumps[j].Name })
	return dumps, nil
}

// Path resolves a dump name inside dir, rejecting anything that is not one.
func Path(dir, name string) (string, error) {
	if !dumpName.MatchString(name) {
		return "", ErrBadName
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

type State string

const (
	StateIdle      State = "idle"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
)

type Status struct {
	State      State                 `json:"state"`
	Dump       string                `json:"dump,omitempty"`
	StartedAt  *time.Time            `json:"started_at,omitempty"`
	FinishedAt *time.Time            `json:"finished_at,omitempty"`
	Error      string                `json:"error,omitempty"`
	Report     *store.ReimportReport `json:"report,omitempty"`
}

// Runner holds the one restore this process may run. Its status lives in memory,
// not the jobs table: the restore replaces that table along with everything else.
type Runner struct {
	pool          *pgxpool.Pool
	dir           string
	pause, resume func()

	running atomic.Bool
	mu      sync.Mutex
	status  Status
}

// NewRunner restores into pool from dumps in dir. pause must return only once
// background work touching the database has stopped; resume restarts it.
func NewRunner(pool *pgxpool.Pool, dir string, pause, resume func()) *Runner {
	return &Runner{pool: pool, dir: dir, pause: pause, resume: resume, status: Status{State: StateIdle}}
}

func (r *Runner) Dir() string { return r.dir }

// InMaintenance reports whether a restore is under way.
func (r *Runner) InMaintenance() bool { return r.running.Load() }

func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// Start validates name and begins restoring it in the background.
func (r *Runner) Start(name string) error {
	path, err := Path(r.dir, name)
	if err != nil {
		return err
	}
	if !r.running.CompareAndSwap(false, true) {
		return ErrBusy
	}
	now := time.Now().UTC()
	r.mu.Lock()
	r.status = Status{State: StateRunning, Dump: name, StartedAt: &now}
	r.mu.Unlock()
	go r.run(path)
	return nil
}

func (r *Runner) run(path string) {
	defer r.running.Store(false)
	r.pause()
	defer r.resume()

	log.Printf("restore: flashing database from %s", path)
	// Not the request's context: the restore outlives the request that started it.
	report, err := store.RestoreDump(context.Background(), r.pool, path)
	// Pooled connections hold statements prepared against the dropped tables.
	r.pool.Reset()

	done := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.FinishedAt = &done
	if err != nil {
		log.Printf("restore: %s failed: %v", path, err)
		r.status.State, r.status.Error = StateFailed, fmt.Sprint(err)
		return
	}
	log.Printf("restore: %s done", path)
	r.status.State, r.status.Report = StateSucceeded, report
}
