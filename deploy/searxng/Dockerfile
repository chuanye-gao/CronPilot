FROM ghcr.io/searxng/searxng:2026.8.20-8d3dd0cd4@sha256:e7bb47bebf338c52c55c7bed92293873cfe757b554b261829c0d710fd8307fa3

COPY deploy/searxng/settings.cloud.yml /etc/searxng/settings.yml

EXPOSE 8080

HEALTHCHECK --interval=20s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/ || exit 1
