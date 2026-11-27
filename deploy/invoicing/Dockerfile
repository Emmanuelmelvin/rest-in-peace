# Builds the Invoicing API from the local repository checkout using `rip build`.
# Intended to be built from the repository root (see docker-compose.yml):
#   docker build -f deploy/invoicing/Dockerfile -t rip-invoicing .

FROM golang:1.26 AS build
WORKDIR /repo
COPY . .
RUN go build -o /usr/local/bin/rip ./cmd/rip
RUN rip build --os linux --arch amd64 -o /out/invoicing examples/invoicing/main.bas

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/invoicing /app
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app"]
