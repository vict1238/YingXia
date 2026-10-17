// AMD FidelityFX Super Resolution 1 RCAS, adapted for an mpv fragment hook.
// Upstream: GPUOpen-Effects/FidelityFX-FSR a21ffb8f6c13233ba336352bdff293894c706575.
// Light uses normal bilinear upscaling followed by this single five-tap pass.
// It does not execute EASU. Exact guarded reciprocals replace bitwise estimates.
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

//!HOOK SCALED
//!BIND HOOKED
//!DESC YingXia FSR1 RCAS Light
//!WHEN NATIVE.w OUTPUT.w < NATIVE.h OUTPUT.h < *

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
    // FsrRcasCon: exp2(-1.5), mild sharpening with FSR_RCAS_DENOISE.
    lobe *= 0.3535533905932738 * noise;
    return vec4(clamp((lobe * (b + d + f + h) + e) / (4.0 * lobe + 1.0), 0.0, 1.0), 1.0);
}
