#!/usr/bin/env python3
"""Fake Riot servers for testing ValoROAD without real accounts.

    python3 scripts/mock_riot.py 9900
    VALOROAD_MOCK=http://127.0.0.1:9900 VALOROAD_BROWSER=/path/to/chromium ...engine

Uses real item ids from valorant-api.com so the app shows real names and images.
Admin endpoints (POST): /admin/expire?acct=a1  /admin/block?on=1  /admin/next?acct=a2  /admin/rotate
                        /admin/blank?n=1 (next n login pages render nothing, like a blank window)
The login page auto-submits the account from /admin/next (default a1) after ~1.2s, so tests can run unattended.
"""
import hashlib, http.server, json, os, random, socketserver, sys, time, urllib.parse, urllib.request

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 9900
BASE = f"http://127.0.0.1:{PORT}"
CACHE = os.environ.get("MOCK_CACHE", "/tmp/valoroad-mock-cache")
os.makedirs(CACHE, exist_ok=True)

def vapi(path):
    fn = os.path.join(CACHE, hashlib.md5(path.encode()).hexdigest() + ".json")
    if not os.path.exists(fn):
        with urllib.request.urlopen("https://valorant-api.com" + path, timeout=60) as r:
            open(fn, "wb").write(r.read())
    return json.load(open(fn))["data"]

weapons = vapi("/v1/weapons?language=ko-KR")
bundles = vapi("/v1/bundles?language=ko-KR")
buddies = vapi("/v1/buddies?language=ko-KR")
sprays = vapi("/v1/sprays?language=ko-KR")
cards = vapi("/v1/playercards?language=ko-KR")

skins = []
for w in weapons:
    for s in w["skins"]:
        if s["uuid"] == w["defaultSkinUuid"] or not s["levels"] or not s["contentTierUuid"]:
            continue
        skins.append(s)

def find_bundle():
    for b in bundles:
        name = b["displayName"]
        if b.get("displayNameSubText"):
            continue
        ss = [s for s in skins if s["displayName"].startswith(name + " ")]
        if len(ss) >= 4 and b.get("displayIcon"):
            bud = [x for x in buddies if name in x["displayName"]]
            spr = [x for x in sprays if name in x["displayName"]]
            crd = [x for x in cards if name in x["displayName"]]
            if bud and spr and crd:
                return b, ss[:5], bud[0], spr[0], crd[0]
    raise SystemExit("no bundle")

BUNDLE, BSKINS, BBUDDY, BSPRAY, BCARD = find_bundle()

ACCOUNTS = {
    "a1": {"name": "레이즈장인", "tag": "KR1", "level": 187, "night": True},
    "a2": {"name": "NightOwl", "tag": "0412", "level": 64, "night": False},
    "a3": {"name": "제트원챔", "tag": "777", "level": 312, "night": True},
}
STATE = {"valid": {}, "block": False, "next": "a1", "day": 0, "blank": 0}

def puuid(a):
    h = hashlib.md5(("puuid" + a).encode()).hexdigest()
    return f"{h[:8]}-{h[8:12]}-{h[12:16]}-{h[16:20]}-{h[20:32]}"

def acct_of_puuid(p):
    for a in ACCOUNTS:
        if puuid(a) == p:
            return a

def rng(a):
    return random.Random(a + str(STATE["day"]) + time.strftime("%Y%m%d"))

TIER_PRICE = {"12683d76-48d7-84a3-4e09-6985794f0445": 875, "0cebb8be-46d7-c12a-d306-e9907bfc5a25": 1275,
              "60bca009-4182-7998-dee7-b8a2558dc369": 1775, "e046854e-406c-37f4-6607-19a9ba8426fc": 2175,
              "411e4a55-4e59-7757-41f0-86a53f101bb5": 2475}
VP = "85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741"
SKIN_T = "e7c63390-eda7-46e0-bb7a-a6abdacd2433"

def wish_forced():
    p = os.path.join(CACHE, "force_daily.json")
    return json.load(open(p)) if os.path.exists(p) else []

