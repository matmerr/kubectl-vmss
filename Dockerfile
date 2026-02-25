FROM --platform=$BUILDPLATFORM golang:1.24 AS builder

ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG GIT_COMMIT=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags="-s -w \
    -X github.com/matmerr/kubectl-vmss/pkg/version.Version=${VERSION} \
    -X github.com/matmerr/kubectl-vmss/pkg/version.GitCommit=${GIT_COMMIT} \
    -X github.com/matmerr/kubectl-vmss/pkg/version.BuildDate=$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
    -o /kubectl-vmss ./cmd/kubectl-vmss

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /kubectl-vmss /usr/local/bin/kubectl-vmss
ENTRYPOINT ["kubectl-vmss"]
