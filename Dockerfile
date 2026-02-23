FROM golang:1.23-alpine AS builder

WORKDIR /workspace

# Cache dependencies.
COPY go.mod go.sum ./
RUN go mod download

# Copy source.
COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/

# Build.
RUN CGO_ENABLED=0 GOOS=linux go build -a -o manager ./cmd/manager/

# Runtime image.
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

EXPOSE 8080 8081 8402
ENTRYPOINT ["/manager"]