def storefront(a):
    r = rng(a)
    daily = r.sample(skins, 4)
    forced = wish_forced()
    if a == "a1" and forced:
        byid = {s["uuid"]: s for s in skins}
        for i, sid in enumerate(forced[:4]):
            if sid in byid:
                daily[i] = byid[sid]
    now = time.time()
    secs_daily = int(86400 - (now % 86400))
    out = {
        "FeaturedBundle": {"Bundles": [{
            "ID": "bundle-1", "DataAssetID": BUNDLE["uuid"], "CurrencyID": VP,
            "Items": [{"Item": {"ItemTypeID": SKIN_T, "ItemID": s["levels"][0]["uuid"], "Amount": 1},
                       "BasePrice": TIER_PRICE[s["contentTierUuid"]], "DiscountPercent": 0.33,
                       "DiscountedPrice": int(TIER_PRICE[s["contentTierUuid"]] * 0.67)} for s in BSKINS] + [
                {"Item": {"ItemTypeID": "dd3bf334-87f3-40bd-b043-682a57a8dc3a", "ItemID": BBUDDY["levels"][0]["uuid"], "Amount": 1}, "BasePrice": 475, "DiscountPercent": 1, "DiscountedPrice": 0},
                {"Item": {"ItemTypeID": "d5f120f8-ff8c-4aac-92ea-f2b5acbe9475", "ItemID": BSPRAY["uuid"], "Amount": 1}, "BasePrice": 325, "DiscountPercent": 1, "DiscountedPrice": 0},
                {"Item": {"ItemTypeID": "3f296c07-64c3-494c-923b-fe692a4fa1bd", "ItemID": BCARD["uuid"], "Amount": 1}, "BasePrice": 375, "DiscountPercent": 1, "DiscountedPrice": 0}],
            "TotalBaseCost": None, "TotalDiscountedCost": None, "TotalDiscountPercent": 0,
            "DurationRemainingInSeconds": 4 * 86400 + 3600 * 7}]},
        "SkinsPanelLayout": {
            "SingleItemOffers": [s["levels"][0]["uuid"] for s in daily],
            "SingleItemStoreOffers": [{"OfferID": s["levels"][0]["uuid"], "Cost": {VP: TIER_PRICE[s["contentTierUuid"]]},
                                       "Rewards": [{"ItemTypeID": SKIN_T, "ItemID": s["levels"][0]["uuid"], "Quantity": 1}]} for s in daily],
            "SingleItemOffersRemainingDurationInSeconds": secs_daily},
    }
    if ACCOUNTS[a]["night"]:
        night = r.sample(skins, 6)
        offers = []
        for s in night:
            base = TIER_PRICE[s["contentTierUuid"]]
            pct = r.choice([12, 18, 23, 27, 31, 36, 42, 49])
            offers.append({"BonusOfferID": s["uuid"], "Offer": {"OfferID": s["levels"][0]["uuid"], "Cost": {VP: base},
                           "Rewards": [{"ItemTypeID": SKIN_T, "ItemID": s["levels"][0]["uuid"], "Quantity": 1}]},
                           "DiscountPercent": pct, "DiscountCosts": {VP: int(base * (100 - pct) / 100)}, "IsSeen": r.random() < .5})
        out["BonusStore"] = {"BonusStoreOffers": offers, "BonusStoreRemainingDurationInSeconds": 9 * 86400 + 5000}
    return out

LOGIN_HTML = """<!doctype html><meta charset=utf-8><title>Riot 로그인 (테스트)</title>
<body style="font:15px sans-serif;background:#111;color:#eee;padding:30px">
<h2>RIOT (테스트 로그인)</h2>
<form method=post action="/login?%(q)s"><input name=acct value="%(acct)s"><label><input type=checkbox name=remember checked> 로그인 상태 유지</label>
<button id=go>로그인</button></form>
<script>setTimeout(()=>document.getElementById('go').click(),1200)</script>"""

