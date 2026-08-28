FROM golang:1.25.0 AS tools

RUN CGO_ENABLED=0 GOBIN=/usr/bin go install github.com/go-task/task/v3/cmd/task@v3.38.0

FROM golang:1.25.0 AS builder

ARG BUILD_RELEASE
ARG BUILD_BRANCH
ARG BUILD_COMMIT

# Unlike the other services, this image is built with the repository root as the
# build context: search_docs serves the product documentation, which lives in
# docs/ outside the service directory. See .dockerignore in the repository root.
WORKDIR /go/src/repo
COPY mcp-server/go.mod mcp-server/go.sum ./
COPY mcp-server/vendor/ vendor/
COPY mcp-server/internal/tools/tools.go internal/tools/

ENV GOCACHE=/root/.cache/go-build
RUN --mount=type=cache,target="/root/.cache/go-build" \
    mkdir -p bin && \
    GOBIN=/go/src/repo/bin go install github.com/google/gops

COPY --from=tools /usr/bin/task /usr/bin
COPY mcp-server/ /go/src/repo
COPY docs/ /go/src/repo/docs/

RUN task build

FROM debian:12.0-slim AS runner

LABEL org.opencontainers.image.source=https://github.com/runtime-radar/runtime-radar
LABEL org.opencontainers.image.description="MCP server exposes Runtime Radar data to AI agents over the Model Context Protocol, read-only"
LABEL org.opencontainers.image.licenses="Apache-2.0"

ARG APP_NAME=mcp-server

COPY --from=builder /go/src/repo/cmd/${APP_NAME}/${APP_NAME} /app
COPY --from=builder /go/src/repo/docs /docs
COPY --from=builder /go/src/repo/bin/gops /usr/bin/gops

ENV DOCS_DIR=/docs

EXPOSE 9000 9090
ENTRYPOINT ["/app"]
