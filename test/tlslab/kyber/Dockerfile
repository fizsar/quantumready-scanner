# Go 1.23 was the last release with the pre-standard X25519Kyber768Draft00
# (0x6399); Go 1.24 replaced it with X25519MLKEM768. Test lab only.
FROM golang:1.23.12@sha256:60deed95d3888cc5e4d9ff8a10c54e5edc008c6ae3fba6187be6fb592e19e8c0 AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -o /kyber .

FROM scratch
COPY --from=build /kyber /kyber
USER 65534:65534
ENTRYPOINT ["/kyber"]