class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass

    def send(self, code, body=b"", ctype="application/json", headers=()):
        if isinstance(body, (dict, list)):
            body = json.dumps(body).encode()
        elif isinstance(body, str):
            body = body.encode()
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        for k, v in headers:
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(body)

    def cookies(self):
        out = {}
        for part in (self.headers.get("Cookie") or "").split(";"):
            if "=" in part:
                k, v = part.strip().split("=", 1)
                out[k] = v
        return out

    def bearer_acct(self):
        auth = self.headers.get("Authorization", "")
        if not auth.startswith("Bearer AT-"):
            return None
        a = auth.split("-")[1]
        return a if a in ACCOUNTS else None

    def do_GET(self):
        u = urllib.parse.urlparse(self.path)
        q = urllib.parse.parse_qs(u.query)
        p = u.path
        if p == "/authorize":
            if STATE["block"] and not self.headers.get("Sec-Fetch-Mode"):
                # 브라우저가 아닌 요청만 막아서 '조용한 브라우저 재로그인'을 시험해요
                return self.send(403, "blocked", "text/html")
            ssid = self.cookies().get("ssid", "")
            a = ssid.replace("valid-", "")
            if ssid.startswith("valid-") and STATE["valid"].get(a) == ssid:
                frag = urllib.parse.urlencode({"access_token": f"AT-{a}-{int(time.time())}", "scope": "account openid",
                                               "iss": BASE, "id_token": f"ID-{a}", "token_type": "Bearer",
                                               "session_state": "x", "expires_in": "3600"})
                return self.send(303, "", "text/html", [("Location", f"{BASE}/opt_in#{frag}"),
                                 ("Set-Cookie", f"ssid={ssid}; Path=/; HttpOnly; Max-Age=2592000")])
            return self.send(303, "", "text/html", [("Location", f"{BASE}/login?{u.query}")])
        if p == "/login" and STATE["blank"] > 0:
            # 내용이 그려지지 않는 로그인 창 흉내 (스크립트 오류)
            STATE["blank"] -= 1
            return self.send(200, "<!doctype html><title>Sign in with your Riot account</title><body style='background:#262626'>"
                             "<script>throw new Error('mock: app bundle failed')</script>", "text/html; charset=utf-8")
        if p == "/login":
            return self.send(200, LOGIN_HTML % {"q": u.query, "acct": STATE["next"]}, "text/html; charset=utf-8")
        if p == "/opt_in":
            return self.send(200, "<meta charset=utf-8><body>ok</body>", "text/html")
        if p == "/userinfo":
            a = self.bearer_acct()
            if not a:
                return self.send(401, {"error": "invalid"})
            return self.send(200, {"sub": puuid(a), "acct": {"game_name": ACCOUNTS[a]["name"], "tag_line": ACCOUNTS[a]["tag"]}})
        parts = p.strip("/").split("/")
        if parts[:1] == ["pd"]:
            a = self.bearer_acct()
            if not a:
                return self.send(400, {"errorCode": "BAD_CLAIMS"})
            rest = "/".join(parts[2:])
            if rest.startswith("store/v1/wallet/"):
                return self.send(200, {"Balances": {VP: 1000 + 1375 * (int(a[1])), "e59aa87c-4cbf-517a-5983-6e81511be9b7": 40 * int(a[1]),
                                                    "85ca954a-41f2-ce94-9b45-8ca3dd39a00d": 8000}})
            if rest.startswith("store/v1/entitlements/"):
                r = rng(a + "owned")
                sf = storefront(a)
                owned = [sf["SkinsPanelLayout"]["SingleItemOffers"][3]] + [s["levels"][0]["uuid"] for s in r.sample(skins, 40)]
                return self.send(200, {"ItemTypeID": SKIN_T, "Entitlements": [{"TypeID": SKIN_T, "ItemID": x} for x in owned]})
            if rest.startswith("account-xp/v1/players/"):
                return self.send(200, {"Progress": {"Level": ACCOUNTS[a]["level"], "XP": 1200}})
            if rest.startswith("personalization/v2/players/"):
                c = cards[{"a1": 40, "a2": 120, "a3": 300}[a]]
                return self.send(200, {"Identity": {"PlayerCardID": c["uuid"], "PlayerTitleID": ""}})
        return self.send(404, {"error": "not found", "path": p})

    def do_PUT(self):
        n = int(self.headers.get("Content-Length") or 0)
        self.rfile.read(n)
        if self.path.startswith("/geo/"):
            return self.send(200, {"token": "x", "affinities": {"pbe": "na", "live": "kr"}})
        return self.send(404, {})

    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(n)
        u = urllib.parse.urlparse(self.path)
        q = urllib.parse.parse_qs(u.query)
        p = u.path
        if p == "/login":
            form = urllib.parse.parse_qs(body.decode())
            a = (form.get("acct") or ["a1"])[0]
            ssid = f"valid-{a}"
            STATE["valid"][a] = ssid
            return self.send(303, "", "text/html", [("Location", f"{BASE}/authorize?{u.query}"),
                             ("Set-Cookie", f"ssid={ssid}; Path=/; HttpOnly; Max-Age=2592000"),
                             ("Set-Cookie", "tdid=device123; Path=/; Max-Age=31536000")])
        if p.startswith("/ent/"):
            a = self.bearer_acct()
            if not a:
                return self.send(401, {})
            return self.send(200, {"entitlements_token": f"ENT-{a}"})
        if p.startswith("/pd/") and "/store/v3/storefront/" in p:
            a = self.bearer_acct()
            if not a:
                return self.send(400, {"errorCode": "BAD_CLAIMS"})
            return self.send(200, storefront(a))
        if p == "/admin/expire":
            a = q.get("acct", ["a1"])[0]
            STATE["valid"].pop(a, None)
            return self.send(200, {"ok": True})
        if p == "/admin/block":
            STATE["block"] = q.get("on", ["1"])[0] == "1"
            return self.send(200, {"ok": True})
        if p == "/admin/next":
            STATE["next"] = q.get("acct", ["a1"])[0]
            return self.send(200, {"ok": True})
        if p == "/admin/blank":
            STATE["blank"] = int(q.get("n", ["1"])[0])
            return self.send(200, {"ok": True})
        if p == "/admin/rotate":
            STATE["day"] += 1
            return self.send(200, {"ok": True})
        if p == "/admin/force":
            open(os.path.join(CACHE, "force_daily.json"), "w").write(body.decode() or "[]")
            return self.send(200, {"ok": True})
        return self.send(404, {})

class TS(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True
    allow_reuse_address = True

print(f"mock riot on {BASE} — bundle {BUNDLE['displayName']}", flush=True)
TS(("127.0.0.1", PORT), H).serve_forever()
