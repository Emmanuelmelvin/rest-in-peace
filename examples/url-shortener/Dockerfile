# Production multi-stage build for the URL Shortener API.
#
# Build:
#   docker build -t rip-url-shortener .
#
# Run with Postgres:
#   docker run -p 8080:8080 \
#     -e DATABASE_URL="postgres://user:pass@host:5432/shortener?sslmode=require" \
#     -e API_KEY="your-production-secret" \
#     rip-url-shortener
#
# Or run with a persistent SQLite volume:
#   docker run -p 8080:8080 \
#     -v shortener_data:/data \
#     -e DATABASE_URL="sqlite:///data/url-shortener.db" \
#     -e API_KEY="your-production-secret" \
#     rip-url-shortener

# Build stage: BASIC → Go → one static Linux binary.
FROM golang:1.26 AS build
RUN go install github.com/Emmanuelmelvin/rest-in-peace/cmd/rip@latest
WORKDIR /src
COPY . .
RUN rip build --os linux --arch amd64 -o /out/app

# Run stage: minimal distroless image with just the static binary.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app"]
