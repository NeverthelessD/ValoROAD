// ValoROAD 화면 — 상점 · 위시리스트 · 설정 (확정 디자인 2026-10-24)
// made by. Nevertheless_D
'use strict';

const TOKEN = new URLSearchParams(location.search).get('t') || '';
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const fmt = (n) => (Number(n) || 0).toLocaleString('en-US');
const pad = (n) => String(n).padStart(2, '0');
const bridge = window.webkit?.messageHandlers?.valoroad;

async function api(path, body) {
  const opt = { headers: { 'x-token': TOKEN } };
  if (body !== undefined) {
    opt.method = 'POST';
    opt.headers['content-type'] = 'application/json';
    opt.body = JSON.stringify(body);
  }
  const r = await fetch(path, opt);
  let j = null;
  try { j = await r.json(); } catch { /* ignore */ }
  if (!r.ok) throw new Error(j?.error || `문제가 생겼어요 (${r.status})`);
  return j;
}

const S = {
  view: 'store', state: null, active: null, store: null, storeLoading: false, storeErr: '',
  bundleIdx: 0, rev: -1, refreshing: false,
  wish: { q: '', cat: 'all', items: [], results: [], total: 0, sum: 0, pricesReady: false, priceError: '' },
  login: null, // { slot, phase: 'wait' | 'error' | 'nobridge', error }
  pv: null, // 미리보기 { id, data, chroma, level, cvideo }
};

const TIERS = {
  select: ['셀렉트', '#5A9FE2'], deluxe: ['디럭스', '#2FC48D'], premium: ['프리미엄', '#D1548D'],
  exclusive: ['익스클루시브', '#F5955B'], ultra: ['울트라', '#FAD663'],
};
const CATS = [['all', 'ALL'], ['rifle', 'RIFLES'], ['sidearm', 'SIDEARMS'], ['smg', 'SMGS'], ['shotgun', 'SHOTGUNS'], ['sniper', 'SNIPERS'], ['heavy', 'HEAVY'], ['melee', 'MELEE']];

function tierOf(t) { return TIERS[t] || ['기본', '#9AA6B1']; }
function tint(t) {
  const c = tierOf(t)[1];
  const a = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--tint-a')) || 0.2;
  const n = parseInt(c.slice(1), 16);
  return `rgba(${n >> 16},${(n >> 8) & 255},${n & 255},${a})`;
}
function img(u) { return u ? `/api/img?t=${encodeURIComponent(TOKEN)}&u=${encodeURIComponent(u)}` : ''; }
function josa(w) {
  const c = String(w || '').trim().slice(-1).charCodeAt(0);
  if (c >= 0xac00 && c <= 0xd7a3) return (c - 0xac00) % 28 ? '이' : '가';
  return '이(가)';
}

// ---------------------------------------------------------------- 아이콘
const I = {
  vp: (c = 'currentColor') => `<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M8 1 L15 8 L8 15 L1 8 Z" fill="none" stroke="${c}" stroke-width="1.6"/><path d="M8 5 L11 8 L8 11 L5 8 Z" fill="${c}"/></svg>`,
  rp: `<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M8 1 L14 4.5 V11.5 L8 15 L2 11.5 V4.5 Z" fill="none" stroke="currentColor" stroke-width="1.6"/></svg>`,
  kc: `<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M2 12 L3 4 L6 7 L8 3 L10 7 L13 4 L14 12 Z" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"/></svg>`,
  refresh: `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="square" aria-hidden="true"><path d="M20 12a8 8 0 1 1-2.4-5.7"/><path d="M20 4v5h-5"/></svg>`,
  heart: (on) => `<svg width="22" height="22" viewBox="0 0 24 24" fill="${on ? 'currentColor' : 'none'}" stroke="currentColor" stroke-width="2" stroke-linejoin="round" aria-hidden="true"><path d="M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10z"/></svg>`,
  bell: `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linejoin="round" aria-hidden="true"><path d="M6 16V11a6 6 0 0 1 12 0v5l2 2H4z"/><path d="M10 20a2 2 0 0 0 4 0"/></svg>`,
  check: `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.8" stroke-linecap="square" aria-hidden="true"><path d="M5 12l5 5 9-10"/></svg>`,
  lock: `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linejoin="round" aria-hidden="true"><rect x="5" y="11" width="14" height="10"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg>`,
  plus: `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="square" aria-hidden="true"><path d="M12 4v16M4 12h16"/></svg>`,
  aim: `<svg class="aim" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" role="img" aria-label="보는 중"><circle cx="12" cy="12" r="6"/><path d="M12 2v5M12 17v5M2 12h5M17 12h5"/></svg>`,
  search: `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="square" aria-hidden="true" style="color:var(--muted)"><circle cx="11" cy="11" r="6"/><path d="M20 20l-4.5-4.5"/></svg>`,
  logo: (s = 38) => `<svg width="${s}" height="${s}" viewBox="0 0 512 512" aria-hidden="true"><rect width="512" height="512" rx="112" fill="#0F1923"/><circle cx="256" cy="256" r="196" fill="none" stroke="#FF4655" stroke-width="26" stroke-dasharray="267.9 40" stroke-dashoffset="-20"/><path d="M256 30 V82 M256 430 V482 M30 256 H82 M430 256 H482" stroke="#FF4655" stroke-width="26"/><g transform="translate(143.4 356) scale(0.11364 -0.11364)" fill="#ECE8E1"><path d="M240 0 23 1761H358L487 563L602 1761H937L720 0Z"/><path transform="translate(1021 0)" d="M78 0V1760H618Q753 1760 822.0 1698.5Q891 1637 914.5 1526.5Q938 1416 938 1267Q938 1123 901.5 1037.0Q865 951 764 918Q847 901 880.5 835.5Q914 770 914 666V0H567V689Q567 766 535.5 784.5Q504 803 434 803V0ZM436 1108H521Q594 1108 594 1267Q594 1370 578.0 1402.0Q562 1434 518 1434H436Z"/></g></svg>`,
};

// ---------------------------------------------------------------- 공통
let toastTimer = 0;
function toast(msg, err = false) {
  const t = $('#toast');
  t.textContent = msg;
  t.classList.toggle('err', err);
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, err ? 5200 : 3200);
}

function applyTheme() {
  const a = S.state?.settings?.appearance || 'system';
  const dark = a === 'dark' || (a === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
}
matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => { applyTheme(); renderAll(); });

const accounts = () => S.state?.accounts || [];
const activeAcc = () => accounts().find((a) => a.id === S.active) || null;
const label = (a) => a ? `${a.name || '이름 없음'}${a.tag ? ' #' + a.tag : ''}` : '';
const labelTight = (a) => a ? `${a.name || '이름 없음'}${a.tag ? '#' + a.tag : ''}` : '';

