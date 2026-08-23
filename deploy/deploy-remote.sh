#!/usr/bin/env bash

set -Eeuo pipefail

readonly environment_name="${1:-}"
readonly release_sha="${2:-}"
readonly archive_path="${3:-}"
readonly deploy_dir="${4:-}"
readonly app_port="${5:-}"
readonly release_dir="${deploy_dir}/releases/${release_sha}"
readonly deploy_port_file=".deploy-port"
compose_project="${deploy_dir##*/}"
compose_project="${compose_project//./-}"
readonly compose_project
temporary_release_dir=""
previous_release=""

fail() {
    printf '部署失败：%s\n' "$*" >&2
    exit 1
}

cleanup() {
    rm -f -- "$archive_path"
    if [[ -n "$temporary_release_dir" && -d "$temporary_release_dir" ]]; then
        rm -rf -- "$temporary_release_dir"
    fi
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "服务器缺少必要命令：$1"
}

wait_for_health() {
    local health_port="$1"
    local attempt

    for attempt in $(seq 1 60); do
        if curl --silent --show-error --fail --max-time 3 \
            "http://127.0.0.1:${health_port}/readyz" >/dev/null 2>&1; then
            return 0
        fi
        sleep 3
    done
    return 1
}

compose_build() {
    local target_release="$1"
    local target_sha="$2"

    cd "$target_release"
    APP_IMAGE_TAG="$target_sha" \
        docker compose --project-name "$compose_project" build backend
}

compose_migrate() {
    local target_release="$1"
    local target_port="$2"
    local target_sha="$3"

    cd "$target_release"
    APP_PORT="$target_port" APP_IMAGE_TAG="$target_sha" \
        docker compose --project-name "$compose_project" \
        run --rm --no-deps backend migrate up \
        -config "/app/configs/config.${environment_name}.yaml" \
        -path /app/migrations
}

compose_up() {
    local target_release="$1"
    local target_port="$2"
    local target_sha="$3"

    cd "$target_release"
    APP_PORT="$target_port" APP_IMAGE_TAG="$target_sha" \
        docker compose --project-name "$compose_project" \
        up --detach --no-build --remove-orphans
}

compose_restore() {
    local target_release="$1"
    local target_port="$2"
    local target_sha="$3"

    cd "$target_release"
    APP_PORT="$target_port" APP_IMAGE_TAG="$target_sha" \
        docker compose --project-name "$compose_project" \
        up --detach --build --remove-orphans
}

rollback_previous() {
    local previous_port
    local previous_sha

    if [[ -n "$previous_release" && -d "$previous_release" \
            && -f "${previous_release}/${deploy_port_file}" ]]; then
        printf '部署：正在回滚到上一个成功版本 %s\n' "$previous_release" >&2
        previous_port="$(<"${previous_release}/${deploy_port_file}")"
        previous_sha="${previous_release##*/}"
        [[ "$previous_port" =~ ^[0-9]{2,5}$ ]] || fail "上一个版本的应用端口无效"
        compose_restore "$previous_release" "$previous_port" "$previous_sha"
        wait_for_health "$previous_port" || fail "已经恢复上一个版本但健康检查仍未通过"
        printf '部署：已成功恢复上一个版本\n' >&2
        return 0
    fi

    cd "$release_dir"
    APP_PORT="$app_port" APP_IMAGE_TAG="$release_sha" \
        docker compose --project-name "$compose_project" down || true
    printf '部署：不存在可回滚版本 已停止本次部署创建的容器\n' >&2
}

[[ "$environment_name" == "stg" || "$environment_name" == "production" ]] \
    || fail "环境名称必须是 stg 或 production"
[[ "$release_sha" =~ ^[0-9a-f]{40}$ ]] || fail "发布版本必须是完整的 Git SHA"
[[ "$deploy_dir" =~ ^/opt/[a-z0-9][a-z0-9._/-]*$ ]] \
    || fail "部署目录必须位于 /opt 下"
