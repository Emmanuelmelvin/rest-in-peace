# Standalone build from examples/invoicing/:
#   docker build -t rip-invoicing .
# For local repo builds with Postgres, see deploy/invoicing/docker-compose.yml.

# Build stage: BASIC → Go → one static binary.
FROM golang:1.26 AS build
RUN go install github.com/Emmanuelmelvin/rest-in-peace/cmd/rip@latest
WORKDIR /src
COPY . .
RUN rip build --os linux --arch amd64 -o /out/app

# Run stage: just the binary.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/app /app
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app"]
