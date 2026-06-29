# England Systems

England Systems is a Go web application backed by SQLite. Contact-form submissions are stored in SQLite and can be viewed through the admin page at `/admin`.

## Requirements

- Go 1.25 or newer
- A writable location for the SQLite database
- All five application environment variables listed below

## Environment variables

Set environment variables **before starting the application**. The application does not load a `.env` file automatically.

The server validates every variable before opening the database or listening on a port. It exits immediately and reports every missing variable if any value is unset or blank.

| Variable | Description | Example |
| --- | --- | --- |
| `ENGLANDSYSTEMS_DB_PATH` | Absolute path to the SQLite database file. Parent directories and the database file are created automatically. | `/var/lib/englandsystems/messages.sqlite3` |
| `ENGLANDSYSTEMS_PORT` | TCP port from 1 through 65535. Supply only the number, without a hostname or colon. | `9944` |
| `ENGLANDSYSTEMS_ADMIN_USERNAME` | Username used to sign in to `/admin`. | `admin` |
| `ENGLANDSYSTEMS_ADMIN_PASSWORD` | Admin password. Use a long, unique value and do not commit it. | `replace-with-a-long-random-password` |
| `ENGLANDSYSTEMS_SESSION_SECRET` | Independent secret used to sign admin session cookies. Changing it invalidates existing sessions. | Generate with `openssl rand -hex 32` |

## Local setup

Export the variables in the same shell that will start the application:

```sh
export ENGLANDSYSTEMS_DB_PATH="$PWD/data/messages.sqlite3"
export ENGLANDSYSTEMS_ADMIN_USERNAME="admin"
export ENGLANDSYSTEMS_ADMIN_PASSWORD="replace-with-a-long-random-password"
export ENGLANDSYSTEMS_SESSION_SECRET="$(openssl rand -hex 32)"
export ENGLANDSYSTEMS_PORT="9944"

go run .
```

Then open <http://localhost:9944>. The admin page is at <http://localhost:9944/admin>.

`$PWD/data/messages.sqlite3` expands to an absolute path before it is passed to the application. A value such as `./data/messages.sqlite3` is rejected because `ENGLANDSYSTEMS_DB_PATH` must be absolute.

If values are kept in a local `.env` file, load that file into the shell explicitly before starting the server, for example with `set -a; source .env; set +a`. Keep `.env` out of version control because it contains credentials and secrets.

## Build and run

```sh
go build -o englandsystems .
./englandsystems
```

The same environment variables must be available to the compiled process. For a service manager or hosting platform, configure them in that service's environment rather than only in an interactive shell.

To verify the resolved database path without starting the server:

```sh
go run . db-path
```

To run the tests:

```sh
go test ./...
```
