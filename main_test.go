package main

import (
	"bytes"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
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
		"englandsystems --db <path>",
		"db [path]",
		"All variables below are required before starting the server:",
		"ENGLANDSYSTEMS_PORT          Web server port (1-65535).",
		"ENGLANDSYSTEMS_ADMIN_USERNAME",
		"ENGLANDSYSTEMS_ADMIN_PASSWORD",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("help missing %q:\n%s", want, content)
		}
	}
}

func TestHandleCLIDBInitializesDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "custom.sqlite3")
	var out bytes.Buffer

	handled, err := handleCLI([]string{"db", path}, &out)
	if err != nil {
		t.Fatalf("handle db: %v", err)
	}
	if !handled {
		t.Fatal("db command should be handled")
	}
	if !strings.Contains(out.String(), path) {
		t.Fatalf("db output = %q, want path %q", out.String(), path)
	}
	db, err := openDB(path)
	if err != nil {
		t.Fatalf("open initialized database: %v", err)
	}
	db.Close()
}

func TestLoadServerConfigRequiresDatabaseArgument(t *testing.T) {
	setServerEnvironment(t)
	if _, err := loadServerConfig(nil); err == nil || !strings.Contains(err.Error(), "--db") {
		t.Fatalf("loadServerConfig error = %v, want required --db argument", err)
	}
}

func TestLoadServerConfigRequiresEveryEnvironmentVariable(t *testing.T) {
	for _, key := range []string{portEnv, adminUsernameEnv, adminPasswordEnv, sessionSecretEnv} {
		t.Setenv(key, "")
	}

	_, err := loadServerConfig([]string{"--db", "database.sqlite3"})
	if err == nil {
		t.Fatal("loadServerConfig should reject missing environment variables")
	}
	for _, key := range []string{portEnv, adminUsernameEnv, adminPasswordEnv, sessionSecretEnv} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("missing-variable error %q does not mention %s", err, key)
		}
	}
}

func TestLoadServerConfigAcceptsExplicitValidEnvironment(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "messages.sqlite3")
	t.Setenv(portEnv, "9944")
	t.Setenv(adminUsernameEnv, "admin")
	t.Setenv(adminPasswordEnv, "correct horse battery staple")
	t.Setenv(sessionSecretEnv, "a-separate-session-signing-secret")

	config, err := loadServerConfig([]string{"--db", dbPath})
	if err != nil {
		t.Fatalf("loadServerConfig: %v", err)
	}
	if config.databasePath != dbPath {
		t.Fatalf("database path = %q, want %q", config.databasePath, dbPath)
	}
	if config.port != "9944" {
		t.Fatalf("port = %q, want 9944", config.port)
	}
}

func TestLoadServerConfigRejectsInvalidPort(t *testing.T) {
	t.Setenv(portEnv, "70000")
	t.Setenv(adminUsernameEnv, "admin")
	t.Setenv(adminPasswordEnv, "correct horse battery staple")
	t.Setenv(sessionSecretEnv, "a-separate-session-signing-secret")

	if _, err := loadServerConfig([]string{"--db", filepath.Join(t.TempDir(), "messages.sqlite3")}); err == nil {
		t.Fatalf("loadServerConfig should reject invalid %s", portEnv)
	}
}

func TestNormalizeDBPathAcceptsRelativePath(t *testing.T) {
	path, err := normalizeDBPath("data/messages.sqlite3")
	if err != nil || !filepath.IsAbs(path) {
		t.Fatalf("normalizeDBPath = %q, %v; want absolute path", path, err)
	}
}

func TestOpenDBRejectsMissingDatabasePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "messages.sqlite3")

	if _, err := openDB(path); err == nil {
		t.Fatal("openDB should reject a missing database")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup created database unexpectedly: %v", err)
	}
}

func TestOpenDBRejectsUninitializedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.sqlite3")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openDB(path); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("openDB error = %v, want uninitialized database error", err)
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

