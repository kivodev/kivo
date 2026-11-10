# Kivo: the server, its web client, and jellyfin-ffmpeg (GPL, shipped as a
# separate binary; its source is published with each release).
# Build from the repository root, since the web client reads shared/:
# docker build -f server/Dockerfile --build-arg COMMIT=$(git rev-parse --short=8 HEAD) -t kivo .
# That's a dev build: the patch after the latest release, with its commit,
# such as 0.3.1-dev+56c0018d, or 0.3.1-dev without COMMIT. A release build
# is given its version instead, with --build-arg VERSION=0.4.0.

FROM node:24.21.0-trixie-slim AS web
WORKDIR /src/server/web
COPY server/web/package.json server/web/package-lock.json ./
RUN npm ci
COPY server/web/ ./
COPY shared/ /src/shared/
COPY server/internal/api/openapi.yaml /src/server/internal/api/openapi.yaml
RUN npm run build

FROM golang:1.26-trixie AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
COPY --from=web /src/server/web/dist ./web/dist
ARG VERSION=
ARG COMMIT=
RUN CGO_ENABLED=0 go build -trimpath -tags webui \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/kivo ./cmd/kivo
# The notices for what's in the image: jellyfin-ffmpeg, the Go modules and
# CLDR (make notices).
COPY shared/credits/ /shared/credits/
RUN make notices

FROM debian:trixie-slim
ARG TARGETARCH=amd64
# Intel's OpenCL runtimes, which tone-map HDR on Intel GPUs: the current one
# for Gen12 and later (Tiger Lake on, Alder Lake-N, Arc, Core Ultra), the
# legacy one for Gen8 to Gen11 (Broadwell to Ice Lake). Trixie doesn't package
# them, so they're installed from Intel's releases.
# https://github.com/intel/compute-runtime/releases
ARG IGC2_VER=2.41.5
ARG IGC2_BUILD=22716
ARG IGC1_LEGACY_VER=1.0.17537.24
ARG NEO_LEGACY_VER=24.35.30872.36
ARG NEO_VER=26.35.39758.10
ARG GMMLIB_VER=22.10.0
RUN set -eu; \
    [ "${TARGETARCH}" = amd64 ] || exit 0; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl; \
    mkdir /tmp/intel-opencl; \
    cd /tmp/intel-opencl; \
    igc="https://github.com/intel/intel-graphics-compiler/releases/download"; \
    neo="https://github.com/intel/compute-runtime/releases/download"; \
    curl -fsSL --remote-name-all \
      "${igc}/v${IGC2_VER}/intel-igc-core-2_${IGC2_VER}+${IGC2_BUILD}_amd64.deb" \
      "${igc}/v${IGC2_VER}/intel-igc-opencl-2_${IGC2_VER}+${IGC2_BUILD}_amd64.deb" \
      "${neo}/${NEO_VER}/intel-opencl-icd_${NEO_VER}-0_amd64.deb" \
      "${igc}/igc-${IGC1_LEGACY_VER}/intel-igc-core_${IGC1_LEGACY_VER}_amd64.deb" \
      "${igc}/igc-${IGC1_LEGACY_VER}/intel-igc-opencl_${IGC1_LEGACY_VER}_amd64.deb" \
      "${neo}/${NEO_LEGACY_VER}/intel-opencl-icd-legacy1_${NEO_LEGACY_VER}_amd64.deb" \
      "${neo}/${NEO_VER}/libigdgmm12_${GMMLIB_VER}_amd64.deb"; \
    printf '%s  %s\n' \
      0a6e64a663ae65a0fa02d6912ae3b6b37cf85b90c21cc423fd9fef70aaf4f628 "intel-igc-core-2_${IGC2_VER}+${IGC2_BUILD}_amd64.deb" \
      779e1b9e88098eb25711e9a8f67c2752665bad22f134aa40ed5649f6e1b87058 "intel-igc-opencl-2_${IGC2_VER}+${IGC2_BUILD}_amd64.deb" \
      61712caaddeba3d38e4f79e2a0fb23fea25596ca2d72c3144c6eea2331ec4301 "intel-opencl-icd_${NEO_VER}-0_amd64.deb" \
      c1e1ecdfe2064c047c552651cfdcdafc504f2033afafba65654338b880048b67 "intel-igc-core_${IGC1_LEGACY_VER}_amd64.deb" \
      dd016400f87fa2b6a9fa9fbcca7eb4a2629174a29de679709f9bec5cede88b0e "intel-igc-opencl_${IGC1_LEGACY_VER}_amd64.deb" \
      bbe71e4f414259e06a10cde72c29a2bd78d41b2bb2f6f8463b1806797fe66e85 "intel-opencl-icd-legacy1_${NEO_LEGACY_VER}_amd64.deb" \
      6031a63d6e8a12ce61c14efc15f2c8e727061286e3820b8594e6d00615e04d54 "libigdgmm12_${GMMLIB_VER}_amd64.deb" \
      | sha256sum -c -; \
    apt-get install -y --no-install-recommends ./*.deb; \
    apt-get purge -y --auto-remove curl; \
    cd /; \
    rm -rf /tmp/intel-opencl /var/lib/apt/lists/*
# packaging/windows/build.ps1 bundles this version too, and pins its
# Windows zip's hash, which needs updating with it.
ARG JELLYFIN_FFMPEG=8.1.3-1
ARG JELLYFIN_FFMPEG_SHA256_amd64=fbef9f81a53e175194e3f67832618a86111f7bd37197df21a5d748c34289f9c0
ARG JELLYFIN_FFMPEG_SHA256_arm64=4af743bd776eda082d40c851a4201e51a77a258145dd6ebe164dade39b35b2d0
ADD https://github.com/jellyfin/jellyfin-ffmpeg/releases/download/v${JELLYFIN_FFMPEG}/jellyfin-ffmpeg8_${JELLYFIN_FFMPEG}-trixie_${TARGETARCH}.deb /tmp/jellyfin-ffmpeg.deb
RUN set -eu; \
    sum="$(eval echo "\$JELLYFIN_FFMPEG_SHA256_${TARGETARCH}")"; \
    echo "${sum}  /tmp/jellyfin-ffmpeg.deb" | sha256sum -c -; \
    apt-get update; \
    apt-get install -y --no-install-recommends /tmp/jellyfin-ffmpeg.deb ca-certificates tzdata util-linux mesa-va-drivers; \
    rm -rf /var/lib/apt/lists/* /tmp/jellyfin-ffmpeg.deb
COPY --from=server /out/kivo /usr/local/bin/kivo
COPY LICENSE /usr/share/doc/kivo/LICENSE
COPY --from=server /src/bin/THIRD-PARTY-NOTICES.txt /usr/share/doc/kivo/THIRD-PARTY-NOTICES.txt
COPY server/docker/entrypoint.sh /usr/local/bin/entrypoint.sh
ENV KIVO_DATA_DIR=/data \
    KIVO_CACHE_DIR=/cache \
    KIVO_BACKUP_DIR=/data/backups \
    KIVO_FFMPEG=/usr/lib/jellyfin-ffmpeg/ffmpeg \
    KIVO_FFPROBE=/usr/lib/jellyfin-ffmpeg/ffprobe \
    PUID=99 PGID=100 UMASK=022
VOLUME ["/config", "/data", "/cache"]
EXPOSE 8484 5353/udp
HEALTHCHECK --interval=30s --timeout=5s CMD ["kivo", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["kivo", "-config", "/config/server.toml"]
