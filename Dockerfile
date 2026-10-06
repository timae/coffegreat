# syntax=docker/dockerfile:1

# ---- build: one static binary, templates/CSS/JS/migrations embedded ----
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/coffee-grate .

# ---- run: distroless, non-root, ~12 MB image ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/coffee-grate /coffee-grate

ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/coffee-grate"]
