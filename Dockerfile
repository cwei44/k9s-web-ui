FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY web ./web
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags '-s -w' -o /out/k9s-web-ui .

FROM scratch
COPY --from=build /out/k9s-web-ui /k9s-web-ui
COPY LICENSE NOTICE /licenses/
COPY licenses /licenses/dependencies
ENTRYPOINT ["/k9s-web-ui"]
CMD ["serve", "--host-bridge", "/host/bridge.sock", "--listen", "0.0.0.0:80", "--password-file", "/state/dashboard-password"]
