#
# Build environment
#
FROM golang:1.12 AS build-env

# Install required dependencies
RUN set -x \
    && apt-get update -y \
    && apt-get install -y locales \
                          make \
                          xz-utils \
                          zip \
    && rm -rf /var/lib/apt/lists/*

# Set default locale for the environment
RUN echo "en_US UTF-8" > /etc/locale.gen; \
    locale-gen en_US.UTF-8

ENV LANG=en_US.UTF-8
ENV LANGUAGE=en_US:en
ENV LC_ALL=en_US.UTF-8

RUN echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] http://packages.cloud.google.com/apt cloud-sdk main" | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list \
    && curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key --keyring /usr/share/keyrings/cloud.google.gpg  add - \
    && apt-get update -y \
    && apt-get install google-cloud-sdk -y

COPY files/promon.crt /usr/local/share/ca-certificates/promon.crt
RUN update-ca-certificates

# Cache required go modules
WORKDIR /build
COPY ./go.mod ./go.sum ./
RUN set -x \
    && go mod download \
    && rm -rf /build

#
# Builder
#
FROM build-env AS builder
WORKDIR /build
COPY . .
RUN set -x \
    && go mod tidy \
    && go build -v -buildmode=exe .

#
# Runner
#
FROM ubuntu:bionic AS runner

RUN set -x \
    && apt-get update -y \
    && apt-get install -y ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/sisyphus /bin/
COPY files/promon.crt /usr/local/share/ca-certificates/promon.crt
RUN update-ca-certificates