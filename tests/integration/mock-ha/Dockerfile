FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY main.go ./
RUN go build -o /mock-ha main.go

FROM alpine:3.21
COPY --from=builder /mock-ha /usr/local/bin/mock-ha
EXPOSE 8123
ENTRYPOINT ["/usr/local/bin/mock-ha"]
