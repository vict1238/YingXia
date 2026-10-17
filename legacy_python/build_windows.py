"""Optional Windows-only packaging; not executed during the Linux delivery build."""
from pathlib import Path
import os
import shutil
import subprocess
import sys

def main():
    if sys.platform != 'win32':
        print('A Windows executable must be built on Windows. No cross-build is attempted.')
        return 1
    root=Path(__file__).resolve().parent
    command=[sys.executable,'-m','PyInstaller','--noconfirm','--clean','--onedir','--console','--name','YingXia',
             '--add-data',str(root/'yingxia'/'web')+os.pathsep+'yingxia/web',
             '--add-data',str(root/'yingxia'/'sources.json')+os.pathsep+'yingxia',str(root/'main.py')]
    subprocess.run(command,cwd=root,check=True)
    target=root/'dist'/'YingXia'
    for name in ('LICENSE.txt','README_zh.md','THIRD_PARTY_NOTICES.md','START_HERE.html'):
        shutil.copy2(root/name,target/name)
    return 0
if __name__=='__main__':raise SystemExit(main())
