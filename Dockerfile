FROM golang:1.26.9-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=build /api /usr/local/bin/api
COPY schema.sql /schema.sql
EXPOSE 8080
CMD ["api"]
