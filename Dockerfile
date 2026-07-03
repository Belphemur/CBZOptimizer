FROM debian:trixie-slim
LABEL authors="Belphemur"
ARG TARGETPLATFORM
ARG APP_PATH=/usr/local/bin/CBZOptimizer
ENV USER=abc
ENV CONFIG_FOLDER=/config
ENV PUID=99
ENV DEBIAN_FRONTEND=noninteractive

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && apt-get install -y --no-install-recommends adduser && \
    addgroup --system users && \
    adduser \
    --system \
    --home "${CONFIG_FOLDER}" \
    --uid "${PUID}" \
    --ingroup users \
    --disabled-password \
    "${USER}" && \
    apt-get purge -y --auto-remove adduser

COPY ${TARGETPLATFORM}/CBZOptimizer ${APP_PATH}

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && \
    apt-get full-upgrade -y && \
    apt-get install -y --no-install-recommends \
    inotify-tools \
    bash \
    ca-certificates \
    bash-completion && \
    chmod +x ${APP_PATH} && \
    ${APP_PATH} completion bash > /etc/bash_completion.d/CBZOptimizer.bash


USER ${USER}

# Need to run as the user to have the right config folder created
RUN --mount=type=bind,source=${TARGETPLATFORM},target=/tmp/target \
    /tmp/target/encoder-setup

ENTRYPOINT ["/usr/local/bin/CBZOptimizer"]