function remain(ends) {
  const s = Math.max(0, Math.floor(ends - Date.now() / 1000));
  return { d: Math.floor(s / 86400), h: Math.floor((s % 86400) / 3600), m: Math.floor((s % 3600) / 60), s: s % 60, total: s };
}
function clockText(ends) {
  const r = remain(ends);
  const h = r.d * 24 + r.h;
  return `${pad(h)}:${pad(r.m)}:${pad(r.s)}`;
}
function dayText(ends) {
  const r = remain(ends);
  return r.d > 0 ? `${r.d}일 ${pad(r.h)}:${pad(r.m)}` : `${pad(r.h)}:${pad(r.m)}:${pad(r.s)}`;
}
setInterval(() => {
  $$('[data-clock]').forEach((el) => { el.textContent = clockText(+el.dataset.clock); });
  $$('[data-days]').forEach((el) => { el.textContent = dayText(+el.dataset.days); });
  // 상점이 바뀌는 시각이 지나면 새로 받아요
  if (S.view === 'store' && S.store && S.active && S.store.dailyEnds && Date.now() / 1000 > S.store.dailyEnds + 20 && !S.storeLoading) {
    S.storeLoading = true;
    loadStore(S.active);
  }
}, 1000);

// ---------------------------------------------------------------- 데이터
async function loadState() {
  S.state = await api('/api/state');
  S.rev = S.state.rev;
  applyTheme();
  const ids = accounts().map((a) => a.id);
  if (!ids.includes(S.active)) {
    const saved = S.state.settings.active;
    S.active = ids.includes(saved) ? saved : (ids[0] || null);
    S.store = null;
    S.bundleIdx = 0;
  }
}

async function loadStore(id, force = false) {
  if (!id) return;
  try {
    const r = force ? await api('/api/store/refresh', { id }) : await api(`/api/store?id=${encodeURIComponent(id)}`);
    if (id !== S.active) return;
    S.store = r.store;
    S.storeLoading = !!r.loading;
    S.storeErr = r.ok === false ? r.error : '';
    if (force && r.ok === false) toast(r.error, true);
  } catch (e) {
    S.storeErr = e.message;
    S.storeLoading = false;
  }
  renderStore();
  renderWallet();
}

async function loadWishlist() {
  try {
    const r = await api('/api/wishlist');
    S.wish.items = r.items || [];
    S.wish.sum = r.total || 0;
    S.wish.pricesReady = !!r.pricesReady;
    S.wish.priceError = r.priceError || '';
  } catch { /* ignore */ }
  if (S.view === 'wish') renderWishList();
}

async function searchSkins() {
  const { q, cat } = S.wish;
  if (!q && cat === 'all') { S.wish.results = []; S.wish.total = 0; renderResults(); return; }
  try {
    const r = await api(`/api/skins?q=${encodeURIComponent(q)}&cat=${cat}&limit=120`);
    if (q !== S.wish.q || cat !== S.wish.cat) return;
    S.wish.results = r.skins || [];
    S.wish.total = r.total || 0;
  } catch { S.wish.results = []; }
  renderResults();
}

async function setWish(id, on) {
  try {
    await api('/api/wish', { id, on });
    toast(on ? '위시리스트에 담았어요' : '위시리스트에서 뺐어요');
  } catch (e) { toast(e.message, true); return; }
  await loadState();
  if (S.store) {
    for (const o of [...(S.store.daily || []), ...(S.store.night || [])]) if (o.id === id) o.wished = on;
  }
  S.wish.results.forEach((s) => { if (s.id === id) s.wished = on; });
  if (S.pv?.data?.skin?.id === id) S.pv.data.wished = on;
  await loadWishlist();
  renderAll();
}

async function saveSetting(k, v) {
  try {
    const s = await api('/api/settings', { [k]: v });
    S.state.settings = s;
    applyTheme();
  } catch (e) { toast(e.message, true); }
}

// ---------------------------------------------------------------- 그리기: 위쪽
function renderTabs() {
  $$('.tab').forEach((b) => b.setAttribute('aria-selected', String(b.dataset.view === S.view)));
  $('#view-store').hidden = S.view !== 'store';
  $('#view-wish').hidden = S.view !== 'wish';
  $('#view-settings').hidden = S.view !== 'settings';
}

function renderWallet() {
  const a = activeAcc();
  const w = S.store?.wallet || a?.wallet;
  const el = $('#wallet');
  if (!a) { el.innerHTML = ''; return; }
  const coin = (ic, n, u) => `<span class="coin" title="${u}">${ic}<span class="n">${w ? fmt(n) : '—'}</span><span class="u">${u}</span></span>`;
  el.innerHTML = coin(I.vp(), w?.vp, 'VP') + coin(I.rp, w?.rp, 'RP') + coin(I.kc, w?.kc, 'KC') +
    `<button class="btn-red btn-icon" id="refreshBtn" aria-label="상점 새로고침" title="상점 새로고침 (⌘R)"${S.refreshing ? ' disabled' : ''}>${S.refreshing ? I.refresh.replace('<svg', '<svg class="spin"') : I.refresh}</button>`;
}

function renderBanner() {
  const u = S.state?.update;
  const b = $('#banner');
  if (u?.status === 'ready' && u.installed) {
    b.innerHTML = `<span class="lbl">UPDATE READY</span><span>새 버전 v${esc(u.installed)} 준비됐어요. 다시 시작하면 바뀌어요.</span><button data-act="restart">지금 다시 시작</button>`;
    b.hidden = false;
  } else if (u?.status === 'available' && !u.autoUpdate && u.latest) {
    b.innerHTML = `<span class="lbl">NEW VERSION</span><span>새 버전 v${esc(u.latest.version)}이 나왔어요.</span><button data-act="install">받기</button>`;
    b.hidden = false;
  } else if (u?.status === 'downloading' || u?.status === 'installing') {
    b.innerHTML = `<span class="lbl">UPDATING</span><span>새 버전을 받는 중이에요… ${Math.round((u.progress || 0) * 100)}%</span>`;
    b.hidden = false;
  } else {
    b.hidden = true;
  }
}

// ---------------------------------------------------------------- 그리기: 계정
function renderSide() {
  const list = accounts();
  const bySlot = Object.fromEntries(list.map((a) => [a.slot, a]));
  let h = `<div class="side-head"><span class="t">ACCOUNTS</span><span class="c">${list.length} / 3</span></div>`;
  for (let slot = 1; slot <= 3; slot++) {
    const a = bySlot[slot];
    if (!a) {
      h += `<button class="acc empty" data-add="${slot}"><span class="slot">${pad(slot)}</span><span class="body"><span class="nm">빈 자리</span><span class="meta"><span class="ko">눌러서 계정 추가</span></span></span></button>`;
      continue;
    }
    const on = a.id === S.active;
    const hits = (a.hits || []).length;
    const w = a.wallet;
    h += `<button class="acc${on ? ' on' : ''}" data-acc="${esc(a.id)}" aria-pressed="${on}" title="⌘${slot}">
      <span class="slot">${pad(slot)}</span>
      <span class="body">
        <span class="nm">${esc(a.name || '이름 없음')} <span>#${esc(a.tag)}</span></span>
        <span class="meta"><span class="reg">${esc(a.region || '—')}</span>${w ? `<span>${fmt(w.vp)} VP</span>` : ''}
          ${a.status === 'expired' ? '<span class="exp">다시 로그인 필요</span>' : ''}
          ${hits ? `<span class="hitb">위시 ${hits}</span>` : ''}</span>
      </span>
      ${on ? I.aim : ''}
    </button>`;
  }
  h += `<button class="add-btn" data-view-go="settings">${I.plus} ADD / MANAGE <span class="ko" style="font-size:13px">계정 관리</span></button>
    <p>계정을 누르면 로그아웃 없이 바로 그 계정의 상점으로 바뀌어요. 최대 3개까지 저장돼요.</p>`;
  $('#side').innerHTML = h;
}

