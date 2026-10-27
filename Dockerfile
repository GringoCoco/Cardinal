# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/relayd ./cmd/relayd

FROM alpine:3.22
RUN adduser -D -H cardinal
COPY --from=build /out/relayd /usr/local/bin/relayd
USER cardinal
ENV CARDINAL_RELAY_LISTEN=":8080"
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/relayd"]
