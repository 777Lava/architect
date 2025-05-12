# syntax=docker/dockerfile:1
FROM golang:1.21 AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./
RUN go build -o main .

FROM debian:bookworm-slim
WORKDIR /app

COPY --from=builder /app/main ./

EXPOSE 8080

CMD ["./main"]
