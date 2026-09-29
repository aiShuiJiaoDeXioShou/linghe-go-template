#!/bin/sh
# lhcli 模板改名脚本：把本模板中的项目标识替换为新项目标识。
#
# 用法：
#   sh scripts/rename.sh --module <go-module-path> --name <project-name> [--dry-run]
#
# 说明：
#   - 只做文本替换，不改动 Git 仓库、不下载依赖，后续动作由 lhcli 控制。
#   - 可在模板根目录手动执行，也可由 lhcli init 调用。
#   - 在 Windows 下请使用 Git Bash 提供 sh。

set -eu

# 模块路径占位符，避免名称替换污染模块路径。
module_placeholder="__LHCLI_MODULE_PLACEHOLDER__"

dry_run="0"
new_module=""
new_name=""
old_module=""
old_name=""

usage() {
	cat <<'EOF'
用法:
  sh scripts/rename.sh --module <go-module-path> --name <project-name> [选项]

选项:
  --module <path>      新的 Go module 路径，例如 github.com/example/order-service
  --name <name>        新的项目名，需匹配 ^[a-z][a-z0-9-]*$
  --old-module <path>  覆盖自动探测到的旧 module 路径
  --old-name <name>    覆盖自动探测到的旧项目名
  --dry-run            只列出将要改写的文件，不写入磁盘
  -h, --help           显示本帮助
EOF
}

err() {
	echo "错误: $*" >&2
	exit 1
}

# escape_dots 转义字符串中的点号，用于 sed 搜索模式。
escape_dots() {
	printf '%s' "$1" | sed 's/[.]/\\./g'
}

# detect_old_module 从 go.mod 读取当前 module 路径。
detect_old_module() {
	[ -f go.mod ] || err "未找到 go.mod，请在模板根目录运行本脚本"
	awk '/^module[ \t]+/ { print $2; exit }' go.mod
}

# detect_old_name 从 configs/config.local.yaml 读取当前应用名。
detect_old_name() {
	[ -f configs/config.local.yaml ] || err "未找到 configs/config.local.yaml"
	awk '/^[ \t]+name:[ \t]*/ { print $2; exit }' configs/config.local.yaml
}

# parse_args 解析命令行参数。
parse_args() {
	while [ "$#" -gt 0 ]; do
		case "$1" in
		--module)
			[ "$#" -ge 2 ] || err "--module 缺少取值"
			new_module="$2"
			shift 2
			;;
		--name)
			[ "$#" -ge 2 ] || err "--name 缺少取值"
			new_name="$2"
			shift 2
			;;
		--old-module)
			[ "$#" -ge 2 ] || err "--old-module 缺少取值"
			old_module="$2"
			shift 2
			;;
		--old-name)
			[ "$#" -ge 2 ] || err "--old-name 缺少取值"
			old_name="$2"
			shift 2
			;;
		--dry-run)
			dry_run="1"
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			err "未知参数: $1"
			;;
		esac
	done
}

# validate 校验参数合法性。
validate() {
	[ -n "$new_module" ] || err "必须提供 --module"
	[ -n "$new_name" ] || err "必须提供 --name"

	case "$new_name" in
	*[!a-z0-9-]* | [!a-z]*) err "--name 必须匹配 ^[a-z][a-z0-9-]*$" ;;
	esac
	case "$new_module" in
	*/*) ;;
	*) err "--module 需为完整模块路径（至少包含一个 /）" ;;
	esac
	case "$new_module" in
	*[!A-Za-z0-9._~/-]*) err "--module 含非法字符" ;;
	esac

	if [ -z "$old_module" ]; then
		old_module="$(detect_old_module)"
	fi
	if [ -z "$old_name" ]; then
		old_name="$(detect_old_name)"
	fi

	if [ "$old_module" = "$new_module" ] && [ "$old_name" = "$new_name" ]; then
		err "新旧模块与名称完全一致，无需改名"
	fi
}

# is_target_file 判断文件是否需要参与替换。
is_target_file() {
	file_norm="$1"
	case "$file_norm" in
	./*) file_norm="${file_norm#./}" ;;
	esac
	[ "$file_norm" = "$self_path" ] && return 1

	base="$(basename "$1")"
	case "$base" in
	Dockerfile | .gitmessage) return 0 ;;
	esac
	case "$1" in
	*.go | *.mod | *.md | *.yaml | *.yml | *.sh | *.sql | *.json) return 0 ;;
	esac
	return 1
}

# rewrite 对单个文件执行替换，返回 0 表示内容发生变化。
rewrite() {
	file="$1"
	set --
	if [ "$old_module" = "$old_name" ]; then
		case "$file" in
		*.go)
			set -- "$@" \
				-e "s|\"${old_module_re}/|\"${module_placeholder}/|g" \
				-e "s|\"${old_module_re}\"|\"${module_placeholder}\"|g"
			;;
		esac
		case "$(basename "$file")" in
		go.mod)
			set -- "$@" -e "s|^module ${old_module_re}\$|module ${module_placeholder}|"
			;;
		esac
	else
		set -- "$@" -e "s|${old_module_re}|${module_placeholder}|g"
	fi
	set -- "$@" \
		-e "s|${old_db}|${new_db}|g" \
		-e "s|${old_name_re}|${new_name}|g" \
		-e "s|${module_placeholder}|${new_module}|g"

	tmp="${file}.lhcli-rename.tmp"
	sed "$@" "$file" >"$tmp" || {
		rm -f "$tmp"
		err "处理失败: $file"
	}
	if cmp -s "$file" "$tmp"; then
		rm -f "$tmp"
		return 1
	fi
	if [ "$dry_run" = "1" ]; then
		rm -f "$tmp"
		echo "(干跑) 将改写 $file"
	else
		mv "$tmp" "$file"
		echo "已改写 $file"
	fi
	return 0
}

main() {
	parse_args "$@"
	validate

	# 记录脚本自身路径，替换时跳过，避免自我改写。
	self_path="$0"
	case "$self_path" in
	./*) self_path="${self_path#./}" ;;
	esac

	old_module_re="$(escape_dots "$old_module")"
	old_name_re="$(escape_dots "$old_name")"
	old_db="$(printf '%s' "$old_name" | tr '-' '_')"
	new_db="$(printf '%s' "$new_name" | tr '-' '_')"

	echo "模板改名: 模块 $old_module -> $new_module, 名称 $old_name -> $new_name"
	if [ "$dry_run" = "1" ]; then
		echo "模式: 干跑（不写入磁盘）"
	fi

	list="$(mktemp)"
	trap 'rm -f "$list"' EXIT
	find . \
		-type d \( -name .git -o -name .deploy -o -name dist -o -name vendor \) -prune \
		-o -type f -print >"$list"

	changed="0"
	while IFS= read -r file; do
		if is_target_file "$file"; then
			if rewrite "$file"; then
				changed=$((changed + 1))
			fi
		fi
	done <"$list"

	if [ "$changed" = "0" ]; then
		echo "没有需要改写的文件"
	else
		echo "共处理 $changed 个文件"
	fi
	echo "改名完成，请检查数据库凭据与 Git 远端地址"
}

main "$@"
