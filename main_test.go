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

func TestSetCredentialsTakesEffectForRunningServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "credentials.json")
	t.Setenv(credentialsPathEnv, path)
	t.Setenv(adminUsernameEnv, "stale-user")
	t.Setenv(adminPasswordEnv, "stale-password")

	if err := setCredentials([]string{"--username", "new-user", "--password", "new-password"}); err != nil {
		t.Fatalf("set credentials: %v", err)
	}

	// A service keeps its original environment. The credential file must take
	// precedence so a live process sees updates made by another CLI process.
	if err := os.Setenv(adminUsernameEnv, "stale-user"); err != nil {
		t.Fatalf("restore stale username: %v", err)
	}
	if err := os.Setenv(adminPasswordEnv, "stale-password"); err != nil {
		t.Fatalf("restore stale password: %v", err)
	}
	if !credentialsValid("new-user", "new-password") {
		t.Fatal("new credentials should be valid without restarting the server")
	}
	if credentialsValid("stale-user", "stale-password") {
		t.Fatal("stale environment credentials should not override the saved credentials")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat credentials file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("credentials file mode = %o, want 600", got)
	}
}

func TestCredentialsWorkAcrossWorkingDirectories(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "user-config")
	setFromDir := filepath.Join(root, "set-from")
	runFromDir := filepath.Join(root, "run-from")
	for _, path := range []string{setFromDir, runFromDir} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("create working directory: %v", err)
		}
	}
	t.Setenv(credentialsPathEnv, "")
	t.Setenv("HOME", root)
	t.Setenv("APPDATA", configDir)
	t.Setenv("USERPROFILE", root)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv(adminUsernameEnv, "stale-user")
	t.Setenv(adminPasswordEnv, "stale-password")

	wantPath, err := credentialsPath()
	if err != nil {
		t.Fatalf("resolve credentials path: %v", err)
	}

	t.Chdir(setFromDir)
	if err := setCredentials([]string{"--username", "portable-user", "--password", "portable-password"}); err != nil {
		t.Fatalf("set credentials from %s: %v", setFromDir, err)
	}

	// Simulate a separately launched server with its original environment and
	// a completely different working directory.
	if err := os.Setenv(adminUsernameEnv, "stale-user"); err != nil {
		t.Fatalf("restore stale username: %v", err)
	}
	if err := os.Setenv(adminPasswordEnv, "stale-password"); err != nil {
		t.Fatalf("restore stale password: %v", err)
	}
	t.Chdir(runFromDir)
	if !credentialsValid("portable-user", "portable-password") {
		t.Fatal("credentials set in one working directory should work when the server runs in another")
	}

	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("credentials were not saved in the user config directory: %v", err)
	}
}

func TestCredentialsPathOverrideMustBeAbsolute(t *testing.T) {
	t.Setenv(credentialsPathEnv, "relative/credentials.json")
	if _, err := credentialsPath(); err == nil {
		t.Fatal("relative credentials path should be rejected because it depends on the working directory")
	}
}

func TestAdminLoginUsesSavedCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv(credentialsPathEnv, path)
	if err := persistAdminCredentials(path, adminCredentialValues{
		Username: "admin",
		Password: "correct horse battery staple",
	}); err != nil {
		t.Fatalf("persist credentials: %v", err)
	}

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
