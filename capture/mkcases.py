"""Writes capture/cases/*.json for xapi-capture. Usage: python3 capture/mkcases.py capture/cases"""
import json, os, sys

OUT = sys.argv[1]
TEXT = ["nexacro-xml-4000", "xplatform-xml-5000", "nexacro-ssv", "xplatform-ssv", "nexacro-json-1.0"]
BIN = ["nexacro-binary-5000", "xplatform-binary-5000"]
SHORT = {"nexacro-xml-4000": "nexacro-xml", "xplatform-xml-5000": "xplatform-xml", "nexacro-ssv": "nexacro-ssv",
         "xplatform-ssv": "xplatform-ssv", "nexacro-json-1.0": "nexacro-json", "nexacro-binary-5000": "nexacro-binary",
         "xplatform-binary-5000": "xplatform-binary"}


def v(state, lexical=None):
    if state == "value":
        return {"state": "value", "lexical": lexical}
    if state == "empty":
        return {"state": "empty", "lexical": ""}
    return {"state": state}


def param(i, t, cell):
    return dict({"id": i, "type": t}, **cell)


def ds(id, cols, rows, consts=(), **extra):
    d = {"id": id, "columns": [{"id": c, "type": t, "index": n} for n, (c, t) in enumerate(cols)],
         "constColumns": list(consts), "rows": rows}
    d.update(extra)
    return d


def row(t, values, org=None):
    r = {"type": t, "values": values}
    if org is not None:
        r["orgRow"] = {"type": "O", "values": org}
    return r


def save(name, description, cases):
    with open(os.path.join(OUT, name + ".json"), "w") as f:
        json.dump({"description": description, "cases": cases}, f, ensure_ascii=False, indent=1)
        f.write("\n")
    print(name, len(cases))


# 1. escaping ---------------------------------------------------------------
ESC = {
    "amp": "a&b", "lt": "a<b", "gt": "a>b", "quot": 'a"b', "apos": "a'b", "tab": "a\tb", "cr": "a\rb",
    "lf": "a\nb", "crlf": "a\r\nb", "edge-space": " a b ", "only-space": "   ", "cdata-end": "a]]>b",
    "entity-text": "&amp;&lt;", "charref-text": "&#10;&#x41;", "korean": "한글", "emoji": "\U0001F600",
    "control": "a\x01b", "ssv-separators": "a\x1eb\x1fc", "ssv-undefined": "\x03", "backslash": "a\\b/c",
    "json-escapes": "\b\f ",
}
cases = []
for p in TEXT:
    for k, s in ESC.items():
        val = {"parameters": [param("p", "STRING", v("value", s))],
               "datasets": [ds("d", [("a", "STRING")], [row("N", {"a": v("value", s)})])]}
        cases.append({"id": f"driver.{SHORT[p]}.escape-{k}.encode", "profile": p, "operation": "encode",
                      "value": val, "alsoDecode": True, "faithful": True})
        val = {"parameters": [], "datasets": [ds("d", [("a", "STRING")], [row("N", {"a": v("value", "x")})],
                                                 consts=[{"id": "k", "type": "STRING", "value": v("value", s)}])]}
        cases.append({"id": f"driver.{SHORT[p]}.escape-const-{k}.encode", "profile": p, "operation": "encode",
                      "value": val, "alsoDecode": True, "faithful": True})
save("escape", "Escaping of text, ids and separators; expected bytes are the original driver's.", cases)

# 2. types: default size and null representation -----------------------------
TYPES = [("STRING", "s"), ("INT", "12"), ("LONG", "1234567890123"), ("FLOAT", "1.5"), ("DOUBLE", "2.25"),
         ("DECIMAL", "3.75"), ("BIGDECIMAL", "123.45"), ("BOOLEAN", "1"), ("DATE", "20261116"),
         ("TIME", "123456789"), ("DATETIME", "20261116123456789"), ("BLOB", "AAEC")]
