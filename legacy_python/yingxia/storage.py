"""Per-user SQLite state. No browser profiles or login credentials are imported."""
from __future__ import annotations
import datetime as dt
import json
import sqlite3
import threading
import uuid
from pathlib import Path
from .models import AppError

class Store:
    def __init__(self, directory: Path):
        directory.mkdir(parents=True,exist_ok=True)
        self.path=directory/'library.sqlite3'; self.lock=threading.RLock()
        self.db=sqlite3.connect(self.path,check_same_thread=False)
        self.db.row_factory=sqlite3.Row
        with self.db:
            self.db.executescript('''
              PRAGMA journal_mode=WAL;
              CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
              CREATE TABLE IF NOT EXISTS favorites(film_id TEXT PRIMARY KEY,metadata TEXT NOT NULL,created_at TEXT NOT NULL);
              CREATE TABLE IF NOT EXISTS history(id TEXT PRIMARY KEY,film_id TEXT NOT NULL,title TEXT NOT NULL,
                episode TEXT NOT NULL,source TEXT NOT NULL,mode TEXT NOT NULL,position REAL DEFAULT 0,
                evidence TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
              CREATE TABLE IF NOT EXISTS bindings(film_id TEXT NOT NULL,episode TEXT NOT NULL,platform TEXT NOT NULL,
                url TEXT NOT NULL,PRIMARY KEY(film_id,episode,platform));
            ''')
    def get(self,key,default=None):
        with self.lock:
            row=self.db.execute('SELECT value FROM settings WHERE key=?',(key,)).fetchone()
            return json.loads(row[0]) if row else default
    def set(self,key,value):
        with self.lock,self.db:
            self.db.execute('INSERT OR REPLACE INTO settings VALUES(?,?)',(key,json.dumps(value,ensure_ascii=False)))
    def favorites(self):
        with self.lock:
            return [json.loads(r[0]) for r in self.db.execute('SELECT metadata FROM favorites ORDER BY created_at DESC')]
    def toggle_favorite(self,film:dict):
        ident=film['id']
        with self.lock,self.db:
            old=self.db.execute('SELECT 1 FROM favorites WHERE film_id=?',(ident,)).fetchone()
            if old: self.db.execute('DELETE FROM favorites WHERE film_id=?',(ident,))
            else:
                data={k:v for k,v in film.items() if k!='groups'}
                self.db.execute('INSERT INTO favorites VALUES(?,?,?)',(ident,json.dumps(data,ensure_ascii=False),self.now()))
            return not bool(old)
    @staticmethod
    def now(): return dt.datetime.now(dt.timezone.utc).isoformat(timespec='seconds')
    def opened(self,film_id,title,episode,source,mode):
        if not self.get('save_history',True): return ''
        ident=uuid.uuid4().hex; now=self.now()
        with self.lock,self.db:
            self.db.execute('INSERT INTO history VALUES(?,?,?,?,?,?,0,?,?,?)',
                (ident,film_id,title,episode,source,mode,'opened',now,now))
            self.db.execute('DELETE FROM history WHERE id NOT IN (SELECT id FROM history ORDER BY created_at DESC LIMIT 300)')
        return ident
    def progress(self,ident,evidence,position=0):
        if evidence not in ('browser_time_advanced','user_confirmed'): raise AppError('无效播放证据类型。')
        if not isinstance(position,(int,float)) or not 0<=position<=604800: raise AppError('无效播放进度。')
        with self.lock,self.db:
            self.db.execute('UPDATE history SET position=?, evidence=?, updated_at=? WHERE id=?',
                (position,evidence,self.now(),ident))
    def history(self):
        with self.lock:
            return [dict(r) for r in self.db.execute('SELECT * FROM history ORDER BY created_at DESC LIMIT 100')]
    def clear_history(self):
        with self.lock,self.db: self.db.execute('DELETE FROM history')
    def bind(self,film_id,episode,platform,url):
        with self.lock,self.db:
            self.db.execute('INSERT OR REPLACE INTO bindings VALUES(?,?,?,?)',(film_id,episode,platform,url))
    def bindings(self,film_id,episode):
        with self.lock:
            return [dict(r) for r in self.db.execute('SELECT * FROM bindings WHERE film_id=? AND episode=?',(film_id,episode))]
    def close(self):
        with self.lock: self.db.close()
