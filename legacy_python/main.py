#!/usr/bin/env python3
"""映匣 v1.0. Standard-library backend + installed Edge/Chrome in app-window mode."""
from __future__ import annotations
import argparse
import logging
from logging.handlers import RotatingFileHandler
import os
from pathlib import Path
import shutil
import subprocess
import sys
import threading
import time
import webbrowser


def data_directory():
    if os.environ.get('YINGXIA_DATA_DIR'):return Path(os.environ['YINGXIA_DATA_DIR']).expanduser().resolve()
    if sys.platform=='win32':return Path(os.environ.get('LOCALAPPDATA',str(Path.home()/'AppData'/'Local')))/'YingXia'
    return Path(os.environ.get('XDG_DATA_HOME',str(Path.home()/'.local'/'share')))/'yingxia'


def open_app(url:str,browser:str='auto'):
    """Launch a separate app window without importing or sharing the normal profile."""
    profile=data_directory()/'browser-profile'
    paths=[]
    if sys.platform=='win32':
        roots=[os.environ.get('PROGRAMFILES(X86)',''),os.environ.get('PROGRAMFILES',''),os.environ.get('LOCALAPPDATA','')]
        targets=['Microsoft/Edge/Application/msedge.exe','Google/Chrome/Application/chrome.exe']
        if browser=='chrome':targets.reverse()
        for target in targets:
            for base in roots:
                if base and (p:=Path(base)/target).is_file():paths.append(str(p))
    else:
        paths=[p for name in ('microsoft-edge','google-chrome','chromium','chromium-browser') if (p:=shutil.which(name))]
    if paths and browser!='default':
        try:
            subprocess.Popen([paths[0],f'--app={url}',f'--user-data-dir={profile}',
                              '--no-first-run','--no-default-browser-check','--window-size=1340,900'],
                             stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            return
        except OSError:pass
    # Explicit fallback is a normal browser tab. Third-party frames still cannot access
    # the loopback application's DOM because of same-origin policy and its sandbox.
    webbrowser.open(url,new=1)


def main(argv=None):
    parser=argparse.ArgumentParser(description='映匣 v1.0 本机影片目录与播放工作台')
    parser.add_argument('--no-browser',action='store_true',help='仅启动服务，用于调试和自动测试')
    parser.add_argument('--port',type=int,default=0,help='0 为系统分配空闲端口')
    parser.add_argument('--data-dir',type=Path,help='覆盖用户数据目录')
    parser.add_argument('--browser',choices=['auto','chrome','default'],default='auto')
    args=parser.parse_args(argv)
    if not 0<=args.port<=65535:parser.error('port 应为 0–65535')
    if args.data_dir:os.environ['YINGXIA_DATA_DIR']=str(args.data_dir.resolve())
    folder=data_directory();folder.mkdir(parents=True,exist_ok=True)
    log=RotatingFileHandler(folder/'app.log',maxBytes=1_000_000,backupCount=2,encoding='utf-8')
    log.setFormatter(logging.Formatter('%(asctime)s %(levelname)s %(message)s'))
    logging.basicConfig(level=logging.WARNING,handlers=[log])
    from yingxia.service import Service
    from yingxia.server import Server
    try:service=Service(folder);server=Server(service,args.port)
    except Exception as e:
        logging.exception('Startup failed');print('启动失败：',str(e));return 1
    print('映匣 v1.0 已启动。仅监听本机 127.0.0.1。',flush=True)
    print('APP_URL='+server.origin,flush=True)
    print('数据目录：'+str(folder),flush=True)
    print('使用软件左下角「退出」停止服务。命令行模式也可按 Ctrl+C。',flush=True)
    if not args.no_browser:open_app(server.origin,args.browser)
    def watchdog():
        while True:
            time.sleep(15)
            if server.had_client and time.monotonic()-server.last_seen>300:
                server.shutdown_later();return
    if not args.no_browser:threading.Thread(target=watchdog,daemon=True).start()
    try:server.serve_forever(poll_interval=.25)
    except KeyboardInterrupt:pass
    finally:server.server_close();service.store.close()
    return 0

if __name__=='__main__':
    if sys.version_info<(3,10):print('请使用 Python 3.10 或更新版本。');raise SystemExit(1)
    raise SystemExit(main())
