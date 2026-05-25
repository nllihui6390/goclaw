# Stage 1: Builder
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /go-claw .

# Stage 2: Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /go-claw .
COPY config.json .

EXPOSE 8080 8081

VOLUME ["/app/data.json"]

ENTRYPOINT ["/go-claw"]
