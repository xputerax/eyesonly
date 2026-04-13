# EyesOnly

## Development

1. Install dependencies

```
$ go get
$ go install github.com/a-h/templ/cmd/templ@latest # v0.2.793 at the time of writing
$ go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest # v1.27.0 at the time of writing
```

2. Create sqlite database and generate the structure

```
$ touch eyesonly.sqlite3
$ sqlite3 eyesonly.sqlite3

sqlite> .read schema.sql
```

3. Generate `sqlc` code

```
$ sqlc generate
```

4. Generate `templ` code

```
$ templ generate
```

5. Run the application

```
$ go run cmd/server/server.go

2024/11/18 05:19:11 INFO starting server at :6969
```