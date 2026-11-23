# Базовый образ Cloud Agent. Каталог проекта сюда не копируется:
# checkout и tools/dev-install.sh выполняет Cursor после сборки образа.
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        xz-utils \
        gnupg \
        python3 \
        postgresql-client \
        docker.io \
        fuse-overlayfs \
        sudo \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /etc/docker \
    && printf '%s\n' \
        '{' \
        '  "storage-driver": "fuse-overlayfs",' \
        '  "iptables": false,' \
        '  "ip6tables": false' \
        '}' > /etc/docker/daemon.json \
    && groupadd -f docker \
    && (id -u ubuntu >/dev/null 2>&1 || useradd -m -s /bin/bash ubuntu) \
    && usermod -aG docker ubuntu \
    && usermod -aG sudo ubuntu \
    && printf '%s\n' 'ubuntu ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/ubuntu \
    && chmod 0440 /etc/sudoers.d/ubuntu
