#!/usr/bin/env bash
# Общие шаги локального окружения. Скрипт подключают, а не запускают.
# Корень репозитория — каталог над tools/, без привязки к машине агента.

dev_in_docker_group() {
  local members
  members="$(getent group docker 2>/dev/null | awk -F: '{print $4}')" || return 1
  case ",${members}," in
    *",$(id -un)",*) return 0 ;;
    *) return 1 ;;
  esac
}

# Штатный доступ: группа docker и сокет root:docker. Права сокета не расширяются.
dev_ensure_docker() {
  if docker info >/dev/null 2>&1; then
    return 0
  fi
  if [[ "${DEV_DOCKER_SG:-}" != 1 ]] && dev_in_docker_group; then
    if sg docker -c "docker info" >/dev/null 2>&1; then
      export DEV_DOCKER_SG=1
      exec sg docker -c "DEV_DOCKER_SG=1 bash $(printf '%q' "$0")"
    fi
  fi
  if [[ -d /run/systemd/system ]] && command -v systemctl >/dev/null 2>&1; then
    sudo systemctl start docker || true
  elif command -v service >/dev/null 2>&1; then
    sudo service docker start || true
  fi
  if docker info >/dev/null 2>&1; then
    return 0
  fi
  if [[ "${DEV_DOCKER_SG:-}" != 1 ]] && dev_in_docker_group && sg docker -c "docker info" >/dev/null 2>&1; then
    export DEV_DOCKER_SG=1
    exec sg docker -c "DEV_DOCKER_SG=1 bash $(printf '%q' "$0")"
  fi
  if [[ ! -d /run/systemd/system ]] && ! pgrep -x dockerd >/dev/null 2>&1; then
    sudo mkdir -p /etc/docker
    if [[ ! -f /etc/docker/daemon.json ]]; then
      sudo tee /etc/docker/daemon.json >/dev/null <<'EOF'
{
  "storage-driver": "fuse-overlayfs",
  "iptables": false,
  "ip6tables": false
}
EOF
    fi
    sudo dockerd >/tmp/ai-mentor-dockerd.log 2>&1 &
    local _i
    for _i in $(seq 1 60); do
      if docker info >/dev/null 2>&1 || { [[ "${DEV_DOCKER_SG:-}" != 1 ]] && sg docker -c "docker info" >/dev/null 2>&1; }; then
        break
      fi
      sleep 1
    done
    if [[ "${DEV_DOCKER_SG:-}" != 1 ]] && ! docker info >/dev/null 2>&1 && sg docker -c "docker info" >/dev/null 2>&1; then
      export DEV_DOCKER_SG=1
      exec sg docker -c "DEV_DOCKER_SG=1 bash $(printf '%q' "$0")"
    fi
  fi
  if ! docker info >/dev/null 2>&1; then
    echo "Нет штатного доступа к Docker. Добавьте пользователя в группу docker и откройте новую сессию." >&2
    exit 1
  fi
}