// ---------------------------------------------------------------- 그리기: 상점
function hitAlert() {
  const list = accounts();
  const all = [];
  for (const a of list) for (const h of a.hits || []) all.push({ a, h });
  if (!all.length) return '';
  const other = all.find((x) => x.a.id !== S.active);
  const first = other || all[0];
  const where = first.h.kind === 'night' ? '밤시장' : '상점';
  const more = all.length > 1 ? ` (모두 ${all.length}개)` : '';
  const txt = first.a.id !== S.active
    ? `${esc(labelTight(first.a))} ${where}에 ‘${esc(first.h.name)}’${josa(first.h.name)} 떴어요.${more}`
    : `지금 보고 있는 ${where}에 ‘${esc(first.h.name)}’${josa(first.h.name)} 있어요.${more}`;
  return `<div class="alert" role="status">${I.bell}<span class="lbl">WISHLIST HIT</span><span class="txt">${txt}</span>
    ${first.a.id !== S.active ? `<button data-acc="${esc(first.a.id)}">그 계정 상점 보기</button>` : ''}</div>`;
}

function cardHTML(o) {
  const [tn, tc] = tierOf(o.tier);
  const hit = o.wished;
  return `<div class="card-frame${hit ? ' hit' : ''}"><article class="card" style="--tc:${tc};--tint:${tint(o.tier)}">
    ${hit ? '<div class="hitbar">WISHLIST · 위시리스트</div>' : ''}
    <div class="art"${o.known ? ` data-skin="${esc(o.id)}" role="button" tabindex="0" aria-label="${esc(o.name)} 미리보기"` : ''}>
      <span class="tier"><i></i>${esc(tn)}</span>
      ${o.known ? '<span class="pv-hint">PREVIEW ▶</span>' : ''}
      ${o.img ? `<img src="${img(o.img)}" alt="${esc(o.name)}" loading="lazy">` : ''}
      ${o.known ? `<button class="heart${hit ? ' on' : ''}" data-wish="${esc(o.id)}" data-on="${hit ? 0 : 1}" aria-pressed="${hit}" aria-label="${hit ? '위시리스트에서 빼기' : '위시리스트에 담기'}">${I.heart(hit)}</button>` : ''}
    </div>
    <div class="tbar"></div>
    <div class="info">
      <span class="wpn">${esc(o.weaponEn || o.weapon || '')}</span>
      <span class="nm">${esc(o.name)}</span>
      <span class="price">${I.vp()}<span class="n">${o.price ? fmt(o.price) : '—'}</span></span>
    </div>
  </article></div>`;
}

function skeletonCards() {
  return Array.from({ length: 4 }, () => `<div class="card-frame"><article class="card"><div class="art skel"></div><div class="tbar" style="--tc:var(--line)"></div>
    <div class="info"><span class="wpn">&nbsp;</span><span class="nm skel" style="min-height:20px;width:70%"></span><span class="price"><span class="n">&nbsp;</span></span></div></article></div>`).join('');
}

function bundleHTML(list) {
  if (!list.length) return '';
  const i = Math.min(S.bundleIdx, list.length - 1);
  const b = list[i];
  const pick = list.length > 1
    ? `<div class="bundle-more">${list.map((x, j) => `<button class="${j === i ? 'on' : ''}" data-bundle="${j}">${pad(j + 1)}</button>`).join('')}</div>` : '';
  return `<section class="bundle" aria-label="추천 번들">
    ${b.img ? `<div class="bg" style="background-image:url('${img(b.img)}')"></div>` : ''}
    <div class="shade"></div><div class="slash"></div>
    <span class="tagred">FEATURED BUNDLE</span>
    <h2>${esc(b.name)}</h2>
    <span class="when">${b.ends ? `남은 기간 <b data-days="${b.ends}">${dayText(b.ends)}</b> · ` : ''}${b.items ? `${b.items}종 · ` : ''}추천 번들</span>
    ${pick}
    <span class="price">${I.vp('#ECE8E1')}<span class="n">${fmt(b.price)}</span>${b.base && b.base > b.price ? `<s>${fmt(b.base)}</s>` : ''}</span>
  </section>`;
}

function nightHTML(list, ends) {
  if (!list.length) return '';
  const days = Math.max(0, Math.ceil((ends - Date.now() / 1000) / 86400));
  return `<section class="night" aria-label="밤시장">
    <div class="night-head"><span class="t">NIGHT MARKET</span><span class="d" title="남은 기간">D-${days}</span></div>
    <span class="muted" style="font-size:12px;margin-top:-6px">밤시장 · 할인 품목 ${list.length}개</span>
    ${list.map((n) => `<div class="nrow${n.wished ? ' hit' : ''}" style="--tint:${tint(n.tier)}"${n.known ? ` data-skin="${esc(n.id)}" role="button" tabindex="0" aria-label="${esc(n.name)} 미리보기"` : ''}>
      <span class="pct">-${n.pct}%</span>
      <span class="th">${n.img ? `<img src="${img(n.img)}" alt="" loading="lazy">` : ''}</span>
      <span class="nb"><b>${esc(n.name)}${n.wished ? '<span class="hb">위시</span>' : ''}</b><span>${esc(n.weaponEn || n.weapon || '')}</span></span>
      <span class="np"><b>${fmt(n.price)} VP</b>${n.base ? `<s>${fmt(n.base)}</s>` : ''}</span>
    </div>`).join('')}
  </section>`;
}

function emptyHero() {
  return `<span class="ghost" style="right:-8px;top:-30px;font-size:170px">VALOROAD</span>
  <div class="empty-hero">
    <span class="kicker">WELCOME</span>
    <h1 class="h1">ADD ACCOUNT</h1>
    <p>계정을 추가하면 그 계정의 오늘의 상점 · 번들 · 밤시장을 바로 볼 수 있어요.<br>최대 3개까지 저장되고, 다음부터는 앱을 켜기만 하면 자동으로 로그인돼요.</p>
    ${loginStepsHTML()}
    <div><button class="btn-red" data-add="1">${I.plus} ADD ACCOUNT <span class="ko">첫 계정 추가하기</span></button></div>
  </div>`;
}

