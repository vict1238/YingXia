'use strict';
(() => {
 const $ = id => document.getElementById(id);
 const S={config:null,film:null,group:null,episode:null,playback:null,favorites:new Set(),seq:0,playSeq:0,resume:0,hls:null,searchJob:null,view:'search',suspended:false,playKey:null};
 let consentResolver=null,toastTimer=null,searchStartTail=Promise.resolve(),librarySeq=0,clearingHistory=false;
 const node=(tag,cls,content)=>{const e=document.createElement(tag);if(cls)e.className=cls;if(content!==undefined)e.textContent=content;return e;};
 const setHidden=(el,hide)=>el.classList.toggle('hidden',hide);
 const names={search:'搜索影片',workspace:'播放工作台',library:'收藏与记录',settings:'来源与诊断'};
 const sourceName=value=>(value||'').replace(/（(?:未实播验证|版本及集数需核对|第三方)）/g,'').replace(' / 定位平台：',' · ');
 const episodeName=value=>(value||'').replace('（集数未核验）','');
 function toast(message){$('toast').textContent=message;setHidden($('toast'),false);clearTimeout(toastTimer);toastTimer=setTimeout(()=>setHidden($('toast'),true),4800);}
 function notice(id,message,error=false){const e=$(id);e.textContent=message;e.classList.toggle('error',error);setHidden(e,!message);}
 async function api(path,data){
   const opt={credentials:'same-origin',cache:'no-store',headers:{}};
   if(data!==undefined){opt.method='POST';opt.headers={'Content-Type':'application/json','X-YingXia-CSRF':window.YX_BOOT.csrf};opt.body=JSON.stringify(data);}
   const controller=new AbortController();opt.signal=controller.signal;const timer=setTimeout(()=>controller.abort(),120000);
   try{const r=await fetch('/api/'+path,opt);const j=await r.json();if(!j.ok){const err=new Error(j.message||'请求失败');err.code=j.error;throw err;}return j.data;}
   catch(e){if(e.name==='AbortError')throw new Error('请求超时，请重试。');if(e instanceof TypeError)throw new Error('连接已断开，请重新打开软件。');throw e;}
   finally{clearTimeout(timer);}
 }
 function show(view){
   if(S.view==='workspace'&&view!=='workspace')suspendPlayer();
   S.view=view;
   document.querySelectorAll('.view').forEach(e=>e.classList.toggle('active',e.id==='view-'+view));
   document.querySelectorAll('.nav').forEach(e=>e.classList.toggle('active',e.dataset.view===view));
   $('breadcrumb').textContent=names[view];
   if(view==='library')loadLibrary();if(view==='settings'){renderSettings();api('state').then(c=>{applyConfig(c);renderSettings();}).catch(e=>toast(e.message));}
 }
 document.querySelectorAll('[data-view]').forEach(e=>e.addEventListener('click',()=>show(e.dataset.view)));
 document.querySelector('.brand').addEventListener('click',e=>{e.preventDefault();show('search');});
 function applyConfig(c){S.config=c;S.favorites=new Set(c.favorite_ids);$('network-consent').checked=c.network_consent;$('save-history').checked=c.save_history;$('auto-next').checked=c.auto_next;$('data-dir').textContent=c.data_dir;renderSources();renderProviders();renderPlayerSources();updateFavorite();}
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
 $('auto-next').onchange=()=>saveSettings({auto_next:$('auto-next').checked}).catch(e=>toast(e.message));
 $('auto-update-sources').onchange=()=>saveSettings({auto_update_sources:$('auto-update-sources').checked}).catch(e=>toast(e.message));
 function card(f){
   const b=node('button','film-card');b.type='button';b.setAttribute('aria-label','打开 '+f.title);
   const cover=node('div','cover');cover.append(node('span','cover-tag',f.kind||'影片'),node('span','cover-character',Array.from(f.title||'映')[0]));
   const info=node('div','card-body');info.append(node('h3','',f.title),node('p','', [f.year,f.area,f.remarks].filter(Boolean).join(' · ')),node('p','card-route',sourceName(f.source)));
   if(f.availability){const a=f.availability,badge=node('span','availability-badge',a.label);badge.dataset.state=a.state;badge.title=a.error||a.label;info.append(badge);b.disabled=a.state==='pending';b.classList.toggle('unavailable',a.state==='failed');}
   b.append(cover,info);b.onclick=()=>openFilm(f.id,null,f.availability).catch(e=>toast(e.message));return b;
 }
 function renderSearch(r){
   $('results').replaceChildren(...r.items.map(card));setHidden($('results-header'),!r.items.length&&r.phase!=='done');
   $('results-heading').textContent='搜索结果';$('results-count').textContent=`${r.items.length} 个结果`;
   if(!r.items.length&&r.phase==='done')$('results').append(node('p','empty-text',r.successful_sources===0?'来源暂不可用':'暂无结果'));
   setHidden($('search-sources'),!r.source_results);if(r.source_results){$('search-source-summary').textContent=`${r.successful_sources} / ${r.source_count} 个来源已响应`;$('search-source-results').replaceChildren(table(['来源','结果'],r.source_results.map(x=>[x.name,x.ok?`${x.count} 个结果`:x.error||'请求失败'])));$('search-sources').open=r.successful_sources===0;}
   setHidden($('official-search-bar'),!r.official_search_url);if(r.official_search_url)$('official-search-link').href=r.official_search_url;
 }
 function searchProgress(r){
   setHidden($('search-progress'),false);$('search-progress-bar').value=r.percent;
   let label=r.source_total?`正在搜索 · ${r.source_completed} / ${r.source_total} 个来源`:'正在搜索…';
   if(r.phase==='checking')label=`检测访问 · ${r.checked_count} / ${r.check_total} 个结果 · ${r.available_count} 个可访问`;
   if(r.phase==='done')label=`检测完成 · ${r.available_count} 个可访问${r.checked_count<r.check_total?` · ${r.check_total-r.checked_count} 个未检测`:''}`;
   $('search-progress-label').textContent=label;setHidden($('search-cancel'),r.phase==='done');
 }
 async function search(){
   const query=$('query').value.trim();if(!query){$('query').focus();toast('请输入片名。');return;}
   if(!await ensureNetwork())return;
   const seq=++S.seq;const submit=$('search-submit');submit.disabled=true;submit.textContent='搜索中…';notice('search-message','');
   $('results').replaceChildren();setHidden($('results-header'),true);setHidden($('search-sources'),true);setHidden($('official-search-bar'),true);
   searchProgress({phase:'searching',percent:0,source_completed:0,source_total:0});
   try{
     const mode=$('search-mode').value;
     const pendingStart=searchStartTail.then(async()=>{if(seq!==S.seq)return null;const r=await api('search/start',{query,mode});if(seq!==S.seq){await api('search/cancel',{id:r.id}).catch(()=>{});return null;}return r;});
     searchStartTail=pendingStart.then(()=>{},()=>{});
     let r=await pendingStart;if(!r||seq!==S.seq)return;
     S.searchJob=r.id;
     while(seq===S.seq){
       searchProgress(r);renderSearch(r);
       if(r.phase==='done')break;
       if(r.phase==='failed'||r.phase==='cancelled')throw new Error(r.message||'搜索已取消');
       await new Promise(resolve=>setTimeout(resolve,400));if(seq!==S.seq)return;
       r=await api('search/status?id='+encodeURIComponent(r.id));if(seq!==S.seq)return;
     }
   }catch(e){if(seq===S.seq){notice('search-message',e.message,true);setHidden($('search-progress'),true);$('results').replaceChildren();setHidden($('results-header'),true);setHidden($('official-search-bar'),true);setHidden($('search-sources'),true);}}
   finally{if(seq===S.seq){S.searchJob=null;submit.disabled=false;submit.textContent='搜索';}}
 }
 $('search-cancel').onclick=()=>{const id=S.searchJob;S.seq++;S.searchJob=null;if(id)api('search/cancel',{id}).catch(()=>{});$('search-submit').disabled=false;$('search-submit').textContent='搜索';setHidden($('search-progress'),true);$('results').replaceChildren();setHidden($('results-header'),true);setHidden($('search-sources'),true);notice('search-message','');};
 $('search-form').onsubmit=e=>{e.preventDefault();search();};
 function populatePlatform(select){select.replaceChildren();Object.entries(S.config.platforms).sort(([a],[b])=>['qq','iqiyi','youku','bilibili','mgtv'].indexOf(a)-['qq','iqiyi','youku','bilibili','mgtv'].indexOf(b)).forEach(([id,p])=>{const o=node('option','',p.name);o.value=id;select.append(o);});}
 function renderPlayerSources(){
   const select=$('player-source'),old=select.value;select.replaceChildren();
   const choices=S.film?.provider_kind==='kazumi'?[['native','内置播放'],['web','网页播放'],['official','官方页面']]:[['native','内置播放'],['web','网页播放'],['catalog','目录播放器'],['official','官方页面']];
   choices.push(...S.config.sources.filter(s=>s.enabled).map(s=>[s.id,s.name]));
   if(S.film&&S.film.demo){choices.splice(0,choices.length,['native','本机演示 / WebM']);}
   choices.forEach(([id,label])=>{const o=node('option','',label);o.value=id;select.append(o);});
   if(choices.some(c=>c[0]===old))select.value=old;
 }
 let detailSeq=0;
 async function openFilm(id,history=null,availability=null){
   const seq=++detailSeq;show('workspace');stopPlayer();notice('workspace-message','加载中…');
   try{
     const f=await api('detail?id='+encodeURIComponent(id));if(seq!==detailSeq)return;
     S.film=f;S.group=f.groups[0]||null;S.episode=S.group?.episodes[0]||null;S.resume=0;
     if(availability?.group_key){const preferred=f.groups.find(g=>g.key===availability.group_key);const ep=preferred?.episodes.find(e=>e.key===availability.episode_key);if(preferred&&ep){S.group=preferred;S.episode=ep;}}
     if(history){
       const found=f.groups.flatMap(g=>g.episodes.map(e=>({g,e}))).find(x=>x.e.name===history.episode);
       if(found){S.group=found.g;S.episode=found.e;S.resume=history.mode==='media'?history.position||0:0;}
       else S.episode=null;
     }
     $('film-title').textContent=f.title;$('film-meta').textContent=[f.year,f.kind,f.area,f.remarks,sourceName(f.source)].filter(Boolean).join(' · ');
     $('film-summary').textContent=f.summary||'';$('film-people').textContent=[f.director?'导演：'+f.director:'',f.actors?'演员：'+f.actors:''].filter(Boolean).join(' / ');
     const sel=$('group-select');sel.replaceChildren();f.groups.forEach(g=>{const o=node('option','',`${g.name}（${g.episodes.length} 项）`);o.value=g.key;sel.append(o);});sel.disabled=!S.group;if(S.group)sel.value=S.group.key;
     renderPlayerSources();$('player-source').value=S.group?.key==='official'?'official':f.demo||f.provider_id?'native':'catalog';
     if(availability?.state==='page_ok'&&S.group?.key!=='official')$('player-source').value='web';
     $('binding-url').value='';$('binding-note').textContent='';renderEpisodes();updateFavorite();await loadBinding();
     notice('workspace-message',!f.groups.length?'暂无选集':history&&!S.episode?'未找到原记录中的集数，请重新选择。':'');
   }catch(e){if(seq===detailSeq){S.film=null;S.group=null;S.episode=null;renderEpisodes();notice('workspace-message',e.message,true);}}
 }
 function renderEpisodes(){
   const eps=S.group?.episodes||[];$('episode-count').textContent=eps.length+' 项';$('selected-episode').textContent=S.episode?'已选：'+episodeName(S.episode.name):'请选择集数';
   $('episodes').replaceChildren(...eps.map(e=>{const b=node('button','episode-button'+(e.key===S.episode?.key?' selected':''),episodeName(e.name));b.title=episodeName(e.name);b.onclick=()=>{S.episode=e;S.resume=0;renderEpisodes();loadBinding();};return b;}));
   const i=eps.findIndex(e=>e.key===S.episode?.key);$('play').disabled=!S.episode;$('prev').disabled=i<=0;$('next').disabled=i<0||i>=eps.length-1;$('favorite-btn').disabled=!S.film;
 }
 $('group-select').onchange=async()=>{
   const old=S.episode;S.group=S.film.groups.find(g=>g.key===$('group-select').value);S.episode=null;stopPlayer();
   if(old){try{const e=await api('match',{film_id:S.film.id,group_key:S.group.key,match_name:old.name});S.episode=S.group.episodes.find(x=>x.key===e.key);notice('workspace-message','');}catch(e){notice('workspace-message',e.message,true);}}
   renderEpisodes();loadBinding();
 };
   $('player-source').onchange=()=>{stopPlayer();if(!['catalog','native','official','web'].includes($('player-source').value))$('binding-details').open=true;};
 async function loadBinding(){
   if(!S.film||!S.episode)return;
   const f=S.film.id,e=S.episode.name;
   try{const r=await api('bindings?id='+encodeURIComponent(f)+'&episode='+encodeURIComponent(e));if(S.film?.id!==f||S.episode?.name!==e)return;
     const b=r.find(x=>x.platform===$('binding-platform').value)||r[0];$('binding-url').value=b?.url||'';if(b)$('binding-platform').value=b.platform;
     $('binding-note').textContent=b?'已绑定':'';
   }catch(err){toast(err.message);}
 }
 $('binding-platform').onchange=()=>{ $('binding-url').value='';$('binding-note').textContent='';};
 $('bind-save').onclick=async()=>{if(!S.film||!S.episode){toast('请选择影片和集数。');return;}try{const r=await api('bind',{film_id:S.film.id,episode:S.episode.name,platform:$('binding-platform').value,url:$('binding-url').value});$('binding-url').value=r.url;$('binding-note').textContent='已绑定';toast('已保存');}catch(e){toast(e.message);}};
 function setPlaybackStatus(label,type=''){ $('playback-status').textContent=label;$('playback-dot').className=type; }
 function clearMedia(){
   if(S.hls){S.hls.destroy();S.hls=null;}setHidden($('speed'),true);setHidden($('quality'),true);
   const video=$('stage').querySelector('video');if(video){video.pause();video.removeAttribute('src');video.load();}
   $('stage').replaceChildren();S.playback=null;S.playKey=null;
 }
 function stageInstruction(){const empty=node('div','stage-empty');empty.append(node('span','play-emblem','▷'),node('h3','','选集之后请点击下方播放所选集'));$('stage').replaceChildren(empty);}
 function stopPlayer(){S.playSeq++;clearMedia();stageInstruction();setPlaybackStatus('已停止');setHidden($('external-player'),true);$('actual-source').textContent='—';}
 function suspendPlayer(){
   S.playSeq++;S.suspended=true;notice('workspace-message','');
   const video=$('stage').querySelector('video');
   if(video){video.pause();setPlaybackStatus('已暂停');}
   else if($('stage').querySelector('iframe')){clearMedia();stageInstruction();setPlaybackStatus('已暂停');setHidden($('external-player'),true);}
 }
 document.addEventListener('visibilitychange',()=>{if(document.hidden&&S.view==='workspace')suspendPlayer();});
 window.addEventListener('yingxia:window-hidden',()=>{if(S.view==='workspace')suspendPlayer();});
 function canPlay(p){return S.playback===p&&S.view==='workspace'&&!S.suspended&&!document.hidden;}
 function startVideo(v,p){
   if(!canPlay(p))return;
   v.play().then(()=>{if(!canPlay(p))v.pause();}).catch(()=>{if(canPlay(p))setPlaybackStatus('点击视频播放');});
 }
 async function play(){
   if(!S.film||!S.episode||S.view!=='workspace'||document.hidden)return;
   if(!S.film.demo&&!await ensureNetwork())return;
   if(S.view!=='workspace'||document.hidden)return;
   const request={film_id:S.film.id,group_key:S.group.key,episode_key:S.episode.key,source:$('player-source').value,platform_url:$('binding-url').value,platform:$('binding-url').value?$('binding-platform').value:''};
   const playKey=JSON.stringify(request),current=$('stage').querySelector('video');
   S.suspended=false;
   if(current&&!current.ended&&S.playback?.kind==='media'&&S.playKey===playKey){S.playSeq++;startVideo(current,S.playback);return;}
   const playSeq=++S.playSeq;const filmId=S.film.id,groupKey=S.group.key,episodeKey=S.episode.key;
   const button=$('play');button.disabled=true;notice('workspace-message','加载中…');
   try{
     const p=await api('resolve',request);
     if(S.playSeq!==playSeq||S.film?.id!==filmId||S.group?.key!==groupKey||S.episode?.key!==episodeKey||S.view!=='workspace'||S.suspended||document.hidden)return;
     clearMedia();S.playback=p;S.playKey=playKey;$('actual-source').textContent=sourceName(p.source_label);
     $('external-player').href=p.external_url||p.url;setHidden($('external-player'),p.demo);
     notice('workspace-message','');
     if(p.kind==='media')mountVideo(p);
     else if(p.kind==='page')mountFrame(p);
     else{
       const div=node('div','stage-empty');div.append(node('span','play-emblem','↗'));
       const a=node('a','primary','打开官方播放页 ↗');a.href=p.url;a.target='_blank';a.rel='noopener noreferrer';div.append(a);$('stage').append(div);setPlaybackStatus('官方页面');
     }
   }catch(e){if(S.playSeq===playSeq){notice('workspace-message',e.message,true);if(e.code==='binding_required')$('binding-details').open=true;}}
   finally{renderEpisodes();}
 }
 function mountFrame(p){
   const f=document.createElement('iframe');f.title=p.title+' 第三方播放器';f.referrerPolicy='no-referrer';
   f.setAttribute('sandbox','allow-scripts allow-same-origin allow-presentation');f.setAttribute('allow','autoplay; fullscreen; encrypted-media');f.allowFullscreen=true;
   f.addEventListener('load',()=>{if(S.playback===p)setPlaybackStatus('播放页已打开');});
   setPlaybackStatus('加载中…');f.src=p.url;$('stage').append(f);
 }
 function mountVideo(p){
   const v=document.createElement('video');v.controls=true;v.playsInline=true;v.preload='metadata';$('stage').append(v);setHidden($('speed'),false);v.playbackRate=Number($('speed').value);
   if(p.format==='hls'&&window.Hls?.isSupported()){
     const hls=new Hls({enableWorker:false,maxBufferLength:30,capLevelToPlayerSize:true});S.hls=hls;hls.attachMedia(v);hls.loadSource(p.url);
     hls.on(Hls.Events.MANIFEST_PARSED,()=>{if(S.playback!==p)return;$('quality').replaceChildren(node('option','','自动画质'));$('quality').firstChild.value='-1';hls.levels.forEach((level,i)=>{const o=node('option','',level.height?`${level.height}p`:`线路 ${i+1}`);o.value=String(i);$('quality').append(o);});setHidden($('quality'),hls.levels.length<2);startVideo(v,p);});
     hls.on(Hls.Events.ERROR,(_,data)=>{if(data.fatal&&S.playback===p){S.playKey=null;setPlaybackStatus('播放失败','warn');notice('workspace-message','加载失败，请切换来源。',true);}});
   }else{v.src=p.url;startVideo(v,p);}
   const manualVideoAction=()=>{if(S.view==='workspace'&&!document.hidden)S.suspended=false;};
   v.addEventListener('pointerdown',manualVideoAction);v.addEventListener('keydown',manualVideoAction);
   let last=0,lastSaved=0;
   v.addEventListener('loadedmetadata',()=>{if(S.resume>0&&S.resume<v.duration)v.currentTime=S.resume;S.resume=0;});
   v.addEventListener('playing',()=>{if(!canPlay(p)){v.pause();return;}setPlaybackStatus('缓冲中…');});
   v.addEventListener('pause',()=>{if(S.playback===p&&!v.ended)setPlaybackStatus('已暂停');});
   v.addEventListener('timeupdate',()=>{
     if(S.playback!==p)return;
     const quality=v.getVideoPlaybackQuality?.();const frames=quality?quality.totalVideoFrames>0:v.videoWidth>0;
     const advanced=v.currentTime>last+.1&&v.currentTime-last<3;last=v.currentTime;
     if(!v.paused&&!v.seeking&&frames&&advanced){
       setPlaybackStatus('播放中','good');
       if(p.history_id&&(v.currentTime-lastSaved>3||!lastSaved)){
         lastSaved=v.currentTime;api('progress',{history_id:p.history_id,evidence:'browser_time_advanced',position:v.currentTime}).catch(()=>{});
       }
     }
   });
   v.addEventListener('error',()=>{if(S.playback!==p)return;S.playKey=null;setPlaybackStatus('播放失败','warn');notice('workspace-message','无法播放，请切换线路。',true);});
   v.addEventListener('ended',()=>{if(S.playback!==p)return;setPlaybackStatus('播放结束');if(canPlay(p)&&S.config.auto_next&&!$('next').disabled)step(1);});
 }
 $('speed').onchange=()=>{const v=$('stage').querySelector('video');if(v)v.playbackRate=Number($('speed').value);};
 $('quality').onchange=()=>{if(S.hls)S.hls.currentLevel=Number($('quality').value);};
 $('play').onclick=play;$('stop').onclick=stopPlayer;
 async function step(delta){const eps=S.group?.episodes||[];const i=eps.findIndex(e=>e.key===S.episode?.key);if(i<0||!eps[i+delta])return;S.episode=eps[i+delta];S.resume=0;renderEpisodes();await loadBinding();await play();}
 $('prev').onclick=()=>step(-1);$('next').onclick=()=>step(1);
 $('fullscreen').onclick=()=>{if(document.fullscreenElement)document.exitFullscreen();else $('stage').requestFullscreen().catch(()=>toast('当前浏览器未允许全屏。'));};
 document.addEventListener('keydown',e=>{if(e.key==='Escape'&&document.fullscreenElement)document.exitFullscreen().catch(()=>{});});
 function updateFavorite(){const saved=S.film&&S.favorites.has(S.film.id);$('favorite-btn').textContent=saved?'✓ 已收藏':'＋ 收藏';$('favorite-btn').disabled=!S.film;}
 $('favorite-btn').onclick=async()=>{if(!S.film)return;try{const r=await api('favorite',{film_id:S.film.id});if(r.saved)S.favorites.add(S.film.id);else S.favorites.delete(S.film.id);updateFavorite();toast(r.saved?'已收藏':'已取消收藏');}catch(e){toast(e.message);}};
 function table(headers,rows){const t=node('table'),h=node('thead'),tr=node('tr');headers.forEach(v=>tr.append(node('th','',v)));h.append(tr);t.append(h);const b=node('tbody');rows.forEach(cells=>{const r=node('tr');cells.forEach(c=>{const td=node('td');if(c instanceof Node)td.append(c);else td.textContent=String(c??'—');r.append(td);});b.append(r);});t.append(b);return t;}
 async function loadLibrary(reportError=true){
   const seq=++librarySeq;
   try{const r=await api('library');if(seq!==librarySeq)return;S.favorites=new Set(r.favorites.map(f=>f.id));updateFavorite();$('favorites-count').textContent=r.favorites.length+' 个收藏';
     $('favorites').replaceChildren(...r.favorites.map(card));if(!r.favorites.length)$('favorites').append(node('p','empty-text','暂无收藏'));
     const evidence={opened:'已打开',browser_time_advanced:'已播放',user_confirmed:'手动确认'};
     if(!r.history.length)$('history').replaceChildren(node('p','empty-text','暂无记录'));
     else $('history').replaceChildren(table(['作品 / 集数','实际来源','状态 / 时间','操作'],r.history.map(h=>{
       const title=node('div','',h.title);title.append(node('small','',episodeName(h.episode)));
       const status=node('div','',evidence[h.evidence]||h.evidence);status.append(node('small','',new Date(h.updated_at).toLocaleString()));
       const btn=node('button','small-button','重新打开');btn.onclick=()=>openFilm(h.film_id,h).catch(e=>toast(e.message));
       return [title,sourceName(h.source),status,btn];
     })));
   }catch(e){if(seq!==librarySeq)return;if(reportError)toast(e.message);else throw e;}
 }
 $('clear-history').onclick=()=>{if(clearingHistory)return;notice('clear-history-message','');$('clear-history-dialog').showModal();$('clear-history-cancel').focus();};
 $('clear-history-cancel').onclick=()=>{if(!clearingHistory)$('clear-history-dialog').close();};
 $('clear-history-dialog').addEventListener('cancel',e=>{if(clearingHistory)e.preventDefault();});
 $('clear-history-confirm').onclick=async()=>{
   if(clearingHistory)return;
   clearingHistory=true;librarySeq++;notice('clear-history-message','');
   const confirm=$('clear-history-confirm'),cancel=$('clear-history-cancel'),open=$('clear-history');
   confirm.disabled=cancel.disabled=open.disabled=true;confirm.textContent='清空中…';
   let cleared=false;
   try{await api('clear-history',{});cleared=true;await loadLibrary(false);$('clear-history-dialog').close();toast('记录已清空');}
   catch(e){notice('clear-history-message',cleared?'记录已清空，刷新失败，请重试。':e.message,true);}
   finally{clearingHistory=false;confirm.disabled=cancel.disabled=open.disabled=false;confirm.textContent='清空';}
 };
 function renderSources(){if(!S.config)return;
   const rows=[['无损云','搜索 / 播放','默认']];
   S.config.sources.forEach(s=>{const checkbox=node('input','source-toggle');checkbox.type='checkbox';checkbox.checked=s.enabled;checkbox.setAttribute('aria-label','启用 '+s.name);checkbox.onchange=async()=>{const enabled=Object.fromEntries(S.config.sources.map(x=>[x.id,x.enabled]));enabled[s.id]=checkbox.checked;try{await saveSettings({enabled_sources:enabled});toast('已保存');}catch(e){toast(e.message);checkbox.checked=!checkbox.checked;}};rows.push([s.name,'平台播放页',checkbox]);});
   $('sources').replaceChildren(table(['来源','用途','启用'],rows));
 }
 function renderProviders(){
   const registry=S.config.source_registry;if(!registry)return;const mode=$('search-mode'),old=mode.value;mode.replaceChildren();const options=[['all','全部来源'],['anime','动漫'],...registry.providers.filter(p=>p.enabled).map(p=>['provider:'+p.id,p.name]),...Object.entries(S.config.platforms).map(([id,p])=>[id,p.name])];for(const [id,name] of options){const o=node('option','',name);o.value=id;mode.append(o);}if(options.some(x=>x[0]===old))mode.value=old;
   $('providers-heading').textContent=`影视来源（${registry.providers.length}）`;
   $('providers').replaceChildren(table(['来源','类型','启用',''],registry.providers.map(p=>{const toggle=node('input','source-toggle');toggle.type='checkbox';toggle.checked=p.enabled;toggle.setAttribute('aria-label','启用 '+p.name);toggle.onchange=async()=>{const enabled=Object.fromEntries(registry.providers.map(x=>[x.id,x.enabled]));enabled[p.id]=toggle.checked;try{await saveSettings({enabled_providers:enabled});}catch(e){toggle.checked=!toggle.checked;toast(e.message);}};let action='';if(p.origin==='custom'){action=node('button','small-button','移除');action.onclick=async()=>{try{await api('sources/remove',{id:p.id});applyConfig(await api('state'));}catch(e){toast(e.message);}};}return [p.name,p.kind==='kazumi'?'动漫':p.kind==='cms'?'CMS':'影视',toggle,action];})));
   $('auto-update-sources').checked=registry.auto_update;$('update-status').textContent=registry.last_checked_at?'上次检查：'+new Date(registry.last_checked_at).toLocaleString():'尚未检查';
   const upstreamNames={'88lin':'88lin',kazumi_rules:'Kazumi 规则',kazumi:'Kazumi 程序'};$('upstream-status').replaceChildren(...Object.entries(registry.upstreams||{}).map(([id,u])=>node('div','upstream-row',`${upstreamNames[id]||id} · ${u.version||'—'} · ${u.status}${u.error?' · '+u.error:''}`)));$('update-changes').textContent=(registry.changes||[]).join('；');
 }
 function renderSettings(){if(!S.config)return;renderSources();renderProviders();$('network-consent').checked=S.config.network_consent;$('save-history').checked=S.config.save_history;}
 $('update-sources').onclick=async()=>{if(!await ensureNetwork())return;const b=$('update-sources');b.disabled=true;b.textContent='检查中…';try{await api('sources/update',{});applyConfig(await api('state'));toast('检查完成');}catch(e){toast(e.message);}finally{b.disabled=false;b.textContent='检查更新';}};
 $('add-source-open').onclick=()=>$('source-dialog').showModal();$('source-cancel').onclick=()=>$('source-dialog').close();
 $('source-form').onsubmit=async e=>{e.preventDefault();try{const raw=$('source-json').value.trim();let data={name:$('source-name').value,url:$('source-url').value};if(raw){const parsed=JSON.parse(raw);data={items:Array.isArray(parsed)?parsed:parsed.sources};if(!Array.isArray(data.items))throw Error('请输入来源 JSON 数组');}await api('sources/add',data);$('source-dialog').close();$('source-form').reset();applyConfig(await api('state'));toast('已添加');}catch(e){toast(e.message);}};
 $('diagnose').onclick=async()=>{if(!await ensureNetwork())return;const b=$('diagnose');b.disabled=true;b.textContent='检测中…';$('diagnostics').replaceChildren(node('p','empty-text','检测中…'));try{const r=await api('diagnose',{});$('diagnostics').replaceChildren(table(['来源','HTTP','结果'],r.results.map(x=>[x.name,x.http||'—',x.state])));}catch(e){$('diagnostics').replaceChildren(node('p','empty-text',e.message));}finally{b.disabled=false;b.textContent='检测连接';}};
 $('export-report').onclick=async()=>{try{const r=await api('report');const blob=new Blob([JSON.stringify(r,null,2)],{type:'application/json'});const url=URL.createObjectURL(blob);const a=node('a');a.href=url;a.download='yingxia_diagnostics.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}catch(e){toast(e.message);}};
 async function init(){
   try{applyConfig(await api('state'));populatePlatform($('binding-platform'));}
   catch(e){notice('search-message',e.message,true);}
   setInterval(()=>api('ping',{}).catch(()=>{}),20000);
   setInterval(()=>api('state').then(c=>{if(c.source_registry.last_checked_at!==S.config?.source_registry.last_checked_at)applyConfig(c);}).catch(()=>{}),60000);
 }
 init();
})();
