FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/garmin-login ./cmd/garmin-login

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /app/server /app/server
COPY --from=builder /app/garmin-login /app/garmin-login

EXPOSE 3000

ENTRYPOINT ["/app/server"]
