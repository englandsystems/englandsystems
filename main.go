package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	adminUsernameEnv = "ENGLANDSYSTEMS_ADMIN_USERNAME"
	adminPasswordEnv = "ENGLANDSYSTEMS_ADMIN_PASSWORD"
	sessionSecretEnv = "ENGLANDSYSTEMS_SESSION_SECRET"
	dbPathEnv        = "ENGLANDSYSTEMS_DB_PATH"

	maxNameLength    = 120
	maxEmailLength   = 254
	maxPhoneLength   = 40
	maxMessageLength = 1000

	loginFailureWindow = 24 * time.Hour
	loginBanDuration   = 24 * time.Hour
	loginBanThreshold  = 5
	sessionDuration    = 12 * time.Hour

	contactMessageWindow = 24 * time.Hour
	contactMessageLimit  = 3
)

var errContactMessageLimit = errors.New("message limit reached")

//go:embed static/*
var staticFiles embed.FS

type app struct {
	db *sql.DB
}

type contactMessage struct {
	ID        int64
	Name      string
	Email     string
	Phone     string
	Message   string
	CreatedAt time.Time
}

func main() {
	handled, err := handleCLI(os.Args[1:], os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
	if handled {
		return
	}

	dbPath, err := databasePath()
	if err != nil {
		log.Fatal(err)
	}

	db, err := openDB(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	application := &app{db: db}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.FS(staticFiles)))
	mux.HandleFunc("/", application.route)
	mux.HandleFunc("/contact", application.contact)
	mux.HandleFunc("/admin", application.admin)
	mux.HandleFunc("/admin/login", application.adminLogin)
	mux.HandleFunc("/admin/logout", application.adminLogout)
	mux.HandleFunc("/admin/messages/delete", application.deleteMessage)

	addr := ":" + getEnv("PORT", "9944")
	log.Printf("England Systems listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleCLI(args []string, stdout io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}

	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprint(stdout, helpText())
		return true, nil
	case "db-path", "database-path":
		path, err := databasePath()
		if err != nil {
			return true, err
		}
		fmt.Fprintln(stdout, path)
		return true, nil
	case "set-credentials":
		return true, setCredentials(args[1:])
	default:
		fmt.Fprint(stdout, helpText())
		return true, fmt.Errorf("unknown command %q", args[0])
	}
}

func helpText() string {
	return `England Systems

Usage:
  englandsystems [command]

Commands:
  help                         Show this help screen.
  db-path                      Print the resolved SQLite database path.
  set-credentials              Save admin login credentials.

Options:
  -h, --help                   Show this help screen.

Environment:
  PORT                         Web server port. Defaults to 9944.
  ENGLANDSYSTEMS_DB_PATH       SQLite database path override.
  ENGLANDSYSTEMS_ADMIN_USERNAME
  ENGLANDSYSTEMS_ADMIN_PASSWORD

Examples:
  englandsystems
  englandsystems db-path
  englandsystems set-credentials --username "admin" --password "password"
`
}

func (a *app) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	http.ServeFileFS(w, r, staticFiles, "static/index.html")
}

