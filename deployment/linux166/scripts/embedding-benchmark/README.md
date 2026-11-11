# 独立向量验证脚本

本目录保留 2026-11-11 在 linux166 实际执行的 Python 编码和评估代码，结果参见 ../../向量模型优化验证.md。不是生产模型部署脚本。

## 运行条件

- Python 3.9，依赖版本见 requirements.txt，CPU-only。
- 工作目录 /home/coze/build/embedding-bench；small-model 目录放 sources.json 对应资产，逐一校验 SHA256。
- 需要提前建立独立 coze-embedding-bench.service，使用现有 Ollama CPU runtime，绑定 127.0.0.1:11435，独立模型目录 /home/coze/embedding-bench/models，别名 bge-m3:latest 指向已校验权重；绝不能使用生产服务作为测试 unit。
- 独立 unit 以 coze-embedding 账号运行，CPUWeight=10、Nice=10、MemoryMax=6G、RuntimeMaxSec=2h、不设置开机自启；CPU 配额由程序按组设为 8 或 12 核。应在容量检查通过后，按服务器负载决定是否允许 12 核预筛。
- 控制进程也应 CPUQuota=800%、CPUWeight=10、Nice=10、MemoryMax=4G，使用 journal 日志；环境变量 OPENBLAS_NUM_THREADS=1、OMP_NUM_THREADS=8、TOKENIZERS_PARALLELISM=false。
- bench-run.py 会重启独立测试 unit 并写入 /run/systemd/system/coze-embedding-bench.service.d/parameters.conf，必须由具备对应权限的运维人员在隔离测试环境执行。程序不修改生产配置。

## 私有样本结构

sample.json 不入库，保存在服务器 0700 目录，文件权限 0600。结构为 records 数组（每项 text 为真实索引文本）、queries 数组（每项 id、kind、text、relevant；relevant 是 records 中从零开始的相关记录下标）和 summary 元数据。实际测评为 500 条候选及 50 个模板问题，种子 20261111。

原样本、问题和 .npy 文件均不随源码交付。更换样本后不能照搬本次质量指标。

## 已知限制与收尾

bench-run.py 顺序发出 HTTP 批请求；另用 bench-client-concurrency.py 在相同 64 行上补测 1 / 2 个真实 HTTP worker。Ollama 在本次 BERT Embedding 路径把 requested parallel=2 实际限制为 -np 1；报告区分了客户端并发与模型 slot 并行。12 线程组因明显低效被人工停止，该组没有完整有效结果。

每组只有一轮完整复测、生产共享 CPU，不能视作独占硬件的统计基准。主质量比较不加 query instruction，额外提示词结果单列。

results/summary.json 的 validation_finished=true 才代表完整完成。异常时先检查 journal 与 summary，不应直接重跑所有昂贵编码。收尾停止独立测试 unit，移除其 /run 参数覆盖，确认 11435 / 11436 无监听且生产服务仍正常。bench-small-server.py 只用于回环测试接口，未配置生产鉴权、服务持久化或监控。

## 地址问法补测

bench-address.py 复用相同 sample.json 和 results/m3-corpus.npy、small-corpus.npy，只编码新增 80 个地址问题。它会先保存协议、再运行独立 BGE-M3 测试实例，并在 finally 停止该实例和移除 /run 参数覆盖。运行条件和隔离要求同上；只导出聚合 address-summary.json，address-queries.json 和逐题结果留在受限目录。结果参见 ../../地址检索专项验证.md。
