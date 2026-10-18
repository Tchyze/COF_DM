# COF DM en mode serveur (sans fenêtre, accès par comptes), pour Docker.
# Même binaire et mêmes fichiers d'univers que l'application de bureau.
FROM golang:1.26.9-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=docker
# La compilation de SQLite (code C) prend près d'une minute sans rien afficher. Le cache de
# compilation est gardé d'une construction à l'autre : les suivantes ne recompilent que le code Go
# modifié.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -trimpath -tags "sqlite_fts5 nogui serveur" -ldflags "-s -w -X main.version=${VERSION}" -o /out/cof-dm .

FROM debian:bookworm-slim
RUN useradd --system --uid 1000 --home-dir /data cof && mkdir -p /data && chown cof /data
COPY --from=build /out/cof-dm /usr/local/bin/cof-dm
USER cof
VOLUME /data
EXPOSE 8100
ENTRYPOINT ["cof-dm"]
CMD ["-serveur", "-port", "8100", "-data", "/data"]
