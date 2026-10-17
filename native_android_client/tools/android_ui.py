"""Bounded UI QA for the project's owned Android emulator; no real phones."""
from __future__ import annotations
import argparse
import re
import subprocess
import time
import xml.etree.ElementTree as ET
from pathlib import Path

CLIENT = Path(__file__).resolve().parents[1]
ROOT = CLIENT.parent
ADB = ROOT / '.tools/native/android_sdk/platform-tools/adb.exe'
SERIAL = 'emulator-5560'

def adb(*args: str, binary=False, timeout=60):
    result = subprocess.run([str(ADB), '-s', SERIAL, *args], capture_output=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError(result.stderr.decode('utf-8', 'replace')[:600])
    return result.stdout if binary else result.stdout.decode('utf-8', 'replace')

def tree():
    adb('shell', 'uiautomator', 'dump', '/sdcard/yingxia_22_window.xml')
    raw = adb('shell', 'cat', '/sdcard/yingxia_22_window.xml')
    return raw, ET.fromstring(raw)

def bounds(node):
    values = list(map(int, re.findall(r'\d+', node.attrib['bounds'])))
    return values

def text_nodes():
    _, root = tree()
    return [n for n in root.iter('node') if n.get('text') or n.get('content-desc')]

def describe():
    return [{'text': n.get('text') or n.get('content-desc'), 'bounds': n.get('bounds'), 'clickable': n.get('clickable')} for n in text_nodes()]

def tap(label, *, contains=False, index=0):
    matches = []
    for node in text_nodes():
        value = node.get('text') or node.get('content-desc') or ''
        if (label in value) if contains else (label == value):
            matches.append(node)
    if not matches:
        raise LookupError(f'UI text not found: {label}')
    buttons = [node for node in matches if node.get('clickable') == 'true']
    if buttons:
        matches = buttons
    # Dropdowns include a screen-sized barrier also named "关闭". Prefer the
    # actual small option/button over that dismiss barrier.
    matches.sort(key=lambda n: (bounds(n)[2]-bounds(n)[0])*(bounds(n)[3]-bounds(n)[1]))
    x1, y1, x2, y2 = bounds(matches[index])
    adb('shell', 'input', 'tap', str((x1+x2)//2), str((y1+y2)//2))
    time.sleep(.5)

def snapshot(name, directory=None):
    destination = directory or CLIENT / 'evidence/android/runtime'
    destination.mkdir(parents=True, exist_ok=True)
    raw, _ = tree()
    (destination / f'{name}.xml').write_text(raw, encoding='utf-8')
    (destination / f'{name}.png').write_bytes(adb('exec-out', 'screencap', '-p', binary=True))
    return describe()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['list', 'tap', 'snapshot', 'swipe', 'text', 'back'])
    parser.add_argument('value', nargs='?', default='screen')
    parser.add_argument('--contains', action='store_true')
    args = parser.parse_args()
    if args.action == 'list': print(describe())
    elif args.action == 'tap': tap(args.value, contains=args.contains)
    elif args.action == 'snapshot': print(snapshot(args.value))
    elif args.action == 'swipe': adb('shell', 'input', 'swipe', *args.value.split(','), '400')
    elif args.action == 'text': adb('shell', 'input', 'text', args.value)
    elif args.action == 'back': adb('shell', 'input', 'keyevent', '4')

if __name__ == '__main__': main()
