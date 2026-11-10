"""备份 MySQL 元数据，限制保留份数和单次大小，不删除业务文件。"""
import fcntl
import gzip
import os
import shutil
import subprocess
from datetime import datetime
from pathlib import Path

base = Path('/home/coze/backups')
lock = (base / '.backup.lock').open('w')
fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
assert shutil.disk_usage('/home').free >= 25*1024**3, '可用空间不足 25 GiB，备份已跳过'
os.umask(0o077)
stamp = datetime.now().strftime('%Y%m%d-%H%M%S')
temp = base / f'metadata-{stamp}.sql.gz.part'
target = temp.with_suffix('')
process = subprocess.Popen(['docker','exec','coze-mysql','sh','-c',
    'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysqldump -uroot --single-transaction --routines --events --triggers --no-tablespaces --databases "$MYSQL_DATABASE"'],
    stdout=subprocess.PIPE, stderr=subprocess.PIPE)
try:
    with gzip.open(temp, 'wb') as output:
        while True:
            chunk = process.stdout.read(1024*1024)
            if not chunk:
                break
            output.write(chunk)
            if temp.stat().st_size > 5*1024**3:
                raise RuntimeError('单次压缩备份超过 5 GiB，需改用外部备份空间')
    error = process.stderr.read().decode(errors='replace')
    if process.wait():
        raise RuntimeError('数据库导出失败：'+error)
    temp.rename(target)
    # 只清理本脚本生成、超过七份的历史 SQL 备份。
    for old in sorted(base.glob('metadata-*.sql.gz'), reverse=True)[7:]:
        old.unlink()
    print(f'元数据备份完成：{target.name}，{target.stat().st_size} 字节')
except Exception:
    process.kill()
    process.wait()
    temp.unlink(missing_ok=True)
    raise
