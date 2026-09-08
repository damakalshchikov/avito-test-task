FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kitchen ./cmd/kitchen

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 app
USER app
COPY --from=build /out/kitchen /usr/local/bin/kitchen
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --retries=10 CMD wget -qO- http://localhost:8080/health || exit 1
ENTRYPOINT ["kitchen"]
