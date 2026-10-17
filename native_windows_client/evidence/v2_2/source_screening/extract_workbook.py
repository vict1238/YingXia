"""Read the supplied source workbook as data; do not execute any workbook text."""
from pathlib import Path
import hashlib, json, re
import openpyxl

SOURCE = Path(r"C:\Users\vict1238\Downloads\YingXia_Video_Sources_20261015.xlsx")
HERE = Path(__file__).resolve().parent
book = openpyxl.load_workbook(SOURCE, data_only=False, read_only=False)
cached = openpyxl.load_workbook(SOURCE, data_only=True, read_only=False)
out = {"file": str(SOURCE), "size": SOURCE.stat().st_size, "sha256": hashlib.sha256(SOURCE.read_bytes()).hexdigest(), "read_only": True, "sheets": [], "urls": []}
url_refs = {}
for sheet in book:
    rows=[]
    for row in sheet:
        cells=[]
        for c in row:
            if c.value is None and not c.hyperlink and not c.comment:
                continue
            item={"cell": c.coordinate, "value": c.value, "type": c.data_type}
            if c.data_type == "f":
                item["cached_value"]=cached[sheet.title][c.coordinate].value
            if c.hyperlink:
                item["hyperlink"]={"target": c.hyperlink.target, "location": c.hyperlink.location}
            if c.comment:
                item["comment"]=c.comment.text
            cells.append(item)
            values=[str(c.value or ""), c.hyperlink.target if c.hyperlink else "", c.comment.text if c.comment else ""]
            for value in values:
                for url in re.findall(r"https?://[^\s\"'<>\[\]，。；、）)]+", value or ""):
                    url_refs.setdefault(url,[]).append({"sheet":sheet.title,"cell":c.coordinate})
        if cells:
            rows.append({"row":row[0].row,"cells":cells})
    out["sheets"].append({"name":sheet.title,"state":sheet.sheet_state,"max_row":sheet.max_row,"max_column":sheet.max_column,"rows":rows})
out["urls"]=[{"url":u,"references":r} for u,r in url_refs.items()]
(HERE/"workbook_extracted.json").write_text(json.dumps(out,ensure_ascii=False,indent=2,default=str),encoding="utf-8")
for s in out["sheets"]:
    print("SHEET",s["name"],s["max_row"],s["max_column"])
    for row in s["rows"]:
        if s["name"] not in ["01_影视采集API", "02_动漫站点", "03_解析服务", "04_网页网盘"]:
            continue
        print(row["row"], " | ".join(str(c["value"]) for c in row["cells"] if re.match(r"^[ABC][0-9]+$",c["cell"])))
print("UNIQUE_URLS",len(url_refs))