function loginStepsHTML() {
  return `<ol class="steps">
      <li><span>01</span><span>라이엇 공식 로그인 창이 열리면 라이엇 계정으로 로그인해요. 이메일 인증 코드(2단계 인증)도 그 창에서 그대로 입력하면 돼요.</span></li>
      <li><span>02</span><span><strong>‘로그인 상태 유지’</strong>를 꼭 켜 주세요. 그래야 다음부터 앱을 켜기만 해도 자동으로 로그인돼요.</span></li>
      <li><span>03</span><span>로그인이 끝나면 창이 저절로 닫히고, 그 계정의 상점이 바로 떠요.</span></li>
    </ol>
    <div class="safe">${I.lock}<div><strong>비밀번호는 저장하지 않아요</strong><span>ValoROAD는 비밀번호를 보지도, 저장하지도 않아요. 로그인 뒤 받은 ‘로그인 열쇠’만 맥 키체인에 잠가 두고, 이 맥 밖으로 보내지 않아요.</span></div></div>`;
}

function renderStore() {
  const el = $('#storeMain');
  if (!S.state) { el.innerHTML = ''; return; }
  const a = activeAcc();
  if (!a) { el.innerHTML = emptyHero(); return; }
  const st = S.store;
  let alerts = hitAlert();
  if (a.status === 'expired') {
    alerts += `<div class="alert warn" role="status">${I.lock}<span class="lbl">SIGN IN AGAIN</span><span class="txt">${esc(labelTight(a))}의 로그인이 만료됐어요. 이 계정만 다시 로그인하면 돼요.${st ? ' (아래는 마지막으로 받은 상점이에요)' : ''}</span><button data-relogin="${a.slot}">다시 로그인</button></div>`;
  } else if (a.status === 'error' || S.storeErr) {
    alerts += `<div class="alert info" role="status"><span class="lbl">CONNECTION</span><span class="txt">${esc(S.storeErr || a.error || '상점을 받지 못했어요')}</span><button data-act="refresh">다시 시도</button></div>`;
  }
  const loading = !st && (S.storeLoading || a.loading || a.status === 'ok');
  let cards = '';
  if (st?.daily?.length) cards = st.daily.map(cardHTML).join('');
  else if (loading) cards = skeletonCards();
  const stale = st && st.stale;
  el.innerHTML = `<span class="ghost" style="right:-8px;top:-22px;font-size:170px">STORE</span>
    ${alerts}
    <div class="headrow">
      <div style="display:flex;flex-direction:column;gap:6px">
        <span class="kicker">${esc(label(a))}</span>
        <h1 class="h1">DAILY OFFERS</h1>
        <span class="h1-sub">오늘의 상점${(S.storeLoading || a.loading) && st ? ' · 새로 받는 중…' : ''}${stale && !(S.storeLoading || a.loading) ? ' · 지난 상점이에요' : ''}</span>
      </div>
      <div class="timer"><span class="lbl">NEXT RESET · 다음 갱신</span>
        <span class="clock" ${st?.dailyEnds ? `data-clock="${st.dailyEnds}"` : ''}>${st?.dailyEnds ? clockText(st.dailyEnds) : '--:--:--'}</span>
        <small>매일 오전 9시</small></div>
    </div>
    ${cards ? `<div class="cards">${cards}</div>` : `<p class="hint">${a.status === 'expired' ? '다시 로그인하면 상점을 볼 수 있어요.' : '상점 정보가 아직 없어요. 오른쪽 위 새로고침을 눌러 보세요.'}</p>`}
    ${st && (st.bundles?.length || st.night?.length) ? `<div class="row2">${bundleHTML(st.bundles || [])}${nightHTML(st.night || [], st.nightEnds)}</div>` : ''}`;
}

// ---------------------------------------------------------------- 그리기: 위시리스트
function renderWish() {
  const s = S.state?.settings || {};
  $('#wishWrap').innerHTML = `<span class="ghost" style="right:-6px;top:-20px;font-size:170px">WISHLIST</span>
  <section class="wish-main">
    <div style="display:flex;flex-direction:column;gap:6px">
      <span class="kicker">3개 계정 모두 확인</span>
      <h1 class="h1">MY WISHLIST <span style="color:var(--red-text)" id="wishCount">${pad(S.wish.items.length)}</span></h1>
      <span class="h1-sub">내 위시리스트<span id="wishSum">${wishSumText()}</span></span>
    </div>
    <div class="searchbar">
      <label class="search">${I.search}<span class="sr">스킨 이름 검색</span>
        <input id="wishQ" type="search" placeholder="스킨 이름으로 찾기 (예: 밴달, 프라임)" value="${esc(S.wish.q)}" autocomplete="off" spellcheck="false"></label>
      <div class="chips" role="group" aria-label="무기 종류">${CATS.map(([k, t]) => `<button class="chip" data-cat="${k}" aria-pressed="${S.wish.cat === k}">${t}</button>`).join('')}</div>
    </div>
    <div id="results"></div>
    <div id="priceNote"></div>
    <div id="wishList"></div>
  </section>
  <aside class="wish-side">
    <section class="panel" aria-label="알림 설정">
      <span class="panel-t">ALERTS <span class="ko">알림</span></span>
      <label class="check"><input type="checkbox" data-set="wishAlert" ${s.wishAlert ? 'checked' : ''}><span><strong>위시리스트 스킨이 뜨면 알림</strong><small>매일 오전 9시 상점이 바뀐 직후 확인해요</small></span></label>
      <label class="check"><input type="checkbox" data-set="nightAlert" ${s.nightAlert ? 'checked' : ''}><span><strong>밤시장에 뜬 것도 알림</strong><small>밤시장이 열리면 할인 품목도 확인해요</small></span></label>
      <label class="check"><input type="checkbox" data-set="sound" ${s.sound ? 'checked' : ''}><span><strong>알림 소리</strong></span></label>
      <div class="note">ValoROAD가 Dock에 켜져 있을 때 확인해요. 앱이 꺼져 있었다면 다음에 켤 때 바로 알려줘요.</div>
    </section>
    <section style="display:flex;flex-direction:column;gap:10px" aria-label="알림 미리 보기">
      <span class="lbl muted" style="font-size:14px;letter-spacing:.14em">PREVIEW <span class="ko">· 맥 알림은 이렇게 와요</span></span>
      <div class="mac-note">${I.logo(38)}<div class="mb"><div class="mh"><span>ValoROAD</span><span>오전 9:00</span></div><b>위시리스트 스킨 등장!</b><span>부캐#KR2 상점에 ‘네온 회로 팬텀’이 떴어요.</span></div></div>
      <div><button class="btn-line" data-act="testnote">알림 시험해 보기</button></div>
    </section>
  </aside>`;
  renderResults();
  renderWishList();
}

function wishSumText() {
  const priced = S.wish.items.filter((w) => w.price > 0);
  if (!priced.length) return '';
  const est = priced.some((w) => w.priceState === 'estimate');
  return ` · 모두 합쳐 ${est ? '약 ' : ''}${fmt(S.wish.sum)} VP`;
}

function priceNote() {
  if (S.wish.pricesReady || !S.wish.items.some((w) => w.priceState === 'estimate')) return '';
  return `<p class="hint" style="margin:0">‘약’이 붙은 가격은 등급별 보통 가격이에요. 라이엇이 전체 가격표를 더 이상 공개하지 않아서, 계정 3개의 상점 · 밤시장 · 번들에 한 번이라도 뜬 스킨은 그때 본 정확한 가격으로 바뀌어요.</p>`;
}

