FROM golang:1.24-alpine

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN swag init -g ./cmd/main.go --output ./docs --parseDependency --parseInternal


RUN go build -o /app/main ./cmd 

EXPOSE 8080 

CMD ["/app/main"]