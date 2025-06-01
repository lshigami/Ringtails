FROM golang:1.24-alpine

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN swag init -g cmd/main.go --output ./docs

RUN go build -o main ./cmd
EXPOSE 8080
CMD ["./main"]