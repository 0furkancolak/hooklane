# One image for every Hooklane process:
#   hooklane-api (default), hooklane-worker, hooklane-admin
# The API also serves the dashboard from /app/web.

FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build

FROM golang:1.27.2-alpine AS go
WORKDIR /src/api
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
ARG VERSION=dev
RUN for app in api worker admin; do \
      CGO_ENABLED=0 GOOS=linux go build -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/hooklane-${app} ./cmd/${app}; \
    done

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S hooklane && adduser -S -G hooklane hooklane
COPY --from=go /out/ /usr/local/bin/
COPY --from=web /src/web/dist /app/web
ENV WEB_DIR=/app/web PORT=8080
USER hooklane
EXPOSE 8080
CMD ["hooklane-api"]
