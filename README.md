# England Systems

England Systems is a Go web application backed by SQLite. Contact-form submissions are stored in SQLite and can be viewed through the admin page at `/admin`.

## Requirements

- Go 1.25 or newer
- A writable location for the SQLite database

## Environment variables

Set the environment variables **before starting the application**. The application does not load a `.env` file automatically.

### Required to start the application

| Variable | Description | Example |
| --- | --- | --- |
| `ENGLANDSYSTEMS_DB_PATH` | Absolute path to the SQLite database file. The application exits at startup if this is missing or relative. Parent directories are created automatically, but the user running the application must be able to write to the directory. | `/var/lib/englandsystems/messages.sqlite3` |

### Required for admin access

The server can start without these variables, but nobody will be able to sign in at `/admin` unless **both** are set.

| Variable | Description | Example |
| --- | --- | --- |
| `ENGLANDSYSTEMS_ADMIN_USERNAME` | Username used to sign in to `/admin`. | `admin` |
| `ENGLANDSYSTEMS_ADMIN_PASSWORD` | Password used to sign in to `/admin`. Use a long, unique value and do not commit it to the repository. | `replace-with-a-long-random-password` |

### Optional variables

| Variable | Default | Description |
| --- | --- | --- |
| `ENGLANDSYSTEMS_SESSION_SECRET` | Derived from the admin username and password | Secret used to sign admin session cookies. Set this explicitly in production so it is independent of the login credentials. Use a long random value and keep it private. Changing it invalidates existing admin sessions. |
| `ENGLANDSYSTEMS_PORT` | `9944` | TCP port on which the HTTP server listens. Supply only the port number, such as `8080`, without a hostname or colon. |

## Local setup

Export the variables in the same shell that will start the application:

```sh
export ENGLANDSYSTEMS_DB_PATH="$PWD/data/messages.sqlite3"
export ENGLANDSYSTEMS_ADMIN_USERNAME="admin"
export ENGLANDSYSTEMS_ADMIN_PASSWORD="replace-with-a-long-random-password"
export ENGLANDSYSTEMS_SESSION_SECRET="$(openssl rand -hex 32)"
export ENGLANDSYSTEMS_PORT="9944" # Optional

go run .
```

Then open <http://localhost:9944>. The admin page is at <http://localhost:9944/admin>.

`$PWD/data/messages.sqlite3` expands to an absolute path before it is passed to the application. A value such as `./data/messages.sqlite3` will be rejected because `ENGLANDSYSTEMS_DB_PATH` must be absolute.

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
