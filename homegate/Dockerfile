FROM golang:1-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /agent .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates jq
COPY --from=builder /agent /usr/local/bin/agent
COPY run.sh /usr/local/bin/run.sh
RUN chmod +x /usr/local/bin/run.sh

EXPOSE 8080
ENTRYPOINT ["run.sh"]
