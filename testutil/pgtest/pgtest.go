// Package pgtest gives each test a PostgreSQL database of its own: on the server the SIIKABOT_TEST_POSTGRES
// connection string names, where the tests may create databases, or else on a throwaway cluster started from the
// PostgreSQL binaries installed here. That cluster runs without fsync and listens only on a Unix socket in a
// temporary directory of its own: no TCP port, no network and no Docker, so test runs never meet each other's
// servers. Every database's name carries the test process's ID, so test runs sharing a server never meet each
// other's databases either.
//
// A package whose tests use it calls Main, or Run, from TestMain, which stops the cluster after the tests.
//
// It ports Detective's helper of the same name.
package pgtest

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Env names a server for the tests instead of a throwaway cluster, as a libpq URL or key=value string. Its user
// has to be able to create databases.
const Env = "SIIKABOT_TEST_POSTGRES"

// minVersion is the oldest PostgreSQL the tests run on: what claude-box has. Production and CI run 18.
const minVersion = 17

var (
	once     sync.Once
	admin    string // the connection string to the server's postgres database
	startErr error
	stop     func()
	inMain   atomic.Bool
	next     atomic.Int64

	templatesMu sync.Mutex
	templates   = map[string]*template{}
	// cloneMu makes copies of a template one at a time: PostgreSQL refuses to copy a database while anything
	// else is connected to it, another copy included
	cloneMu sync.Mutex
)

// template is a database prepared once per test process, which Prepared copies for each test
type template struct {
	once sync.Once
	name string
	err  error
}

// Main runs a package's tests, then stops the cluster they started, if they started one
func Main(m *testing.M) {
	os.Exit(Run(m))
}

// Run runs a package's tests and stops the cluster after them, returning the exit code. For a TestMain that does
// more than Main does.
func Run(m *testing.M) int {
	inMain.Store(true)
	code := m.Run()
	dropTemplates()
	if stop != nil {
		stop()
	}
	return code
}

// URL returns the connection string of a new, empty database for t, dropped when t ends
func URL(t testing.TB) string {
	t.Helper()
	server(t)
	name := newName("test")
	if err := run(context.Background(), "CREATE DATABASE "+name); err != nil {
		t.Fatalf("pgtest: %v", err)
	}
	dropAfter(t, name)
	return WithDatabase(admin, name)
}

// Prepared returns the connection string of a new database for t, dropped when t ends, as prepare leaves a
// database. prepare runs once per test process, under key, on a template that every later call copies, which is
// much faster than preparing each test's database anew. prepare has to close its connections when it's done.
func Prepared(t testing.TB, key string, prepare func(ctx context.Context, connString string) error) string {
	t.Helper()
	server(t)

	templatesMu.Lock()
	tmpl, ok := templates[key]
	if !ok {
		tmpl = &template{name: newName("tmpl")}
		templates[key] = tmpl
	}
	templatesMu.Unlock()

	tmpl.once.Do(func() {
		ctx := context.Background()
		if tmpl.err = run(ctx, "CREATE DATABASE "+tmpl.name); tmpl.err != nil {
			return
		}
		if tmpl.err = prepare(ctx, WithDatabase(admin, tmpl.name)); tmpl.err != nil {
			tmpl.err = fmt.Errorf("preparing the %s template: %w", key, tmpl.err)
		}
	})
	if tmpl.err != nil {
		t.Fatalf("pgtest: %v", tmpl.err)
	}

	name := newName("test")
	if err := clone(context.Background(), name, tmpl.name); err != nil {
		t.Fatalf("pgtest: %v", err)
	}
	dropAfter(t, name)
	return WithDatabase(admin, name)
}

// server starts the server for the tests, or finds the one Env names, once per test process
func server(t testing.TB) {
	t.Helper()
	if !inMain.Load() {
		t.Fatal("pgtest: call pgtest.Main or pgtest.Run from the package's TestMain, so the cluster stops after the tests")
	}
	once.Do(func() { admin, stop, startErr = start() })
	if startErr != nil {
		t.Fatal(startErr)
	}
}

// newName makes a database name no other test, nor any other test process, uses
func newName(kind string) string {
	return fmt.Sprintf("siikabot_%s_%d_%d", kind, os.Getpid(), next.Add(1))
}

// clone copies a template, retrying while the server still counts a connection to the template that has just
// closed
func clone(ctx context.Context, name, templateName string) error {
	cloneMu.Lock()
	defer cloneMu.Unlock()
	var err error
	for range 50 {
		err = run(ctx, "CREATE DATABASE "+name+" TEMPLATE "+templateName)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55006" { // object_in_use
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return err
}

// dropAfter drops a database when t ends
func dropAfter(t testing.TB, name string) {
	t.Cleanup(func() {
		if err := run(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("pgtest: %v", err)
		}
	})
}

// dropTemplates drops the templates, which matters on a server the tests don't throw away
func dropTemplates() {
	if admin == "" {
		return
	}
	templatesMu.Lock()
	defer templatesMu.Unlock()
	for _, tmpl := range templates {
		_ = run(context.Background(), "DROP DATABASE IF EXISTS "+tmpl.name+" WITH (FORCE)")
	}
}

// run runs one statement on the server's postgres database
func run(ctx context.Context, sql string) error {
	c, err := pgx.Connect(ctx, admin)
	if err != nil {
		return err
	}
	defer c.Close(ctx)
	_, err = c.Exec(ctx, sql)
	return err
}

// WithDatabase is the connection string s with its database replaced by name
func WithDatabase(s, name string) string {
	if strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://") {
		u, err := url.Parse(s)
		if err == nil {
			u.Path = "/" + name
			return u.String()
		}
	}
	return s + " dbname=" + name // in a key=value string, the last value given counts
}

