FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /out/stockroom ./cmd/stockroom
RUN mkdir -p /runtime/data && chown 65532:65532 /runtime/data

FROM build AS test
COPY Makefile ./
CMD ["make", "test", "TEST_FLAGS=-v -count=1"]

FROM scratch
COPY --from=build /out/stockroom /stockroom
COPY --from=build --chown=65532:65532 /runtime/ /
USER 65532:65532
WORKDIR /data
ENTRYPOINT ["/stockroom"]
CMD ["--help"]
