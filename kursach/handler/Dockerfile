FROM golang:1.23.2-alpine

WORKDIR /app

# Копируем go.mod и go.sum из handler/
COPY handler/go.mod handler/go.sum ./

RUN go mod download

# Копируем весь код из handler/
COPY handler/ ./

# Собираем main.go из cmd/
RUN go build -o app ./cmd/main.go

CMD ["./app"]
