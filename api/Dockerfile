FROM golang:1.27.2-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG APP=api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${APP}

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/app /app/app
COPY --from=build /src/migrations /app/migrations
USER app
EXPOSE 8080
ENTRYPOINT ["/app/app"]
