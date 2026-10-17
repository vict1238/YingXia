from pathlib import Path
from concurrent.futures import ThreadPoolExecutor
import json,re
import probe_sources as p
HERE=Path(__file__).resolve().parent
cmeta,c,_=p.fetch("https://api.github.com/repos/Predidit/Kazumi/commits/main","kazumi_app_latest_commit",cap=1000000)
sha=c.get("sha") if isinstance(c,dict) else None
if not sha:raise ValueError("missing app commit")
paths=["lib/plugins/api_rule_config.dart","lib/services/plugin/api_rule_engine.dart","lib/services/plugin/api_rule_strategy.dart","test/api_rule_engine_test.dart","test/plugin_api_config_test.dart","lib/plugins/plugins.dart"]
def one(path):
    meta,_,text=p.fetch("https://raw.githubusercontent.com/Predidit/Kazumi/"+sha+"/"+path,"kazumi_api9_"+path.replace("/","_"),cap=500000)
    lines=text.splitlines();excerpts=[]
    for i,line in enumerate(lines):
        if any(k.lower() in line.lower() for k in ("sourceTemplate","source_template","sourceValue","expandTemplate","apiVersion","api =","api:")):
            excerpts.append({"line":i+1,"context":"\n".join(lines[max(0,i-4):min(len(lines),i+9)])})
    print(path)
    for e in excerpts:print(e["line"],e["context"])
    return {"path":path,"probe":meta,"relevant_excerpts":excerpts}
with ThreadPoolExecutor(max_workers=6) as pool:results=list(pool.map(one,paths))
p.save("kazumi_api9_primary_source.json",{"repository":"Predidit/Kazumi","commit":sha,"commit_probe":cmeta,"files":results})
