from pathlib import Path
import hashlib,json
import probe_sources as p
HERE=Path(__file__).resolve().parent
repo="Predidit/KazumiRules"
cmeta,commit,_=p.fetch("https://api.github.com/repos/"+repo+"/commits/main","kazumi_latest_commit",cap=1000000)
sha=commit.get("sha") if isinstance(commit,dict) else None
url="https://raw.githubusercontent.com/"+repo+"/"+(sha or "main")+"/giriGiriLove.json"
rmeta,rule,_=p.fetch(url,"kazumi_girigirilove_2_5",cap=250000)
if not isinstance(rule,dict) or rule.get("name")!="giriGiriLove" or rule.get("version")!="2.5":raise ValueError("official rule/index mismatch")
out={"schema":1,"revision":"20261015","upstream":{"repository":repo,"commit":sha,"commit_probe":cmeta,"rule_probe":rmeta},"providers":[{"id":"kazumi_girigirilove","name":"giriGiriLove","kind":"kazumi","origin":"kazumi","enabled":True,"rule":rule}]}
out["validated_for_direct_import"]=False
out["requires_adapter_change"]="Kazumi API 9 searchApiConfig.sourceTemplate is absent from baseline API 8 adapter"
search_meta,search,_=p.fetch(p.query("https://ani.girigirilove.com/index.php/ajax/suggest",mid="1",wd="海贼王"),"kazumi_giri_2_5_search")
entries=search.get("list",[]) if isinstance(search,dict) else []
out["live_search_probe"]={**search_meta,"valid_items":len(entries),"query":"海贼王"}
if entries and isinstance(entries[0],dict) and entries[0].get("id"):
    source=str(entries[0]["id"])
    detail_meta,_,detail=p.fetch("https://ani.girigirilove.com/GV"+source+"/","kazumi_giri_2_5_detail")
    out["live_detail_probe"]={**detail_meta,"sample_id":source,"has_chapter_list_class":"anthology-list-play size" in detail,"decode_playback_tested":False}
p.save("kazumi_rule_updates.json",out)
print("COMMIT",sha,"RULE",rule["version"],"API",rule.get("api"),"SOURCE",rule.get("baseURL"))
print("SHA",hashlib.sha256((HERE/"kazumi_rule_updates.json").read_bytes()).hexdigest())
