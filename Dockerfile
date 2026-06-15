FROM golang:1.26 AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /url-shortener ./cmd/api

FROM debian:bookworm-slim

RUN useradd --system --create-home appuser
WORKDIR /app
COPY --from=build /url-shortener /usr/local/bin/url-shortener

ENV ADDR=:8080
ENV BASE_URL=http://localhost:8080
ENV DATABASE_PATH=/app/data/shortener.db

RUN mkdir -p /app/data && chown -R appuser:appuser /app
USER appuser

EXPOSE 8080
CMD ["url-shortener"]