// start finds the server: the one Env names, or a throwaway cluster it starts
func start() (string, func(), error) {
	if s := os.Getenv(Env); s != "" {
		return WithDatabase(s, "postgres"), nil, nil
	}
	bin, err := binaries()
	if err != nil {
		return "", nil, err
	}
	removeAbandonedClusters()
	dir, err := os.MkdirTemp("", clusterDirPrefix)
	if err != nil {
		return "", nil, err
	}
	// The socket's path, with its name .s.PGSQL.5432, must fit in 107 bytes
	sock := dir
	if len(sock) > 80 {
		if sock, err = os.MkdirTemp("/tmp", "pg"); err != nil {
			return "", nil, err
		}
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
		_ = os.RemoveAll(sock)
	}
	data := filepath.Join(dir, "data")
	if out, err := exec.CommandContext(context.Background(), filepath.Join(bin, "initdb"), "-D", data, "-U", "postgres",
		"-A", "trust", "--no-sync", "--no-locale", "-E", "UTF8").CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("pgtest: initdb: %w: %s", err, out)
	}
	logFile, err := os.Create(filepath.Join(dir, "postgres.log"))
	if err != nil {
		cleanup()
		return "", nil, err
	}
	pg := exec.CommandContext(context.Background(), filepath.Join(bin, "postgres"), "-D", data, "-k", sock,
		"-c", "listen_addresses=", "-c", "fsync=off", "-c", "synchronous_commit=off", "-c", "full_page_writes=off",
		"-c", "shared_buffers=16MB", "-c", "max_connections=300")
	pg.Stdout, pg.Stderr = logFile, logFile
	// A panicking test ends the process without Run stopping the cluster, so the kernel stops it then
	pg.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err := pg.Start(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("pgtest: postgres: %w", err)
	}
	// exited closes when postgres has stopped, with why in waitErr
	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = pg.Wait()
		close(exited)
	}()
	stop := func() {
		_ = pg.Process.Signal(os.Interrupt) // a fast shutdown
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			_ = pg.Process.Kill()
			<-exited
		}
		_ = logFile.Close()
		cleanup()
	}
	conn := fmt.Sprintf("host='%s' user=postgres dbname=postgres sslmode=disable", sock)
	for deadline := time.Now().Add(30 * time.Second); ; {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		c, err := pgx.Connect(ctx, conn)
		cancel()
		if err == nil {
			_ = c.Close(context.Background())
			return conn, stop, nil
		}
		select {
		case <-exited:
			log, _ := os.ReadFile(logFile.Name())
			stop()
			return "", nil, fmt.Errorf("pgtest: postgres stopped while starting: %w: %s", waitErr, log)
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			stop()
			return "", nil, fmt.Errorf("pgtest: postgres didn't accept connections in 30 s: %w", err)
		}
	}
}

// clusterDirPrefix starts the names of the clusters' temporary directories
const clusterDirPrefix = "siikabot-pg-"

// removeAbandonedClusters removes the directories of clusters whose test processes ended without stopping
// them, such as with a panic, before the kernel stopped them. Other test runs' clusters may be starting or
// running at the same time, so only a directory with no running server in it, and old enough not to be one
// that's just starting, goes.
func removeAbandonedClusters() {
	dirs, _ := filepath.Glob(filepath.Join(os.TempDir(), clusterDirPrefix+"*"))
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || time.Since(info.ModTime()) < time.Hour {
			continue
		}
		pidFile, err := os.ReadFile(filepath.Join(dir, "data", "postmaster.pid"))
		if err == nil {
			pid, _ := strconv.Atoi(strings.TrimSpace(strings.SplitN(string(pidFile), "\n", 2)[0]))
			if pid > 0 && syscall.Kill(pid, 0) == nil {
				continue // still running
			}
		}
		_ = os.RemoveAll(dir)
	}
}

var majorRE = regexp.MustCompile(`/postgresql/(\d+)/bin$`)

// binaries finds the directory of PostgreSQL's server binaries: initdb's on PATH, or the newest of the versioned
// directories Debian and Ubuntu install them in
func binaries() (string, error) {
	if p, err := exec.LookPath("initdb"); err == nil {
		dir := filepath.Dir(p)
		return dir, checkVersion(dir)
	}
	dirs, _ := filepath.Glob("/usr/lib/postgresql/*/bin")
	sort.Slice(dirs, func(i, j int) bool { return major(dirs[i]) < major(dirs[j]) })
	for i := len(dirs) - 1; i >= 0; i-- {
		if _, err := os.Stat(filepath.Join(dirs[i], "initdb")); err == nil {
			return dirs[i], checkVersion(dirs[i])
		}
	}
	return "", errors.New("pgtest: the database tests need PostgreSQL: install its server binaries (initdb on PATH, " +
		"or under /usr/lib/postgresql/<version>/bin), or set " + Env + " to a server where the tests may create databases")
}

func major(dir string) int {
	m := majorRE.FindStringSubmatch(dir)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

var versionRE = regexp.MustCompile(`\(PostgreSQL\) (\d+)`)

// checkVersion fails for a PostgreSQL older than the tests run on
func checkVersion(dir string) error {
	out, err := exec.CommandContext(context.Background(), filepath.Join(dir, "postgres"), "--version").Output()
	if err != nil {
		return fmt.Errorf("pgtest: %s: %w", filepath.Join(dir, "postgres"), err)
	}
	m := versionRE.FindSubmatch(out)
	if m == nil {
		return fmt.Errorf("pgtest: can't read the version in %q", out)
	}
	if n, _ := strconv.Atoi(string(m[1])); n < minVersion {
		return fmt.Errorf("pgtest: PostgreSQL %d in %s is older than the %d the tests run on", n, dir, minVersion)
	}
	return nil
}
