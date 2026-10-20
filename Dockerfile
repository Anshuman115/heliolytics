# syntax=docker/dockerfile:1

FROM golang:1.26.9-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG TARGETOS
ARG TARGETARCH
ARG GIT_SHA
ARG BUILD_TIME
# GOOS/GOARCH are set by BuildKit; an empty value falls back to the builder platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath -buildvcs=false \
    -ldflags="-s -w -X main.commit=${GIT_SHA} -X main.builtAt=${BUILD_TIME}" \
    -o /out/api ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
ARG GIT_SHA
ARG BUILD_TIME
LABEL org.opencontainers.image.title="heliolytics-api" \
      org.opencontainers.image.description="Heliolytics backend API" \
      org.opencontainers.image.source="https://github.com/Anshuman115/heliolytics" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.revision="${GIT_SHA}" \
      org.opencontainers.image.created="${BUILD_TIME}"
COPY --from=build /out/api /usr/local/bin/api
ENV ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/api", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/api"]
