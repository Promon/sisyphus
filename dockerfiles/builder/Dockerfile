FROM golang:latest

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

COPY files/promon.crt /usr/local/share/ca-certificates/promon.crt
RUN update-ca-certificates