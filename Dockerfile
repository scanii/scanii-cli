FROM alpine:3

LABEL org.opencontainers.image.source="https://github.com/scanii/scanii-cli"
LABEL org.opencontainers.image.description="Scanii mock server for local development and SDK testing"
LABEL org.opencontainers.image.licenses="Apache-2.0"

RUN apk add --no-cache ca-certificates

COPY sc /sc

EXPOSE 4000

HEALTHCHECK --interval=5s --timeout=3s --start-period=2s --retries=3 \
  CMD wget -q --spider http://localhost:4000/healthcheck || exit 1

ENTRYPOINT ["/sc"]
CMD ["server"]