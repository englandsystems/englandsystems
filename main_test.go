package main

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleCLIHelpPrintsClearHelp(t *testing.T) {
	var out bytes.Buffer

	handled, err := handleCLI([]string{"help"}, &out)
	if err != nil {
		t.Fatalf("handle help: %v", err)
	}
	if !handled {
		t.Fatal("help command should be handled")
	}

	content := out.String()
	for _, want := range []string{
		"Usage:",
		"englandsystems [command]",
		"db-path",
		"set-credentials",
		"PORT                         Web server port. Defaults to 9944.",
		"ENGLANDSYSTEMS_DB_PATH",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("help missing %q:\n%s", want, content)
		}
	}
}

func TestHandleCLIDBPathPrintsResolvedPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.sqlite3")
	t.Setenv(dbPathEnv, path)
	var out bytes.Buffer

	handled, err := handleCLI([]string{"db-path"}, &out)
	if err != nil {
		t.Fatalf("handle db-path: %v", err)
	}
	if !handled {
		t.Fatal("db-path command should be handled")
	}
	if got := strings.TrimSpace(out.String()); got != path {
		t.Fatalf("db-path output = %q, want %q", got, path)
	}
}

func TestHandleCLIUnknownCommandShowsHelp(t *testing.T) {
	var out bytes.Buffer

	handled, err := handleCLI([]string{"bogus"}, &out)
	if err == nil {
		t.Fatal("unknown command should return an error")
	}
	if !handled {
		t.Fatal("unknown command should be handled")
	}
	if content := out.String(); !strings.Contains(content, "Usage:") {
		t.Fatalf("unknown command should print help:\n%s", content)
	}
}

func TestSaveContactMessageLimitsSubmissionsPerIP(t *testing.T) {
	db := newTestDB(t)
	application := &app{db: db}

	ip := "203.0.113.10"
	for i := 0; i < contactMessageLimit; i++ {
		if err := application.saveContactMessage(ip, "Test User", "test@example.com", "555-0100", "Hello"); err != nil {
			t.Fatalf("save message %d: %v", i+1, err)
		}
	}

	err := application.saveContactMessage(ip, "Test User", "test@example.com", "555-0100", "Hello again")
	if !errors.Is(err, errContactMessageLimit) {
		t.Fatalf("expected message limit error, got %v", err)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM contact_messages`); got != contactMessageLimit {
		t.Fatalf("contact_messages count = %d, want %d", got, contactMessageLimit)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM contact_message_submissions WHERE ip = ?`, ip); got != contactMessageLimit {
		t.Fatalf("contact_message_submissions count = %d, want %d", got, contactMessageLimit)
	}
}

func TestSaveContactMessagePurgesOnlyOldTrackingRows(t *testing.T) {
	db := newTestDB(t)
	application := &app{db: db}

	old := time.Now().UTC().Add(-(contactMessageWindow + time.Hour))
	ip := "203.0.113.20"
	if _, err := db.Exec(`INSERT INTO contact_message_submissions (ip, created_at) VALUES (?, ?)`, ip, old); err != nil {
		t.Fatalf("seed old tracking row: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO contact_messages (name, email, phone, message, created_at) VALUES (?, ?, ?, ?, ?)`,
		"Old User",
		"old@example.com",
		"555-0199",
		"Keep this message",
		old,
	); err != nil {
		t.Fatalf("seed old contact message: %v", err)
	}

	if err := application.saveContactMessage(ip, "Test User", "test@example.com", "555-0100", "Hello"); err != nil {
		t.Fatalf("save message: %v", err)
	}

	if got := countRows(t, db, `SELECT COUNT(*) FROM contact_message_submissions WHERE ip = ?`, ip); got != 1 {
		t.Fatalf("tracking rows after purge = %d, want 1", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM contact_messages`); got != 2 {
		t.Fatalf("contact messages after purge = %d, want 2", got)
	}
}

func TestPersistPosixProfileEnvCreatesManagedBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".profile")
	values := map[string]string{
		adminUsernameEnv: "admin",
		adminPasswordEnv: "pa'ss word",
	}

	if err := persistPosixProfileEnv(path, values); err != nil {
		t.Fatalf("persist profile env: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "export ENGLANDSYSTEMS_ADMIN_USERNAME='admin'") {
		t.Fatalf("profile missing username export:\n%s", content)
	}
	if !strings.Contains(content, "export ENGLANDSYSTEMS_ADMIN_PASSWORD='pa'\\''ss word'") {
		t.Fatalf("profile missing shell-quoted password export:\n%s", content)
	}
}

func TestPersistPosixProfileEnvReplacesExistingManagedBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".profile")
	initial := "export KEEP_ME=1\n\n# >>> englandsystems environment >>>\nexport ENGLANDSYSTEMS_ADMIN_USERNAME='old'\n# <<< englandsystems environment <<<\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	values := map[string]string{
		adminUsernameEnv: "new",
		adminPasswordEnv: "password",
	}
	if err := persistPosixProfileEnv(path, values); err != nil {
		t.Fatalf("persist profile env: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "old") {
		t.Fatalf("profile kept old managed value:\n%s", content)
	}
	if !strings.Contains(content, "export KEEP_ME=1") {
		t.Fatalf("profile removed unmanaged content:\n%s", content)
	}
	if strings.Count(content, "# >>> englandsystems environment >>>") != 1 {
		t.Fatalf("profile should contain one managed block:\n%s", content)
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.sqlite3")
	db, err := openDB(path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close test db: %v", err)
		}
	})
	return db
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()

	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}
