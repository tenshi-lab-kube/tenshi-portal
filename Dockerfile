FROM golang:1.23-bookworm AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/tenshi-portal ./cmd/portal

FROM gcr.io/distroless/static-debian12:nonroot

ENV PORT=8080
COPY --from=build /out/tenshi-portal /tenshi-portal
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/tenshi-portal"]
