FROM golang:1.21-alpine
WORKDIR /app
COPY . .
RUN go build -o forum
EXPOSE 8080
CMD ["./forum"]