function priceHTML(p, state, cls = 'wprice') {
  if (p > 0 && state === 'estimate') return `<span class="${cls} est" title="라이엇 가격표를 아직 못 받아서 등급별 보통 가격으로 보여 드려요">${I.vp()}<small>약</small><b>${fmt(p)}</b></span>`;
  if (p > 0) return `<span class="${cls}">${I.vp()}<b>${fmt(p)}</b></span>`;
  const t = state === 'none' ? '상점 판매 없음' : '가격 확인 중';
  const tip = state === 'none' ? '배틀패스 · 이벤트 보상처럼 상점에서 팔지 않는 스킨이에요' : '계정이 로그인되면 라이엇 상점 가격표를 받아 와요';
  return `<span class="${cls} none" title="${tip}">${t}</span>`;
}

function renderResults() {
  const el = $('#results');
  if (!el) return;
  const { q, cat, results, total } = S.wish;
  if (!q && cat === 'all') { el.innerHTML = ''; return; }
  if (!S.state?.content?.ready) { el.innerHTML = '<p class="hint">스킨 정보를 받는 중이에요…</p>'; return; }
  el.innerHTML = `<p class="hint" style="margin-bottom:10px">찾은 스킨 ${fmt(total)}개${total > results.length ? ` (앞의 ${results.length}개만 보여요 · 이름을 더 입력해 보세요)` : ''}</p>
    <div class="results">${results.map((s) => {
      const [tn, tc] = tierOf(s.tier);
      return `<div class="rcard" style="--tint:${tint(s.tier)}"><div class="th" data-skin="${esc(s.id)}" role="button" tabindex="0" aria-label="${esc(s.name)} 미리보기">${s.img ? `<img src="${img(s.img)}" alt="" loading="lazy">` : ''}<span class="pv-hint">PREVIEW ▶</span></div>
        <div class="rb" data-skin="${esc(s.id)}"><b>${esc(s.name)}</b><span><i style="display:inline-block;width:8px;height:8px;background:${tc};transform:rotate(45deg);margin-right:6px"></i>${esc(s.weaponEn)} · ${esc(tn)}</span>${priceHTML(s.price, s.priceState, 'rprice')}</div>
        <button class="add${s.wished ? ' on' : ''}" data-wish="${esc(s.id)}" data-on="${s.wished ? 0 : 1}">${s.wished ? '✓ 담김' : '+ 담기'}</button></div>`;
    }).join('')}</div>`;
}

function renderWishList() {
  const el = $('#wishList');
  if (!el) return;
  const c = $('#wishCount');
  if (c) c.textContent = pad(S.wish.items.length);
  const sum = $('#wishSum');
  if (sum) sum.textContent = wishSumText();
  const items = S.wish.items;
  const pn = $('#priceNote');
  if (pn) pn.innerHTML = priceNote();
  if (!items.length) {
    el.innerHTML = `<p class="hint">아직 담은 스킨이 없어요. 위에서 스킨을 찾아 담거나, 상점 카드의 하트를 눌러 보세요.</p>`;
    return;
  }
  el.innerHTML = `<div class="wlist">${items.map((w) => {
    const [, tc] = tierOf(w.tier);
    const hit = w.where.length > 0;
    const where = w.where.map((x) => x.kind === 'night'
      ? `${esc(x.account)} 밤시장 · ${fmt(x.price)} VP${x.base && x.base > x.price ? ` (-${Math.round((1 - x.price / x.base) * 100)}%)` : ''}`
      : `${esc(x.account)} 상점`).join(', ');
    return `<div class="wrow${hit ? ' hit' : ''}" style="--tc:${tc};--tint:${tint(w.tier)}">
      <div class="th" data-skin="${esc(w.id)}" role="button" tabindex="0" aria-label="${esc(w.name)} 미리보기">${w.img ? `<img src="${img(w.img)}" alt="" loading="lazy">` : ''}</div>
      <div class="wb" data-skin="${esc(w.id)}"><b>${esc(w.name)}</b><span><i></i>${esc(w.weaponEn || '')}</span></div>
      ${priceHTML(w.price, w.priceState)}
      ${hit ? `<span class="where">${I.check}<span>지금 ${where}</span></span>` : '<span class="nope">아직 안 떴어요</span>'}
      <button class="heart on" data-wish="${esc(w.id)}" data-on="0" aria-label="${esc(w.name)} 위시리스트에서 빼기">${I.heart(true)}</button>
    </div>`;
  }).join('')}</div>`;
}

