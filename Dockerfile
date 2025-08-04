FROM golang:1.24-alpine

WORKDIR /app

# Установить зависимости для CGO и SQLite
RUN apk add --no-cache build-base sqlite-dev

COPY . .

# Важно: включить CGO
ENV CGO_ENABLED=1

RUN go build -o forum

EXPOSE 8080
CMD ["./forum"]
