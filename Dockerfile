FROM linuxkit/alpine:86cd4f51b49fb9a078b50201d892a3c7973d48ec AS mirror

RUN mkdir -p /out/etc/apk && cp -r /etc/apk/* /out/etc/apk/
RUN apk add --no-cache --initdb -p /out \
    alpine-baselayout

RUN rm -rf /out/etc/apk /out/lib/apk /out/var/cache

FROM golang:1.13-alpine AS build

# TODO: build go

FROM scratch
ENTRYPOINT []
CMD []
WORKDIR /
COPY --from=mirror /out/ /
COPY --from=build /go/bin/linuxkit-pkg-image usr/bin/linuxkit-pkg-image

# TODO: needs flags so use dumb-init or something
CMD ["/usr/bin/linuxkit-pkg-image"]