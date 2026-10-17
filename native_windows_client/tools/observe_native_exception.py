"""Read-only debug exception/stack observer for this task's own native runner.

No registry changes, external debugger installation, memory dump or user data.
Only attaches after verifying the exact workspace runner path.
"""
import argparse
import ctypes as C
from ctypes import wintypes as W
import json
import os
from pathlib import Path
import struct
import time

K = C.WinDLL("kernel32", use_last_error=True)
P = C.WinDLL("psapi", use_last_error=True)
D = C.WinDLL("dbghelp", use_last_error=True)
HANDLE = C.c_void_p
ULONG64 = C.c_ulonglong


class ExceptionRecord(C.Structure):
    _fields_ = [("code", W.DWORD), ("flags", W.DWORD), ("record", HANDLE), ("address", HANDLE),
                ("parameter_count", W.DWORD), ("parameters", ULONG64 * 15)]


class ExceptionInfo(C.Structure):
    _fields_ = [("record", ExceptionRecord), ("first_chance", W.DWORD)]


class DebugUnion(C.Union):
    _fields_ = [("exception", ExceptionInfo), ("raw", C.c_byte * 160)]


class DebugEvent(C.Structure):
    _fields_ = [("event", W.DWORD), ("pid", W.DWORD), ("tid", W.DWORD), ("info", DebugUnion)]


class ModuleInfo(C.Structure):
    _fields_ = [("base", HANDLE), ("size", W.DWORD), ("entry", HANDLE)]


class Address(C.Structure):
    _fields_ = [("offset", ULONG64), ("segment", W.WORD), ("mode", W.DWORD)]


class StackFrame(C.Structure):
    # Current/older Windows STACKFRAME64 tails fit within this extra allocation.
    _fields_ = [("pc", Address), ("ret", Address), ("frame", Address),
                ("stack", Address), ("backing", Address), ("remainder", C.c_byte * 512)]


K.OpenProcess.argtypes = [W.DWORD, W.BOOL, W.DWORD]
K.OpenProcess.restype = HANDLE
K.QueryFullProcessImageNameW.argtypes = [HANDLE, W.DWORD, W.LPWSTR, C.POINTER(W.DWORD)]
K.OpenThread.argtypes = [W.DWORD, W.BOOL, W.DWORD]
K.OpenThread.restype = HANDLE
K.GetThreadContext.argtypes = [HANDLE, HANDLE]
K.ReadProcessMemory.argtypes = [HANDLE, HANDLE, HANDLE, C.c_size_t, C.POINTER(C.c_size_t)]
K.CloseHandle.argtypes = [HANDLE]
K.DebugActiveProcess.argtypes = [W.DWORD]
K.DebugActiveProcessStop.argtypes = [W.DWORD]
K.DebugSetProcessKillOnExit.argtypes = [W.BOOL]
K.WaitForDebugEvent.argtypes = [C.POINTER(DebugEvent), W.DWORD]
K.ContinueDebugEvent.argtypes = [W.DWORD, W.DWORD, W.DWORD]
P.EnumProcessModulesEx.argtypes = [HANDLE, C.POINTER(HANDLE), W.DWORD, C.POINTER(W.DWORD), W.DWORD]
P.GetModuleInformation.argtypes = [HANDLE, HANDLE, C.POINTER(ModuleInfo), W.DWORD]
P.GetModuleFileNameExW.argtypes = [HANDLE, HANDLE, W.LPWSTR, W.DWORD]
D.SymInitializeW.argtypes = [HANDLE, W.LPCWSTR, W.BOOL]
D.SymCleanup.argtypes = [HANDLE]
D.StackWalk64.argtypes = [W.DWORD, HANDLE, HANDLE, C.POINTER(StackFrame), HANDLE, HANDLE, HANDLE, HANDLE, HANDLE]


