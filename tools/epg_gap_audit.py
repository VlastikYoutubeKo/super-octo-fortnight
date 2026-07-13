#!/usr/bin/env python3
"""Find CS-FILM-class bugs: a channel mapped to a guide that only covers part of each
day, while another id in the universe carries the SAME schedule for a fuller day.

Evidence, not name heuristics:
  1. mapped id has real intra-day gaps (mean coverage < 20h on its full days);
  2. some other id reproduces >=60% of its (start,title) pairs exactly  -> same channel;
  3. that other id covers materially more of the day                    -> strictly better.
"""
import json, re, unicodedata
from collections import defaultdict
from datetime import datetime, timedelta

XMLTV = "/var/lib/tvheadend/.xmltv/tv_grab_file.xmltv"
MAPPING = "/root/tvheadend_proxies/super-octo-fortnight/epg_mapping.json"
reProg = re.compile(rb'<programme\b[^>]*>')
reAttr = re.compile(rb'(\w+)="([^"]*)"')
reTitle = re.compile(rb'<title[^>]*>([^<]*)</title>')

def parse_ts(s):
    s = s.decode().strip()
    dt = datetime.strptime(s[:14], "%Y%m%d%H%M%S")
    off = s[15:] if len(s) > 15 else "+0000"
    if len(off) >= 5 and off[0] in '+-':
        sign = 1 if off[0] == '+' else -1
        dt -= sign * timedelta(hours=int(off[1:3]), minutes=int(off[3:5]))
    return dt

def ntitle(b):
    s = unicodedata.normalize('NFKD', b.decode('utf-8', 'replace'))
    s = ''.join(c for c in s if not unicodedata.combining(c)).lower()
    return re.sub(r'[^a-z0-9]', '', s)[:24]

mapping = json.load(open(MAPPING))
mapped_ids = set(v for v in mapping.values() if v)

# ---- pass 1: per-id per-day covered seconds, and (start,title) signatures ----
day_cov = defaultdict(lambda: defaultdict(float))   # id -> day -> secs
sig = defaultdict(set)                              # id -> {(start_epoch, ntitle)}
with open(XMLTV, 'rb') as f:
    cur = None
    for line in f:
        if b'<programme' in line:
            m = reProg.search(line)
            if not m:
                continue
            a = dict(reAttr.findall(m.group(0)))
            ch, st, sp = a.get(b'channel', b'').decode(), a.get(b'start'), a.get(b'stop')
            cur = None
            if not ch or not st or not sp:
                continue
            try:
                s, e = parse_ts(st), parse_ts(sp)
            except Exception:
                continue
            d = (e - s).total_seconds()
            if not (0 < d < 24 * 3600):
                continue
            day_cov[ch][s.date()] += d
            cur = (ch, int(s.timestamp()))
            t = reTitle.search(line)
            if t:
                sig[ch].add((cur[1], ntitle(t.group(1))))
                cur = None
        elif cur is not None:
            t = reTitle.search(line)
            if t:
                sig[cur[0]].add((cur[1], ntitle(t.group(1))))
                cur = None

def full_day_mean(cid):
    days = sorted(day_cov[cid])
    if len(days) < 3:
        return None
    inner = days[1:-1]                      # drop partial first/last day
    return sum(day_cov[cid][d] for d in inner) / len(inner) / 3600.0

# ---- pass 2: which mapped ids have real gaps? ----
gappy = {}
for cid in mapped_ids:
    if cid not in day_cov:
        continue
    h = full_day_mean(cid)
    if h is not None and h < 20.0:
        gappy[cid] = h
print(f"{len(gappy)} mapped ids have < 20h/day of guide (of {len(mapped_ids)} mapped ids)\n")

# ---- pass 3: find a fuller id carrying the SAME schedule ----
owner = defaultdict(list)
for cid, s in sig.items():
    for pair in s:
        owner[pair].append(cid)

rows = []
for cid, h in gappy.items():
    mine = sig.get(cid, set())
    if len(mine) < 10:
        continue
    hits = defaultdict(int)
    for pair in mine:
        for other in owner[pair]:
            if other != cid:
                hits[other] += 1
    for other, n in hits.items():
        agree = n / len(mine)
        oh = full_day_mean(other)
        if oh is None or agree < 0.60 or oh < h + 2.0:
            continue
        rows.append((oh - h, agree, cid, h, other, oh))

rows.sort(reverse=True)
chan_of = defaultdict(list)
for ch, cid in mapping.items():
    chan_of[cid].append(ch)

print(f"{'mapped id':28} {'h/day':>6}  {'->':2} {'fuller id (same schedule)':30} {'h/day':>6} {'agree':>6}  channels")
print('-' * 130)
best = {}
for gain, agree, cid, h, other, oh in rows:
    if cid in best:
        continue
    best[cid] = (other, agree, oh)
    chs = ', '.join(sorted(chan_of[cid])[:3])
    print(f"{cid[:27]:28} {h:6.1f}  -> {other[:29]:30} {oh:6.1f} {agree*100:5.0f}%  {chs[:60]}")
print(f"\n{len(best)} mapped ids have a strictly fuller same-schedule alternative")
