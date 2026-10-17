// AMD FidelityFX Super Resolution 1, EASU + RCAS, adapted for mpv fragment hooks.
// Upstream: GPUOpen-Effects/FidelityFX-FSR a21ffb8f6c13233ba336352bdff293894c706575.
// Uses the official full 12-tap EASU kernel and five-tap denoised RCAS.
// RGB point samples replace per-channel textureGather; exact guarded reciprocal
// and inverse square root replace bitwise approximations for GLSL ES 1.00/3.00.
// The input must be perceptual SDR RGB. UI policy excludes HDR PQ/HLG inputs.
//
// Copyright (c) 2021 Advanced Micro Devices, Inc. All rights reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

//!HOOK MAIN
//!BIND HOOKED
//!DESC YingXia FSR1 EASU
//!WIDTH OUTPUT.w
//!HEIGHT OUTPUT.h
//!WHEN HOOKED.w OUTPUT.w < HOOKED.h OUTPUT.h < *

vec3 fsrLoad(vec2 pixel) {
    vec2 uv = (pixel + vec2(0.5)) * HOOKED_pt;
    uv = clamp(uv, 0.5 * HOOKED_pt, vec2(1.0) - 0.5 * HOOKED_pt);
    return HOOKED_tex(uv).rgb;
}

float fsrLuma(vec3 c) { return c.g + 0.5 * (c.r + c.b); }

void fsrEasuSet(inout vec2 dir, inout float edge, float weight,
                float la, float lb, float lc, float ld, float le) {
    float dx = ld - lb;
    float dy = le - la;
    dir += vec2(dx, dy) * weight;
    float lx = clamp(abs(dx) / max(max(abs(ld - lc), abs(lc - lb)), 1e-8), 0.0, 1.0);
    float ly = clamp(abs(dy) / max(max(abs(le - lc), abs(lc - la)), 1e-8), 0.0, 1.0);
    edge += (lx * lx + ly * ly) * weight;
}

void fsrEasuTap(inout vec3 color, inout float weight, vec2 offset,
                vec2 dir, vec2 stretch, float lobe, float clipDistance, vec3 tap) {
    vec2 v = vec2(dot(offset, dir), dot(offset, vec2(-dir.y, dir.x))) * stretch;
    float distance2 = min(dot(v, v), clipDistance);
    float base = 0.4 * distance2 - 1.0;
    float window = lobe * distance2 - 1.0;
    base = (25.0 / 16.0) * base * base - (9.0 / 16.0);
    float w = base * window * window;
    color += tap * w;
    weight += w;
}

