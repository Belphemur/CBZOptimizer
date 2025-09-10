FROM debian:trixie-slim
LABEL authors="Belphemur"
ARG TARGETPLATFORM
ARG APP_PATH=/usr/local/bin/CBZOptimizer
ENV USER=abc
ENV CONFIG_FOLDER=/config
ENV PUID=99
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends adduser && \
    addgroup --system users && \
    adduser \
    --system \
    --home "${CONFIG_FOLDER}" \
    --uid "${PUID}" \
    --ingroup users \
    --disabled-password \
    "${USER}" && \
    apt-get purge -y --auto-remove adduser && \
    rm -rf /var/lib/apt/lists/*

COPY ${TARGETPLATFORM}/CBZOptimizer ${APP_PATH}

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y --no-install-recommends \
    inotify-tools \
    bash \
    ca-certificates \
    bash-completion && \
    rm -rf /var/lib/apt/lists/* && \
    chmod +x ${APP_PATH} && \
    ${APP_PATH} completion bash > /etc/bash_completion.d/CBZOptimizer.bash

VOLUME ${CONFIG_FOLDER}
USER ${USER}
ENTRYPOINT ["/usr/local/bin/CBZOptimizer"]
