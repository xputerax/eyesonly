TEMPL ?= $(HOME)/go/bin/templ
SQLC ?= $(HOME)/go/bin/sqlc

.PHONY: build

build:
	$(TEMPL) generate
	$(SQLC) generate
	npm run build
	go build -o server ./cmd/server/server.go