from __future__ import annotations
import concurrent.futures
import hashlib
import json
import re
import threading
from pathlib import Path
from urllib.parse import quote,urlsplit
from . import __version__
from .models import AppError,Film,Group,Episode,text,choose_episode
from .security import public_url,platform_url,official_search,PLATFORMS
from .net import Network
from .catalog import Catalog,locate_platform
from .storage import Store

ROOT=Path(__file__).resolve().parent

def demo_films():
    """Synthetic local clips; never mixed into online search results."""
    return [Film(id='demo-orbit-1',title='轨道片段 第一季',year='演示',kind='本地自生成测试',
        remarks='2 段短视频 · 非影视资源',summary='本内容由程序合成，用于离线验证搜索、选集、切源、播放时间推进及历史记录。不是电影或电视剧片源。',
        source='本机合成演示',demo=True,groups=[
            Group('g0','本机 WebM',[Episode('e0','第01集','/demo/clip1.webm',1),Episode('e1','第02集','/demo/clip2.webm',2)]),
            Group('g1','备用演示组（故意缺第2集）',[Episode('e0','第1集','/demo/clip1.webm',1)])]),
        Film(id='demo-orbit-2',title='轨道片段 第二季',year='演示',kind='季数区分测试',remarks='1 段短视频 · 非影视资源',
             summary='第二季独立保留；搜索和收藏不会与第一季自动合并。',source='本机合成演示',demo=True,
             groups=[Group('g0','本机 WebM',[Episode('e0','第一集','/demo/clip2.webm',1)])])]

