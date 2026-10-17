# RCAS hook metadata correction

Candidate 5's actual 720p to 1080p fullscreen playback exposed a metadata error: the EASU pass executed, but RCAS Enhanced did not, and the controller correctly fell back after observing five missing-stage samples. The actual evidence is `evidence/windows/diagnostics/candidate5_preselected_enhanced/视频测试库_fsr.jsonl`. The previous pbuffer algorithm/compiler checks did not exercise mpv's hook-condition lookup, so they could not validate this integration detail.

Both RCAS passes had `WHEN NATIVE.w OUTPUT.w < NATIVE.h OUTPUT.h < *` while only binding `HOOKED`. In pinned mpv v0.36.0 [`video.c`](https://github.com/mpv-player/mpv/blob/v0.36.0/video/out/gpu/video.c), `pass_opt_hook_point` only preserves optional textures referenced by a HOOK or BIND; a WHEN reference does not preserve one. `szexp_lookup` returns false for a missing saved image. Thus the absent `NATIVE` image made this condition skip RCAS.

The correction removes the extra WHEN from each RCAS SCALED hook. The unchanged application controller already clears shader options before changing output size, loads the shader only when an SDR upscale target exists, and removes it for original-sized or HDR playback. EASU retains its own direct HOOKED/OUTPUT size condition. No extra NATIVE texture binding or processing stage is added, and the EASU/RCAS pixel formulas, strength, sampler layout and verification requirements are unchanged.

The prior asset/algorithm evidence is preserved in `candidate5_metadata/`. The current `angle_rgb_validation.json` and `shader_provenance.json` record the corrected asset bytes. Actual playback proof of both Enhanced stages and the Light RCAS stage remains a separate subsequent candidate runtime check.