def modules(process):
    entries = (HANDLE * 1024)()
    needed = W.DWORD()
    result = []
    if P.EnumProcessModulesEx(process, entries, C.sizeof(entries), C.byref(needed), 3):
        for module in entries[:min(1024, needed.value // C.sizeof(HANDLE))]:
            detail = ModuleInfo()
            name = C.create_unicode_buffer(32768)
            if P.GetModuleInformation(process, module, C.byref(detail), C.sizeof(detail)):
                P.GetModuleFileNameExW(process, module, name, len(name))
                result.append({"name": Path(name.value).name, "base": int(detail.base or 0), "size": detail.size})
    return result


def locate(address, loaded):
    row = next((m for m in loaded if m["base"] <= address < m["base"] + m["size"]), None)
    return {"address": hex(address), "module": row["name"] if row else None,
            "module_offset": hex(address - row["base"]) if row else None}


def read(process, address, size):
    buffer = C.create_string_buffer(size)
    used = C.c_size_t()
    if not K.ReadProcessMemory(process, address, buffer, size, C.byref(used)):
        raise OSError(C.get_last_error(), "Read-only exception metadata unavailable")
    return buffer.raw[:used.value]


def exception_types(process, record):
    # Read only compiler-generated type metadata, never the thrown object's data.
    if record.code != 0xE06D7363 or record.parameter_count < 4:
        return []
    base, throw_info = record.parameters[3], record.parameters[2]
    try:
        array = base + struct.unpack_from("<I", read(process, throw_info, 16), 12)[0]
        count = struct.unpack("<I", read(process, array, 4))[0]
        if not 1 <= count <= 32:
            return []
        types = []
        for index in range(count):
            catchable = base + struct.unpack("<I", read(process, array + 4 + index * 4, 4))[0]
            descriptor = base + struct.unpack_from("<I", read(process, catchable, 8), 4)[0]
            name = read(process, descriptor + 16, 256).split(b"\0", 1)[0].decode("ascii", "replace")
            types.append(name)
        return types
    except (OSError, struct.error):
        return []


def stack(process, thread_id, loaded, search):
    thread = K.OpenThread(0x48, False, thread_id)
    if not thread:
        return {"error": f"OpenThread:{C.get_last_error()}"}
    initialized = False
    try:
        storage = C.create_string_buffer(1280)
        context = (C.addressof(storage) + 15) & ~15
        C.c_uint32.from_address(context + 48).value = 0x100003
        if not K.GetThreadContext(thread, context):
            return {"error": f"GetThreadContext:{C.get_last_error()}"}
        rip = C.c_uint64.from_address(context + 248).value
        rsp = C.c_uint64.from_address(context + 152).value
        rbp = C.c_uint64.from_address(context + 160).value
        initialized = bool(D.SymInitializeW(process, search, True))
        rows = [locate(rip, loaded)]
        frame = StackFrame()
        frame.pc.offset, frame.pc.mode = rip, 3
        frame.stack.offset, frame.stack.mode = rsp, 3
        frame.frame.offset, frame.frame.mode = rbp, 3
        if initialized:
            for _ in range(35):
                ok = D.StackWalk64(0x8664, process, thread, C.byref(frame), context, None,
                                  C.cast(D.SymFunctionTableAccess64, HANDLE), C.cast(D.SymGetModuleBase64, HANDLE), None)
                if not ok or not frame.pc.offset:
                    break
                row = locate(frame.pc.offset, loaded)
                if not rows or row != rows[-1]:
                    rows.append(row)
        return {"rip": hex(rip), "rsp": hex(rsp), "rbp": hex(rbp), "frames": rows,
                "symbol_engine_initialized": initialized}
    finally:
        if initialized:
            D.SymCleanup(process)
        K.CloseHandle(thread)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--pid", type=int, required=True)
    parser.add_argument("--exe", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    client = Path(__file__).resolve().parents[1]
    expected = (client / "build/windows/native_bundle/yingxia_native.exe").resolve()
    if Path(args.exe).resolve() != expected:
        raise RuntimeError("Only the exact owned workspace runner is allowed")
    output = Path(args.out).resolve()
    if not output.is_relative_to((client / "evidence").resolve()):
        raise RuntimeError("Diagnostic output must stay in this client evidence")
    process = K.OpenProcess(0x0410, False, args.pid)
    if not process:
        raise C.WinError(C.get_last_error())
    attached = False
    report = {"owned_runner": expected.name, "pid": args.pid, "debug_api_only": True,
              "no_memory_dump": True, "no_os_configuration_changes": True, "exceptions": []}
    try:
        name = C.create_unicode_buffer(32768)
        length = W.DWORD(len(name))
        if not K.QueryFullProcessImageNameW(process, 0, name, C.byref(length)) or Path(name.value).resolve() != expected:
            raise RuntimeError("Process image does not match the owned workspace runner")
        if not K.DebugActiveProcess(args.pid):
            raise C.WinError(C.get_last_error())
        attached = True
        K.DebugSetProcessKillOnExit(False)
        print(json.dumps({"debug_attached": True, "pid": args.pid}), flush=True)
        end = time.monotonic() + 100
        while time.monotonic() < end:
            event = DebugEvent()
            if not K.WaitForDebugEvent(C.byref(event), 250):
                continue
            status = 0x00010002
            if event.event == 1:
                info = event.info.exception
                record = info.record
                code = record.code
                status = 0x00010002 if code in (0x80000003, 0x80000004) else 0x80010001
                if code not in (0x80000003, 0x80000004):
                    loaded = modules(process)
                    row = {"code": hex(code), "first_chance": bool(info.first_chance), "thread_id": event.tid,
                           "exception": locate(int(record.address or 0), loaded),
                           "parameters": [hex(record.parameters[i]) for i in range(min(record.parameter_count, 15))]}
                    if code == 0xE06D7363:
                        row["cpp_exception_types"] = exception_types(process, record)
                        row["throw_info_module"] = locate(record.parameters[2], loaded)
                    if not info.first_chance or code in (0xC0000409, 0xC0000005, 0xE06D7363):
                        row["stack"] = stack(process, event.tid, loaded, str(expected.parent))
                    report["exceptions"].append(row)
                    output.parent.mkdir(parents=True, exist_ok=True)
                    output.write_text(json.dumps(report, indent=2), encoding="utf8")
            elif event.event == 5:
                report["process_exit_code"] = struct.unpack_from("<I", bytes(event.info.raw))[0]
                K.ContinueDebugEvent(event.pid, event.tid, status)
                break
            K.ContinueDebugEvent(event.pid, event.tid, status)
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, indent=2), encoding="utf8")
    finally:
        if attached:
            K.DebugActiveProcessStop(args.pid)
        K.CloseHandle(process)


if __name__ == "__main__":
    main()
