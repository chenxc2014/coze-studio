"""从原始 Markdown 生成可离线阅读的 HTML 部署文档。"""
import argparse
import hashlib
import html
import re
import sys
from pathlib import Path

STYLE = r"""
:root{--ink:#172b3a;--muted:#60717f;--line:#dce5e9;--accent:#087c70;--paper:#fff;--canvas:#f3f6f7;--nav:#122c38}
*{box-sizing:border-box}html{scroll-behavior:smooth;scroll-padding-top:28px}body{margin:0;color:var(--ink);background:var(--canvas);font:16px/1.85 -apple-system,BlinkMacSystemFont,"Segoe UI","Microsoft YaHei",sans-serif}
a{color:var(--accent);text-underline-offset:3px}a:hover{color:#045d54}button,a{touch-action:manipulation}button{font:inherit;cursor:pointer}button:focus-visible,a:focus-visible{outline:3px solid #e6ad42;outline-offset:4px}
.skip{position:absolute;left:20px;top:-80px;background:white;padding:8px 16px;z-index:10}.skip:focus{top:10px}
.layout{max-width:1440px;margin:auto;display:grid;grid-template-columns:268px minmax(0,1fr);min-height:100vh}
.sidebar{background:var(--nav);color:#edf5f7;padding:36px 25px;position:sticky;top:0;height:100vh;overflow:auto}.brand{font-size:21px;letter-spacing:.02em;font-weight:750}.brand small{display:block;font-size:12px;color:#92b6c2;font-weight:500;letter-spacing:.12em;margin-top:4px}.nav-caption{font-size:12px;color:#92b6c2;margin:32px 0 14px}
.toc ul{list-style:none;padding:0;margin:0}.toc li{margin:4px 0}.toc a{display:block;color:#d7e6eb;text-decoration:none;font-size:14px;padding:7px 10px;border-radius:6px;line-height:1.55;overflow-wrap:anywhere}.toc a:hover,.toc a.active{background:#244550;color:white}.toc ul ul a{font-size:12px;color:#a5c1cb;padding-left:23px}.sidebar-note{border-top:1px solid #36515c;margin-top:30px;padding-top:18px;font-size:12px;color:#a5c1cb}
main{min-width:0;padding:46px 54px 30px}.eyebrow{font-size:12px;color:var(--accent);letter-spacing:.12em;font-weight:750}.hero h1{font-size:34px;line-height:1.4;letter-spacing:-.025em;margin:10px 0 12px}.intro{color:var(--muted);font-size:14px;margin:0 0 22px}.actions{display:flex;gap:10px;flex-wrap:wrap}.button{display:inline-flex;align-items:center;justify-content:center;border:1px solid var(--line);padding:8px 14px;border-radius:7px;text-decoration:none;background:white;font-size:13px;color:var(--ink);line-height:1.5}.button.primary{background:var(--accent);border-color:var(--accent);color:white}.button:hover{filter:brightness(.97)}
.summary{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin:28px 0 32px}.metric{background:white;border:1px solid var(--line);border-radius:9px;padding:16px 20px}.metric span{display:block;color:var(--muted);font-size:12px}.metric strong{display:block;font-size:24px;font-weight:700;margin-top:4px;line-height:1.5}.metric small{font-size:11px;color:var(--muted)}
article{background:var(--paper);padding:12px 34px 30px;border:1px solid var(--line);border-radius:10px;overflow-wrap:anywhere}article h2{font-size:23px;line-height:1.5;padding-top:22px;margin:16px 0 18px;border-top:1px solid var(--line)}article h2:first-child{border-top:0}article h3{font-size:18px;margin:25px 0 12px}article p{margin:12px 0}article ul,article ol{padding-left:24px;margin:12px 0}article li{margin:7px 0}article code{font:13px/1.6 Consolas,"Cascadia Code",monospace;color:#175b65;background:#edf4f5;padding:3px 5px;border-radius:4px;overflow-wrap:anywhere}
.table-wrap{overflow:auto;border:1px solid var(--line);border-radius:7px;margin:20px 0}table{width:100%;border-collapse:collapse;font-size:14px;text-align:left}th{background:#eef4f4;font-weight:650;color:#2c4654}th,td{padding:11px 14px;border-bottom:1px solid var(--line);vertical-align:top}tr:last-child td{border-bottom:0}tbody tr:nth-child(even){background:#fafcfc}
.code-wrap{position:relative;margin:20px 0}pre{margin:0;padding:45px 20px 20px;background:#142c38;color:#deedf2;border-radius:7px;overflow:auto;line-height:1.75}pre code{background:none!important;color:inherit!important;padding:0!important;white-space:pre;overflow-wrap:normal!important}.copy{position:absolute;right:10px;top:8px;background:#2b4652;border:1px solid #48616b;color:#eff7fa;padding:3px 10px;border-radius:5px;font-size:12px;line-height:1.6}.copy:hover{background:#385865}.code-label{position:absolute;left:18px;top:10px;color:#91b4c3;font-size:11px;letter-spacing:.08em}
.footer{display:flex;justify-content:space-between;gap:15px;color:var(--muted);font-size:12px;padding:23px 2px}.toast{position:fixed;bottom:25px;left:50%;transform:translateX(-50%);padding:9px 20px;background:var(--nav);color:white;border-radius:7px;font-size:13px;box-shadow:0 4px 20px #122c3820}.toast[hidden]{display:none}
@media(min-width:1500px){.layout{margin:24px auto;min-height:calc(100vh - 48px);box-shadow:0 10px 45px #1733440b}.sidebar{border-radius:12px 0 0 12px;height:calc(100vh - 48px);top:24px}}
@media(max-width:1000px){main{padding:32px 28px}.layout{grid-template-columns:225px minmax(0,1fr)}article{padding:10px 23px 25px}.sidebar{padding:30px 18px}.hero h1{font-size:29px}.metric{padding:13px}.metric strong{font-size:21px}}
@media(max-width:720px){.layout{display:block}.sidebar{height:auto;position:static;padding:20px}.brand small{display:inline;margin-left:12px;letter-spacing:.02em}.nav-caption,.sidebar-note{display:none}.toc>ul{display:flex;flex-wrap:wrap;gap:3px;margin-top:12px}.toc ul ul{display:none}.toc a{font-size:12px;padding:5px 8px}main{padding:24px 16px}.hero h1{font-size:26px}.summary{gap:7px;margin:22px 0}.metric{padding:10px}.metric strong{font-size:18px}.metric span{font-size:11px}.metric small{font-size:10px}article{padding:4px 18px 24px}article h2{font-size:21px}.footer{display:block}.footer span{display:block;margin:4px 0}body{font-size:15px}}
@media print{body{background:white;color:black;font-size:10pt;line-height:1.6}.layout{display:block;max-width:none;margin:0;box-shadow:none}.sidebar,.actions,.copy,.toast,.skip{display:none!important}main{padding:0}.hero h1{font-size:24pt}.intro{font-size:10pt}.summary{margin:16px 0;gap:8px}.metric{padding:10px}.metric strong{font-size:16pt}article{border:0;padding:0}article h2{font-size:16pt;break-after:avoid}article h3{font-size:12pt;break-after:avoid}pre{background:#f3f5f6!important;color:black!important;white-space:pre-wrap;overflow-wrap:anywhere;padding:14px;break-inside:avoid}pre code{white-space:pre-wrap!important}.code-label{display:none}.code-wrap{break-inside:avoid}table{font-size:9pt}th,td{padding:7px}tr{break-inside:avoid}.table-wrap{overflow:visible}a{color:inherit}.footer{border-top:1px solid #ddd;margin-top:20px;font-size:8pt}@page{size:A4;margin:17mm}}
"""