cases = []
for p in TEXT + BIN:
    cols = [(t.lower(), t) for t, _ in TYPES]
    vals = {t.lower(): v("value", lx) for t, lx in TYPES}
    nulls = {t.lower(): v("null") for t, _ in TYPES}
    cases.append({"id": f"driver.{SHORT[p]}.types-columns.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [], "datasets": [ds("d", cols, [row("N", vals), row("N", nulls)])]},
                  "alsoDecode": True})
    cases.append({"id": f"driver.{SHORT[p]}.types-parameters.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [param(t.lower(), t, v("value", lx)) for t, lx in TYPES], "datasets": []},
                  "alsoDecode": True})
    cases.append({"id": f"driver.{SHORT[p]}.types-null-parameters.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [param(t.lower(), t, v("null")) for t, _ in TYPES], "datasets": []},
                  "alsoDecode": True})
    cases.append({"id": f"driver.{SHORT[p]}.types-constants.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [], "datasets": [ds("d", [("a", "STRING")], [row("N", {"a": v("value", "x")})],
                            consts=[{"id": t.lower(), "type": t, "value": v("value", lx)} for t, lx in TYPES
                                    # A binary TIME constant is written on the current date.
                                    if not (t == "TIME" and p in BIN)])]},
                  "alsoDecode": True})
    sized = [{"id": "s", "type": "STRING", "index": 0, "size": "7"}, {"id": "i", "type": "INT", "index": 1, "size": "2"},
             {"id": "b", "type": "BIGDECIMAL", "index": 2, "size": "5"}]
    cases.append({"id": f"driver.{SHORT[p]}.types-explicit-size.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [], "datasets": [{"id": "d", "columns": sized, "constColumns": [],
                            "rows": [row("N", {"s": v("value", "x"), "i": v("value", "1"), "b": v("value", "1.5")})]}]},
                  "alsoDecode": True})
    cases.append({"id": f"driver.{SHORT[p]}.empty-string-and-blob.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [param("s", "STRING", v("empty")), param("b", "BLOB", v("empty"))],
                            "datasets": [ds("d", [("s", "STRING"), ("b", "BLOB")], [row("N", {"s": v("empty"), "b": v("empty")})])]},
                  "alsoDecode": True})
save("types", "Type names, default sizes, null and empty representation; expected result is the original driver's.", cases)

# 3. decode of empty values and BLOB parameters -------------------------------
DT = ["string", "int", "long", "float", "double", "decimal", "bigdecimal", "boolean", "date", "time", "datetime", "blob"]
NX = 'xmlns="http://www.nexacroplatform.com/platform/dataset"'
XP = 'xmlns="http://www.tobesoft.com/platform/dataset" ver="5000"'
cases = []
for p, root in (("nexacro-xml-4000", NX), ("xplatform-xml-5000", XP)):
    params = "".join(f'<Parameter id="{t}" type="{t}"></Parameter><Parameter id="{t}2" type="{t}"/>' for t in DT)
    colinfo = "".join(f'<Column id="{t}" type="{t}" size="8"/>' for t in DT)
    r1 = "".join(f'<Col id="{t}"></Col>' for t in DT)
    r2 = "".join(f'<Col id="{t}"/>' for t in DT)
    text = (f'<?xml version="1.0" encoding="UTF-8"?>\n<Root {root}><Parameters>{params}</Parameters>'
            f'<Dataset id="d"><ColumnInfo>{colinfo}</ColumnInfo><Rows><Row>{r1}</Row><Row>{r2}</Row><Row></Row></Rows></Dataset></Root>')
    cases.append({"id": f"driver.{SHORT[p]}.empty-values.decode", "profile": p, "operation": "decode", "text": text})
    text = (f'<?xml version="1.0" encoding="UTF-8"?>\n<Root {root}><Parameters><Parameter id="b" type="blob">AAEC</Parameter>'
            f'<Parameter id="e" type="blob"></Parameter></Parameters><Dataset id="d"><ColumnInfo><Column id="b" type="blob"/>'
            f'</ColumnInfo><Rows><Row><Col id="b">AAEC</Col></Row></Rows></Dataset></Root>')
    cases.append({"id": f"driver.{SHORT[p]}.blob-values.decode", "profile": p, "operation": "decode", "text": text})
RS, US = "\x1e", "\x1f"
for p in ("nexacro-ssv", "xplatform-ssv"):
    params = "".join(f"{t}:{t}={RS}" for t in DT)
    hdr = US.join(["_RowType_"] + [f"{t}:{t}(8)" for t in DT])
    r1 = US.join(["N"] + [""] * len(DT))
    r2 = US.join(["N"] + ["\x03"] * len(DT))
    text = f"SSV:UTF-8{RS}{params}Dataset:d{RS}{hdr}{RS}{r1}{RS}{r2}{RS}{RS}"
    cases.append({"id": f"driver.{SHORT[p]}.empty-values.decode", "profile": p, "operation": "decode", "text": text})
    text = f"SSV:UTF-8{RS}b:blob=AAEC{RS}e:blob={RS}Dataset:d{RS}_RowType_{US}b:blob(8){RS}N{US}AAEC{RS}{RS}"
    cases.append({"id": f"driver.{SHORT[p]}.blob-values.decode", "profile": p, "operation": "decode", "text": text})
