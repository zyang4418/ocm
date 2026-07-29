FROM golang:1.26-alpine

WORKDIR /app

# Hot-reload during development
RUN go install github.com/air-verse/air@latest

# Cache module downloads (no external deps yet, but ready for future use)
COPY go.mod go.sum* ./
RUN go mod download

# Source is mounted in via docker-compose for live reload
COPY . .

EXPOSE 8080

CMD ["air"]
