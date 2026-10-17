from __future__ import annotations
import threading
import time
from html.parser import HTMLParser
from urllib.parse import urljoin
from .models import AppError,Film,Group,Episode,film_from_vod,ranking,text
from .security import official_search,platform_url,PLATFORMS

class Catalog:
    def __init__(self,config,network):
        self.config,self.network=config,network
        self.lock=threading.RLock(); self.films={}; self.cache={}
    def _save(self,films):
        with self.lock:
            for f in films: self.films[f.id]=(time.monotonic(),f)
            if len(self.films)>1000:
                for key in sorted(self.films,key=lambda k:self.films[k][0])[:300]: del self.films[key]
    def search(self,query:str):
        key=query.strip()
        with self.lock:
            cached=self.cache.get(key)
            if cached and time.monotonic()-cached[0]<300:
                return cached[1],['来自 5 分钟本机会话缓存；不是实时播放检测。']
        warning=[]; films=[]
        try:
            data=self.network.json(self.config['suggest_url'],{'mid':1,'wd':key})
            entries=data.get('list')
            if not isinstance(entries,list): raise AppError('搜索结果缺少 list 数组。','bad_schema',502)
            summaries=[]
            for row in entries[:40]:
                if not isinstance(row,dict): continue
                try: summaries.append(film_from_vod(row))
                except AppError: continue
            if summaries:
                try:
                    details=self.network.json(self.config['api_url'],{'ac':'detail','ids':','.join(f.id for f in summaries)})
                    rows=details.get('list',[])
                    byid={}
                    if not isinstance(rows,list): raise AppError('详情格式不正确。','bad_schema',502)
                    for row in rows:
                        if isinstance(row,dict):
                            try:
                                f=film_from_vod(row);byid[f.id]=f
                            except AppError: continue
                    films=[byid.get(f.id,f) for f in summaries]
                    if len(byid)<len(summaries): warning.append('部分年份和选集尚未取得；选择作品时会再次读取详情。')
                except AppError:
                    films=summaries; warning.append('已取得片名，但详情接口未成功；点击作品可重试。')
            else:
                films=[]
        except AppError as first:
            # Same provider's public VOD API as fallback; not another verified catalogue.
            try:
                data=self.network.json(self.config['api_url'],{'ac':'detail','wd':key,'pg':1})
                rows=data.get('list')
                if not isinstance(rows,list): raise AppError('目录格式不正确。','bad_schema',502)
                for row in rows[:60]:
                    if isinstance(row,dict):
                        try: films.append(film_from_vod(row))
                        except AppError: continue
                warning.append('联想搜索未成功，已改用同一目录的详情搜索；仅显示返回的第一页。')
            except AppError as second:
                raise AppError(f'目录搜索未完成：{second.message} 可先运行离线演示或检查来源；没有伪造搜索结果。',second.code,second.status) from first
        films.sort(key=lambda f:ranking(key,f.title,f.year),reverse=True)
        self._save(films)
        with self.lock: self.cache[key]=(time.monotonic(),films)
        return films,warning
    def detail(self,ident):
        with self.lock:
            cached=self.films.get(ident)
            if cached and cached[1].groups and time.monotonic()-cached[0]<300: return cached[1]
        if not ident.isdecimal(): raise AppError('无效影片编号。','bad_id')
        data=self.network.json(self.config['api_url'],{'ac':'detail','ids':ident})
        rows=data.get('list')
        if not isinstance(rows,list): raise AppError('详情接口缺少影片列表。','bad_schema',502)
        for row in rows:
            if not isinstance(row,dict): continue
            if str(row.get('vod_id') or row.get('id'))==ident:
                film=film_from_vod(row);self._save([film]);return film
        raise AppError('接口未返回所选影片，已停止匹配，不会改播其他作品。','not_found',404)

class LinkParser(HTMLParser):
    def __init__(self):
        super().__init__();self.links=[];self.current=None;self.parts=[]
    def handle_starttag(self,tag,attrs):
        if tag=='a':
            d=dict(attrs);self.current=d;self.parts=[]
        elif tag=='img' and self.current is not None:
            self.parts.append(dict(attrs).get('alt',''))
    def handle_data(self,data):
        if self.current is not None:self.parts.append(data)
    def handle_endtag(self,tag):
        if tag=='a' and self.current is not None:
            d=self.current;self.links.append((d.get('href',''),d.get('title') or ''.join(self.parts)))
            self.current=None;self.parts=[]

def locate_platform(network,platform,query):
    """Best-effort static public links. Does not defeat login or anti-bot pages."""
    url=official_search(platform,query)
    raw,_=network.fetch(url,2_000_000)
    parser=LinkParser();parser.feed(raw.decode('utf-8',errors='replace'))
    result=[];seen=set()
    for href,label in parser.links:
        try: _,link=platform_url(urljoin(url,href),platform)
        except AppError:continue
        label=text(label,180)
        if not label or link in seen:continue
        seen.add(link)
        if ranking(query,label)>=30:result.append({'title':label,'url':link,'platform':platform})
    result.sort(key=lambda x:ranking(query,x['title']),reverse=True)
    return result[:30],url