vec4 hook() {
    // HOOKED_pos maps each output pixel center into the original image.
    // It implements FsrEasuCon's input/output ratio and half-pixel offset.
    vec2 pp = HOOKED_pos * HOOKED_size - vec2(0.5);
    vec2 fp = floor(pp);
    pp -= fp;
    vec3 b = fsrLoad(fp + vec2(0.0, -1.0));
    vec3 c = fsrLoad(fp + vec2(1.0, -1.0));
    vec3 e = fsrLoad(fp + vec2(-1.0, 0.0));
    vec3 f = fsrLoad(fp);
    vec3 g = fsrLoad(fp + vec2(1.0, 0.0));
    vec3 h = fsrLoad(fp + vec2(2.0, 0.0));
    vec3 i = fsrLoad(fp + vec2(-1.0, 1.0));
    vec3 j = fsrLoad(fp + vec2(0.0, 1.0));
    vec3 k = fsrLoad(fp + vec2(1.0, 1.0));
    vec3 l = fsrLoad(fp + vec2(2.0, 1.0));
    vec3 n = fsrLoad(fp + vec2(0.0, 2.0));
    vec3 o = fsrLoad(fp + vec2(1.0, 2.0));

    vec2 dir = vec2(0.0);
    float edge = 0.0;
    fsrEasuSet(dir, edge, (1.0 - pp.x) * (1.0 - pp.y), fsrLuma(b), fsrLuma(e), fsrLuma(f), fsrLuma(g), fsrLuma(j));
    fsrEasuSet(dir, edge, pp.x * (1.0 - pp.y), fsrLuma(c), fsrLuma(f), fsrLuma(g), fsrLuma(h), fsrLuma(k));
    fsrEasuSet(dir, edge, (1.0 - pp.x) * pp.y, fsrLuma(f), fsrLuma(i), fsrLuma(j), fsrLuma(k), fsrLuma(n));
    fsrEasuSet(dir, edge, pp.x * pp.y, fsrLuma(g), fsrLuma(j), fsrLuma(k), fsrLuma(l), fsrLuma(o));
    float norm2 = dot(dir, dir);
    dir = norm2 < (1.0 / 32768.0) ? vec2(1.0, 0.0) : dir * inversesqrt(norm2);
    edge *= 0.5;
    edge *= edge;
    float diagonalStretch = dot(dir, dir) / max(max(abs(dir.x), abs(dir.y)), 1e-8);
    vec2 stretch = vec2(1.0 + (diagonalStretch - 1.0) * edge, 1.0 - 0.5 * edge);
    float lobe = 0.5 + ((0.25 - 0.04) - 0.5) * edge;
    float clipDistance = 1.0 / lobe;
    vec3 color = vec3(0.0);
    float weight = 0.0;
    fsrEasuTap(color, weight, vec2(0.0, -1.0) - pp, dir, stretch, lobe, clipDistance, b);
    fsrEasuTap(color, weight, vec2(1.0, -1.0) - pp, dir, stretch, lobe, clipDistance, c);
    fsrEasuTap(color, weight, vec2(-1.0, 1.0) - pp, dir, stretch, lobe, clipDistance, i);
    fsrEasuTap(color, weight, vec2(0.0, 1.0) - pp, dir, stretch, lobe, clipDistance, j);
    fsrEasuTap(color, weight, -pp, dir, stretch, lobe, clipDistance, f);
    fsrEasuTap(color, weight, vec2(-1.0, 0.0) - pp, dir, stretch, lobe, clipDistance, e);
    fsrEasuTap(color, weight, vec2(1.0, 1.0) - pp, dir, stretch, lobe, clipDistance, k);
    fsrEasuTap(color, weight, vec2(2.0, 1.0) - pp, dir, stretch, lobe, clipDistance, l);
    fsrEasuTap(color, weight, vec2(2.0, 0.0) - pp, dir, stretch, lobe, clipDistance, h);
    fsrEasuTap(color, weight, vec2(1.0, 0.0) - pp, dir, stretch, lobe, clipDistance, g);
    fsrEasuTap(color, weight, vec2(1.0, 2.0) - pp, dir, stretch, lobe, clipDistance, o);
    fsrEasuTap(color, weight, vec2(0.0, 2.0) - pp, dir, stretch, lobe, clipDistance, n);
    vec3 nearestMin = min(min(f, g), min(j, k));
    vec3 nearestMax = max(max(f, g), max(j, k));
    return vec4(clamp(color / max(weight, 1e-8), nearestMin, nearestMax), 1.0);
}

//!HOOK SCALED
//!BIND HOOKED
//!DESC YingXia FSR1 RCAS Enhanced

// The application loads this pass only for a valid SDR upscale target.
// No NATIVE texture is retained merely to evaluate an extra size condition.

vec3 fsrRcasLoad(vec2 offset) {
    vec2 uv = clamp(HOOKED_pos + offset * HOOKED_pt, 0.5 * HOOKED_pt, vec2(1.0) - 0.5 * HOOKED_pt);
    return HOOKED_tex(uv).rgb;
}
float fsrRcasLuma(vec3 c) { return c.g + 0.5 * (c.r + c.b); }
vec4 hook() {
    vec3 b = fsrRcasLoad(vec2(0.0, -1.0));
    vec3 d = fsrRcasLoad(vec2(-1.0, 0.0));
    vec3 e = fsrRcasLoad(vec2(0.0));
    vec3 f = fsrRcasLoad(vec2(1.0, 0.0));
    vec3 h = fsrRcasLoad(vec2(0.0, 1.0));
    float bl = fsrRcasLuma(b), dl = fsrRcasLuma(d), el = fsrRcasLuma(e);
    float fl = fsrRcasLuma(f), hl = fsrRcasLuma(h);
    float lumaMin = min(min(min(bl, dl), el), min(fl, hl));
    float lumaMax = max(max(max(bl, dl), el), max(fl, hl));
    float noise = abs(0.25 * (bl + dl + fl + hl) - el) / max(lumaMax - lumaMin, 1e-8);
    noise = 1.0 - 0.5 * clamp(noise, 0.0, 1.0);
    vec3 mn = min(min(b, d), min(f, h));
    vec3 mx = max(max(b, d), max(f, h));
    vec3 hitMin = min(mn, e) / max(4.0 * mx, vec3(1e-8));
    vec3 hitMax = (vec3(1.0) - max(mx, e)) / min(4.0 * mn - vec3(4.0), vec3(-1e-8));
    vec3 channelLobe = max(-hitMin, hitMax);
    float lobe = max(-0.1875, min(max(max(channelLobe.r, channelLobe.g), channelLobe.b), 0.0));
    // FsrRcasCon: exp2(-1.0), one stop of sharpening, with FSR_RCAS_DENOISE.
    lobe *= 0.5 * noise;
    return vec4(clamp((lobe * (b + d + f + h) + e) / (4.0 * lobe + 1.0), 0.0, 1.0), 1.0);
}
