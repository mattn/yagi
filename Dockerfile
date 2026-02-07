# syntax=docker/dockerfile:1.4

FROM golang:1.25-alpine AS build-dev
WORKDIR /go/src/app
COPY --link go.* ./
RUN apk add --no-cache upx || \
    go version && \
    go mod download
COPY --link . .
RUN CGO_ENABLED=0 go install -buildvcs=false -trimpath -ldflags '-w -s'
RUN [ -e /usr/bin/upx ] && upx /go/bin/yagi || echo
FROM build-dev AS profiles
RUN apk add --no-cache git
RUN git clone https://github.com/yagi-agent/yagi-profiles.git /yagi-profiles

FROM scratch
COPY --link --from=build-dev /go/bin/yagi /go/bin/yagi
COPY --from=build-dev /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=profiles /yagi-profiles /root/.config/yagi
COPY --from=build-dev /etc/passwd /etc/passwd
ENTRYPOINT ["/go/bin/yagi"]