SCRIPT = r"""
const toast=document.getElementById('toast');let toastTimer;
function announce(message){toast.textContent=message;toast.hidden=false;clearTimeout(toastTimer);toastTimer=setTimeout(()=>toast.hidden=true,2200)}
document.querySelectorAll('pre').forEach(pre=>{const wrap=document.createElement('div');wrap.className='code-wrap';pre.before(wrap);wrap.append(pre);const label=document.createElement('span');label.className='code-label';label.textContent='命令';wrap.append(label);const button=document.createElement('button');button.type='button';button.className='copy';button.textContent='复制';button.setAttribute('aria-label','复制此命令块');wrap.append(button);button.addEventListener('click',async()=>{const text=pre.textContent;try{if(navigator.clipboard&&window.isSecureContext){await navigator.clipboard.writeText(text)}else{const input=document.createElement('textarea');input.value=text;input.style.position='fixed';input.style.opacity='0';document.body.append(input);input.select();const copied=document.execCommand('copy');input.remove();button.focus();if(!copied)throw new Error('copy')}announce('命令已复制')}catch{announce('请选中命令后手动复制')}})});
document.querySelectorAll('table').forEach(table=>{const wrap=document.createElement('div');wrap.className='table-wrap';table.before(wrap);wrap.append(table)});
document.getElementById('print').addEventListener('click',()=>window.print());
if('IntersectionObserver' in window){const observer=new IntersectionObserver(entries=>{entries.forEach(entry=>{if(entry.isIntersecting){document.querySelectorAll('.toc a').forEach(a=>a.classList.toggle('active',a.hash==='#'+entry.target.id))}})},{rootMargin:'0px 0px -65% 0px'});document.querySelectorAll('article h2,article h3').forEach(h=>observer.observe(h))}
"""


