// Backend-only fixture validation: private authenticated transport, no playback claim.
const assert=require('node:assert/strict');const fs=require('node:fs');const path=require('node:path');const os=require('node:os');
const {spawn}=require('node:child_process');const {createInterface}=require('node:readline');
const root=path.resolve(__dirname,'..');const executable=path.join(root,'build/qa_core/yingxia_backend_fsr_fixture.exe');
const data=fs.mkdtempSync(path.join(os.tmpdir(),'yingxia-fsr-fixture-'));const checks=[];
const child=spawn(executable,['--data-dir',data],{windowsHide:true,stdio:['pipe','pipe','pipe']});
async function main(){
 const reader=createInterface({input:child.stdout});const boot=await new Promise((resolve,reject)=>{reader.once('line',line=>{try{resolve(JSON.parse(line));}catch(e){reject(e);}});child.once('error',reject);setTimeout(()=>reject(Error('Bootstrap timeout')),20000).unref();});
 assert.ok(boot.origin);const headers={Cookie:boot.cookie,Origin:boot.origin,'X-YingXia-CSRF':boot.csrf};
 const api=async (endpoint,body)=>{const response=await fetch(boot.origin+'/api/'+endpoint,{headers:{...headers,'Content-Type':'application/json'},method:body?'POST':'GET',body:body?JSON.stringify(body):undefined});const value=await response.json();assert.ok(value.ok,JSON.stringify(value));return value.data;};
 let job=await api('search/start',{query:'FSR',mode:'all'});const end=Date.now()+20000;
 while(job.phase!=='done'&&Date.now()<end){await new Promise(resolve=>setTimeout(resolve,150));job=await api('search/status?id='+job.id);}
 assert.equal(job.phase,'done');assert.equal(job.items.length,4);assert.equal(job.available_count,4);checks.push('All four synthetic FSR candidates passed the private availability search');
 for(const item of job.items){const detail=await api('detail?id='+encodeURIComponent(item.id));const group=detail.groups[0];const play=await api('resolve',{film_id:detail.id,group_key:group.key,episode_key:group.episodes[0].key,source:'native'});
  assert.equal(play.kind,'media');assert.match(play.url,/^\/api\/media\?ticket=/);
  if(item.title.includes('HLS')){const response=await fetch(boot.origin+play.url,{headers});assert.equal(response.status,200);const text=await response.text();assert.ok(text.includes('#EXTM3U'));const link=text.split('\n').find(line=>line&&!line.startsWith('#'));const segment=await fetch(boot.origin+link,{headers:{...headers,Range:'bytes=0-187'}});assert.equal(segment.status,206);assert.equal((await segment.arrayBuffer()).byteLength,188);checks.push('720p HLS authenticated playlist and TS byte range');}
  else {const response=await fetch(boot.origin+play.url,{headers:{...headers,Range:'bytes=0-63'}});assert.equal(response.status,206);assert.equal((await response.arrayBuffer()).byteLength,64);checks.push(item.title+' authenticated MP4 byte range');}
 }
 await api('quit',{});await new Promise(resolve=>child.once('exit',resolve));assert.equal(child.exitCode,0);checks.push('Authenticated quit exits the isolated fixture process');
 fs.writeFileSync(path.join(root,'evidence/fixture/transport.json'),JSON.stringify({checks,passed:checks.length,backend_only:true,synthetic_media_only:true},null,2));console.log(JSON.stringify({passed:checks.length,checks},null,2));
}
main().catch(error=>{console.error(error.stack);process.exitCode=1;}).finally(()=>{child.stdin.end();if(child.exitCode===null)child.kill();if(path.dirname(data)===os.tmpdir()&&path.basename(data).startsWith('yingxia-fsr-fixture-'))fs.rmSync(data,{recursive:true,force:true,maxRetries:20,retryDelay:100});});