func TestValidateMessage(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		phone   string
		message string
		valid   bool
	}{
		{name: "Phillip England", email: "phillip@example.com", phone: "+1 (918) 555-0123", message: "Please call me.", valid: true},
		{name: "Phillip England", email: "phillip@example.com", phone: "", message: "Email me instead.", valid: true},
		{name: "Phillip England", email: "not-an-email", phone: "918-555-0123", message: "Hello", valid: false},
		{name: "Phillip England", email: "phillip@example", phone: "918-555-0123", message: "Hello", valid: false},
		{name: "Phillip England", email: "phillip@example..com", phone: "918-555-0123", message: "Hello", valid: false},
		{name: "Phillip England", email: "phillip@example.com", phone: "call-me-maybe", message: "Hello", valid: false},
		{name: "Phillip England", email: "phillip@example.com", phone: "12345", message: "Hello", valid: false},
		{name: "Phillip England", email: "phillip@example.com", phone: "111-111-1111", message: "Hello", valid: false},
		{name: "Phillip England", email: "phillip@example.com", phone: "(918 555-0123", message: "Hello", valid: false},
		{name: "Phillip\nEngland", email: "phillip@example.com", phone: "918-555-0123", message: "Hello", valid: false},
		{name: "12345", email: "phillip@example.com", phone: "918-555-0123", message: "Hello", valid: false},
		{name: "Phillip England", email: "phillip@example.com", phone: "918-555-0123", message: "!!!", valid: false},
	}

	for _, test := range tests {
		t.Run(test.email+"/"+test.phone, func(t *testing.T) {
			err := validateMessage(test.name, test.email, test.phone, test.message)
			if test.valid && err != nil {
				t.Fatalf("validateMessage returned %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("validateMessage accepted invalid input")
			}
		})
	}
}

func TestContactRejectsInvalidPhoneWithoutSaving(t *testing.T) {
	db := newTestDB(t)
	application := &app{db: db}
	request := httptest.NewRequest(
		http.MethodPost,
		"/contact",
		strings.NewReader("name=Test+User&email=test%40example.com&phone=definitely-fake&message=Hello"),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	application.contact(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("contact status = %d, want %d; body: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM contact_messages`); got != 0 {
		t.Fatalf("contact_messages count = %d, want 0", got)
	}
}

func TestServicesRedirectsToHomepageSection(t *testing.T) {
	application := &app{}

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		request := httptest.NewRequest(method, "/services", nil)
		response := httptest.NewRecorder()

		application.services(response, request)

		if response.Code != http.StatusSeeOther {
			t.Fatalf("%s services status = %d, want %d", method, response.Code, http.StatusSeeOther)
		}
		if location := response.Header().Get("Location"); location != "/#services" {
			t.Fatalf("%s services redirect = %q, want %q", method, location, "/#services")
		}
	}
}

func TestCredentialsValidUsesEnvironment(t *testing.T) {
	t.Setenv(adminUsernameEnv, "admin")
	t.Setenv(adminPasswordEnv, "correct horse battery staple")

	if !credentialsValid("admin", "correct horse battery staple") {
		t.Fatal("matching environment credentials should be valid")
	}
	if credentialsValid("admin", "wrong password") {
		t.Fatal("incorrect password should be invalid")
	}
}

func TestCredentialsValidRejectsMissingEnvironment(t *testing.T) {
	t.Setenv(adminUsernameEnv, "")
	t.Setenv(adminPasswordEnv, "")

	if credentialsValid("", "") {
		t.Fatal("empty environment credentials must not allow login")
	}
}

func TestAdminLoginUsesEnvironmentCredentials(t *testing.T) {
	t.Setenv(adminUsernameEnv, "admin")
	t.Setenv(adminPasswordEnv, "correct horse battery staple")

	application := &app{db: newTestDB(t)}
	request := httptest.NewRequest(
		http.MethodPost,
		"/admin/login",
		strings.NewReader("username=admin&password=correct+horse+battery+staple"),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	application.adminLogin(response, request)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d; body: %s", response.Code, http.StatusSeeOther, response.Body.String())
	}
	if location := response.Header().Get("Location"); location != "/admin" {
		t.Fatalf("login redirect = %q, want /admin", location)
	}
	if len(response.Result().Cookies()) == 0 {
		t.Fatal("successful login should set an authentication cookie")
	}
	authenticatedRequest := httptest.NewRequest(http.MethodGet, "/admin", nil)
	for _, cookie := range response.Result().Cookies() {
		authenticatedRequest.AddCookie(cookie)
	}
	if !isAuthenticated(authenticatedRequest) {
		t.Fatal("cookie created from saved credentials should authenticate the next request")
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.sqlite3")
	if err := initializeDB(path); err != nil {
		t.Fatalf("initialize test db: %v", err)
	}
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

func setServerEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv(portEnv, "9944")
	t.Setenv(adminUsernameEnv, "admin")
	t.Setenv(adminPasswordEnv, "correct horse battery staple")
	t.Setenv(sessionSecretEnv, "a-separate-session-signing-secret")
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()

	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}
