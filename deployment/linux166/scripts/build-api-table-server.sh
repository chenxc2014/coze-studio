#!/usr/bin/env bash
# 在隔离副本验证并构建 API 表格导入，不自动切换运行服务。
set -euo pipefail
source_dir=${1:-/home/coze/build/source}
build_dir=/home/coze/build
export PATH=/home/coze/tools/go/bin:$PATH
export GOMODCACHE=$build_dir/gomod GOCACHE=$build_dir/gocache GOTMPDIR=$build_dir/tmp TMPDIR=$build_dir/tmp
export GOPROXY=https://goproxy.cn,direct GOMAXPROCS=3 GOMEMLIMIT=3GiB GOTOOLCHAIN=local CGO_ENABLED=0
python3 /home/coze/ops/capacity.py --guard
install -d -m 0750 "$build_dir"/{gomod,gocache,tmp,out}
cd "$source_dir/backend"
go test -p 3 -run '^(TestAPITable.*|TestAddress.*|TestPluginCredential.*|TestCachedTableDocumentsPreserveMetadata|TestCachedDocumentsTextAndHeaders|TestPackInsertDataRejectsMalformedRows|TestResegmentTablePreservesSheetRange|TestDocumentStatusClearsOldFailure|TestMessageTouchContinuesUntilStopped|TestResolveTokenExpiration|TestTokenExpirationPermission)$' ./domain/knowledge/apitable ./domain/knowledge/service ./domain/knowledge/address ./domain/knowledge/internal/dal/dao ./domain/agent/singleagent/internal/agentflow ./application/knowledge ./api/handler/coze ./api/router ./bizpkg/config/plugincredentials ./domain/plugin/service ./domain/plugin/service/tool ./infra/eventbus/impl/nsq ./application/openauth ./domain/openauth/openapiauth
go build -p 3 -trimpath -o "$build_dir/out/opencoze-api-table" .
sha256sum "$build_dir/out/opencoze-api-table"
