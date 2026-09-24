FROM golang:1.26-trixie AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/pal-gphoto ./cmd/gphoto \
	&& mkdir -p /out/data/media

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /out/pal-gphoto /usr/local/bin/pal-gphoto
COPY --from=build --chown=65532:65532 /out/data /data

USER nonroot:nonroot
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/pal-gphoto"]
