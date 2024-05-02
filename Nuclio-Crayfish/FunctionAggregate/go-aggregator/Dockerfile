FROM golang:1.21-alpine as builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download


COPY . .


RUN CGO_ENABLED=0 GOOS=linux go build -v -o myapp ./func-aggregate.go

FROM scratch

WORKDIR /

COPY --from=builder /app/myapp .

CMD ["./myapp"]