# Goreleaser builds the static md2confluence binary and drops it in the
# build context, then invokes `docker build`. We just copy it into a
# distroless base - no RUN steps, so this Dockerfile builds for any
# target arch on a native host without QEMU.
FROM gcr.io/distroless/static-debian12:nonroot

COPY md2confluence /usr/local/bin/md2confluence

USER nonroot:nonroot
WORKDIR /work

ENTRYPOINT ["/usr/local/bin/md2confluence"]
