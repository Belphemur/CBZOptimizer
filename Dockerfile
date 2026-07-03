FROM alpine:latest
LABEL authors="Belphemur"
ARG TARGETPLATFORM
ARG APP_PATH=/usr/local/bin/CBZOptimizer
ENV USER=abc
ENV CONFIG_FOLDER=/config
ENV PUID=99
# libwebp-tools (cwebp) is installed via apk below into /usr/bin; point go-webpbin
# at it directly so it doesn't try to download a glibc prebuilt binary.
ENV VENDOR_PATH=/usr/bin

RUN adduser \
    -S \
    -D \
    -H \
    -h "${CONFIG_FOLDER}" \
    -u "${PUID}" \
    -G users \
    -s /bin/bash \
    "${USER}"

COPY ${TARGETPLATFORM}/CBZOptimizer ${APP_PATH}

RUN apk add --no-cache \
    bash \
    ca-certificates \
    bash-completion \
    libwebp-tools && \
    chmod +x ${APP_PATH} && \
    ${APP_PATH} completion bash > /etc/bash_completion.d/CBZOptimizer.bash


USER ${USER}

# Need to run as the user to have the right config folder created
RUN --mount=type=bind,source=${TARGETPLATFORM},target=/tmp/target \
    /tmp/target/encoder-setup

ENTRYPOINT ["/usr/local/bin/CBZOptimizer"]