jp = [{"id": t, "type": t, "value": ""} for t in DT] + [{"id": t + "2", "type": t, "value": None} for t in DT]
jd = {"id": "d", "ColumnInfo": {"Column": [{"id": t, "type": t, "size": "8"} for t in DT]},
      "Rows": [{t: "" for t in DT}, {t: None for t in DT}, {}]}
cases.append({"id": "driver.nexacro-json.empty-values.decode", "profile": "nexacro-json-1.0", "operation": "decode",
              "text": json.dumps({"version": "1.0", "Parameters": jp, "Datasets": [jd]})})
cases.append({"id": "driver.nexacro-json.blob-values.decode", "profile": "nexacro-json-1.0", "operation": "decode",
              "text": json.dumps({"version": "1.0", "Parameters": [{"id": "b", "type": "blob", "value": "AAEC"},
                                 {"id": "e", "type": "blob", "value": ""}],
                                 "Datasets": [{"id": "d", "ColumnInfo": {"Column": [{"id": "b", "type": "blob"}]},
                                               "Rows": [{"b": "AAEC"}]}]})})
cases.append({"id": "driver.nexacro-json.numeric-size.decode", "profile": "nexacro-json-1.0", "operation": "decode",
              "text": json.dumps({"version": "1.0", "Datasets": [{"id": "d", "ColumnInfo": {"Column": [{"id": "a", "type": "string", "size": 32}]},
                                  "Rows": [{"a": "x"}]}]})})
save("empty", "How the original driver reads empty values and BLOB text.", cases)

# 4. row state x saveType -----------------------------------------------------
def changed(**extra):
    return ds("d", [("a", "STRING")], [
        row("N", {"a": v("value", "n1")}),
        row("I", {"a": v("value", "i1")}),
        row("U", {"a": v("value", "u1")}, org={"a": v("value", "o1")}),
        row("U", {"a": v("value", "u2")}),
        row("D", {"a": v("value", "d1")}),
        row("N", {"a": v("value", "n2")}),
    ], **extra)


