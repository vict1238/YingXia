"""Compile and numerically check shipped FSR1 shaders on bundled ANGLE.

Uses a private pbuffer and synthetic RGB test texture; does not open the app,
touch playback state, or infer integrated-GPU/video throughput from shader QA.
"""
from __future__ import annotations

import argparse
import ctypes as C
import hashlib
import json
import os
from pathlib import Path
import re
import time

import numpy as np


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


class Angle:
    def __init__(self, dll_dir: Path, version: int):
        self._dll_path = os.add_dll_directory(str(dll_dir.resolve()))
        self.egl = C.WinDLL(str(dll_dir / "libEGL.dll"))
        self.gl = C.WinDLL(str(dll_dir / "libGLESv2.dll"))
        self.version = version
        self.display = self.surface = self.context = None
        self.resources: list[tuple[str, int]] = []
        self.fn(self.egl, "eglGetProcAddress", C.c_void_p, [C.c_char_p])
        self.fn(self.egl, "eglInitialize", C.c_uint, [C.c_void_p, C.c_void_p, C.c_void_p])
        self.fn(self.egl, "eglChooseConfig", C.c_uint, [C.c_void_p, C.POINTER(C.c_int), C.POINTER(C.c_void_p), C.c_int, C.POINTER(C.c_int)])
        self.fn(self.egl, "eglCreateContext", C.c_void_p, [C.c_void_p, C.c_void_p, C.c_void_p, C.POINTER(C.c_int)])
        self.fn(self.egl, "eglCreatePbufferSurface", C.c_void_p, [C.c_void_p, C.c_void_p, C.POINTER(C.c_int)])
        self.fn(self.egl, "eglMakeCurrent", C.c_uint, [C.c_void_p, C.c_void_p, C.c_void_p, C.c_void_p])
        self.fn(self.egl, "eglDestroyContext", C.c_uint, [C.c_void_p, C.c_void_p])
        self.fn(self.egl, "eglDestroySurface", C.c_uint, [C.c_void_p, C.c_void_p])
        self.fn(self.egl, "eglTerminate", C.c_uint, [C.c_void_p])
        self.fn(self.egl, "eglGetError", C.c_int, [])
        platform = C.WINFUNCTYPE(C.c_void_p, C.c_uint, C.c_void_p, C.POINTER(C.c_int))(self.egl.eglGetProcAddress(b"eglGetPlatformDisplayEXT"))
        # ANGLE D3D11 hardware, as used by the production Windows plugin.
        self.display = platform(0x3202, None, self.ints(0x3203, 0x3208, 0x3038))
        assert self.egl.eglInitialize(self.display, None, None), "EGL initialize"
        attrs = self.ints(0x3024, 8, 0x3023, 8, 0x3022, 8, 0x3021, 8,
                          0x3033, 1, 0x3040, 0x40 if version == 3 else 4, 0x3038)
        config = C.c_void_p()
        count = C.c_int()
        assert self.egl.eglChooseConfig(self.display, attrs, C.byref(config), 1, C.byref(count)) and count.value, "EGL config"
        self.context = self.egl.eglCreateContext(self.display, config, None, self.ints(0x3098, version, 0x3038))
        assert self.context, f"EGL ES{version} context: {self.egl.eglGetError():x}"
        self.surface = self.egl.eglCreatePbufferSurface(self.display, config, self.ints(0x3057, 1, 0x3056, 1, 0x3038))
        assert self.surface and self.egl.eglMakeCurrent(self.display, self.surface, self.surface, self.context), "EGL current"
        signatures = {
            "glGetString": (C.c_char_p, [C.c_uint]),
            "glGetError": (C.c_uint, []),
            "glCreateShader": (C.c_uint, [C.c_uint]),
            "glShaderSource": (None, [C.c_uint, C.c_int, C.POINTER(C.c_char_p), C.c_void_p]),
            "glCompileShader": (None, [C.c_uint]),
            "glGetShaderiv": (None, [C.c_uint, C.c_uint, C.POINTER(C.c_int)]),
            "glGetShaderInfoLog": (None, [C.c_uint, C.c_int, C.c_void_p, C.c_void_p]),
            "glCreateProgram": (C.c_uint, []),
            "glAttachShader": (None, [C.c_uint, C.c_uint]),
            "glBindAttribLocation": (None, [C.c_uint, C.c_uint, C.c_char_p]),
            "glLinkProgram": (None, [C.c_uint]),
            "glGetProgramiv": (None, [C.c_uint, C.c_uint, C.POINTER(C.c_int)]),
            "glGetProgramInfoLog": (None, [C.c_uint, C.c_int, C.c_void_p, C.c_void_p]),
            "glUseProgram": (None, [C.c_uint]),
            "glGetUniformLocation": (C.c_int, [C.c_uint, C.c_char_p]),
            "glUniform1i": (None, [C.c_int, C.c_int]),
            "glUniform2f": (None, [C.c_int, C.c_float, C.c_float]),
            "glGenTextures": (None, [C.c_int, C.POINTER(C.c_uint)]),
            "glBindTexture": (None, [C.c_uint, C.c_uint]),
            "glTexParameteri": (None, [C.c_uint, C.c_uint, C.c_int]),
            "glTexImage2D": (None, [C.c_uint, C.c_int, C.c_int, C.c_int, C.c_int, C.c_int, C.c_uint, C.c_uint, C.c_void_p]),
            "glGenFramebuffers": (None, [C.c_int, C.POINTER(C.c_uint)]),
            "glBindFramebuffer": (None, [C.c_uint, C.c_uint]),
            "glFramebufferTexture2D": (None, [C.c_uint, C.c_uint, C.c_uint, C.c_uint, C.c_int]),
            "glCheckFramebufferStatus": (C.c_uint, [C.c_uint]),
            "glGenBuffers": (None, [C.c_int, C.POINTER(C.c_uint)]),
            "glBindBuffer": (None, [C.c_uint, C.c_uint]),
            "glBufferData": (None, [C.c_uint, C.c_ssize_t, C.c_void_p, C.c_uint]),
            "glEnableVertexAttribArray": (None, [C.c_uint]),
            "glVertexAttribPointer": (None, [C.c_uint, C.c_int, C.c_uint, C.c_ubyte, C.c_int, C.c_void_p]),
            "glViewport": (None, [C.c_int, C.c_int, C.c_int, C.c_int]),
            "glDrawArrays": (None, [C.c_uint, C.c_int, C.c_int]),
            "glReadPixels": (None, [C.c_int, C.c_int, C.c_int, C.c_int, C.c_uint, C.c_uint, C.c_void_p]),
            "glFinish": (None, []),
            "glDeleteTextures": (None, [C.c_int, C.POINTER(C.c_uint)]),
            "glDeleteFramebuffers": (None, [C.c_int, C.POINTER(C.c_uint)]),
            "glDeleteBuffers": (None, [C.c_int, C.POINTER(C.c_uint)]),
            "glDeleteProgram": (None, [C.c_uint]),
            "glDeleteShader": (None, [C.c_uint]),
        }
        for name, (restype, args) in signatures.items():
            self.fn(self.gl, name, restype, args)
        self.actual_version = self.gl.glGetString(0x1F02).decode()
        self.renderer = self.gl.glGetString(0x1F01).decode()

    @staticmethod
    def fn(lib, name, restype, args):
        function = getattr(lib, name)
        function.restype, function.argtypes = restype, args
        return function

    @staticmethod
    def ints(*args):
        return (C.c_int * len(args))(*args)

    def shader(self, source: str, kind: int) -> int:
        shader = self.gl.glCreateShader(kind)
        text = C.c_char_p(source.encode())
        self.gl.glShaderSource(shader, 1, C.byref(text), None)
        self.gl.glCompileShader(shader)
        status = C.c_int()
        self.gl.glGetShaderiv(shader, 0x8B81, C.byref(status))
        log = C.create_string_buffer(32768)
        self.gl.glGetShaderInfoLog(shader, len(log), None, log)
        assert status.value, log.value.decode()
        self.resources.append(("Shader", shader))
        return shader

    def program(self, body: str) -> int:
        if self.version == 3:
            vertex = "#version 300 es\nprecision highp float; in vec2 position; out vec2 HOOKED_pos; void main(){HOOKED_pos=(position+1.0)*0.5;gl_Position=vec4(position,0.0,1.0);}"
            prefix = "#version 300 es\nprecision highp float; uniform sampler2D input_tex; uniform vec2 HOOKED_size, HOOKED_pt; in vec2 HOOKED_pos; out vec4 result;\n#define HOOKED_tex(p) texture(input_tex,p)\n"
            suffix = "\nvoid main(){result=hook();}"
        else:
            vertex = "#version 100\nprecision highp float; attribute vec2 position; varying vec2 HOOKED_pos; void main(){HOOKED_pos=(position+1.0)*0.5;gl_Position=vec4(position,0.0,1.0);}"
            prefix = "#version 100\nprecision highp float; uniform sampler2D input_tex; uniform vec2 HOOKED_size, HOOKED_pt; varying vec2 HOOKED_pos;\n#define HOOKED_tex(p) texture2D(input_tex,p)\n"
            suffix = "\nvoid main(){gl_FragColor=hook();}"
        program = self.gl.glCreateProgram()
        self.resources.append(("Program", program))
        self.gl.glAttachShader(program, self.shader(vertex, 0x8B31))
        self.gl.glAttachShader(program, self.shader(prefix + body + suffix, 0x8B30))
        self.gl.glBindAttribLocation(program, 0, b"position")
        self.gl.glLinkProgram(program)
        status = C.c_int()
        self.gl.glGetProgramiv(program, 0x8B82, C.byref(status))
        log = C.create_string_buffer(32768)
        self.gl.glGetProgramInfoLog(program, len(log), None, log)
        assert status.value, log.value.decode()
        return program

    def texture(self, pixels: np.ndarray) -> int:
        height, width, _ = pixels.shape
        texture = C.c_uint()
        self.gl.glGenTextures(1, C.byref(texture))
        self.resources.append(("Textures", texture.value))
        self.gl.glBindTexture(0x0DE1, texture.value)
        for name in (0x2801, 0x2800):
            self.gl.glTexParameteri(0x0DE1, name, 0x2601)
        for name in (0x2802, 0x2803):
            self.gl.glTexParameteri(0x0DE1, name, 0x812F)
        self.gl.glTexImage2D(0x0DE1, 0, 0x1908, width, height, 0, 0x1908, 0x1401, pixels.ctypes.data)
        return texture.value

    def render(self, program: int, pixels: np.ndarray, width: int, height: int, iterations=1) -> tuple[np.ndarray, list[float]]:
        input_texture = self.texture(pixels)
        output = np.zeros((height, width, 4), dtype=np.uint8)
        output_texture = self.texture(output)
        fbo = C.c_uint()
        self.gl.glGenFramebuffers(1, C.byref(fbo))
        self.resources.append(("Framebuffers", fbo.value))
        self.gl.glBindFramebuffer(0x8D40, fbo.value)
        self.gl.glFramebufferTexture2D(0x8D40, 0x8CE0, 0x0DE1, output_texture, 0)
        assert self.gl.glCheckFramebufferStatus(0x8D40) == 0x8CD5
        self.gl.glBindTexture(0x0DE1, input_texture)
        self.gl.glUseProgram(program)
        self.gl.glUniform1i(self.gl.glGetUniformLocation(program, b"input_tex"), 0)
        self.gl.glUniform2f(self.gl.glGetUniformLocation(program, b"HOOKED_size"), pixels.shape[1], pixels.shape[0])
        self.gl.glUniform2f(self.gl.glGetUniformLocation(program, b"HOOKED_pt"), 1 / pixels.shape[1], 1 / pixels.shape[0])
        buffer = C.c_uint()
        self.gl.glGenBuffers(1, C.byref(buffer))
        self.resources.append(("Buffers", buffer.value))
        self.gl.glBindBuffer(0x8892, buffer.value)
        vertices = np.array([-1, -1, 3, -1, -1, 3], dtype=np.float32)
        self.gl.glBufferData(0x8892, vertices.nbytes, vertices.ctypes.data, 0x88E4)
        self.gl.glEnableVertexAttribArray(0)
        self.gl.glVertexAttribPointer(0, 2, 0x1406, 0, 0, None)
        self.gl.glViewport(0, 0, width, height)
        timings = []
        for _ in range(iterations):
            start = time.perf_counter()
            self.gl.glDrawArrays(4, 0, 3)
            self.gl.glFinish()
            timings.append((time.perf_counter() - start) * 1000)
        self.gl.glReadPixels(0, 0, width, height, 0x1908, 0x1401, output.ctypes.data)
        assert self.gl.glGetError() == 0, "OpenGL error"
        return output, timings

    def close(self):
        for kind, identifier in reversed(self.resources):
            function = getattr(self.gl, "glDelete" + kind)
            if kind in ("Program", "Shader"):
                function(identifier)
            else:
                item = C.c_uint(identifier)
                function(1, C.byref(item))
        self.egl.eglMakeCurrent(self.display, None, None, None)
        self.egl.eglDestroySurface(self.display, self.surface)
        self.egl.eglDestroyContext(self.display, self.context)
        self.egl.eglTerminate(self.display)
        self._dll_path.close()