// ---------------------------------------------------------------- 그리기: 설정
function renderSettings() {
  const st = S.state;
  if (!st) return;
  const s = st.settings;
  const u = st.update || {};
  const bySlot = Object.fromEntries(accounts().map((a) => [a.slot, a]));
  let rows = '';
  for (let slot = 1; slot <= 3; slot++) {
    const a = bySlot[slot];
    if (!a) {
      rows += `<div class="srow"><span class="slot">${pad(slot)}</span><span class="sb"><b class="muted">빈 자리</b><span class="muted">계정을 추가할 수 있어요</span></span>
        <button class="btn-line" data-add="${slot}">${I.plus} 계정 추가</button></div>`;
      continue;
    }
    const status = a.status === 'expired' ? '<span class="bad">로그인 만료 · 다시 로그인이 필요해요</span>'
      : a.status === 'error' ? `<span class="bad">${esc(a.error || '연결 문제')}</span>` : '<span class="ok">자동 로그인 유지 중</span>';
    rows += `<div class="srow"><span class="slot">${pad(slot)}</span><span class="sb"><b>${esc(labelTight(a))}</b>${status}</span>
      <button class="btn-line" data-relogin="${slot}">다시 로그인</button>
      <button class="btn-line danger" data-remove="${esc(a.id)}">슬롯 비우기</button></div>`;
  }
  const upStatus = {
    uptodate: '<span class="pill">' + I.check + ' 최신 버전이에요</span>',
    checking: '<span class="pill gray">확인하는 중…</span>',
    available: `<span class="pill warn">새 버전 v${esc(u.latest?.version || '')}</span>`,
    downloading: `<span class="pill gray">받는 중 ${Math.round((u.progress || 0) * 100)}%</span>`,
    installing: '<span class="pill gray">설치하는 중…</span>',
    ready: `<span class="pill warn">v${esc(u.installed || '')} 준비됨 · 다시 시작하면 바뀌어요</span>`,
    error: `<span class="pill warn">${esc(u.error || '확인하지 못했어요')}</span>`,
  }[u.status] || '';
  const last = u.lastCheck ? new Date(u.lastCheck).toLocaleString('ko-KR', { month: 'long', day: 'numeric', hour: 'numeric', minute: '2-digit' }) : '';
  $('#setWrap').innerHTML = `<span class="ghost" style="right:-6px;bottom:-40px;font-size:170px">SETTINGS</span>
  <div class="set-main">
    <h1 class="h1">SETTINGS <span class="ko h1-sub">설정</span></h1>
    <section class="panel" aria-label="계정" style="gap:4px">
      <span class="panel-t" style="margin-bottom:8px">ACCOUNTS <span class="ko">계정 · 최대 3개</span></span>
      ${rows}
    </section>
    <section class="panel" aria-label="업데이트">
      <span class="panel-t">UPDATE <span class="ko">업데이트</span></span>
      <div class="ver">
        <div style="display:flex;flex-direction:column;gap:2px"><span class="lbl muted" style="font-size:14px;letter-spacing:.14em">VERSION</span><span class="v">v${esc(u.current || s.build)}</span></div>
        ${upStatus}
        ${last ? `<span class="muted" style="font-size:12px">마지막 확인 · ${esc(last)}</span>` : ''}
        <span style="margin-left:auto;display:flex;gap:8px;flex-wrap:wrap">
          ${u.status === 'ready' ? '<button class="btn-red" data-act="restart">RESTART <span class="ko">다시 시작</span></button>'
            : u.status === 'available' ? '<button class="btn-red" data-act="install">INSTALL <span class="ko">받기</span></button>'
            : '<button class="btn-red" data-act="check">CHECK <span class="ko">업데이트 확인</span></button>'}
        </span>
      </div>
      <label class="check"><input type="checkbox" data-set="autoUpdate" ${s.autoUpdate ? 'checked' : ''}><span>자동 업데이트</span></label>
      <p class="hint">새 버전이 나오면 앱이 알아서 받아요. 받은 뒤에는 앱이 스스로 다시 켜지고, 문제가 생기면 이전 버전으로 되돌릴 수 있어요.</p>
      ${u.canRollback ? `<div><button class="btn-line" data-act="rollback">이전 버전(v${esc(u.backupVersion)})으로 되돌리기</button></div>` : ''}
    </section>
  </div>
  <div class="set-side">
    <section class="panel" aria-label="화면">
      <span class="panel-t">DISPLAY <span class="ko">화면</span></span>
      <fieldset><legend>테마</legend>
        ${[['system', '맥 설정 따르기'], ['dark', '항상 다크'], ['light', '항상 라이트']].map(([k, t]) => `<label class="radio"><input type="radio" name="theme" value="${k}" ${s.appearance === k ? 'checked' : ''}>${t}</label>`).join('')}
      </fieldset>
    </section>
    <section class="panel" aria-label="도움말" style="gap:10px">
      <span class="panel-t">HELP <span class="ko">도움말</span></span>
      <button class="btn-line" style="justify-content:flex-start" data-act="guide">사용 가이드 보기</button>
      <button class="btn-line" style="justify-content:flex-start" data-act="diag">진단 정보 보기</button>
      <button class="btn-line" style="justify-content:flex-start" data-act="quit">ValoROAD 종료</button>
      <span class="muted" style="font-size:12px;line-height:1.6">문제가 생기면 ‘진단 정보 보기’ 내용을 보내 주세요. (로그인 정보는 들어 있지 않아요)</span>
    </section>
    <section class="about" aria-label="앱 정보">
      ${I.logo(60)}
      <span class="word" style="font-size:30px">Valo<b>ROAD</b> <span class="lbl" style="font-size:18px;opacity:.7">v${esc(s.build)}</span></span>
      <span class="lbl" style="font-size:17px">made by. Nevertheless_D</span>
      <small>상점 확인 전용 앱이에요. 구매 기능은 없어요. ValoROAD는 Riot Games가 만들거나 보증한 앱이 아니며, 게임 이름과 아이템 이미지는 Riot Games의 자산이에요. 스킨 정보 · 이미지: valorant-api.com</small>
    </section>
  </div>`;
}

function renderAll() {
  renderTabs();
  renderWallet();
  renderBanner();
  if (S.view === 'store') { renderSide(); renderStore(); }
  if (S.view === 'wish') renderWish();
  if (S.view === 'settings') renderSettings();
  renderOverlay();
}

// ---------------------------------------------------------------- 로그인 · 안내 창
function renderOverlay() {
  const o = $('#overlay');
  const L = S.login;
  if (S.guide) { o.innerHTML = guideHTML(); o.hidden = false; return; }
  if (S.pv) { o.innerHTML = pvHTML(); o.hidden = false; return; }
  if (!L) { o.hidden = true; o.innerHTML = ''; return; }
  let tail = '';
  if (L.phase === 'wait') tail = `<div class="waiting"><i></i>라이엇 로그인 창에서 로그인이 끝나기를 기다리는 중…</div>
    <div class="row"><button class="btn-line" data-act="login-cancel">로그인 창 닫기</button></div>`;
  if (L.phase === 'saving') tail = `<div class="waiting"><i></i>계정 정보를 확인하고 저장하는 중…</div>`;
  if (L.phase === 'error') tail = `<div class="alert warn"><span class="txt">${esc(L.error)}</span></div>
    <div class="row"><button class="btn-line" data-act="login-close">닫기</button><button class="btn-red" data-add="${L.slot}">TRY AGAIN <span class="ko">다시 시도</span></button></div>`;
  if (L.phase === 'nobridge') tail = `<div class="alert info"><span class="txt">로그인은 ValoROAD 앱 창에서만 할 수 있어요. Dock 의 ValoROAD 를 눌러 앱에서 다시 시도해 주세요.</span></div>
    <div class="row"><button class="btn-line" data-act="login-close">닫기</button></div>`;
  o.innerHTML = `<div class="modal" role="dialog" aria-modal="true" aria-label="계정 추가">
    <span class="kicker">SLOT ${pad(L.slot)}</span>
    <h1 class="h1">${L.relogin ? 'SIGN IN AGAIN' : 'ADD ACCOUNT'}</h1>
    <strong style="font-size:16px">라이엇 공식 로그인 창에서 직접 로그인해요</strong>
    ${loginStepsHTML()}
    ${tail}
  </div>`;
  o.hidden = false;
}

// ---------------------------------------------------------------- 미리보기 (게임에서 아이템을 누르면 보이는 것)
const LEVEL_LABEL = {
  '': '기본', vfx: '시각 효과', animation: '애니메이션', finisher: '피니셔', soundeffects: '사운드 효과', killbanner: '처치 배너',
  topfrag: '탑 프래그', transformation: '변신', killeffect: '처치 효과', killcounter: '처치 카운터', randomizer: '랜덤 변화',
  inspectandkill: '검사 · 처치', songshuffle: '음악 바꾸기', voiceover: '음성', heartbeatandmapsensor: '심장 박동 · 맵 감지',
  fishanimation: '물고기 애니메이션', attackerdefenderswap: '공수 전환',
};
I.play = '<svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true"><path d="M3 1.5 L10.5 6 L3 10.5 Z" fill="currentColor"/></svg>';
I.close = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="square" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg>';

async function openPreview(id) {
  S.pv = { id, data: null, chroma: 0, level: -1, cvideo: false };
  renderOverlay();
  try {
    const d = await api(`/api/skin?id=${encodeURIComponent(id)}`);
    if (S.pv?.id !== id) return;
    S.pv.data = d;
  } catch (e) {
    if (S.pv?.id !== id) return;
    S.pv.error = e.message;
  }
  renderOverlay();
}

