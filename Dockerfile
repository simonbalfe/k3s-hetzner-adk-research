FROM golang:1.26.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM node:22-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates bubblewrap && rm -rf /var/lib/apt/lists/*
RUN npm install --global @openai/codex@0.158.0 && npm cache clean --force
COPY --from=build /out/api /app/api
COPY --from=build /out/worker /app/worker
USER 1000:1000
