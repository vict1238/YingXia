'use strict';
(() => {
 const $ = id => document.getElementById(id);
 const S={config:null,film:null,group:null,episode:null,playback:null,favorites:new Set(),seq:0,playSeq:0,resume:0,playingTimer:null};
 let consentResolver=null,toastTimer=null;
 const node=(tag,cls,content)=>{const e=document.createElement(tag);if(cls)e.className=cls;if(content!==undefined)e.textContent=content;return e;};
 const setHidden=(el,hide)=>el.classList.toggle('hidden',hide);
 const names={search:'发现 / 搜索影片',workspace:'放映 / 播放工作台',library:'本机 / 收藏与记录',settings:'设置 / 来源与诊断'};
 function toast(message){$('toast').textContent=message;setHidden($('toast'),false);clearTimeout(toastTimer);toastTimer=setTimeout(()=>setHidden($('toast'),true),4800);}
 function notice(id,message,error=false){const e=$(id);e.textContent=message;e.classList.toggle('error',error);setHidden(e,!message);}
 async function api(path,data){
   const opt={credentials:'same-origin',cache:'no-store',headers:{}};
   if(data!==undefined){opt.method='POST';opt.headers={'Content-Type':'application/json','X-YingXia-CSRF':window.YX_BOOT.csrf};opt.body=JSON.stringify(data);}
   const controller=new AbortController();opt.signal=controller.signal;const timer=setTimeout(()=>controller.abort(),90000);
   try{const r=await fetch('/api/'+path,opt);const j=await r.json();if(!j.ok){const err=new Error(j.message||'请求失败');err.code=j.error;throw err;}return j.data;}
   catch(e){if(e.name==='AbortError')throw new Error('请求等待超时。请检查网络；此提示不代表来源一定失效。');if(e instanceof TypeError)throw new Error('本机服务未响应。请检查启动窗口是否已关闭，或重新启动软件。');throw e;}
   finally{clearTimeout(timer);}
 }
 function show(view){
   document.querySelectorAll('.view').forEach(e=>e.classList.toggle('active',e.id==='view-'+view));
   document.querySelectorAll('.nav').forEach(e=>e.classList.toggle('active',e.dataset.view===view));
   $('breadcrumb').textContent=names[view];
   if(view==='library')loadLibrary();if(view==='settings')renderSettings();
 }
 document.querySelectorAll('[data-view]').forEach(e=>e.addEventListener('click',()=>show(e.dataset.view)));
 document.querySelector('.brand').addEventListener('click',e=>{e.preventDefault();show('search');});
 function applyConfig(c){S.config=c;S.favorites=new Set(c.favorite_ids);$('network-consent').checked=c.network_consent;$('save-history').checked=c.save_history;$('data-dir').textContent=c.data_dir;renderSources();renderPlayerSources();updateFavorite();}
 async function saveSettings(patch){applyConfig(await api('settings',patch));}
 async function ensureNetwork(){
   if(S.config.network_consent)return true;
   if(consentResolver)return false;
   $('consent-dialog').showModal();
   return new Promise(resolve=>consentResolver=resolve);
 }
 function finishConsent(result){$('consent-dialog').close();if(consentResolver){consentResolver(result);consentResolver=null;}}
 $('consent-cancel').onclick=()=>finishConsent(false);
 $('consent-dialog').addEventListener('cancel',()=>finishConsent(false));
 $('consent-accept').onclick=async()=>{try{await saveSettings({network_consent:true});finishConsent(true);}catch(e){toast(e.message);}};
 $('network-consent').onchange=async()=>{try{if($('network-consent').checked){$('network-consent').checked=await ensureNetwork();}else{stopPlayer();await saveSettings({network_consent:false});}}catch(e){toast(e.message);}};
 $('save-history').onchange=()=>saveSettings({save_history:$('save-history').checked}).catch(e=>toast(e.message));
 function card(f){
   const b=node('button','film-card');b.type='button';b.setAttribute('aria-label','打开 '+f.title);
   const cover=node('div','cover');cover.append(node('span','cover-tag',f.demo?'离线合成演示':f.kind||'目录候选'),node('span','cover-character',Array.from(f.title||'映')[0]),node('span','cover-note','文字封面 · 非官方海报'));
   const info=node('div','card-body');info.append(node('h3','',f.title),node('p','', [f.year||'年份未提供',f.area||'',f.remarks||''].filter(Boolean).join(' · ')),node('p','card-route',f.source));
   b.append(cover,info);b.onclick=()=>openFilm(f.id).catch(e=>toast(e.message));return b;
 }
 async function search(demo=false){
   const query=$('query').value.trim();if(!demo&&!query){$('query').focus();toast('先输入片名。');return;}
   if(!demo&&!await ensureNetwork())return;
   const seq=++S.seq;const submit=$('search-submit');submit.disabled=true;submit.textContent='查询中…';notice('search-message','正在查询；网络结果不会与演示数据混合。');
   try{
     const r=await api('search',demo?{demo:true}:{query,mode:$('search-mode').value});if(seq!==S.seq)return;
     $('search-welcome').classList.add('hidden');$('results').replaceChildren(...r.items.map(card));
     $('results-heading').textContent=demo?'离线演示内容':'搜索结果';$('results-count').textContent=`${r.items.length} 个候选 · 请核对年份 / 季数`;
     if(!r.items.length)$('results').append(node('p','empty-text','未取得可展示的候选。可以调整片名，或通过平台播放页导入。'));
     notice('search-message',r.warnings.join(' '));
     setHidden($('official-search-bar'),!r.official_search_url);
     if(r.official_search_url)$('official-search-link').href=r.official_search_url;
   }catch(e){if(seq===S.seq){notice('search-message',e.message,true);$('results').replaceChildren();$('results-heading').textContent='查询未完成';$('results-count').textContent='没有可验证的搜索结果';setHidden($('search-welcome'),true);setHidden($('official-search-bar'),true);}}
   finally{if(seq===S.seq){submit.disabled=false;submit.textContent='搜索';}}
 }
 $('search-form').onsubmit=e=>{e.preventDefault();search();};$('demo-btn').onclick=()=>search(true);
 function populatePlatform(select){select.replaceChildren();Object.entries(S.config.platforms).forEach(([id,p])=>{const o=node('option','',p.name);o.value=id;select.append(o);});}
 function renderPlayerSources(){
   const select=$('player-source'),old=select.value;select.replaceChildren();
   const choices=[['catalog','目录网页播放器'],['native','媒体直连（浏览器支持时）'],['official','打开已绑定的官方页面']];
   choices.push(...S.config.sources.filter(s=>s.enabled).map(s=>[s.id,s.name+' · 未验收']));
   if(S.film&&S.film.demo){choices.splice(0,choices.length,['native','本机演示 / WebM']);}
   choices.forEach(([id,label])=>{const o=node('option','',label);o.value=id;select.append(o);});
   if(choices.some(c=>c[0]===old))select.value=old;
 }
 let detailSeq=0;
 async function openFilm(id,history=null){
   const seq=++detailSeq;show('workspace');stopPlayer();notice('workspace-message','正在读取所选作品详情…');
   try{
     const f=await api('detail?id='+encodeURIComponent(id));if(seq!==detailSeq)return;
     S.film=f;S.group=f.groups[0]||null;S.episode=S.group?.episodes[0]||null;S.resume=0;
     if(history){
       const found=f.groups.flatMap(g=>g.episodes.map(e=>({g,e}))).find(x=>x.e.name===history.episode);
       if(found){S.group=found.g;S.episode=found.e;S.resume=history.mode==='media'?history.position||0:0;}
       else S.episode=null;
     }
     $('film-title').textContent=f.title;$('film-meta').textContent=[f.year||'年份未提供',f.kind,f.area,f.remarks,f.source].filter(Boolean).join(' · ');
     $('film-summary').textContent=f.summary||'目录未提供简介。';$('film-people').textContent=[f.director?'导演：'+f.director:'',f.actors?'演员：'+f.actors:''].filter(Boolean).join(' / ');
     const sel=$('group-select');sel.replaceChildren();f.groups.forEach(g=>{const o=node('option','',`${g.name}（${g.episodes.length} 项）`);o.value=g.key;sel.append(o);});sel.disabled=!S.group;if(S.group)sel.value=S.group.key;
     renderPlayerSources();$('player-source').value=f.demo?'native':S.group?.key==='official'?'official':'catalog';
     $('binding-url').value='';$('binding-note').textContent='';renderEpisodes();updateFavorite();await loadBinding();
     notice('workspace-message',!f.groups.length?'该目录没有返回可用选集；不会构造虚假的播放地址。':history&&!S.episode?'原记录中的集数已无法唯一找到，请重新选择。':f.demo?'离线演示：本机合成短片，不是影视资源。':'尚未验证实际播放。请核对作品、版本与集数。');
   }catch(e){if(seq===detailSeq){S.film=null;S.group=null;S.episode=null;renderEpisodes();notice('workspace-message',e.message,true);}}
 }
 function renderEpisodes(){
   const eps=S.group?.episodes||[];$('episode-count').textContent=eps.length+' 项';$('selected-episode').textContent=S.episode?'已选：'+S.episode.name:'请手动选择集数';
   $('episodes').replaceChildren(...eps.map(e=>{const b=node('button','episode-button'+(e.key===S.episode?.key?' selected':''),e.name);b.title=e.name;b.onclick=()=>{S.episode=e;S.resume=0;renderEpisodes();loadBinding();};return b;}));
   const i=eps.findIndex(e=>e.key===S.episode?.key);$('play').disabled=!S.episode;$('prev').disabled=i<=0;$('next').disabled=i<0||i>=eps.length-1;$('favorite-btn').disabled=!S.film;
 }
 $('group-select').onchange=async()=>{
   const old=S.episode;S.group=S.film.groups.find(g=>g.key===$('group-select').value);S.episode=null;stopPlayer();
   if(old){try{const e=await api('match',{film_id:S.film.id,group_key:S.group.key,match_name:old.name});S.episode=S.group.episodes.find(x=>x.key===e.key);notice('workspace-message','已按集名匹配到目标来源；点击播放，不会自动跳集。');}catch(e){notice('workspace-message',e.message,true);}}
   renderEpisodes();loadBinding();
 };
 $('player-source').onchange=()=>{stopPlayer();if(!['catalog','native','official'].includes($('player-source').value))$('binding-details').open=true;};
 async function loadBinding(){
   if(!S.film||!S.episode)return;
   const f=S.film.id,e=S.episode.name;
   try{const r=await api('bindings?id='+encodeURIComponent(f)+'&episode='+encodeURIComponent(e));if(S.film?.id!==f||S.episode?.name!==e)return;
     const b=r.find(x=>x.platform===$('binding-platform').value)||r[0];$('binding-url').value=b?.url||'';if(b)$('binding-platform').value=b.platform;
     $('binding-note').textContent=b?'已载入此集的用户绑定；未自动证明同一影片。':'此集尚无平台绑定。目录网页播放器不需要绑定。';
   }catch(err){toast(err.message);}
 }
 $('binding-platform').onchange=()=>{ $('binding-url').value='';$('binding-note').textContent='请粘贴所选平台的这一集播放页。';};
 $('bind-save').onclick=async()=>{if(!S.film||!S.episode){toast('先选择作品和集数。');return;}try{const r=await api('bind',{film_id:S.film.id,episode:S.episode.name,platform:$('binding-platform').value,url:$('binding-url').value});$('binding-url').value=r.url;$('binding-note').textContent=r.note;toast('已保存此集的平台绑定。');}catch(e){toast(e.message);}};
 function setPlaybackStatus(label,type=''){ $('playback-status').textContent=label;$('playback-dot').className=type; }
 function clearMedia(){
   clearTimeout(S.playingTimer);const video=$('stage').querySelector('video');if(video){video.pause();video.removeAttribute('src');video.load();}
   $('stage').replaceChildren();S.playback=null;
 }
 function stopPlayer(){S.playSeq++;clearMedia();const empty=node('div','stage-empty');empty.append(node('span','play-emblem','▷'),node('h3','','选择集数，开始播放'),node('p','','已移除当前播放器；不会自动重新开始播放。'));$('stage').append(empty);setPlaybackStatus('已停止 / 尚未打开播放入口');setHidden($('external-player'),true);setHidden($('confirm-playing'),true);$('actual-source').textContent='尚未打开';}
 async function play(){
   if(!S.film||!S.episode)return;
   if(!S.film.demo&&!await ensureNetwork())return;
   const playSeq=++S.playSeq;const filmId=S.film.id,groupKey=S.group.key,episodeKey=S.episode.key;
   const button=$('play');button.disabled=true;notice('workspace-message','正在生成所选集的播放入口…');
   try{
     const p=await api('resolve',{film_id:filmId,group_key:groupKey,episode_key:episodeKey,source:$('player-source').value,
       platform_url:$('binding-url').value,platform:$('binding-url').value?$('binding-platform').value:''});
     if(S.playSeq!==playSeq||S.film?.id!==filmId||S.group?.key!==groupKey||S.episode?.key!==episodeKey)return;
     clearMedia();S.playback=p;$('actual-source').textContent=p.source_label;
     $('external-player').href=p.url;setHidden($('external-player'),p.demo);setHidden($('confirm-playing'),p.kind!=='page');
     notice('workspace-message',p.demo?'本地合成短片测试中；这不验证任何在线影视来源。':p.notice);
     if(p.kind==='media')mountVideo(p);
     else if(p.kind==='page')mountFrame(p);
     else{
       const div=node('div','stage-empty');div.append(node('span','play-emblem','↗'),node('h3','','官方页面需在浏览器中打开'),node('p','','登录和会员判断由官方页面负责。本软件不会读取或保存平台账号。'));
       const a=node('a','primary','打开官方播放页 ↗');a.href=p.url;a.target='_blank';a.rel='noopener noreferrer';div.append(a);$('stage').append(div);setPlaybackStatus('已定位官方页面；尚未验证播放','warn');
     }
   }catch(e){notice('workspace-message',e.message,true);if(e.code==='binding_required')$('binding-details').open=true;}
   finally{renderEpisodes();}
 }
 function mountFrame(p){
   const f=document.createElement('iframe');f.title=p.title+' 第三方播放器';f.referrerPolicy='no-referrer';
   f.setAttribute('sandbox','allow-scripts allow-same-origin allow-presentation');f.setAttribute('allow','autoplay; fullscreen; encrypted-media');f.allowFullscreen=true;
   f.addEventListener('load',()=>{if(S.playback===p)setPlaybackStatus('网页加载事件已触发；是否有视频画面仍待确认','warn');});
   setPlaybackStatus('正在加载第三方网页；尚无实播证据','warn');f.src=p.url;$('stage').append(f);
   $('mode-hint').textContent='网页可能禁止内嵌或依赖被拦截的弹窗。仅页面加载不能证明实播；无画面可切线或在浏览器中打开。';
   S.playingTimer=setTimeout(()=>{if(S.playback===p)setPlaybackStatus('尚未取得自动实播证据；看到画面后可手动确认','warn');},15000);
 }
 function mountVideo(p){
   const v=document.createElement('video');v.controls=true;v.playsInline=true;v.preload='metadata';v.src=p.url;$('stage').append(v);
   let last=0,lastSaved=0,reported=false;
   v.addEventListener('loadedmetadata',()=>{if(S.resume>0&&S.resume<v.duration)v.currentTime=S.resume;S.resume=0;});
   v.addEventListener('playing',()=>setPlaybackStatus('媒体开始运行；等待视频帧与时间推进'));
   v.addEventListener('timeupdate',()=>{
     if(S.playback!==p)return;
     const quality=v.getVideoPlaybackQuality?.();const frames=quality?quality.totalVideoFrames>0:v.videoWidth>0;
     const advanced=v.currentTime>last+.1&&v.currentTime-last<3;last=v.currentTime;
     if(!v.paused&&!v.seeking&&frames&&advanced){
       if(!reported){setPlaybackStatus('浏览器报告视频帧与时间推进'+(p.demo?'（仅本机演示）':''),'good');reported=true;}
       if(p.history_id&&(v.currentTime-lastSaved>3||!lastSaved)){
         lastSaved=v.currentTime;api('progress',{history_id:p.history_id,evidence:'browser_time_advanced',position:v.currentTime}).catch(()=>{});
       }
     }
   });
   v.addEventListener('error',()=>{setPlaybackStatus('浏览器未能播放该媒体；地址或格式可能不受支持','warn');notice('workspace-message','媒体直连失败。Chrome/Edge 不保证原生支持所有 HLS 清单；可改用目录网页播放器，不会自动跳到其他集。',true);});
   v.addEventListener('ended',()=>setPlaybackStatus('本段播放结束'+(p.demo?'（仅本机演示）':''),'good'));
   $('mode-hint').textContent='直连模式使用浏览器原生媒体能力。记录的是浏览器报告，不代表其他来源或 Windows 环境已通过验收。';
   v.play().catch(()=>setPlaybackStatus('媒体已加载；请点击视频内的播放按钮','warn'));
 }
 $('play').onclick=play;$('stop').onclick=stopPlayer;
 async function step(delta){const eps=S.group?.episodes||[];const i=eps.findIndex(e=>e.key===S.episode?.key);if(i<0||!eps[i+delta])return;S.episode=eps[i+delta];S.resume=0;renderEpisodes();await loadBinding();await play();}
 $('prev').onclick=()=>step(-1);$('next').onclick=()=>step(1);
 $('fullscreen').onclick=()=>{if(document.fullscreenElement)document.exitFullscreen();else $('stage').requestFullscreen().catch(()=>toast('当前浏览器未允许全屏。'));};
 $('confirm-playing').onclick=async()=>{if(!S.playback)return;try{if(S.playback.history_id)await api('progress',{history_id:S.playback.history_id,evidence:'user_confirmed',position:0});clearTimeout(S.playingTimer);setPlaybackStatus('用户已确认看到画面（非程序自动验证）','good');toast('已记为用户确认；没有把整条来源标记为可用。');}catch(e){toast(e.message);}};
 function updateFavorite(){const saved=S.film&&S.favorites.has(S.film.id);$('favorite-btn').textContent=saved?'✓ 已收藏':'＋ 收藏';$('favorite-btn').disabled=!S.film;}
 $('favorite-btn').onclick=async()=>{if(!S.film)return;try{const r=await api('favorite',{film_id:S.film.id});if(r.saved)S.favorites.add(S.film.id);else S.favorites.delete(S.film.id);updateFavorite();toast(r.saved?'已收藏到本机。':'已取消收藏。');}catch(e){toast(e.message);}};
 function table(headers,rows){const t=node('table'),h=node('thead'),tr=node('tr');headers.forEach(v=>tr.append(node('th','',v)));h.append(tr);t.append(h);const b=node('tbody');rows.forEach(cells=>{const r=node('tr');cells.forEach(c=>{const td=node('td');if(c instanceof Node)td.append(c);else td.textContent=String(c??'—');r.append(td);});b.append(r);});t.append(b);return t;}
 async function loadLibrary(){
   try{const r=await api('library');S.favorites=new Set(r.favorites.map(f=>f.id));updateFavorite();$('favorites-count').textContent=r.favorites.length+' 个收藏';
     $('favorites').replaceChildren(...r.favorites.map(card));if(!r.favorites.length)$('favorites').append(node('p','empty-text','还没有收藏。选择作品后，点击右上角「收藏」。'));
     const evidence={opened:'仅生成播放入口',browser_time_advanced:'浏览器报告时间推进',user_confirmed:'用户确认有画面'};
     if(!r.history.length)$('history').replaceChildren(node('p','empty-text','还没有打开记录。'));
     else $('history').replaceChildren(table(['作品 / 集数','实际来源','状态 / 时间','操作'],r.history.map(h=>{
       const title=node('div','',h.title);title.append(node('small','',h.episode));
       const status=node('div','',evidence[h.evidence]||h.evidence);status.append(node('small','',new Date(h.updated_at).toLocaleString()));
       const btn=node('button','small-button','重新打开');btn.onclick=()=>openFilm(h.film_id,h).catch(e=>toast(e.message));
       return [title,h.source,status,btn];
     })));
   }catch(e){toast(e.message);}
 }
 $('clear-history').onclick=async()=>{if(!confirm('仅清空本机最近打开记录，不影响收藏。继续吗？'))return;try{await api('clear-history',{});loadLibrary();}catch(e){toast(e.message);}};
 function renderSources(){if(!S.config)return;
   const rows=[['无损云目录 / 播放器','片名检索 + 详情 + 选集',S.config.catalog.status,'在线功能使用时调用']];
   S.config.sources.forEach(s=>{const checkbox=node('input','source-toggle');checkbox.type='checkbox';checkbox.checked=s.enabled;checkbox.setAttribute('aria-label','启用 '+s.name);checkbox.onchange=async()=>{const enabled=Object.fromEntries(S.config.sources.map(x=>[x.id,x.enabled]));enabled[s.id]=checkbox.checked;try{await saveSettings({enabled_sources:enabled});toast('来源设置已保存。');}catch(e){toast(e.message);checkbox.checked=!checkbox.checked;}};rows.push([s.name,'需具体平台播放页',s.status,checkbox]);});
   $('sources').replaceChildren(table(['来源','用途','当前证据','启用'],rows));
 }
 function renderSettings(){if(!S.config)return;renderSources();$('network-consent').checked=S.config.network_consent;$('save-history').checked=S.config.save_history;}
 $('diagnose').onclick=async()=>{if(!await ensureNetwork())return;const b=$('diagnose');b.disabled=true;b.textContent='诊断中…';$('diagnostics').replaceChildren(node('p','empty-text','正在检查启用的来源入口与网络对照；不会访问你当前播放的影片。'));try{const r=await api('diagnose',{});$('diagnostics').replaceChildren(table(['入口','HTTP','结果'],r.results.map(x=>[x.name,x.http||'—',x.state])));}catch(e){$('diagnostics').replaceChildren(node('p','empty-text',e.message));}finally{b.disabled=false;b.textContent='运行连接诊断';}};
 $('export-report').onclick=async()=>{try{const r=await api('report');const blob=new Blob([JSON.stringify(r,null,2)],{type:'application/json'});const url=URL.createObjectURL(blob);const a=node('a');a.href=url;a.download='yingxia_diagnostics.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}catch(e){toast(e.message);}};
 $('import-open').onclick=()=>{$('import-dialog').showModal();};$('import-cancel').onclick=()=>$('import-dialog').close();
 $('import-form').onsubmit=async e=>{e.preventDefault();try{const f=await api('import-platform',{platform:$('import-platform').value,url:$('import-url').value,title:$('import-title').value});$('import-dialog').close();await openFilm(f.id);toast('已导入当前播放页；未自动取得全剧选集。');}catch(err){toast(err.message);}};
 $('quit').onclick=async()=>{if(!confirm('停止播放并退出本机服务？'))return;stopPlayer();try{await api('quit',{});}catch(e){}document.querySelector('.connection').textContent='本机服务已退出';toast('软件已退出，可以关闭此窗口。');window.close();};
 async function init(){
   try{applyConfig(await api('state'));populatePlatform($('import-platform'));populatePlatform($('binding-platform'));}
   catch(e){notice('search-message',e.message,true);}
   setInterval(()=>api('ping',{}).catch(()=>{}),20000);
 }
 init();
})();
