#!/usr/bin/env bash
# 仅构建与验证；正式切换由运维在备份之后执行。
set -euo pipefail
source_dir=${1:-/home/coze/build/subpath-source}
export PATH=/home/coze/tools/node/bin:$PATH
export NPM_CONFIG_CACHE=/home/coze/build/npm-cache RUSH_GLOBAL_FOLDER=/home/coze/build/rush-global
export TMPDIR=/home/coze/build/frontend-tmp XDG_CACHE_HOME=/home/coze/build/frontend-cache
export NODE_OPTIONS=--max-old-space-size=6144 RAYON_NUM_THREADS=3 UV_THREADPOOL_SIZE=3
export BUILD_BRANCH=opencoze-local
python3 /home/coze/ops/capacity.py --guard
install -d -m 0750 "$TMPDIR" "$NPM_CONFIG_CACHE" "$RUSH_GLOBAL_FOLDER"
cd "$source_dir"
node common/scripts/install-run-rush.js install
node --test deployment/linux166/tests/subpath-runtime.test.cjs
(
  cd frontend/packages/arch/web-context
  ./node_modules/.bin/vitest --run src/base-path.test.ts
)
node common/scripts/install-run-rush.js rebuild --to @coze-studio/app --parallelism 1
echo '前端构建完成；产物位于 frontend/apps/coze-studio/dist。'
