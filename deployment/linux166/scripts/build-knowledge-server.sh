#!/usr/bin/env bash
# 在独立源码副本构建并验证知识库重试、地址查询及长任务修复，不自动部署。
set -euo pipefail
source_dir=${1:-/home/coze/build/source}
build_dir=/home/coze/build
export PATH=/home/coze/tools/go/bin:$PATH
export GOMODCACHE=$build_dir/gomod GOCACHE=$build_dir/gocache GOTMPDIR=$build_dir/tmp TMPDIR=$build_dir/tmp
export GOPROXY=https://goproxy.cn,direct GOMAXPROCS=3 GOMEMLIMIT=3GiB GOTOOLCHAIN=local CGO_ENABLED=0
python3 /home/coze/ops/capacity.py --guard
install -d -m 0750 "$build_dir"/{gomod,gocache,tmp,out}
cd "$source_dir/backend"
go test -p 3 -run '^(TestAddress.*|TestCachedTableDocumentsPreserveMetadata|TestCachedDocumentsTextAndHeaders|TestPackInsertDataRejectsMalformedRows|TestResegmentTablePreservesSheetRange|TestDocumentStatusClearsOldFailure|TestMessageTouchContinuesUntilStopped|TestResolveTokenExpiration|TestTokenExpirationPermission)$' ./domain/agent/singleagent/internal/agentflow ./domain/knowledge/address ./domain/knowledge/service ./application/knowledge ./api/handler/coze ./api/router ./domain/knowledge/internal/dal/dao ./infra/eventbus/impl/nsq ./application/openauth ./domain/openauth/openapiauth
go build -p 3 -trimpath -o "$build_dir/out/opencoze" .
sha256sum "$build_dir/out/opencoze"