function pvHTML() {
  const P = S.pv;
  const close = `<button class="pv-close" data-act="pv-close" aria-label="미리보기 닫기">${I.close}</button>`;
  if (!P.data) {
    return `<div class="modal pv" role="dialog" aria-modal="true" aria-label="미리보기">${close}<span class="kicker">PREVIEW</span>
      <p class="hint">${P.error ? esc(P.error) : '미리보기를 불러오는 중…'}</p></div>`;
  }
  const d = P.data;
  const sk = d.skin;
  const [tn, tc] = tierOf(sk.tier);
  const chromas = d.chromas?.length ? d.chromas : [{ name: '기본', img: sk.img }];
  const ch = chromas[Math.min(P.chroma, chromas.length - 1)];
  const lv = P.level >= 0 ? d.levels[P.level] : null;
  const video = lv?.video || (P.cvideo ? ch.video : '');
  const stage = video
    ? `<video src="${esc(video)}" autoplay controls playsinline loop></video>`
    : `<img src="${img(ch.img || sk.img)}" alt="${esc(sk.name)} ${esc(ch.name)}">`;
  const levels = (d.levels || []).map((l, i) => `<button class="pv-chip${P.level === i ? ' on' : ''}" data-pv-level="${i}"${l.video ? '' : ' disabled title="영상이 없어요"'}>
      <span class="lbl">LV ${i + 1}</span><span>${esc(LEVEL_LABEL[l.item] ?? '레벨 효과')}</span>${l.video ? I.play : ''}</button>`).join('');
  const swatches = chromas.map((c, i) => `<button class="pv-sw${P.chroma === i && !lv ? ' on' : ''}" data-pv-chroma="${i}" title="${esc(c.name)}" aria-label="${esc(c.name)}">${c.swatch ? `<img src="${img(c.swatch)}" alt="">` : `<span class="lbl">${pad(i + 1)}</span>`}</button>`).join('');
  return `<div class="modal pv" role="dialog" aria-modal="true" aria-label="${esc(sk.name)} 미리보기" style="--tc:${tc};--tint:${tint(sk.tier)}">
    ${close}
    <div class="pv-head">
      <div style="display:flex;flex-direction:column;gap:6px;min-width:0">
        <span class="kicker">PREVIEW · ${esc((sk.weaponEn || '').toUpperCase())}</span>
        <span class="pv-title">${esc(sk.name)}</span>
        <span class="pv-tier"><i></i>${esc(tn)}${sk.weapon ? ` · ${esc(sk.weapon)}` : ''}</span>
      </div>
      <div class="pv-price">
        ${priceHTML(d.price, d.priceState)}
        <button class="heart${d.wished ? ' on' : ''}" data-wish="${esc(sk.id)}" data-on="${d.wished ? 0 : 1}" aria-pressed="${d.wished}" aria-label="${d.wished ? '위시리스트에서 빼기' : '위시리스트에 담기'}">${I.heart(d.wished)}</button>
      </div>
    </div>
    <div class="pv-stage">${stage}</div>
    ${levels ? `<div class="pv-row"><span class="pv-label">LEVELS · 레벨 (누르면 영상)</span><div class="pv-chips">${levels}</div></div>` : ''}
    <div class="pv-row"><span class="pv-label">VARIANTS · 변형</span>
      <div class="pv-chips">${swatches}<span class="pv-cname">${esc(lv ? '' : ch.name)}</span>
        ${!lv && ch.video ? `<button class="pv-chip${P.cvideo ? ' on' : ''}" data-act="pv-cvideo">${I.play}<span>이 변형 영상</span></button>` : ''}</div></div>
    ${d.detailReady === false ? '<p class="hint" style="margin:0">레벨 영상 정보를 받는 중이에요. 잠시 뒤 다시 열어 보세요.</p>' : ''}
  </div>`;
}

function guideHTML() {
  return `<div class="modal" role="dialog" aria-modal="true" aria-label="사용 가이드">
    <span class="kicker">HOW TO USE</span>
    <h1 class="h1">GUIDE</h1>
    <ol class="steps">
      <li><span>01</span><span><strong>계정 추가</strong> — 왼쪽 빈 자리(또는 설정 › 계정)를 눌러 라이엇 공식 창에서 로그인해요. 최대 3개까지 저장돼요.</span></li>
      <li><span>02</span><span><strong>계정 바꾸기</strong> — 왼쪽 계정을 누르면 로그아웃 없이 그 계정의 오늘의 상점 · 번들 · 밤시장이 바로 떠요. ⌘1 · ⌘2 · ⌘3 으로도 바꿀 수 있어요.</span></li>
      <li><span>03</span><span><strong>위시리스트</strong> — 상점 카드의 하트나 위시리스트 화면에서 스킨을 담아 두면, 3개 계정 중 어디든 그 스킨이 뜰 때 맥 알림으로 알려줘요.</span></li>
      <li><span>04</span><span><strong>상점은 매일 오전 9시</strong>에 바뀌어요. 앱이 Dock 에 켜져 있으면 바뀌자마자 확인해요.</span></li>
    </ol>
    <div class="panel" style="gap:8px">
      <span class="panel-t">CAUTION <span class="ko">주의</span></span>
      <span class="hint">· 확인 전용 앱이에요. 구매는 게임 안에서 해 주세요.<br>· 라이엇 보안 정책 때문에 가끔(보통 몇 주에 한 번) 그 계정만 다시 로그인해야 해요.<br>· 라이엇이 연결 방식을 바꾸면 잠시 상점이 안 보일 수 있어요. 새 버전이 나오면 앱이 알아서 받아요.<br>· ValoROAD는 Riot Games가 만들거나 보증한 앱이 아니에요.</span>
    </div>
    <div class="row"><button class="btn-red" data-act="guide-close">OK <span class="ko">확인</span></button></div>
  </div>`;
}

async function startLogin(slot, relogin = false) {
  S.login = { slot, relogin, phase: bridge ? 'wait' : 'nobridge' };
  renderOverlay();
  if (!bridge) return;
  try {
    const r = await bridge.postMessage({ type: 'login', slot });
    if (!S.login) return;
    if (r?.canceled) { S.login = null; renderOverlay(); return; }
    if (r?.error) { S.login.phase = 'error'; S.login.error = r.error; renderOverlay(); return; }
    S.login = null;
    renderOverlay();
    await loadState();
    if (r?.id) { S.active = r.id; S.store = null; }
    S.view = 'store';
    renderAll();
    loadStore(S.active);
    toast(r?.note ? `${r.name || ''} 계정을 저장했어요. ${r.note}` : `${r?.name ? r.name + (r.tag ? '#' + r.tag : '') + ' ' : ''}계정을 저장했어요`);
  } catch (e) {
    if (!S.login) return;
    S.login.phase = 'error';
    S.login.error = e?.message || String(e);
    renderOverlay();
  }
}

// ---------------------------------------------------------------- 동작
function selectAccount(id) {
  if (!id || id === S.active) { S.view = 'store'; renderAll(); return; }
  S.active = id;
  S.store = null;
  S.storeErr = '';
  S.bundleIdx = 0;
  S.view = 'store';
  renderAll();
  loadStore(id);
}

