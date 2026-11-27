# The playground host needs the Go toolchain at runtime, because it builds
# every submitted program. Build from the repo root:
#   docker build -f deploy/playground.Dockerfile -t rip-playground .

FROM node:20-alpine AS web
WORKDIR /web
COPY playground/web/package*.json ./
RUN npm install
COPY playground/web ./
RUN npm run build

FROM golang:1.26 AS host
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/playground ./cmd/playground

FROM golang:1.26
# The host container holds no secrets (security.md). Programs run as separate
# processes with a 128 MB cap; see internal/playground/sandbox.go.
RUN useradd --create-home rip
ENV GOPATH=/home/rip/go GOCACHE=/home/rip/.cache/go-build
COPY --from=host /out/playground /usr/local/bin/playground
COPY --from=web /web/dist /srv/web
USER rip
RUN playground -warm -data /tmp/rip-playground && rm -rf /tmp/rip-playground
EXPOSE 8090
ENTRYPOINT ["playground", "-addr", ":8090", "-web", "/srv/web", "-data", "/tmp/rip-playground"]
