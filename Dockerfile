FROM node:22-alpine AS client
WORKDIR /build/client
COPY client/package.json client/package-lock.json ./
RUN npm ci
COPY client/index.html client/tsconfig*.json client/vite.config.ts ./
COPY client/src ./src
RUN npm run build

FROM golang:1.27-alpine AS api
WORKDIR /build/api
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/main.go ./
COPY api/config ./config
COPY api/model ./model
COPY api/upstream ./upstream
COPY api/dashboard ./dashboard
COPY api/notifications ./notifications
COPY api/storage ./storage
COPY api/server ./server
RUN CGO_ENABLED=0 go build -trimpath -o /reqarr .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && mkdir -p /data && chown 65532:65532 /data
WORKDIR /app
COPY --from=api /reqarr /reqarr
COPY --from=client /build/client/dist /app/client
USER 65532:65532
ENV HTTP_ADDR=:8080
ENV STATIC_DIR=/app/client
ENV NOTIFICATION_DB=/data/reqarr.db
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/reqarr"]