cases = []
for p in TEXT + BIN:
    s = SHORT[p]
    cases.append({"id": f"driver.{s}.savetype-unset.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [], "datasets": [changed()]}, "alsoDecode": True})
    for st in range(1, 6):
        cases.append({"id": f"driver.{s}.savetype-dataset-{st}.encode", "profile": p, "operation": "encode",
                      "value": {"parameters": [], "datasets": [changed(saveType=st)]}, "alsoDecode": True})
    cases.append({"id": f"driver.{s}.savetype-root-1.encode", "profile": p, "operation": "encode",
                  "value": {"saveType": 1, "parameters": [], "datasets": [changed()]}, "alsoDecode": True})
    cases.append({"id": f"driver.{s}.savetype-dataset-overrides-root.encode", "profile": p, "operation": "encode",
                  "value": {"saveType": 1, "parameters": [], "datasets": [changed(saveType=3)]}, "alsoDecode": True})
xml_rows = ('<Rows><Row><Col id="a">n1</Col></Row><Row type="delete"><Col id="a">d1</Col></Row>'
            '<Row type="update"><Col id="a">u1</Col><OrgRow><Col id="a">o1</Col></OrgRow></Row>'
            '<Row type="insert"><Col id="a">i1</Col></Row><Row><Col id="a">n2</Col></Row></Rows>')
for p, root in (("nexacro-xml-4000", NX), ("xplatform-xml-5000", XP)):
    text = (f'<?xml version="1.0" encoding="UTF-8"?>\n<Root {root}><Dataset id="d"><ColumnInfo>'
            f'<Column id="a" type="string" size="32"/></ColumnInfo>{xml_rows}</Dataset></Root>')
    cases.append({"id": f"driver.{SHORT[p]}.row-types-order.decode", "profile": p, "operation": "decode", "text": text})
for p in ("nexacro-ssv", "xplatform-ssv"):
    recs = ["N" + US + "n1", "D" + US + "d1", "U" + US + "u1", "O" + US + "o1", "I" + US + "i1", "N" + US + "n2"]
    text = f"SSV:UTF-8{RS}Dataset:d{RS}_RowType_{US}a:string(32){RS}" + RS.join(recs) + RS + RS
    cases.append({"id": f"driver.{SHORT[p]}.row-types-order.decode", "profile": p, "operation": "decode", "text": text})
cases.append({"id": "driver.nexacro-json.row-types-order.decode", "profile": "nexacro-json-1.0", "operation": "decode",
              "text": json.dumps({"version": "1.0", "Datasets": [{"id": "d", "ColumnInfo": {"Column": [{"id": "a", "type": "string", "size": "32"}]},
                                  "Rows": [{"_RowType_": "N", "a": "n1"}, {"_RowType_": "D", "a": "d1"}, {"_RowType_": "U", "a": "u1"},
                                           {"_RowType_": "O", "a": "o1"}, {"_RowType_": "I", "a": "i1"}, {"_RowType_": "N", "a": "n2"}]}]})})
save("savetype", "Row types written for each saveType, and the row order the original driver reads.", cases)

# 5. binary -------------------------------------------------------------------
cases = []
for p in BIN:
    s = SHORT[p]
    cases.append({"id": f"driver.{s}.dataset-without-parameters.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [], "datasets": [ds("d", [("a", "STRING"), ("b", "INT")],
                            [row("N", {"a": v("value", "x"), "b": v("value", "1")})])]}, "alsoDecode": True})
    cases.append({"id": f"driver.{s}.parameters-and-datasets.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [param("p", "STRING", v("value", "v")), param("q", "INT", v("value", "7"))],
                            "datasets": [ds("d", [("a", "STRING")], [row("N", {"a": v("value", "x")}), row("N", {"a": v("value", "한글")})]),
                                         ds("e", [("b", "STRING")], [])]}, "alsoDecode": True})
    cases.append({"id": f"driver.{s}.empty.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [], "datasets": []}, "alsoDecode": True})
    cases.append({"id": f"driver.{s}.long-string.encode", "profile": p, "operation": "encode",
                  "value": {"parameters": [param("p", "STRING", v("value", "x" * 300))],
                            "datasets": [ds("d", [("a", "STRING")], [row("N", {"a": v("value", "y" * 70000)})])]}, "alsoDecode": True})
save("binary", "PlatformBinary framing written and read by the original driver.", cases)

# 6. scalar lexical conversion ------------------------------------------------
NUM = ["001.20", "1.50", "-0.0", "1e3", "abc", " 2 ", "10", "-7", "+5", "2147483648", "0x1F", "1,234", "NaN",
       "Infinity", "-1.5e-3", "1E7", "0.0001", "123456789.123", "9223372036854775808"]
SCALARS = {
    "int": NUM, "long": NUM, "float": NUM, "double": NUM, "bigdecimal": NUM,
    "boolean": ["true", "TRUE", "1", "0", "yes", "Y", "false", "abc", " true", "2"],
    "date": ["20261116", "2026111", "20261340", "abcdefgh", "2026-11-16", "202611160000"],
    "time": ["123456789", "123456", "1234", "246060000", "12:34:56"],
    "datetime": ["20261116123456789", "20261116123456", "2026", "20261116250000000", "2026-11-16 12:34:56"],
}
CANON = {"int": "INT", "long": "LONG", "float": "FLOAT", "double": "DOUBLE", "bigdecimal": "BIGDECIMAL",
         "boolean": "BOOLEAN", "date": "DATE", "time": "TIME", "datetime": "DATETIME"}
cases = []
for t, vals in SCALARS.items():
    for p, root in (("nexacro-xml-4000", NX), ("xplatform-xml-5000", XP)):
        params = "".join(f'<Parameter id="p{i}" type="{t}">{x}</Parameter>' for i, x in enumerate(vals))
        cols = "".join(f'<Col id="c{i}">{x}</Col>' for i, x in enumerate(vals))
        info = "".join(f'<Column id="c{i}" type="{t}" size="8"/>' for i in range(len(vals)))
        text = (f'<?xml version="1.0" encoding="UTF-8"?>\n<Root {root}><Parameters>{params}</Parameters><Dataset id="d">'
                f'<ColumnInfo>{info}</ColumnInfo><Rows><Row>{cols}</Row></Rows></Dataset></Root>')
        cases.append({"id": f"driver.{SHORT[p]}.scalar-{t}.decode", "profile": p, "operation": "decode", "text": text})
    for p in ("nexacro-ssv", "xplatform-ssv"):
        params = "".join(f"p{i}:{t}={x}{RS}" for i, x in enumerate(vals))
        hdr = US.join(["_RowType_"] + [f"c{i}:{t}(8)" for i in range(len(vals))])
        text = f"SSV:UTF-8{RS}{params}Dataset:d{RS}{hdr}{RS}" + US.join(["N"] + vals) + RS + RS
        cases.append({"id": f"driver.{SHORT[p]}.scalar-{t}.decode", "profile": p, "operation": "decode", "text": text})
    cases.append({"id": f"driver.nexacro-json.scalar-{t}.decode", "profile": "nexacro-json-1.0", "operation": "decode",
                  "text": json.dumps({"version": "1.0", "Parameters": [{"id": f"p{i}", "type": t, "value": x} for i, x in enumerate(vals)],
                                      "Datasets": [{"id": "d", "ColumnInfo": {"Column": [{"id": f"c{i}", "type": t, "size": "8"} for i in range(len(vals))]},
                                                    "Rows": [{f"c{i}": x for i, x in enumerate(vals)}]}]})})
    for p in TEXT + BIN:
        val = {"parameters": [param(f"p{i}", CANON[t], v("value", x)) for i, x in enumerate(vals)],
               "datasets": [ds("d", [(f"c{i}", CANON[t]) for i in range(len(vals))], [row("N", {f"c{i}": v("value", x) for i, x in enumerate(vals)})])]}
        cases.append({"id": f"driver.{SHORT[p]}.scalar-{t}.encode", "profile": p, "operation": "encode", "value": val})
save("scalar", "How the original drivers convert scalar lexicals on decode and encode.", cases)

# 7. constant columns and unchanged values of updated rows ----------------------
CONSTS = {
    "STRING": [v("value", "x"), v("empty"), v("null")],
    "INT": [v("value", "1,000"), v("value", "abc"), v("value", "1.9"), v("empty"), v("null")],
    "LONG": [v("value", "1,000"), v("value", "abc"), v("empty"), v("null")],
    "FLOAT": [v("value", "123456.789"), v("value", "abc"), v("empty"), v("null")],
    "DOUBLE": [v("value", "123456.789"), v("value", "abc"), v("empty"), v("null")],
    "BIGDECIMAL": [v("value", "001.50"), v("value", "abc"), v("empty"), v("null")],
    "BOOLEAN": [v("value", "yes"), v("value", "abc"), v("empty"), v("null")],
    "DATE": [v("value", "20261116"), v("value", "2026-11-16"), v("empty"), v("null")],
    "DATETIME": [v("value", "20261116123456789"), v("value", "abc"), v("empty"), v("null")],
    "TIME": [v("value", "123456789"), v("empty"), v("null")],
    "BLOB": [v("value", "eA=="), v("empty"), v("null")],
}
cases = []
for p in TEXT + BIN:
    s = SHORT[p]
    for t, cells in CONSTS.items():
        if p in BIN and t == "TIME":
            continue  # the binary drivers put a TIME constant on the current date
        consts = [{"id": f"k{i}", "type": t, "value": c} for i, c in enumerate(cells)]
        val = {"parameters": [], "datasets": [ds("d", [("a", "STRING")], [row("N", {"a": v("value", "x")})], consts)]}
        cases.append({"id": f"driver.{s}.const-{t.lower()}.encode", "profile": p, "operation": "encode",
                      "value": val, "alsoDecode": True})
    for name, extra in (("unset", {}), ("all", {"saveType": 1})):
        val = {"parameters": [], "datasets": [ds("d", [("a", "STRING"), ("b", "INT"), ("c", "STRING")], [
            row("U", {"a": v("value", "new")}, org={"a": v("value", "old"), "b": v("value", "5"), "c": v("null")}),
        ], **extra)]}
        cases.append({"id": f"driver.{s}.update-row-unchanged-values-{name}.encode", "profile": p, "operation": "encode",
                      "value": val, "alsoDecode": True})
save("const", "Constant column values and the unchanged values of updated rows, as the original drivers write them.", cases)