def render(source, markdown):
    text = source.read_text(encoding='utf-8')
    title = re.match(r'^#\s+(.+)', text).group(1)
    body = re.sub(r'^#\s+.+\n', '', text, count=1)
    converter = markdown.Markdown(extensions=['tables', 'fenced_code', 'toc', 'sane_lists'],
                                  extension_configs={'toc': {'toc_depth': '2-3'}})
    content = converter.convert(body)
    digest = hashlib.sha256(source.read_bytes()).hexdigest()
    version_match = re.search(r'基线：`([^`]+)`', text)
    version = version_match.group(1) if version_match else '以原文为准'
    server_match = re.search(r'服务器：`ssh ([^`]+)`', text)
    server = server_match.group(1) if server_match else '部署环境'
    url_match = re.search(r'平台入口(?:为\s*|[：:]\s*)`(https?://[^`]+)`', text)
    platform_url = url_match.group(1) if url_match else 'http://192.168.1.166:8888'
    date = re.search(r'20\d{2}-\d{2}-\d{2}', text)
    snapshot = date.group(0) if date else '以原文记录为准'
    storage = re.search(r'验收时系统分区可用约\s*([\d.]+) GiB，`/home` 可用约\s*([\d.]+) GiB', text)
    metrics = ''
    if storage:
        root, home = storage.groups()
        metrics = f'''<div class="summary" aria-label="验收时容量快照">
<div class="metric"><span>运行环境</span><strong>{html.escape(server)}</strong><small>Coze Studio · {html.escape(version)}</small></div>
<div class="metric"><span>系统分区可用</span><strong>{root} GiB</strong><small>{snapshot} 验收快照</small></div>
<div class="metric"><span>/home 分区可用</span><strong>{home} GiB</strong><small>容量以服务器实际检查为准</small></div></div>'''
    page = f'''<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="description" content="Coze Studio linux166 部署与运维说明，包含存储规划、容量控制、服务管理、模型配置与备份。">
<meta name="source-sha256" content="{digest}"><title>{html.escape(title)} · 部署与运维</title>
<style>{STYLE}</style></head><body><a href="#content" class="skip">跳转到正文</a>
<div class="layout"><aside class="sidebar"><div class="brand">Coze Studio<small>部署与运维手册</small></div>
<div class="nav-caption">文档目录</div><nav class="toc" aria-label="文档目录">{converter.toc}</nav>
<div class="sidebar-note">来源：{html.escape(source.name)}<br>与 Markdown 同步生成<br>支持离线阅读与打印</div></aside>
<main><header class="hero"><div class="eyebrow">{html.escape(server.upper())} / 开发验证环境</div><h1>{html.escape(title)}</h1>
<p class="intro">部署配置、磁盘规划与日常维护操作。容量和验收状态为原文记录的历史快照。</p>
<div class="actions"><a class="button primary" href="{html.escape(platform_url, quote=True)}" target="_blank" rel="noopener noreferrer">打开平台</a>
<a class="button" href="{html.escape(source.name)}" download>下载 Markdown</a><button class="button" id="print" type="button">打印 / 保存 PDF</button></div></header>
{metrics}<article id="content">{content}</article><footer class="footer"><span>原始 Markdown 保留；本页为对应 HTML 阅读版本。</span><span>内容记录：{snapshot} · 无外部资源依赖</span></footer></main></div>
<div class="toast" id="toast" role="status" aria-live="polite" hidden></div><script>{SCRIPT}</script></body></html>
'''
    target = source.with_suffix('.html')
    target.write_text(page, encoding='utf-8', newline='\n')
    print(f'已生成：{target}')
    return target


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dependency-dir', type=Path, help='工作区中 Markdown 库所在的临时目录')
    parser.add_argument('--source', type=Path, help='需要转换的 Markdown 文件；默认转换本目录下的全部 Markdown')
    args = parser.parse_args()
    if args.dependency_dir:
        sys.path.insert(0, str(args.dependency_dir.resolve()))
    try:
        import markdown
    except ImportError:
        parser.error('请先安装 requirements-docs.txt 中的依赖，或使用 --dependency-dir 指定已有依赖目录')
    sources = [args.source.resolve()] if args.source else sorted(Path(__file__).resolve().parents[1].glob('*.md'))
    for source in sources:
        render(source, markdown)


if __name__ == '__main__':
    sys.stdout.reconfigure(encoding='utf-8')
    main()
