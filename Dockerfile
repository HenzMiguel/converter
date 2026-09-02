# syntax=docker/dockerfile:1

FROM golang:1.27.1-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/converter .

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=build /out/converter ./converter

# uploads/ and output/ are created at startup by the app; owned by the non-root user.
RUN mkdir -p uploads output && chown -R app:app /app
USER app

ENV PORT=8082
EXPOSE 8082

ENTRYPOINT ["./converter"]
