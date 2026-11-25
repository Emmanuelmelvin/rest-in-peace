# Build stage: BASIC → Go → one static binary.
FROM golang:1.24 AS build
RUN go install github.com/rip-basic/rip/cmd/rip@latest
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