func (a *app) contact(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		http.ServeFileFS(w, r, staticFiles, "static/contact.html")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Unable to read the contact form.", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		email := strings.TrimSpace(r.FormValue("email"))
		phone := strings.TrimSpace(r.FormValue("phone"))
		message := strings.TrimSpace(r.FormValue("message"))

		if err := validateMessage(name, email, phone, message); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := a.saveContactMessage(clientIP(r), name, email, phone, message); errors.Is(err, errContactMessageLimit) {
			http.Error(w, "You have reached the message limit for today. Please try again tomorrow.", http.StatusTooManyRequests)
			return
		} else if err != nil {
			log.Printf("save contact message: %v", err)
			http.Error(w, "Unable to save your message right now.", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/contact?sent=1", http.StatusSeeOther)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) admin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !isAuthenticated(r) {
		renderAdminLogin(w, "")
		return
	}

	rows, err := a.db.Query(`SELECT id, name, email, phone, message, created_at FROM contact_messages ORDER BY created_at DESC`)
	if err != nil {
		log.Printf("load messages: %v", err)
		http.Error(w, "Unable to load messages.", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var messages []contactMessage
	for rows.Next() {
		var message contactMessage
		if err := rows.Scan(&message.ID, &message.Name, &message.Email, &message.Phone, &message.Message, &message.CreatedAt); err != nil {
			log.Printf("scan message: %v", err)
			http.Error(w, "Unable to load messages.", http.StatusInternalServerError)
			return
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		log.Printf("messages rows: %v", err)
		http.Error(w, "Unable to load messages.", http.StatusInternalServerError)
		return
	}

	renderAdminDashboard(w, messages)
}

func (a *app) adminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := clientIP(r)
	if err := a.purgeLoginSecurity(); err != nil {
		log.Printf("purge login security: %v", err)
	}

	banned, err := a.isBanned(ip)
	if err != nil {
		log.Printf("check ban: %v", err)
		http.Error(w, "Unable to process login.", http.StatusInternalServerError)
		return
	}
	if banned {
		renderAdminLogin(w, "Too many failed attempts. Try again later.")
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Unable to read login form.", http.StatusBadRequest)
		return
	}

	if credentialsValid(r.FormValue("username"), r.FormValue("password")) {
		setSession(w)
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	if err := a.recordLoginFailure(ip); err != nil {
		log.Printf("record login failure: %v", err)
		http.Error(w, "Unable to process login.", http.StatusInternalServerError)
		return
	}

	renderAdminLogin(w, "Username or password is incorrect.")
}

func (a *app) adminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clearSession(w)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (a *app) deleteMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isAuthenticated(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Unable to read delete request.", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "Invalid message id.", http.StatusBadRequest)
		return
	}

	if _, err := a.db.Exec(`DELETE FROM contact_messages WHERE id = ?`, id); err != nil {
		log.Printf("delete message: %v", err)
		http.Error(w, "Unable to delete message.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func openDB(path string) (*sql.DB, error) {
	if err := ensureDBDir(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func databasePath() (string, error) {
	if path := os.Getenv(dbPathEnv); path != "" {
		return path, nil
	}

	dataDir, err := userDataDir()
	if err != nil {
		return "", fmt.Errorf("locate user data directory: %w", err)
	}
	return filepath.Join(dataDir, "englandsystems.sqlite3"), nil
}

func userDataDir() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "EnglandSystems"), nil
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "EnglandSystems"), nil
		}
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(configDir, "EnglandSystems"), nil
	default:
		if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
			return filepath.Join(dataHome, "englandsystems"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", "englandsystems"), nil
	}
}

func ensureDBDir(path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}

	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create database directory %q: %w", dir, err)
	}
	return nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS contact_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			phone TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS login_failures (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ip TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_login_failures_ip_created_at
			ON login_failures (ip, created_at);

		CREATE TABLE IF NOT EXISTS banned_ips (
			ip TEXT PRIMARY KEY,
			banned_until DATETIME NOT NULL,
			created_at DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS contact_message_submissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ip TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_contact_message_submissions_ip_created_at
			ON contact_message_submissions (ip, created_at);
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE contact_messages ADD COLUMN phone TEXT NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	return nil
}

func (a *app) saveContactMessage(ip, name, email, phone, message string) error {
	now := time.Now().UTC()
	cutoff := now.Add(-contactMessageWindow)

	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM contact_message_submissions WHERE created_at < ?`, cutoff); err != nil {
		return err
	}

	var count int
	err = tx.QueryRow(
		`SELECT COUNT(*) FROM contact_message_submissions WHERE ip = ? AND created_at >= ?`,
		ip,
		cutoff,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count >= contactMessageLimit {
		return errContactMessageLimit
	}

	if _, err := tx.Exec(`INSERT INTO contact_message_submissions (ip, created_at) VALUES (?, ?)`, ip, now); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`INSERT INTO contact_messages (name, email, phone, message, created_at) VALUES (?, ?, ?, ?, ?)`,
		name,
		email,
		phone,
		message,
		now,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (a *app) purgeLoginSecurity() error {
	cutoff := time.Now().UTC().Add(-loginFailureWindow)
	_, err := a.db.Exec(`DELETE FROM login_failures WHERE created_at < ?`, cutoff)
	if err != nil {
		return err
	}

	_, err = a.db.Exec(`DELETE FROM banned_ips WHERE banned_until <= ?`, time.Now().UTC())
	return err
}

func (a *app) isBanned(ip string) (bool, error) {
	var bannedUntil time.Time
	err := a.db.QueryRow(`SELECT banned_until FROM banned_ips WHERE ip = ?`, ip).Scan(&bannedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return bannedUntil.After(time.Now().UTC()), nil
}

func (a *app) recordLoginFailure(ip string) error {
	now := time.Now().UTC()
	if _, err := a.db.Exec(`INSERT INTO login_failures (ip, created_at) VALUES (?, ?)`, ip, now); err != nil {
		return err
	}

	var count int
	err := a.db.QueryRow(
		`SELECT COUNT(*) FROM login_failures WHERE ip = ? AND created_at >= ?`,
		ip,
		now.Add(-loginFailureWindow),
	).Scan(&count)
	if err != nil {
		return err
	}

	if count >= loginBanThreshold {
		_, err = a.db.Exec(
			`INSERT INTO banned_ips (ip, banned_until, created_at)
			 VALUES (?, ?, ?)
			 ON CONFLICT(ip) DO UPDATE SET banned_until = excluded.banned_until`,
			ip,
			now.Add(loginBanDuration),
			now,
		)
	}
	return err
}

func validateMessage(name, email, phone, message string) error {
	switch {
	case name == "":
		return errors.New("Name is required.")
	case email == "":
		return errors.New("Email is required.")
	case message == "":
		return errors.New("How can we help is required.")
	case len([]rune(name)) > maxNameLength:
		return fmt.Errorf("Name must be %d characters or fewer.", maxNameLength)
	case len([]rune(email)) > maxEmailLength:
		return fmt.Errorf("Email must be %d characters or fewer.", maxEmailLength)
	case len([]rune(phone)) > maxPhoneLength:
		return fmt.Errorf("Phone must be %d characters or fewer.", maxPhoneLength)
	case len([]rune(message)) > maxMessageLength:
		return fmt.Errorf("How can we help must be %d characters or fewer.", maxMessageLength)
	case !strings.Contains(email, "@"):
		return errors.New("Email must be valid.")
	default:
		return nil
	}
}

func credentialsValid(username, password string) bool {
	expectedUsername := os.Getenv(adminUsernameEnv)
	expectedPassword := os.Getenv(adminPasswordEnv)
	if expectedUsername == "" || expectedPassword == "" {
		return false
	}

	usernameOK := hmac.Equal([]byte(username), []byte(expectedUsername))
	passwordOK := hmac.Equal([]byte(password), []byte(expectedPassword))
	return usernameOK && passwordOK
}

func setSession(w http.ResponseWriter) {
	expires := time.Now().UTC().Add(sessionDuration)
	payload := fmt.Sprintf("%s|%d", os.Getenv(adminUsernameEnv), expires.Unix())
	signature := sign(payload)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + signature))

	http.SetCookie(w, &http.Cookie{
		Name:     "englandsystems_admin",
		Value:    value,
		Path:     "/admin",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "englandsystems_admin",
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie("englandsystems_admin")
	if err != nil {
		return false
	}

	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return false
	}

	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		return false
	}

	payload := parts[0] + "|" + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(sign(payload))) {
		return false
	}

	if parts[0] != os.Getenv(adminUsernameEnv) {
		return false
	}

	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	return time.Now().UTC().Before(time.Unix(expiresUnix, 0))
}

func sign(value string) string {
	mac := hmac.New(sha256.New, sessionSecret())
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func sessionSecret() []byte {
	if secret := os.Getenv(sessionSecretEnv); secret != "" {
		return []byte(secret)
	}
	sum := sha256.Sum256([]byte(os.Getenv(adminUsernameEnv) + ":" + os.Getenv(adminPasswordEnv)))
	return sum[:]
}

func clientIP(r *http.Request) string {
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		ip := strings.TrimSpace(strings.Split(forwardedFor, ",")[0])
		if ip != "" {
			return ip
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func setCredentials(args []string) error {
	var username, password string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--username":
			i++
			if i >= len(args) {
				return errors.New("--username requires a value")
			}
			username = args[i]
		case "--password":
			i++
			if i >= len(args) {
				return errors.New("--password requires a value")
			}
			password = args[i]
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}

	if username == "" || password == "" {
		return errors.New(`usage: englandsystems set-credentials --username "someusername" --password "somepassword"`)
	}

	values := map[string]string{
		adminUsernameEnv: username,
		adminPasswordEnv: password,
	}
	if err := persistUserEnv(values); err != nil {
		return err
	}
	for key, value := range values {
		os.Setenv(key, value)
	}

	fmt.Println("Saved admin credentials to the user environment.")
	return nil
}

func persistUserEnv(values map[string]string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate user home directory: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		if err := persistLaunchctlEnv(values); err != nil {
			return err
		}
		return persistPosixProfileEnv(filepath.Join(home, ".zprofile"), values)
	case "windows":
		for key, value := range values {
			if err := exec.Command("setx", key, value).Run(); err != nil {
				return fmt.Errorf("persist %s with setx: %w", key, err)
			}
		}
		return nil
	default:
		return persistPosixProfileEnv(filepath.Join(home, ".profile"), values)
	}
}

func persistLaunchctlEnv(values map[string]string) error {
	for key, value := range values {
		if err := exec.Command("launchctl", "setenv", key, value).Run(); err != nil {
			return fmt.Errorf("persist %s with launchctl: %w", key, err)
		}
	}
	return nil
}

func persistPosixProfileEnv(path string, values map[string]string) error {
	const (
		startMarker = "# >>> englandsystems environment >>>"
		endMarker   = "# <<< englandsystems environment <<<"
	)

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	content := string(existing)
	if start := strings.Index(content, startMarker); start >= 0 {
		if end := strings.Index(content[start:], endMarker); end >= 0 {
			end += start + len(endMarker)
			content = strings.TrimRight(content[:start], "\n") + "\n" + strings.TrimLeft(content[end:], "\n")
		}
	}

	var block strings.Builder
	block.WriteString(startMarker)
	block.WriteByte('\n')
	for _, key := range []string{adminUsernameEnv, adminPasswordEnv} {
		value, ok := values[key]
		if !ok {
			continue
		}
		block.WriteString("export ")
		block.WriteString(key)
		block.WriteByte('=')
		block.WriteString(posixShellQuote(value))
		block.WriteByte('\n')
	}
	block.WriteString(endMarker)
	block.WriteByte('\n')

	content = strings.TrimRight(content, "\n")
	if content != "" {
		content += "\n\n"
	}
	content += block.String()

	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func posixShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func renderAdminLogin(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminLoginTemplate.Execute(w, map[string]string{"Message": message}); err != nil {
		log.Printf("render admin login: %v", err)
	}
}

func renderAdminDashboard(w http.ResponseWriter, messages []contactMessage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminDashboardTemplate.Execute(w, map[string]any{"Messages": messages}); err != nil {
		log.Printf("render admin dashboard: %v", err)
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

var adminLoginTemplate = template.Must(template.New("admin-login").Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Admin | England Systems</title>
    <link rel="stylesheet" href="/static/styles.css" />
  </head>
  <body>
    <canvas id="edge-particles" aria-hidden="true"></canvas>
    <header class="site-header" aria-label="Primary">
      <a class="brand" href="/" aria-label="England Systems home">
        <span class="brand-mark"><img src="/static/logo.png" alt="" /></span>
        <span class="brand-name">England Systems</span>
      </a>
    </header>
    <main class="contact-page admin-page">
      <section class="contact-layout" aria-labelledby="admin-title">
        <form class="contact-form" action="/admin/login" method="post">
          <div class="form-heading">
            <p class="eyebrow">Admin</p>
            <h1 id="admin-title">Sign in</h1>
          </div>
          {{if .Message}}<p class="form-alert">{{.Message}}</p>{{end}}
          <label>
            <span>Username</span>
            <input name="username" autocomplete="username" required />
          </label>
          <label>
            <span>Password</span>
            <input name="password" type="password" autocomplete="current-password" required />
          </label>
          <button class="button primary" type="submit">Login</button>
        </form>
      </section>
    </main>
    <script src="/static/particles.js"></script>
    <script src="/static/app.js"></script>
  </body>
</html>`))

var adminDashboardTemplate = template.Must(template.New("admin-dashboard").Funcs(template.FuncMap{
	"formatTime": func(value time.Time) string {
		return value.Local().Format("Jan 2, 2006 3:04 PM")
	},
}).Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Messages | England Systems</title>
    <link rel="stylesheet" href="/static/styles.css" />
  </head>
  <body>
    <canvas id="edge-particles" aria-hidden="true"></canvas>
    <header class="site-header" aria-label="Primary">
      <a class="brand" href="/" aria-label="England Systems home">
        <span class="brand-mark"><img src="/static/logo.png" alt="" /></span>
        <span class="brand-name">England Systems</span>
      </a>
      <button class="nav-toggle" type="button" aria-controls="primary-navigation" aria-expanded="false" aria-label="Open navigation">
        <span></span>
        <span></span>
        <span></span>
      </button>
      <nav class="site-nav" id="primary-navigation" aria-label="Admin actions">
        <a href="/">Home</a>
        <form class="inline-form" action="/admin/logout" method="post">
          <button type="submit">Logout</button>
        </form>
      </nav>
    </header>
    <main class="admin-dashboard">
      <section class="admin-shell" aria-labelledby="messages-title">
        <div class="admin-heading">
          <p class="eyebrow">Admin</p>
          <h1 id="messages-title">Messages</h1>
        </div>
        {{if .Messages}}
          <div class="message-list">
            {{range .Messages}}
              <article class="message-item">
                <div class="message-meta">
                  <div>
                    <h2>{{.Name}}</h2>
                    <a href="mailto:{{.Email}}">{{.Email}}</a>
                    {{if .Phone}}<a href="tel:{{.Phone}}">{{.Phone}}</a>{{end}}
                  </div>
                  <time datetime="{{.CreatedAt.Format "2006-01-02T15:04:05Z07:00"}}">{{formatTime .CreatedAt}}</time>
                </div>
                <p>{{.Message}}</p>
                <form action="/admin/messages/delete" method="post">
                  <input type="hidden" name="id" value="{{.ID}}" />
                  <button class="button secondary danger" type="submit">Delete</button>
                </form>
              </article>
            {{end}}
          </div>
        {{else}}
          <p class="empty-state">No messages yet.</p>
        {{end}}
      </section>
    </main>
    <script src="/static/particles.js"></script>
    <script src="/static/app.js"></script>
  </body>
</html>`))
