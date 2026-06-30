# England Systems

England Systems is a Go web application backed by SQLite. Contact-form submissions are stored in SQLite and can be viewed through the admin page at `/admin`.

## Requirements

- Go 1.25 or newer
- A writable location for the SQLite database
- The four application environment variables listed below
- An initialized SQLite database supplied with `--db`

## Environment variables

Set environment variables **before starting the application**. The application does not load a `.env` file automatically.

The server validates every variable before opening the database or listening on a port. It exits immediately and reports every missing variable if any value is unset or blank.

| Variable | Description | Example |
| --- | --- | --- |
| `ENGLANDSYSTEMS_PORT` | TCP port from 1 through 65535. Supply only the number, without a hostname or colon. | `9944` |
| `ENGLANDSYSTEMS_ADMIN_USERNAME` | Username used to sign in to `/admin`. | `admin` |
| `ENGLANDSYSTEMS_ADMIN_PASSWORD` | Admin password. Use a long, unique value and do not commit it. | `replace-with-a-long-random-password` |
| `ENGLANDSYSTEMS_SESSION_SECRET` | Independent secret used to sign admin session cookies. Changing it invalidates existing sessions. | Generate with `openssl rand -hex 32` |

## Local setup

Export the variables in the same shell that will start the application:

```sh
export ENGLANDSYSTEMS_ADMIN_USERNAME="admin"
export ENGLANDSYSTEMS_ADMIN_PASSWORD="replace-with-a-long-random-password"
export ENGLANDSYSTEMS_SESSION_SECRET="$(openssl rand -hex 32)"
export ENGLANDSYSTEMS_PORT="9944"

go run . db
go run . --db ./data/englandsystems.sqlite3.db
```

Then open <http://localhost:9944>. The admin page is at <http://localhost:9944/admin>.

`englandsystems db` creates `./data/englandsystems.sqlite3.db` and all required tables. To initialize another location, pass it to the command:

```sh
go run . db /var/lib/englandsystems/production.sqlite3.db
```

Starting the server never creates or migrates a database. The `--db` path is required on every launch and may be relative or absolute. Startup fails if the file does not exist or lacks required tables.

If values are kept in a local `.env` file, load that file into the shell explicitly before starting the server, for example with `set -a; source .env; set +a`. Keep `.env` out of version control because it contains credentials and secrets.

## Build and run

```sh
go build -o englandsystems .
./englandsystems db
./englandsystems --db ./data/englandsystems.sqlite3.db
```

The same environment variables and explicit `--db` argument must be available to the compiled process. For a service manager or hosting platform, configure them in that service rather than only in an interactive shell.

To run the tests:

```sh
go test ./...
```
