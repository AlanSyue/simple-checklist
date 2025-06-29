FROM golang:1.21-bullseye

WORKDIR /app
COPY . .

ENV CGO_ENABLED=1
RUN apt-get update && apt-get install -y sqlite3 gcc libc6-dev && rm -rf /var/lib/apt/lists/*
RUN go mod tidy
RUN go build -o /app/main .

RUN mkdir -p /app/data && chown -R 1000:1000 /app && chmod +x /app/main

USER 1000:1000

CMD ["/app/main"]
