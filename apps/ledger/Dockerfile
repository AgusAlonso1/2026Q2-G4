FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
      -ldflags "-s -w" -o /out/archivador ./cmd/archivador

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /out/archivador /
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
