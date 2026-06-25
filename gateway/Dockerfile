FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /sentinel ./cmd/sentinel

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /sentinel /usr/local/bin/sentinel
EXPOSE 8080
CMD ["sentinel"]
