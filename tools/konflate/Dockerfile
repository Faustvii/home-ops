# syntax=docker/dockerfile:1

# The konflate command for kritika's runner pods, mounted through the chart's
# runner.tools as an image volume: the runner execs /bin/konflate from its own
# rootfs (distroless, CA roots included), so the image only has to carry a
# static binary.
FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.27.2 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY main.go ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w" -o /out/konflate .

FROM scratch
COPY --from=build /out/konflate /bin/konflate
ENTRYPOINT ["/bin/konflate"]
