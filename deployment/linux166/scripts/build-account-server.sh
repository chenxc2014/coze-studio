#!/usr/bin/env bash
# 在独立目录构建，不自动切换运行镜像或修改模型凭据。
set -euo pipefail
source_dir=${1:-/home/coze/src/coze-studio}
build_dir=/home/coze/build
export PATH=/home/coze/tools/go/bin:$PATH
export GOMODCACHE=$build_dir/gomod GOCACHE=$build_dir/gocache GOTMPDIR=$build_dir/tmp TMPDIR=$build_dir/tmp
export GOPROXY=https://goproxy.cn,direct GOMAXPROCS=3 GOMEMLIMIT=3GiB GOTOOLCHAIN=local CGO_ENABLED=0
python3 /home/coze/ops/capacity.py --guard
install -d -m 0750 "$build_dir"/{gomod,gocache,tmp,out}
cd "$source_dir/backend"
go test -p 3 -run '^(TestProtectCurrentAdministrator|TestAccountPasswordValidation|TestDisabledAccountCannotLoginOrReuseSession|TestAccountAdminWriteOrigin)$' ./domain/user/service ./application/user ./api/handler/coze
go build -p 3 -trimpath -o "$build_dir/out/opencoze" .
sha256sum "$build_dir/out/opencoze"
echo '二进制已生成；切换运行镜像前请先备份数据库与运行配置。'