def passes(path: Path) -> list[tuple[str, str]]:
    chunks = re.split(r"(?=^//!HOOK )", path.read_text(encoding="utf-8"), flags=re.M)[1:]
    result = []
    for chunk in chunks:
        name = re.search(r"^//!DESC (.+)$", chunk, flags=re.M).group(1)
        body = re.sub(r"^//!.*\n", "", chunk, flags=re.M)
        result.append((name, body))
    return result


def fixture(width=1280, height=720):
    y, x = np.mgrid[:height, :width]
    rgb = np.stack((x * 255 // (width - 1), y * 255 // (height - 1), ((x // 7 + y // 5) % 2) * 192 + 31), axis=-1).astype(np.uint8)
    line = ((x + 2 * y) % 71) < 2
    rgb[line] = (240, 232, 217)
    rgb[:40, :40] = 0
    rgb[:40, -40:] = 255
    rgb[-40:, :40] = 128
    return np.concatenate((rgb, np.full((height, width, 1), 255, dtype=np.uint8)), axis=-1)


def fetch(image, x, y):
    return image[np.clip(y, 0, image.shape[0] - 1), np.clip(x, 0, image.shape[1] - 1), :3].astype(np.float64) / 255


def easu_reference(image, x, y, width=1920, height=1080):
    p = np.stack(((x + 0.5) * image.shape[1] / width - 0.5,
                  (y + 0.5) * image.shape[0] / height - 0.5), axis=-1)
    fp = np.floor(p).astype(int)
    pp = p - fp
    offsets = {"b": (0, -1), "c": (1, -1), "e": (-1, 0), "f": (0, 0), "g": (1, 0), "h": (2, 0), "i": (-1, 1), "j": (0, 1), "k": (1, 1), "l": (2, 1), "n": (0, 2), "o": (1, 2)}
    colors = {name: fetch(image, fp[:, 0] + off[0], fp[:, 1] + off[1]) for name, off in offsets.items()}
    luma = {name: color[:, 1] + (color[:, 0] + color[:, 2]) * 0.5 for name, color in colors.items()}
    weights = [(1-pp[:,0])*(1-pp[:,1]), pp[:,0]*(1-pp[:,1]), (1-pp[:,0])*pp[:,1], pp[:,0]*pp[:,1]]
    direction = np.zeros_like(pp)
    edge = np.zeros(len(x))
    for names, weight in zip(["befgj", "cfghk", "fijkn", "gjklo"], weights):
        a, b, c, d, e = (luma[n] for n in names)
        dx, dy = d-b, e-a
        direction += np.stack((dx, dy), axis=-1) * weight[:, None]
        lx = np.clip(abs(dx)/np.maximum(np.maximum(abs(d-c), abs(c-b)), 1e-8), 0, 1)
        ly = np.clip(abs(dy)/np.maximum(np.maximum(abs(e-c), abs(c-a)), 1e-8), 0, 1)
        edge += (lx**2 + ly**2) * weight
    norm2 = (direction**2).sum(axis=-1)
    zero = norm2 < 1/32768
    direction[zero] = (1,0)
    direction[~zero] /= np.sqrt(norm2[~zero,None])
    edge = (edge*0.5)**2
    diag = (direction**2).sum(axis=-1) / np.maximum(abs(direction).max(axis=-1), 1e-8)
    stretch = np.stack((1+(diag-1)*edge, 1-0.5*edge), axis=-1)
    lobe = 0.5 + ((0.25-0.04)-0.5)*edge
    accum = np.zeros((len(x),3))
    weight_sum = np.zeros(len(x))
    for name, offset in offsets.items():
        off = np.array(offset)-pp
        v = np.stack(((off*direction).sum(axis=-1), (off*np.stack((-direction[:,1],direction[:,0]),axis=-1)).sum(axis=-1)),axis=-1)*stretch
        distance = np.minimum((v**2).sum(axis=-1), 1/lobe)
        w = ((25/16)*(0.4*distance-1)**2-9/16)*(lobe*distance-1)**2
        accum += colors[name]*w[:,None]
        weight_sum += w
    nearby = np.stack([colors[n] for n in "fgjk"])
    return np.clip(accum/np.maximum(weight_sum[:,None],1e-8), nearby.min(axis=0), nearby.max(axis=0))


def rcas_reference(image, x, y, sharpness):
    b,d,e,f,h = [fetch(image,x+dx,y+dy) for dx,dy in [(0,-1),(-1,0),(0,0),(1,0),(0,1)]]
    lumas = np.stack([c[:,1]+0.5*(c[:,0]+c[:,2]) for c in [b,d,e,f,h]])
    bl,dl,el,fl,hl = lumas
    noise = 1 - 0.5*np.clip(abs(0.25*(bl+dl+fl+hl)-el)/np.maximum(lumas.max(axis=0)-lumas.min(axis=0),1e-8),0,1)
    ring = np.stack([b,d,f,h])
    mn,mx = ring.min(axis=0),ring.max(axis=0)
    hit_min = np.minimum(mn,e)/np.maximum(4*mx,1e-8)
    hit_max = (1-np.maximum(mx,e))/np.minimum(4*mn-4,-1e-8)
    lobe = np.maximum(-0.1875,np.minimum(np.maximum(-hit_min,hit_max).max(axis=-1),0))*2**(-sharpness)*noise
    return np.clip((lobe[:,None]*(b+d+f+h)+e)/(4*lobe[:,None]+1),0,1)


def comparison(actual, reference, x, y):
    error = abs(actual[y,x,:3].astype(np.float64)/255-reference)
    max_error = float(error.max())
    assert max_error <= 2/255 + 1e-7, f"RGB reference mismatch {max_error}"
    return {"sampled_pixels": len(x), "max_absolute_rgb_error": max_error,
            "mean_absolute_rgb_error": float(error.mean()), "tolerance": 2/255, "passed": True}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--dll-dir", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    shader_root = Path(__file__).resolve().parents[1] / "assets/shaders"
    source = fixture()
    rng = np.random.default_rng(20261014)
    x = np.concatenate((rng.integers(0,1920,8192), np.array([0,1,2,1917,1918,1919]*3)))
    y = np.concatenate((rng.integers(0,1080,8192), np.repeat([0,539,1079],6)))
    result = {"scope": "Synthetic 1280x720 SDR RGBA8 texture -> 1920x1080 private ANGLE pbuffer. Compiler, pixel algorithm and isolated draw/finish timing only; not video playback or integrated-GPU throughput.",
              "upstream_commit": "a21ffb8f6c13233ba336352bdff293894c706575",
              "assets": {p.name: sha256(p) for p in shader_root.glob("*.glsl")},
              "angle_libraries": {p.name: sha256(p) for p in [args.dll_dir/"libEGL.dll",args.dll_dir/"libGLESv2.dll"]},
              "contexts": []}
    for version in [2,3]:
        angle = Angle(args.dll_dir.resolve(), version)
        try:
            context = {"requested_es_version":version,"actual_gl_version":angle.actual_version,"renderer":angle.renderer,"passes":[]}
            enhanced = passes(shader_root / "fsr1_enhanced.glsl")
            name,body = enhanced[0]
            easu, timings = angle.render(angle.program(body),source,1920,1080,iterations=8)
            context["passes"].append({"description":name, "compiled_and_linked":True, "draw_finish_ms":timings, "rgb":comparison(easu,easu_reference(source,x,y),x,y)})
            name,body = enhanced[1]
            output,timings = angle.render(angle.program(body),easu,1920,1080,iterations=8)
            context["passes"].append({"description":name,"compiled_and_linked":True,"draw_finish_ms":timings,"rgb":comparison(output,rcas_reference(easu,x,y,1.0),x,y)})
            bilinear,_ = angle.render(angle.program("vec4 hook(){return HOOKED_tex(HOOKED_pos);}"),source,1920,1080)
            name,body = passes(shader_root / "fsr1_light.glsl")[0]
            light,timings = angle.render(angle.program(body),bilinear,1920,1080,iterations=8)
            context["passes"].append({"description":name,"compiled_and_linked":True,"draw_finish_ms":timings,"rgb":comparison(light,rcas_reference(bilinear,x,y,1.5),x,y)})
            assert not np.array_equal(output,bilinear), "EASU+RCAS must differ from bilinear"
            context["enhanced_differs_from_bilinear"] = True
            context["flat_black_white_gray_regions_stable"] = all(np.all(output[y0,x0,:3] == c) for y0,x0,c in [(10,10,0),(10,1910,255),(1070,10,128)])
            assert context["flat_black_white_gray_regions_stable"]
            result["contexts"].append(context)
        finally:
            angle.close()
    result["passed"] = True
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(result,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
    print(json.dumps({"passed":True,"output":str(args.output),"contexts":[c["renderer"] for c in result["contexts"]]},ensure_ascii=False))


if __name__ == "__main__":
    main()