class Service:
    def __init__(self,data_dir:Path,network=None,config=None):
        self.data_dir=data_dir;self.store=Store(data_dir);self.network=network or Network()
        custom=data_dir/'sources.json'
        self.config=config or json.loads((custom if custom.exists() else ROOT/'sources.json').read_text(encoding='utf-8-sig'))
        self.validate_config()
        self.catalog=Catalog(self.config['catalog'],self.network)
        self.demo={f.id:f for f in demo_films()};self.link_films={};self.link_lock=threading.RLock()
        self.diagnostics=[]
    def validate_config(self):
        try:
            cat=self.config['catalog']
            for key in ('suggest_url','api_url'):public_url(cat[key])
            for item in [{'template':cat['player_url']}]+self.config['parsers']:
                t=item['template']
                if t.count('{url}')!=1 or re.sub(r'\{url\}','',t).count('{'):raise ValueError()
                public_url(t.replace('{url}','https%3A%2F%2Fexample.com%2F'))
            ids=[s['id'] for s in self.config['parsers']]
            if len(ids)!=len(set(ids)) or not all(re.fullmatch(r'[a-z0-9_]{1,30}',s) for s in ids):raise ValueError()
        except (KeyError,TypeError,ValueError): raise AppError('sources.json 格式不正确，请恢复默认配置。','config')
    def require_network(self):
        if not self.store.get('network_consent',False):
            raise AppError('首次使用请先确认第三方网络提示；离线演示无需联网。','consent_required',403)
    def enabled_sources(self):
        overrides=self.store.get('enabled_sources',{})
        return [{**s,'enabled':overrides.get(s['id'],s['enabled'])} for s in self.config['parsers']]
    def state(self):
        return {'version':__version__,'network_consent':self.store.get('network_consent',False),
            'save_history':self.store.get('save_history',True),'platforms':{k:{'name':v['name']} for k,v in PLATFORMS.items()},
            'sources':self.enabled_sources(),'catalog':{'name':self.config['catalog']['name'],'status':self.config['catalog']['status']},
            'data_dir':str(self.data_dir),'favorite_ids':[f['id'] for f in self.store.favorites()],
            'last_diagnostics':self.diagnostics,
            'limits':['仅一条在线目录，不是全网搜索','平台静态检索不保证取得链接','网页加载不等于视频播放','Windows 真机与第三方实播尚未验收']}
    def settings(self,data):
        for key in ('network_consent','save_history'):
            if key in data:
                if not isinstance(data[key],bool):raise AppError('设置值应为布尔类型。')
                self.store.set(key,data[key])
        if 'enabled_sources' in data:
            items=data['enabled_sources'];known={s['id'] for s in self.config['parsers']}
            if not isinstance(items,dict) or any(k not in known or not isinstance(v,bool) for k,v in items.items()):
                raise AppError('来源设置不正确。')
            self.store.set('enabled_sources',items)
        return self.state()
    @staticmethod
    def query(data):
        q=data.get('query','')
        if not isinstance(q,str) or not 1<=len(q.strip())<=120:raise AppError('请输入 1–120 个字符的片名。','query')
        return q.strip()
    def search(self,data):
        if data.get('demo') is True:
            return {'items':[f.public() for f in self.demo.values()], 'warnings':['离线演示：自生成测试视频，不是在线影视资源。'],'demo':True}
        q=self.query(data);self.require_network();mode=data.get('mode','catalog')
        if mode=='catalog':
            films,warnings=self.catalog.search(q)
            return {'items':[f.public() for f in films],'warnings':warnings+['目录名称相同不代表年份、季数和版本相同，请核对后选择。'],'demo':False}
        if mode not in PLATFORMS:raise AppError('未知搜索模式。')
        url=official_search(mode,q)
        try: links,url=locate_platform(self.network,mode,q);err=''
        except AppError as e:links=[];err=e.message
        films=[]
        with self.link_lock:
            for link in links:
                film=self.make_link_film(link['url'],link['title'],mode)
                films.append(film.public())
        warning='平台公开搜索页静态检索；候选标题、版本和集数需要人工核对，不读取登录账号。'
        if not links:warning='未取得可确认的静态播放链接。可打开官方搜索，进入具体播放页后粘贴网址；不会绕过验证页。'+(' '+err if err else '')
        return {'items':films,'warnings':[warning],'official_search_url':url,'demo':False}
    def make_link_film(self,url,title,platform):
        key='link-'+hashlib.sha256(url.encode()).hexdigest()[:20]
        film=Film(id=key,title=text(title,200) or '用户提供的平台播放页',kind=PLATFORMS[platform]['name'],
            source=PLATFORMS[platform]['name']+'播放页（版本及集数需核对）',remarks='仅当前播放页，非完整选集',
            summary='该候选来自公开搜索页或用户粘贴。此模式不自动取得全剧选集；换集需提供对应播放页。',
            groups=[Group('official',PLATFORMS[platform]['name']+'页面',[Episode('page','当前播放页（集数未核验）',url,None)])])
        with self.link_lock:self.link_films[key]=film
        self.store.bind(key,film.groups[0].episodes[0].name,platform,url)
        return film
    def import_platform(self,data):
        p,url=platform_url(data.get('url',''),data.get('platform') or None)
        # No request or external forwarding occurs here.
        return self.make_link_film(url,data.get('title','用户提供的平台播放页'),p).public()
    def detail(self,ident):
        if not isinstance(ident,str):raise AppError('无效影片编号。')
        if ident in self.demo:return self.demo[ident]
        with self.link_lock:
            if ident in self.link_films:return self.link_films[ident]
        if ident.startswith('link-'):
            # Restore only the explicit binding; never silently search a different title.
            binds=self.store.bindings(ident,'当前播放页（集数未核验）')
            if binds:
                saved=next((f for f in self.store.favorites() if f['id']==ident),{})
                return self.make_link_film(binds[0]['url'],saved.get('title','已保存的平台播放页'),binds[0]['platform'])
            raise AppError('该平台候选已过期，请重新粘贴播放页。','not_found',404)
        self.require_network();return self.catalog.detail(ident)
    def detail_public(self,ident):
        film=self.detail(ident);d=film.public()
        if film.groups and film.groups[0].episodes:
            d['bindings']=self.store.bindings(ident,film.groups[0].episodes[0].name)
        return d
    def bind(self,data):
        film=self.detail(data.get('film_id',''));name=text(data.get('episode',''),120)
        if not name:raise AppError('请先选择对应集数。')
        p,url=platform_url(data.get('url',''),data.get('platform') or None)
        self.store.bind(film.id,name,p,url)
        return {'platform':p,'url':url,'note':'用户绑定；未自动验证同一作品/集数。'}
    def select(self,data):
        film=self.detail(data.get('film_id',''))
        group=next((g for g in film.groups if g.key==data.get('group_key')),None)
        if not group:raise AppError('请先选择有效的来源组。','bad_group')
        if 'match_name' in data:ep=choose_episode(group,str(data['match_name']))
        else:ep=next((e for e in group.episodes if e.key==data.get('episode_key')),None)
        if ep is None:raise AppError('该集已不存在，请刷新详情；不会自动改播第一集。','episode_missing')
        return film,group,ep
    def match(self,data):
        f,g,e=self.select(data);return e.public()
    def resolve(self,data):
        f,g,e=self.select(data)
        mode=data.get('source','catalog')
        if f.demo:
            kind,url,source='media',e.url,'本机合成演示 / '+g.name
        else:
            self.require_network()
            if mode in ('catalog','native') and g.key!='official':
                public_url(e.url)
                if mode=='native':
                    kind,url='media',e.url
                    source='目录媒体直连 / '+g.name
                else:
                    kind='page';url=self.config['catalog']['player_url'].replace('{url}',quote(e.url,safe=''))
                    source='无损云网页播放器 / '+g.name
            else:
                provided=data.get('platform_url','')
                if not provided:
                    binds=self.store.bindings(f.id,e.name)
                    provided=binds[0]['url'] if binds else e.url if g.key=='official' else ''
                if not provided:raise AppError('此线路需要官方播放页。请先绑定这一集的平台网址，不能把目录地址冒充平台地址。','binding_required')
                p,official=platform_url(provided,data.get('platform') or None)
                if mode=='official' or (mode=='catalog' and g.key=='official'):
                    kind,url,source='external',official,PLATFORMS[p]['name']+'官方页面'
                else:
                    entry=next((s for s in self.enabled_sources() if s['id']==mode),None)
                    if not entry or not entry['enabled']:raise AppError('该解析来源未启用，请到来源设置检查。','source_disabled')
                    kind='page';url=entry['template'].replace('{url}',quote(official,safe=''))
                    source=entry['name']+'（第三方） / 定位平台：'+PLATFORMS[p]['name']
        ident=self.store.opened(f.id,f.title,e.name,source,kind)
        return {'kind':kind,'url':url,'source_label':source,'title':f.title,'episode':e.name,'history_id':ident,
            'film_id':f.id,'group_key':g.key,'episode_key':e.key,'demo':f.demo,'playback_verified':False,
            'notice':'仅已生成播放入口，网页模式需要播放器自己加载，不能据此确认片源可播。'}
    def diagnose(self):
        self.require_network()
        cat=self.config['catalog']
        jobs=[('网络对照 python.org','https://www.python.org/'),('无损云目录接口',cat['api_url']),
              ('无损云网页播放器',cat['player_url'].split('?')[0])]
        jobs += [(s['name'],s['template'].split('?')[0]) for s in self.enabled_sources() if s['enabled']]
        with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
            results=list(pool.map(lambda pair:self.network.probe(*pair),jobs))
        self.diagnostics=results
        return {'results':results,'scope':'仅 HTTP/JSON 预检，不执行远程脚本，也不解码视频。',
                'playback_verified':False}
    def export_report(self):
        return {'app':'YingXia','version':__version__,'created_at':self.store.now(),
            'scope':'诊断仅代表接口响应；无来源被软件自动认证为可播。',
            'diagnostics':self.diagnostics,'config_provenance':self.config['provenance'],
            'notes':['未导出搜索词、Cookie、媒体地址、平台链接或观看记录','远程页面与 Windows 真机尚未开发端验收']}
