FROM golang:1.24-bullseye

WORKDIR /app

ENV CGO_ENABLED=1
RUN apt-get update && apt-get install -y sqlite3 gcc libc6-dev && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod tidy

COPY . .
RUN go build -o /app/main .

RUN mkdir -p /app/data && chown -R 1000:1000 /app && chmod +x /app/main

USER 1000:1000

CMD ["/app/main"]
