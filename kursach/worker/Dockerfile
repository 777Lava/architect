FROM golang:1.23.2-alpine

WORKDIR /app

COPY worker/go.mod worker/go.sum ./
RUN go mod download

COPY . .

RUN go build -o app ./worker/cmd/main.go

CMD ["./app"]
