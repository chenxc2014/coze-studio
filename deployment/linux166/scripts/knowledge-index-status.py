"""只读检查知识文档导入状态，不输出知识正文或数据库凭据。"""
import argparse
import datetime
import json
from pathlib import Path
import subprocess

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--document-id',required=True,type=int)
    parser.add_argument('--expected-rows',type=int)
    parser.add_argument('--output',type=Path)
    args=parser.parse_args()
    if args.document_id<=0:parser.error('文档 ID 必须为正整数')
    query=f'SELECT id,knowledge_id,status,slice_count,LENGTH(fail_reason) FROM knowledge_document WHERE id={args.document_id} AND deleted_at IS NULL; SELECT status,COUNT(*) FROM knowledge_document_slice WHERE document_id={args.document_id} AND deleted_at IS NULL GROUP BY status;'
    command=['docker','exec','coze-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot "$MYSQL_DATABASE" -Nse "$1"','sh',query]
    result=subprocess.run(command,capture_output=True,text=True,check=True)
    lines=[line.split('\t') for line in result.stdout.strip().splitlines()]
    if not lines or len(lines[0])!=5:raise RuntimeError('未找到指定文档')
    doc=lines[0];counts={int(row[0]):int(row[1]) for row in lines[1:]}
    report={'checked_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'document_id':doc[0],'knowledge_id':doc[1],'document_status':int(doc[2]),'document_final_slice_count':int(doc[3]),'error_present':doc[4] not in ['0','NULL'],'completed_rows':counts.get(1,0),'processing_rows':counts.get(0,0),'failed_rows':counts.get(2,0),'import_complete':int(doc[2])==1}
    if args.expected_rows:
        report['expected_rows']=args.expected_rows
        report['percent']=round(min(100,counts.get(1,0)*100/args.expected_rows),2)
    content=json.dumps(report,ensure_ascii=False,indent=2)+'\n'
    if args.output:args.output.write_bytes(content.encode('utf-8'))
    print(content,end='')

if __name__=='__main__':main()
