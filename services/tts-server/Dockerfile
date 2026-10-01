# syntax=docker/dockerfile:1
#
# tts-server: Go API + espeak-ng + lame + Piper (rhasspy/piper standalone build).
#
#   docker build -t tts-server .
#   docker build -t tts-server --build-arg WITH_VOICES=en_US-lessac-medium,pl_PL-gosia-medium .
#
# Every base image is pinned by digest; Piper and baked voices are verified
# against SHA-256 sums kept in this directory.

ARG GO_IMAGE=golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414
ARG RUNTIME_IMAGE=debian:trixie-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a

# ---------------------------------------------------------------- build
FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/tts-server .

# ---------------------------------------------------------------- fetch: piper + optional voices
FROM ${RUNTIME_IMAGE} AS fetch
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl \
 && rm -rf /var/lib/apt/lists/*
# rhasspy/piper 2023.11.14-2 is the last standalone (no Python) release; the
# repository is archived and development moved to OHF-Voice/piper1-gpl, which
# ships Python wheels only. See README.md, "Engines".
ARG PIPER_VERSION=2023.11.14-2
ARG PIPER_SHA256_AMD64=a50cb45f355b7af1f6d758c1b360717877ba0a398cc8cbe6d2a7a3a26e225992
ARG PIPER_SHA256_ARM64=fea0fd2d87c54dbc7078d0f878289f404bd4d6eea6e7444a77835d1537ab88eb
ARG TARGETARCH
RUN set -eu; \
    case "${TARGETARCH:-amd64}" in \
      amd64) arch=x86_64;  sum="${PIPER_SHA256_AMD64}" ;; \
      arm64) arch=aarch64; sum="${PIPER_SHA256_ARM64}" ;; \
      *) echo "unsupported arch ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL --retry 3 -o /tmp/piper.tgz \
      "https://github.com/rhasspy/piper/releases/download/${PIPER_VERSION}/piper_linux_${arch}.tar.gz"; \
    echo "${sum}  /tmp/piper.tgz" | sha256sum -c -; \
    tar -xzf /tmp/piper.tgz -C /opt; \
    rm /tmp/piper.tgz

ARG WITH_VOICES=""
ARG PIPER_VOICES_REV=c10ece1aade47bb51c153c893d14e5bf8e5b7117
COPY voices.sha256 /tmp/voices.sha256
RUN set -eu; mkdir -p /voices; \
    for id in $(echo "${WITH_VOICES}" | tr ',' ' '); do \
      lines=$(grep -E "/${id}\.onnx(\.json)?$" /tmp/voices.sha256 || true); \
      [ "$(echo "$lines" | grep -c .)" -eq 2 ] || { echo "voice ${id} is not listed in voices.sha256" >&2; exit 1; }; \
      echo "$lines" | while read -r sum path; do \
        file="/voices/$(basename "$path")"; \
        curl -fsSL --retry 3 -o "$file" \
          "https://huggingface.co/rhasspy/piper-voices/resolve/${PIPER_VOICES_REV}/${path}"; \
        echo "${sum}  ${file}" | sha256sum -c -; \
      done; \
    done

# ---------------------------------------------------------------- runtime
FROM ${RUNTIME_IMAGE}
RUN apt-get update \
 && apt-get install -y --no-install-recommends espeak-ng lame \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --uid 10001 --home-dir /nonexistent --shell /usr/sbin/nologin tts \
 && mkdir -p /voices /cache \
 && chown tts:tts /voices /cache
COPY --from=fetch /opt/piper /opt/piper
COPY --from=fetch --chown=tts:tts /voices /voices
COPY --from=build /out/tts-server /usr/local/bin/tts-server

ENV TTS_ADDR=:8080 \
    TTS_VOICES_DIR=/voices \
    TTS_CACHE_DIR=/cache \
    TTS_PIPER_BIN=/opt/piper/piper \
    TTS_ESPEAK_BIN=espeak-ng \
    TTS_LAME_BIN=lame

USER tts
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/tts-server", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/tts-server"]
