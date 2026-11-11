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
credential_key = Path('/home/coze/secrets/plugin-credential.key')
key_target = base / f'metadata-{stamp}.plugin-key'
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
    # 密文数据库与独立加密密钥必须成对备份，文件仅限服务器管理员读取。
    if credential_key.exists():
        key_bytes = credential_key.read_bytes()
        if len(key_bytes) != 32:
            raise RuntimeError('插件凭据加密密钥长度不正确，备份未完成')
        key_target.write_bytes(key_bytes)
        key_target.chmod(0o600)
    temp.rename(target)
    # 只清理本脚本生成、超过七份的历史 SQL 备份。
    for old in sorted(base.glob('metadata-*.sql.gz'), reverse=True)[7:]:
        old.with_name(old.name.removesuffix('.sql.gz') + '.plugin-key').unlink(missing_ok=True)
        old.unlink()
    print(f'元数据备份完成：{target.name}，{target.stat().st_size} 字节')
except Exception:
    process.kill()
    process.wait()
    temp.unlink(missing_ok=True)
    key_target.unlink(missing_ok=True)
    raise
