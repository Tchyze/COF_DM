# COF DM en mode serveur (sans fenêtre, accès par comptes), pour Docker.
# Même binaire et mêmes fichiers d'univers que l'application de bureau.
FROM golang:1.26.9-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# La version vient de git (la dernière version publiée, « 1.2.3 », et « +4.1a2b3c4 » quand le dépôt
# a quelques commits de plus) : le serveur la montre à son administrateur et aux postes reliés, et
# la compare aux versions publiées. --build-arg VERSION=… la remplace.
ARG VERSION=
# La compilation de SQLite (code C) prend près d'une minute sans rien afficher. Le cache de
# compilation est gardé d'une construction à l'autre : les suivantes ne recompilent que le code Go
# modifié.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    v="${VERSION:-$(git describe --tags --match 'v[0-9]*' --exclude '*-*' 2>/dev/null | sed -E 's/^v//; s/-([0-9]+)-g([0-9a-f]+)$/+\1.\2/')}"; \
    CGO_ENABLED=1 go build -trimpath -buildvcs=false -tags "sqlite_fts5 nogui serveur" -ldflags "-s -w -X main.version=${v:-docker}" -o /out/cof-dm .

FROM debian:bookworm-slim
RUN useradd --system --uid 1000 --home-dir /data cof && mkdir -p /data && chown cof /data
COPY --from=build /out/cof-dm /usr/local/bin/cof-dm
USER cof
VOLUME /data
EXPOSE 8100
ENTRYPOINT ["cof-dm"]
CMD ["-serveur", "-port", "8100", "-data", "/data"]
