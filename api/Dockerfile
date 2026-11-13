FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY config ./config
COPY model ./model
COPY upstream ./upstream
COPY dashboard ./dashboard
COPY notifications ./notifications
COPY storage ./storage
COPY server ./server
RUN CGO_ENABLED=0 go build -o /reqarr .
FROM alpine:3.22
COPY --from=builder /reqarr /reqarr
RUN mkdir -p /data && chown 65532:65532 /data
USER 65532:65532
ENV HTTP_ADDR=:8080
ENV NOTIFICATION_DB=/data/reqarr.db
EXPOSE 8080
ENTRYPOINT ["/reqarr"]
