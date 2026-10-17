from pathlib import Path
from concurrent.futures import ThreadPoolExecutor
import hashlib,json
import probe_sources as p
HERE=Path(__file__).resolve().parent
u=json.loads((HERE/"kazumi_rule_updates.json").read_text(encoding="utf-8"))
sha=u["upstream"]["commit"]
base="https://raw.githubusercontent.com/Predidit/KazumiRules/"+sha+"/"
imeta,index,_=p.fetch(base+"index.json","kazumi_pinned_index",cap=500000)
if not isinstance(index,list) or not(0<len(index)<=80):raise ValueError("index shape")
def one(i):
    meta,rule,_=p.fetch(base+i["name"]+".json","kazumi_pinned_"+i["name"],cap=250000)
    if not isinstance(rule,dict) or rule.get("name")!=i["name"] or rule.get("version")!=i["version"]:raise ValueError("rule/index mismatch "+i["name"])
    return {"provider":{"id":"kazumi_"+i["name"].lower(),"name":i["name"],"kind":"kazumi","origin":"kazumi","enabled":True,"rule":rule},"probe":meta}
with ThreadPoolExecutor(max_workers=8) as pool:rules=list(pool.map(one,index))
out={"schema":1,"revision":"20261015","upstream":{"repository":"Predidit/KazumiRules","commit":sha,"index_probe":imeta},"providers":[r["provider"] for r in rules],"evidence":[r["probe"] for r in rules],"needs_api9_source_template_support":True}
p.save("kazumi_current_pinned_registry.json",out)
print("PINNED",sha,"COUNT",len(rules),"SHA",hashlib.sha256((HERE/"kazumi_current_pinned_registry.json").read_bytes()).hexdigest())
for r in rules:print(r["provider"]["id"],r["provider"]["rule"].get("api"),r["provider"]["rule"].get("version"))
tmeta,tree,_=p.fetch("https://api.github.com/repos/Predidit/Kazumi/git/trees/main?recursive=1","kazumi_code_tree",cap=2000000)
paths=[x["path"] for x in tree.get("tree",[]) if x.get("type")=="blob" and x["path"].endswith(".dart") and any(k in x["path"].lower() for k in ("plugin","rule","api"))] if isinstance(tree,dict) else []
p.save("kazumi_code_paths.json",{"probe":tmeta,"paths":paths})
print("DART_PATHS",paths)
