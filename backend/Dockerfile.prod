# --- Build stage ---
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/server ./...

# --- Runtime stage ---
FROM alpine:3.20 AS runtime

# Non-root user for the running process
RUN adduser -D -u 10001 appuser

WORKDIR /app
COPY --from=build /out/server /app/server

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/server"]