async function refreshStore() {
  if (!S.active || S.refreshing) return;
  S.refreshing = true;
  renderWallet();
  await loadStore(S.active, true);
  S.refreshing = false;
  await loadState().catch(() => {});
  renderAll();
}

document.addEventListener('click', async (e) => {
  if (e.target.id === 'overlay' && S.pv) { S.pv = null; renderOverlay(); return; }
  const t = e.target.closest('button, [data-view-go], [data-skin]');
  if (!t) return;
  if (t.dataset.skin && t.tagName !== 'BUTTON') { openPreview(t.dataset.skin); return; }
  if (t.dataset.pvLevel) { S.pv.level = +t.dataset.pvLevel; S.pv.cvideo = false; renderOverlay(); return; }
  if (t.dataset.pvChroma) { S.pv.chroma = +t.dataset.pvChroma; S.pv.level = -1; S.pv.cvideo = false; renderOverlay(); return; }
  if (t.classList.contains('tab')) { S.view = t.dataset.view; renderAll(); if (S.view === 'wish') { loadWishlist(); searchSkins(); } return; }
  if (t.dataset.viewGo) { S.view = t.dataset.viewGo; renderAll(); return; }
  if (t.dataset.acc) { selectAccount(t.dataset.acc); return; }
  if (t.dataset.add) { startLogin(+t.dataset.add); return; }
  if (t.dataset.relogin) { startLogin(+t.dataset.relogin, true); return; }
  if (t.dataset.wish) { setWish(t.dataset.wish, t.dataset.on === '1'); return; }
  if (t.dataset.bundle) { S.bundleIdx = +t.dataset.bundle; renderStore(); return; }
  if (t.dataset.cat) { S.wish.cat = t.dataset.cat; $$('.chip').forEach((c) => c.setAttribute('aria-pressed', String(c.dataset.cat === S.wish.cat))); searchSkins(); return; }
  if (t.dataset.remove) {
    const a = accounts().find((x) => x.id === t.dataset.remove);
    if (!confirm(`${labelTight(a)} 계정을 이 자리에서 뺄까요?\n저장된 로그인 정보도 함께 지워져요.`)) return;
    try { await api('/api/accounts/remove', { id: t.dataset.remove }); toast('슬롯을 비웠어요'); } catch (er) { toast(er.message, true); }
    await loadState(); renderAll();
    return;
  }
  if (t.id === 'refreshBtn') { refreshStore(); return; }
  switch (t.dataset.act) {
    case 'refresh': refreshStore(); break;
    case 'login-cancel': bridge?.postMessage({ type: 'loginCancel' }).catch(() => {}); S.login = null; renderOverlay(); break;
    case 'login-close': S.login = null; renderOverlay(); break;
    case 'guide': S.guide = true; renderOverlay(); break;
    case 'guide-close': S.guide = false; renderOverlay(); break;
    case 'pv-close': S.pv = null; renderOverlay(); break;
    case 'pv-cvideo': S.pv.cvideo = !S.pv.cvideo; renderOverlay(); break;
    case 'testnote': await api('/api/notify/test', {}); toast(bridge ? '잠시 뒤 맥 알림이 와요' : '맥 앱에서 알림이 와요'); break;
    case 'diag': await api('/api/open-diag', {}); break;
    case 'quit':
      if (confirm('ValoROAD를 종료할까요?')) { await api('/api/quit', {}).catch(() => {}); document.body.innerHTML = '<p style="padding:40px;font-size:15px">ValoROAD를 종료했어요.</p>'; }
      break;
    case 'check': S.state.update = await api('/api/update/check'); renderAll(); break;
    case 'install': await api('/api/update/install', {}); toast('새 버전을 받기 시작했어요'); break;
    case 'restart': await api('/api/update/restart', {}).catch((er) => toast(er.message, true)); break;
    case 'rollback':
      if (confirm('이전 버전으로 되돌릴까요? 다시 시작하면 적용돼요. (자동 업데이트는 꺼져요)')) {
        try { await api('/api/update/rollback', {}); toast('되돌렸어요. 다시 시작하면 적용돼요'); } catch (er) { toast(er.message, true); }
        await loadState(); renderAll();
      }
      break;
    default:
  }
});

document.addEventListener('change', (e) => {
  const t = e.target;
  if (t.dataset.set) saveSetting(t.dataset.set, t.checked);
  if (t.name === 'theme') { saveSetting('appearance', t.value).then(() => renderAll()); }
});

let qTimer = 0;
document.addEventListener('input', (e) => {
  if (e.target.id !== 'wishQ') return;
  S.wish.q = e.target.value.trim();
  clearTimeout(qTimer);
  qTimer = setTimeout(searchSkins, 160);
});

document.addEventListener('keydown', (e) => {
  if ((e.key === 'Enter' || e.key === ' ') && e.target.dataset?.skin && e.target.tagName !== 'BUTTON') {
    e.preventDefault();
    openPreview(e.target.dataset.skin);
    return;
  }
  if (e.key === 'Escape') {
    if (S.guide) { S.guide = false; renderOverlay(); }
    else if (S.pv) { S.pv = null; renderOverlay(); }
    else if (S.login && S.login.phase !== 'wait' && S.login.phase !== 'saving') { S.login = null; renderOverlay(); }
  }
  if (!e.metaKey) return;
  if (e.key === 'r') { e.preventDefault(); refreshStore(); }
  if (['1', '2', '3'].includes(e.key)) {
    const a = accounts().find((x) => x.slot === +e.key);
    if (a) { e.preventDefault(); selectAccount(a.id); }
  }
});

// 화면 문제는 진단 기록에 남겨요
window.addEventListener('error', (e) => { api('/api/clientlog', { items: [`${e.message} @ ${e.filename}:${e.lineno}`] }).catch(() => {}); });
window.addEventListener('unhandledrejection', (e) => { api('/api/clientlog', { items: [`promise: ${e.reason?.message || e.reason}`] }).catch(() => {}); });

// ---------------------------------------------------------------- 시작 · 주기적으로 확인
async function poll() {
  try {
    const p = await api('/api/poll');
    const updChanged = JSON.stringify(p.update) !== JSON.stringify(S.state?.update);
    if (S.state) S.state.update = p.update;
    if (p.rev !== S.rev) {
      await loadState();
      if (S.view === 'store' && S.active) {
        const st = await api(`/api/store?id=${encodeURIComponent(S.active)}`).catch(() => null);
        if (st) { S.store = st.store; S.storeLoading = !!st.loading; }
      }
      if (S.view === 'wish') await loadWishlist();
      renderAll();
    } else if (updChanged) {
      renderBanner();
      if (S.view === 'settings') renderSettings();
    }
  } catch { /* 엔진이 잠깐 끊겨도 다음에 다시 시도 */ }
  setTimeout(poll, 2500);
}

(async function start() {
  try {
    await loadState();
  } catch (e) {
    $('#storeMain').innerHTML = `<p class="hint" style="padding:40px">ValoROAD를 시작하지 못했어요. 앱을 다시 실행해 주세요. (${esc(e.message)})</p>`;
    return;
  }
  renderAll();
  if (S.active) loadStore(S.active);
  loadWishlist();
  setTimeout(poll, 2500);
})();
