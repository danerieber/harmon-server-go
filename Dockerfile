FROM golang:1.21.9
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /server

EXPOSE 8080
CMD ["/server"]