[[ "$deploy_dir" != *".."* ]] || fail "部署目录不能包含上级路径"
[[ "$deploy_dir" != */ ]] || fail "部署目录不能以斜杠结尾"
[[ "$archive_path" == "/tmp/go-template-${environment_name}-${release_sha}.tar.gz" ]] \
    || fail "部署包路径不符合约定"
[[ "$app_port" =~ ^[0-9]{2,5}$ ]] || fail "应用端口无效"
(( app_port >= 1024 && app_port <= 65535 )) || fail "应用端口必须位于 1024 到 65535"
[[ -f "$archive_path" ]] || fail "找不到部署压缩包"

trap cleanup EXIT

printf '部署：开始发布 %s 环境 提交版本为 %s\n' "$environment_name" "$release_sha"

require_command curl
require_command docker
require_command mktemp
require_command readlink
require_command seq
require_command tar
docker compose version >/dev/null
docker network inspect 1panel-network >/dev/null
printf '部署：服务器命令与 Docker 网络检查通过\n'

mkdir -p "${deploy_dir}/releases"
previous_release="$(readlink -f "${deploy_dir}/current" 2>/dev/null || true)"
if [[ "$previous_release" == "$release_dir" ]]; then
    previous_release=""
fi

if [[ ! -d "$release_dir" ]]; then
    printf '部署：正在解压发布文件到 %s\n' "$release_dir"
    temporary_release_dir="$(mktemp -d "${deploy_dir}/releases/.${release_sha}.XXXXXX")"
    tar -xzf "$archive_path" -C "$temporary_release_dir"
    [[ -x "${temporary_release_dir}/server" ]] || fail "部署包中缺少可执行文件"
    [[ -f "${temporary_release_dir}/Dockerfile" ]] || fail "部署包中缺少 Dockerfile"
    [[ -f "${temporary_release_dir}/docker-compose.yml" ]] || fail "部署包中缺少 Docker Compose 配置"
    [[ -f "${temporary_release_dir}/configs/config.${environment_name}.yaml" ]] \
        || fail "部署包中缺少环境配置"
    mv "$temporary_release_dir" "$release_dir"
    temporary_release_dir=""
fi

[[ -d "${release_dir}/migrations" ]] || fail "部署包中缺少 migrations 目录"
compgen -G "${release_dir}/migrations/*.up.sql" >/dev/null \
    || fail "部署包中缺少升级迁移文件"
compgen -G "${release_dir}/migrations/*.down.sql" >/dev/null \
    || fail "部署包中缺少回滚迁移文件"

printf '%s\n' "$app_port" >"${release_dir}/${deploy_port_file}"
cd "$release_dir"
printf '部署：正在校验 Docker Compose 配置\n'
APP_PORT="$app_port" APP_IMAGE_TAG="$release_sha" \
    docker compose --project-name "$compose_project" config --quiet

printf '部署：正在构建新版本镜像\n'
if ! compose_build "$release_dir" "$release_sha"; then
    fail "Docker 镜像构建失败 旧版本继续运行"
fi

printf '部署：正在执行数据库升级\n'
if ! compose_migrate "$release_dir" "$app_port" "$release_sha"; then
    fail "数据库升级失败 旧版本继续运行"
fi

printf '部署：数据库升级成功 正在启动新版本容器\n'
if ! compose_up "$release_dir" "$app_port" "$release_sha"; then
    docker compose --project-name "$compose_project" logs --no-color --tail 200 backend || true
    rollback_previous
    fail "Docker Compose 部署失败"
fi

printf '部署：正在等待依赖就绪检查 最长等待 180 秒\n'
if wait_for_health "$app_port"; then
    ln -sfn "$release_dir" "${deploy_dir}/current.next"
    mv -Tf "${deploy_dir}/current.next" "${deploy_dir}/current"
    printf '%s\n' "$release_sha" >"${deploy_dir}/last-successful-sha"
    printf '部署：%s 环境已通过就绪检查 提交版本为 %s\n' \
        "$environment_name" "$release_sha"
    exit 0
fi

printf '部署：%s 环境就绪检查失败 提交版本为 %s\n' \
    "$environment_name" "$release_sha" >&2
docker compose --project-name "$compose_project" logs --no-color --tail 200 backend || true
rollback_previous
exit 1
