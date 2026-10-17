// Summarizes recorded actual shipping-EXE samples; never injects QA metrics.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict');
const [directory,executable,output]=process.argv.slice(2);
assert.ok(directory&&executable&&output,'Provide evidence directory, exact tested EXE, and output JSON');
const report=JSON.parse(fs.readFileSync(path.join(directory,'native_windows_fsr_tests.json'),'utf8'));
const sha256=crypto.createHash('sha256').update(fs.readFileSync(executable)).digest('hex');assert.equal(report.sha256,sha256);
const log=path.join(directory,'视频测试库_fsr.jsonl');
const rows=fs.readFileSync(log,'utf8').trim().split('\n').map(JSON.parse);
const summary={executable_sha256:sha256,source_report:path.resolve(directory,'native_windows_fsr_tests.json'),source_diagnostics:path.resolve(log),hardware:'NVIDIA GeForce RTX 4060 Laptop GPU',integrated_gpu_benchmarked:false,synthetic_media_only:true,real_third_party_playback_tested:false,performance_unit:'libmpv actual GPU pass averages summed in milliseconds; not an end-to-end or integrated-GPU throughput guarantee',fps_runs:[]};
for(const fps of [24,30,60]){
  const selected=rows.filter(r=>r.event==='sample'&&r.active&&r.shader_verified&&r.requested==='enhanced'&&r.effective==='enhanced'&&r.fps===fps&&r.source?.width===1280&&r.source?.height===720&&r.output?.width===1920&&r.output?.height===1080);
  assert.ok(selected.length>=(fps===30?10:4),'Insufficient actual enhanced samples for '+fps+'fps');
  for(const row of selected){const names=row.vo_passes.map(pass=>pass.desc);assert.ok(names.some(name=>name.includes('YingXia FSR1 EASU')));assert.ok(names.some(name=>name.includes('YingXia FSR1 RCAS Enhanced')));}
  const advance=selected.at(-1).position-selected[0].position;assert.ok(advance>=(fps===30?9:3.5),'Actual video clock did not keep advancing for '+fps+'fps');
  const times=selected.map(r=>r.render_ms).sort((a,b)=>a-b);
  summary.fps_runs.push({fps,sample_count:selected.length,first_time:selected[0].time,last_time:selected.at(-1).time,first_position:selected[0].position,last_position:selected.at(-1).position,video_clock_advance_seconds:advance,render_ms_min:times[0],render_ms_median:times[Math.floor(times.length/2)],render_ms_max:times.at(-1),vo_drops_first:selected[0].vo_drops,vo_drops_last:selected.at(-1).vo_drops,vo_drop_delta:selected.at(-1).vo_drops-selected[0].vo_drops,decoder_drops_last:selected.at(-1).decoder_drops,hardware_decoder:[...new Set(selected.map(r=>r.hwdec))],renderer:[...new Set(selected.map(r=>r.renderer))],passed:true});
}
summary.passed=true;fs.writeFileSync(output,JSON.stringify(summary,null,2));console.log(JSON.stringify(summary));
