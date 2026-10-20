FROM --platform=$BUILDPLATFORM golang:1.27.3 AS build
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Cross-compile natively on the build platform (fast) rather than relying on qemu
# to emulate the Go toolchain itself when --platform != the build host's arch.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -o /out/provider ./cmd/provider

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/provider /usr/local/bin/provider
USER nonroot:nonroot
ENTRYPOINT ["provider"]
