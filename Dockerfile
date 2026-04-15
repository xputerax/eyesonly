# ========== Stage 1: Build CSS ==========
FROM node:22-trixie-slim AS css

WORKDIR /app

# Cache npm dependencies
COPY package.json package-lock.json ./
RUN npm ci

# Copy source files needed by Vite/TailwindCSS
COPY vite.config.ts tailwind.config.js ./
COPY src/ src/
COPY views/ views/

RUN npm run build

# ========== Stage 2: Compile ==========
FROM golang:1.26.2-alpine AS compile

# CGO dependencies (required by go-sqlite3)
RUN apk add --no-cache gcc musl-dev

# Install templ and sqlc CLI tools
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001 \
 && go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

WORKDIR /app

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Generate templ and sqlc code, then build
RUN templ generate \
 && sqlc generate \
 && CGO_ENABLED=1 go build -o server ./cmd/server/server.go

# ========== Stage 3: Run ==========
FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=compile /app/server .
COPY --from=css /app/static/ static/

EXPOSE 6969

CMD ["./server"]
