// ValoROAD — 발로란트 오늘의 상점 · 야시장 확인 앱
// made by. Nevertheless_D
'use strict';

const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];
const T = new URLSearchParams(location.search).get('t');
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const CUR = {
  vp: 'https://media.valorant-api.com/currencies/85ad13f7-3d1b-5128-9eb2-7cd8ee0b5741/displayicon.png',
  rp: 'https://media.valorant-api.com/currencies/e59aa87c-4cbf-517a-5983-6e81511be9b7/displayicon.png',
  kc: 'https://media.valorant-api.com/currencies/85ca954a-41f2-ce94-9b45-8ca3dd39a00d/displayicon.png',
};
const cardArt = (id, kind = 'wideart') => (id ? `https://media.valorant-api.com/playercards/${id}/${kind}.png` : '');
const num = (n) => Number(n || 0).toLocaleString('ko-KR');

async function api(path, body) {
  const r = await fetch(path, {
    method: body ? 'POST' : 'GET',
    headers: { 'x-token': T, 'content-type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) { const e = new Error(j.error || `HTTP ${r.status}`); e.data = j; throw e; }
  return j;
}

const S = {
  settings: null, accounts: [], sel: null, tab: 'daily',
  stores: {}, storeErr: {}, loading: {}, poll: null,
  wish: new Set(), catalog: null, catalogLoading: false,
  update: null, tried: {},
};
const W = { q: '', cat: '전체', only: false, limit: 60 };

// ════════════════════════════════════════════════ 작은 도구들
function toast(msg, ms = 2600) {
  const el = document.createElement('div');
  el.className = 'toast';
  el.textContent = msg;
  $('#toasts').append(el);
  setTimeout(() => { el.classList.add('hide'); setTimeout(() => el.remove(), 320); }, ms);
}

function pad(n) { return String(n).padStart(2, '0'); }
function fmtLeft(ms, long = false) {
  if (!ms || ms <= 0) return '곧 바뀌어요';
  const s = Math.floor(ms / 1000);
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
  if (long && d > 0) return `${d}일 ${h}시간`;
  if (d > 0) return `${d}일 ${pad(h)}:${pad(m)}:${pad(sec)}`;
  return `${pad(h)}:${pad(m)}:${pad(sec)}`;
}
function fmtAgo(ms) {
  if (!ms) return '아직 확인 전';
  const s = Math.max(0, (Date.now() - ms) / 1000);
  if (s < 50) return '방금 확인했어요';
  if (s < 3600) return `${Math.round(s / 60)}분 전에 확인`;
  if (s < 86400) return `${Math.round(s / 3600)}시간 전에 확인`;
  return `${Math.round(s / 86400)}일 전에 확인`;
}
function fmtWhen(ms) {
  if (!ms) return '아직 안 했어요';
  const d = new Date(ms);
  return `${d.getMonth() + 1}월 ${d.getDate()}일 ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
function nextUtcMidnight() {
  const n = new Date();
  return Date.UTC(n.getUTCFullYear(), n.getUTCMonth(), n.getUTCDate() + 1);
}
const accName = (a) => (a ? `${a.name || '이름 없음'}${a.tag ? '#' + a.tag : ''}` : '');

// ════════════════════════════════════════════════ 테마
function applyTheme(v) {
  if (v === 'light' || v === 'dark') document.documentElement.dataset.theme = v;
  else delete document.documentElement.dataset.theme;
  try { localStorage.setItem('valoroad-theme', v || 'system'); } catch { /* ignore */ }
  $$('#themeToggle button').forEach((b) => b.classList.toggle('on', b.dataset.theme === (v || 'system')));
}
$$('#themeToggle button').forEach((b) => (b.onclick = async () => {
  applyTheme(b.dataset.theme);
  S.settings = await api('/api/settings', { appearance: b.dataset.theme }).catch(() => S.settings);
}));

// ════════════════════════════════════════════════ 모달 · 메뉴
const M = { open: false, onClose: null };
function openModal({ title, body, wide = false, onClose = null, actions = [] }) {
  if (M.open && M.onClose) { const f = M.onClose; M.onClose = null; f(); }
  $('#modalTitle').textContent = title;
  const mb = $('#modalBody');
  mb.innerHTML = '';
  if (typeof body === 'string') mb.innerHTML = body; else mb.append(body);
  $('#modal').classList.toggle('wide', wide);
  $('#modalVeil').hidden = false;
  M.open = true; M.onClose = onClose;
  setModalActions(actions);
  mb.scrollTop = 0;
}
function setModalActions(actions) {
  const box = $('#modalActions');
  box.innerHTML = '';
  for (const a of actions) {
    const b = document.createElement('button');
    b.className = 'btn' + (a.primary ? '' : ' ghost');
    b.innerHTML = a.label;
    b.disabled = !!a.disabled;
    b.onclick = a.onClick;
    box.append(b);
  }
}
function closeModal() {
  if (!M.open) return;
  $('#modalVeil').hidden = true;
  $('#modalBody').innerHTML = '';
  M.open = false;
  const f = M.onClose; M.onClose = null;
  if (f) f();
}
$('#modalClose').onclick = closeModal;
$('#modalVeil').addEventListener('mousedown', (e) => { if (e.target.id === 'modalVeil') closeModal(); });

function confirmBox(title, msg, okLabel = '확인', danger = false) {
  return new Promise((res) => {
    let done = false;
    openModal({
      title, body: `<p class="note" style="font-size:14px">${msg}</p>`,
      onClose: () => { if (!done) res(false); },
      actions: [
        { label: '취소', onClick: () => { done = true; closeModal(); res(false); } },
        { label: okLabel, primary: true, onClick: () => { done = true; closeModal(); res(true); } },
      ],
    });
    if (danger) $('#modalActions .btn:not(.ghost)').style.background = 'var(--red)';
  });
}

let menuEl = null;
function closeMenu() { if (menuEl) { menuEl.remove(); menuEl = null; } }
function openMenu(anchor, items) {
  closeMenu();
  menuEl = document.createElement('div');
  menuEl.className = 'menu';
  for (const it of items) {
    const b = document.createElement('button');
    b.className = it.danger ? 'danger' : '';
    b.innerHTML = `${it.icon || ''}<span>${esc(it.label)}</span>`;
    b.onclick = (e) => { e.stopPropagation(); closeMenu(); it.onClick(); };
    menuEl.append(b);
  }
  document.body.append(menuEl);
  const r = anchor.getBoundingClientRect();
  const w = menuEl.offsetWidth, h = menuEl.offsetHeight;
  menuEl.style.left = Math.min(r.left, innerWidth - w - 10) + 'px';
  menuEl.style.top = (r.bottom + h + 6 > innerHeight ? r.top - h - 6 : r.bottom + 6) + 'px';
}
document.addEventListener('mousedown', (e) => { if (menuEl && !menuEl.contains(e.target)) closeMenu(); });

// ════════════════════════════════════════════════ 계정 목록 (왼쪽)
function selAcc() { return S.accounts.find((a) => a.id === S.sel) || null; }
function hitsFor(id) { return (S.poll?.hits || []).filter((h) => h.account === id); }

function renderAccounts() {
  const box = $('#accList');
  const max = S.settings?.maxAccounts || 3;
  $('#accCount').textContent = `${S.accounts.length}/${max}`;
  const loginBusy = ['opening', 'waiting', 'login', 'finishing'].includes(S.poll?.login?.stage);
  let html = '';
  S.accounts.forEach((a) => {
    let st = '';
    if (a.busy) st = '<span class="st busy"><i class="spin" style="width:9px;height:9px;border-width:1.5px"></i> 확인 중</span>';
    else if (a.status === 'expired') st = '<span class="st">다시 로그인</span>';
    else if (a.error) st = '<span class="st">확인 실패</span>';
    const hits = hitsFor(a.id).length;
    html += `<div class="acc ${a.id === S.sel ? 'on' : ''}" data-id="${esc(a.id)}" role="button" tabindex="0">
      <div class="acc-bg" style="background-image:url('${cardArt(a.card)}')"></div>
      ${hits ? `<span class="hit">WISH ×${hits}</span>` : ''}
      <div class="acc-in">
        <div class="av" style="background-image:url('${cardArt(a.card, 'smallart')}')"></div>
        <div class="who">
          <b>${esc(a.name || '이름 없음')}<em>#${esc(a.tag)}</em></b>
          <div class="meta"><span>LV ${a.level || '-'}</span>${a.hasStore ? `<span class="vp"><img src="${CUR.vp}" alt="">${num(a.vp)}</span>` : ''}${st}</div>
        </div>
      </div>
      <button class="more" data-more="${esc(a.id)}" title="계정 메뉴">${ART.more}</button>
    </div>`;
  });
  for (let i = S.accounts.length; i < max; i++) {
    html += `<button class="acc slot ${loginBusy ? 'locked' : ''}" data-add="1"><span class="plus"><span>+</span></span><div><b>계정 추가</b><br><small>SLOT ${i + 1}</small></div></button>`;
  }
  box.innerHTML = html;
  $$('.acc[data-id]', box).forEach((el) => {
    el.onclick = (e) => { if (e.target.closest('.more')) return; selectAccount(el.dataset.id); };
    el.onkeydown = (e) => { if (e.key === 'Enter') selectAccount(el.dataset.id); };
  });
  $$('[data-more]', box).forEach((b) => (b.onclick = (e) => { e.stopPropagation(); accountMenu(b, b.dataset.more); }));
  $$('[data-add]', box).forEach((b) => (b.onclick = () => openLogin('')));
  renderSideCards();
}

function renderSideCards() {
  const st = S.settings;
  if (st) {
    $('#dailyDot').classList.toggle('on', !!st.dailyCheck);
    $('#dailyText').textContent = st.dailyCheck ? `매일 ${st.dailyTime} 자동 확인 켜짐` : '매일 자동 확인 꺼짐';
    $('#dailySub').textContent = st.dailyCheck
      ? '앱이 꺼져 있어도 상점이 바뀌면 계정들을 확인하고, 위시리스트 스킨이 뜨면 알려 드려요.'
      : '설정에서 켜면 앱을 열지 않아도 위시리스트 스킨을 알려 드려요.';
  }
  $('#wishCount').textContent = S.wish.size;
  const hits = S.poll?.hits || [];
  $('#wishMini').classList.toggle('hot', hits.length > 0);
  $('#wishHitText').textContent = hits.length
    ? `지금 상점에 ${[...new Set(hits.map((h) => h.name))].slice(0, 2).join(', ')}${hits.length > 2 ? ' 외' : ''}이(가) 있어요!`
    : (S.wish.size ? '상점에 뜨면 바로 알려 드려요.' : '마음에 드는 스킨에 ♥를 눌러 보세요.');
  const n = $('#wishTabN');
  n.textContent = hits.length || '';
  n.hidden = !hits.length;
}

function accountMenu(anchor, id) {
  const a = S.accounts.find((x) => x.id === id);
  if (!a) return;
  openMenu(anchor, [
    { label: '상점 새로 고침', icon: ART.refresh, onClick: () => { selectAccount(id); loadStore(id, true); } },
    { label: '다시 로그인', icon: ART.user, onClick: () => openLogin(id) },
    { label: '이 계정 지우기', icon: ART.trash, danger: true, onClick: () => removeAccount(a) },
  ]);
}

async function removeAccount(a) {
  const ok = await confirmBox('계정 지우기', `<b>${esc(accName(a))}</b> 계정을 ValoROAD에서 지울까요?<br>저장된 자동 로그인 정보도 함께 지워져요. (Riot 계정 자체는 아무 영향이 없어요)`, '지우기', true);
  if (!ok) return;
  await api('/api/accounts/remove', { id: a.id }).catch(() => {});
  delete S.stores[a.id];
  if (S.sel === a.id) S.sel = null;
  toast('계정을 지웠어요');
  await poll();
}

function selectAccount(id) {
  if (S.sel !== id) {
    S.sel = id;
    api('/api/accounts/select', { id }).catch(() => {});
    if (S.tab === 'wish') S.tab = 'daily';
  }
  renderAccounts();
  renderAll();
  loadStore(id, false);
}

// ════════════════════════════════════════════════ 상점 불러오기
async function loadStore(id, force) {
  if (!id || S.loading[id]) return;
  S.loading[id] = true;
  if (S.sel === id) renderHero();
  try {
    const j = await api(`/api/store?id=${encodeURIComponent(id)}${force ? '&refresh=1' : ''}`);
    S.stores[id] = j.store;
    delete S.storeErr[id];
    if (force && S.sel === id) toast('🔄 상점을 새로 불러왔어요');
  } catch (e) {
    S.storeErr[id] = { msg: e.message, code: e.data?.code || '' };
    if (e.data?.cached) S.stores[id] = e.data.cached;
  } finally {
    S.loading[id] = false;
  }
  if (S.sel === id) renderAll();
}

// ════════════════════════════════════════════════ 위쪽 계정 정보
function renderHero() {
  const a = selAcc();
  const hero = $('#hero');
  hero.hidden = !a;
  $('#tabs').hidden = !a;
  if (!a) return;
  const v = S.stores[a.id];
  $('#heroBg').style.backgroundImage = a.card ? `url('${cardArt(a.card)}')` : '';
  $('#heroAvatar').style.backgroundImage = a.card ? `url('${cardArt(a.card, 'smallart')}')` : '';
  $('#heroName').textContent = a.name || '이름 없음';
  $('#heroTag').textContent = a.tag ? '#' + a.tag : '';
  $('#heroRegion').textContent = `// ${String(a.region || 'kr').toUpperCase()} SERVER`;
  $('#heroLevel').textContent = `LV ${(v && v.level) || a.level || '-'}`;
  $('#heroUpdated').textContent = S.loading[a.id] ? '상점을 확인하고 있어요…' : fmtAgo(v?.fetchedAt);
  const w = v?.wallet;
  $('#wallet').innerHTML = w ? `
    <div class="coin vp"><img class="cur" src="${CUR.vp}" alt=""><div><b>${num(w.vp)}</b><small>발로란트 포인트</small></div></div>
    <div class="coin"><img class="cur" src="${CUR.rp}" alt=""><div><b>${num(w.rp)}</b><small>레디어나이트</small></div></div>
    <div class="coin"><img class="cur" src="${CUR.kc}" alt=""><div><b>${num(w.kc)}</b><small>킹덤 크레딧</small></div></div>` : '';
  const rb = $('#refreshBtn');
  rb.classList.toggle('busy', !!(S.loading[a.id] || a.busy));
  rb.disabled = !!S.loading[a.id];
  const v2 = S.stores[a.id];
  $('#nightDot').hidden = !(v2 && v2.night && v2.night.length);
}
$('#refreshBtn').onclick = () => { if (S.sel) loadStore(S.sel, true); };

// ════════════════════════════════════════════════ 탭
$$('#tabs button').forEach((b) => (b.onclick = () => setTab(b.dataset.tab)));
function setTab(t) {
  S.tab = t;
  $$('#tabs button').forEach((b) => b.classList.toggle('on', b.dataset.tab === t));
  renderContent();
  $('#main').scrollTop = 0;
}

function renderAll() {
  renderHero();
  $$('#tabs button').forEach((b) => b.classList.toggle('on', b.dataset.tab === S.tab));
  renderContent();
}

function renderContent() {
  const c = $('#content');
  if (!S.accounts.length) { c.innerHTML = welcomeHTML(); bindWelcome(); return; }
  if (!selAcc()) { c.innerHTML = ''; return; }
  if (S.tab === 'wish') { renderWish(); return; }
  const a = selAcc();
  const v = S.stores[a.id];
  const err = S.storeErr[a.id];
  let top = '';
  if (a.status === 'expired' || err?.code === 'expired') {
    if (!v) { c.innerHTML = expiredHTML(a); bindExpired(a); return; }
    top = `<div class="err-box"><div><b>자동 로그인이 만료됐어요</b><small>아래는 마지막으로 확인한 상점이에요. 다시 로그인하면 최신 상점을 볼 수 있어요.</small></div><span class="sp"></span><button class="btn" data-relogin>다시 로그인</button></div>`;
  } else if (err) {
    top = `<div class="err-box"><div><b>상점을 불러오지 못했어요</b><small>${esc(err.msg)}</small></div><span class="sp"></span><button class="btn ghost" data-retry>다시 시도</button></div>`;
  }
  if (!v) {
    c.innerHTML = top + (err ? '' : `<div class="sec-head"><div><span class="tag">// LOADING</span><h2>상점을 불러오고 있어요…</h2></div></div><div class="grid4">${'<div class="skeleton"></div>'.repeat(4)}</div>`);
    bindTop(a);
    return;
  }
  let html = top;
  if (S.tab === 'daily') html += dailyHTML(v);
  else if (S.tab === 'night') html += nightHTML(v, a);
  else if (S.tab === 'bundle') html += bundleHTML(v);
  c.innerHTML = html;
  bindTop(a);
  bindCards(c, v);
  if (S.tab === 'night') bindNight(v, a);
  if (S.tab === 'bundle') bindBundle(v);
  tickTimers();
}

function bindTop(a) {
  const rl = $('[data-relogin]');
  if (rl) rl.onclick = () => openLogin(a.id);
  const rt = $('[data-retry]');
  if (rt) rt.onclick = () => loadStore(a.id, true);
}

// ════════════════════════════════════════════════ 카드
function itemFromCatalog(s) {
  return { kind: 'skin', id: s.uuid, skinId: s.uuid, name: s.name, weapon: s.weapon, category: s.category, icon: s.icon, tier: S.catalog?.tiers?.[s.tier], video: s.video };
}

function cardHTML(it, { size = '', night = false, showPrice = true } = {}) {
  const tc = it.tier?.color || '#7d8b99';
  const wish = it.skinId && S.wish.has(it.skinId);
  const hot = wish && !it.owned && showPrice;
  const isSkin = it.kind === 'skin';
  const sub = isSkin ? `${esc(it.weapon || '')}${it.tier ? ' · ' + esc(it.tier.name) : ''}` : (it.amount > 1 ? `×${it.amount}` : '');
  const priceHTML = !showPrice ? '' : it.price === 0 && it.base > 0
    ? `<div class="price"><span class="was">${num(it.base)}</span>무료</div>`
    : `<div class="price">${it.base && it.base > it.price ? `<span class="was">${num(it.base)}</span>` : ''}<img src="${CUR.vp}" alt="">${num(it.price)}</div>`;
  return `<div class="card ${size} ${hot ? 'wish' : ''} ${isSkin ? '' : 'other'}" style="--tc:${tc}" data-item="${esc(it.id)}" ${isSkin ? `data-skin="${esc(it.skinId || it.id)}"` : ''} role="button" tabindex="0">
    ${it.tier ? `<div class="tier"><img src="${esc(it.tier.icon)}" alt=""><span>${esc(it.tier.key || '')}</span></div>` : ''}
    <div class="badges">${it.owned ? '<span class="chip own">보유 중</span>' : ''}${it.video && size !== 'mini' ? `<span class="chip vid">${ART.play}영상</span>` : ''}</div>
    ${hot ? '<div class="ribbon">WISHLIST</div>' : ''}
    ${isSkin && it.skinId ? `<button class="heart-btn ${wish ? 'on' : ''}" data-heart="${esc(it.skinId)}" title="${wish ? '위시리스트에서 빼기' : '위시리스트에 담기'}">${ART.heart}</button>` : ''}
    ${night && it.discount ? `<div class="discount">-${it.discount}%</div>` : ''}
    <div class="gun">${it.icon ? `<img src="${esc(it.icon)}" alt="" loading="lazy" onerror="this.style.display='none'">` : ''}</div>
    <div class="shade"></div>
    <div class="info"><div class="nm"><b>${esc(it.name)}</b><small>${sub}</small></div>${priceHTML}</div>
  </div>`;
}

function bindCards(root, v) {
  $$('[data-heart]', root).forEach((b) => (b.onclick = (e) => { e.stopPropagation(); toggleWish(b.dataset.heart, b); }));
  $$('.card[data-skin]', root).forEach((c) => {
    const open = () => openSkin(c.dataset.skin, findItem(v, c.dataset.item));
    c.onclick = open;
    c.onkeydown = (e) => { if (e.key === 'Enter') open(); };
  });
}

function findItem(v, id) {
  if (!v) return null;
  const all = [...(v.daily || []), ...(v.night || []), ...(v.bundles || []).flatMap((b) => b.items)];
  return all.find((x) => x.id === id) || null;
}

async function toggleWish(skin, btn) {
  const on = !S.wish.has(skin);
  if (on) S.wish.add(skin); else S.wish.delete(skin);
  if (btn) { btn.classList.toggle('on', on); btn.classList.add('pop'); }
  toast(on ? '💗 위시리스트에 담았어요. 상점에 뜨면 알려 드릴게요!' : '위시리스트에서 뺐어요');
  try { await api('/api/wishlist', { skin, on }); } catch (e) { toast('⚠️ ' + e.message); }
  setTimeout(() => { renderContent(); renderSideCards(); }, 260);
  poll();
}

// ════════════════════════════════════════════════ 오늘의 상점
function dailyHTML(v) {
  const hits = v.daily.filter((it) => it.skinId && S.wish.has(it.skinId) && !it.owned);
  return `
    <div class="sec-head"><div><span class="tag">// DAILY OFFERS · 오늘의 상점</span>
      <p style="margin-top:6px">${hits.length ? `위시리스트 스킨 <b style="color:var(--red)">${hits.map((h) => esc(h.name)).join(', ')}</b>이(가) 떴어요! 🎯` : '스킨을 누르면 영상과 색상을 크게 볼 수 있어요.'}</p></div>
      <span class="sp"></span></div>
    <div class="grid4">${v.daily.map((it) => cardHTML(it)).join('')}</div>`;
}

// ════════════════════════════════════════════════ 야시장
function flipKey(a, v) { return `valoroad-flip-${a.id}-${Math.round((v.nightEndsAt || 0) / 3600000)}`; }
function flippedSet(a, v) {
  try { return new Set(JSON.parse(localStorage.getItem(flipKey(a, v)) || '[]')); } catch { return new Set(); }
}
function saveFlipped(a, v, set) { try { localStorage.setItem(flipKey(a, v), JSON.stringify([...set])); } catch { /* ignore */ } }

function nightHTML(v, a) {
  if (!v.night || !v.night.length) {
    return `<div class="empty"><div class="art">${ART.moon}</div><h2>지금은 야시장이 열려 있지 않아요</h2>
      <p>야시장은 보통 몇 달에 한 번, 2주 정도 열려요. 열리면 이 탭에 할인 스킨 6개가 나타나고 왼쪽 탭에 표시가 생겨요.</p></div>`;
  }
  const f = flippedSet(a, v);
  const closed = v.night.filter((it) => !it.seen && !f.has(it.id)).length;
  const saved = v.night.reduce((s, it) => s + Math.max(0, (it.base || 0) - (it.price || 0)), 0);
  return `
    <div class="sec-head"><div><span class="tag" style="color:#b98aff">// NIGHT MARKET</span><h2>야시장 🌙</h2>
      <p>${closed ? '카드를 눌러 하나씩 뒤집어 보세요!' : `6개를 모두 사면 <b>${num(saved)} VP</b>를 아낄 수 있어요.`} · 끝나기까지 <b data-ends="${v.nightEndsAt}" data-long="1"></b></p></div>
      <span class="sp"></span>${closed ? '<button class="btn ghost" id="flipAll">모두 뒤집기</button>' : ''}</div>
    <div class="grid6">${v.night.map((it) => {
      const open = it.seen || f.has(it.id);
      return `<div class="flip ${open ? 'open' : ''}" data-flip="${esc(it.id)}"><div class="flip-in">
        <div class="flip-face back">${ART.cardBack}<b>NIGHT MARKET</b><small>눌러서 뒤집기</small></div>
        <div class="flip-face front">${cardHTML(it, { size: 'compact', night: true })}</div>
      </div></div>`;
    }).join('')}</div>`;
}

function bindNight(v, a) {
  const f = flippedSet(a, v);
  $$('.flip').forEach((el) => {
    el.addEventListener('click', (e) => {
      if (el.classList.contains('open')) return;
      e.stopPropagation(); e.preventDefault();
      el.classList.add('open');
      f.add(el.dataset.flip);
      saveFlipped(a, v, f);
      if (!$$('.flip:not(.open)').length) setTimeout(() => { if (S.tab === 'night') renderContent(); }, 900);
    }, true);
  });
  const all = $('#flipAll');
  if (all) all.onclick = () => {
    const closed = $$('.flip:not(.open)');
    closed.forEach((el, i) => setTimeout(() => { el.classList.add('open'); f.add(el.dataset.flip); saveFlipped(a, v, f); }, i * 140));
    all.remove();
    setTimeout(() => { if (S.tab === 'night') renderContent(); }, closed.length * 140 + 900);
  };
}

// ════════════════════════════════════════════════ 번들
let bundleIdx = 0;
function bundleHTML(v) {
  if (!v.bundles || !v.bundles.length) {
    return `<div class="empty"><div class="art">${ART.welcome}</div><h2>지금 판매 중인 번들이 없어요</h2></div>`;
  }
  if (bundleIdx >= v.bundles.length) bundleIdx = 0;
  const b = v.bundles[bundleIdx];
  const tabs = v.bundles.length > 1 ? `<div class="bundle-tabs">${v.bundles.map((x, i) => `<button data-bi="${i}" class="${i === bundleIdx ? 'on' : ''}">${esc(x.name)}</button>`).join('')}</div>` : '';
  return `${tabs}
    <div class="bundle-hero" style="background-image:url('${esc(b.image)}')">
      <div class="bh-in"><span class="tag">// FEATURED BUNDLE</span><h2>${esc(b.name)}</h2>${b.sub ? `<p>${esc(b.sub)}</p>` : '<p>번들로 사면 할인된 가격이에요.</p>'}
        <div class="bh-price">${b.discount ? `<span class="disc-tag">-${b.discount}%</span>` : ''}<div class="price">${b.base > b.price ? `<span class="was">${num(b.base)}</span>` : ''}<img src="${CUR.vp}" alt="">${num(b.price)}</div></div>
        <div class="left"><i class="ic-clock"></i>판매 종료까지 <b data-ends="${b.endsAt}" data-long="1"></b></div>
      </div>
    </div>
    <div class="sec-head" style="margin-top:20px"><div><span class="tag">// BUNDLE ITEMS</span><h2>구성 (${b.items.length}개)</h2><p>스킨은 따로 사도 돼요. 가격은 따로 살 때 기준이에요.</p></div></div>
    <div class="bundle-items">${b.items.map((it) => cardHTML(it, { size: 'mini' })).join('')}</div>`;
}
function bindBundle() {
  $$('[data-bi]').forEach((b) => (b.onclick = () => { bundleIdx = +b.dataset.bi; renderContent(); }));
}

// ════════════════════════════════════════════════ 위시리스트
const CATS = ['전체', '소총', '권총', '기관단총', '산탄총', '저격총', '기관총', '근접 무기'];
async function ensureCatalog() {
  if (S.catalog || S.catalogLoading) return;
  S.catalogLoading = true;
  try { S.catalog = await api('/api/catalog'); } catch (e) { toast('⚠️ ' + e.message); }
  S.catalogLoading = false;
  if (S.tab === 'wish') renderWish();
}

function renderWish() {
  const c = $('#content');
  if (!S.catalog) {
    c.innerHTML = `<div class="sec-head"><div><span class="tag">// WISHLIST</span><h2>스킨 목록을 불러오고 있어요…</h2></div></div><div class="wish-grid">${'<div class="skeleton" style="aspect-ratio:5/4"></div>'.repeat(10)}</div>`;
    ensureCatalog();
    return;
  }
  const hits = S.poll?.hits || [];
  const q = W.q.trim().toLowerCase();
  let list = S.catalog.skins.filter((s) => (W.cat === '전체' || s.category === W.cat) && (!W.only || S.wish.has(s.uuid)) &&
    (!q || s.name.toLowerCase().includes(q) || s.weapon.toLowerCase().includes(q)));
  const rank = (s) => S.catalog.tiers[s.tier]?.rank ?? -1;
  list.sort((a, b) => (S.wish.has(b.uuid) - S.wish.has(a.uuid)) || (rank(b) - rank(a)) || a.name.localeCompare(b.name, 'ko'));
  const shown = list.slice(0, W.limit);
  const hitHTML = hits.length ? `<div class="hits-bar">${ART.heart}<div><b>지금 상점에 위시리스트 스킨이 있어요!</b><br><span class="note">${hits.map((h) => {
    const acc = S.accounts.find((a) => a.id === h.account);
    return `${esc(h.name)} — ${esc(acc ? acc.name : '')} ${esc(h.where)}`;
  }).join(' · ')}</span></div></div>` : '';
  const empty = !S.wish.size && W.only
    ? `<div class="empty"><div class="art" style="width:200px">${ART.wish}</div><h2>아직 담은 스킨이 없어요</h2><p>‘위시리스트만’을 끄고 갖고 싶은 스킨의 ♥를 눌러 보세요. 그 스킨이 내 계정 상점이나 야시장에 뜨면 알려 드려요.</p></div>`
    : (!list.length ? `<div class="empty" style="min-height:240px"><h2>찾는 스킨이 없어요</h2><p>다른 이름으로 검색해 보세요. (예: 프라임, 리버, 밴달)</p></div>` : '');
  c.innerHTML = `
    <div class="sec-head"><div><span class="tag">// WISHLIST</span><h2>위시리스트</h2>
      <p>♥를 누른 스킨이 계정 3개 중 어느 상점 · 야시장 · 번들에 떠도 알려 드려요. ${S.settings?.dailyCheck ? `(매일 ${esc(S.settings.dailyTime)} 자동 확인)` : ''}</p></div></div>
    ${hitHTML}
    <div class="wish-tools">
      <label class="search">${ART.search}<input id="wishQ" placeholder="스킨 이름으로 찾기 (예: 프라임 밴달)" value="${esc(W.q)}" autocomplete="off"></label>
      <button class="pill red ${W.only ? 'on' : ''}" id="wishOnly">♥ 위시리스트만 (${S.wish.size})</button>
      <span class="wish-count">${num(list.length)}개</span>
    </div>
    <div class="pills" style="margin:-4px 0 14px">${CATS.map((k) => `<button class="pill ${W.cat === k ? 'on' : ''}" data-cat="${k}">${k}</button>`).join('')}</div>
    ${empty || `<div class="wish-grid">${shown.map((s) => cardHTML(itemFromCatalog(s), { size: 'mini', showPrice: false })).join('')}</div>`}
    ${list.length > W.limit ? `<div class="more-row"><button class="btn ghost" id="wishMore">더 보기 (${num(list.length - W.limit)}개 남음)</button></div>` : ''}`;
  const qi = $('#wishQ');
  qi.oninput = () => {
    W.q = qi.value; W.limit = 60;
    clearTimeout(renderWish.t);
    renderWish.t = setTimeout(() => { const pos = qi.selectionStart; renderWish(); const n = $('#wishQ'); n.focus(); n.setSelectionRange(pos, pos); }, 160);
  };
  $('#wishOnly').onclick = () => { W.only = !W.only; W.limit = 60; renderWish(); };
  $$('[data-cat]').forEach((b) => (b.onclick = () => { W.cat = b.dataset.cat; W.limit = 60; renderWish(); }));
  const more = $('#wishMore');
  if (more) {
    more.onclick = () => { W.limit += 60; const y = $('#main').scrollTop; renderWish(); $('#main').scrollTop = y; };
    new IntersectionObserver((ents, ob) => { if (ents[0].isIntersecting) { ob.disconnect(); more.click(); } }, { root: $('#main'), rootMargin: '300px' }).observe(more);
  }
  bindCards(c, null);
}

// ════════════════════════════════════════════════ 스킨 자세히 보기
function videoHTML(src, still) {
  return `<video src="${esc(src)}" poster="${esc(still)}" data-still="${esc(still)}" autoplay loop playsinline controls></video>`;
}
async function openSkin(id, item) {
  let d;
  try { d = await api(`/api/skin?id=${encodeURIComponent(id)}`); } catch (e) { toast('⚠️ ' + e.message); return; }
  const s = d.skin, tier = d.tier;
  const tc = tier?.color || '#7d8b99';
  const body = document.createElement('div');
  const sel = { level: 0, chroma: 0, mode: 'level' };
  const hasVid = (x) => !!x.video;
  // 처음에는 영상이 있는 가장 높은 레벨을 보여 줘요
  const lastVid = s.levels.map(hasVid).lastIndexOf(true);
  if (lastVid > 0) sel.level = lastVid;
  const a = selAcc();
  const where = (() => {
    const v = a && S.stores[a.id];
    if (!v) return '';
    if (v.daily.some((x) => x.skinId === s.uuid)) return '오늘의 상점';
    if ((v.night || []).some((x) => x.skinId === s.uuid)) return '야시장';
    if ((v.bundles || []).some((b) => b.items.some((x) => x.skinId === s.uuid))) return '번들';
    return '';
  })();
  const draw = () => {
    let media = '';
    const lv = s.levels[sel.level], ch = s.chromas[sel.chroma];
    if (sel.mode === 'chroma' && ch) {
      const still = ch.render || ch.icon || s.icon;
      media = ch.video ? videoHTML(ch.video, still) : `<img src="${esc(still)}" alt="">`;
    } else if (lv) {
      media = lv.video ? videoHTML(lv.video, s.icon || lv.icon) : `<img src="${esc(lv.icon || s.icon)}" alt="">`;
    } else media = `<img src="${esc(s.icon)}" alt="">`;
    const wish = S.wish.has(s.uuid);
    body.innerHTML = `<div class="detail">
      <div>
        <div class="stage" style="--tc:${tc}">${media}<i class="hud-corner tl"></i><i class="hud-corner br"></i></div>
        ${s.levels.length > 1 ? `<div class="picker"><span class="tag lbl">// LEVELS · 업그레이드</span><div class="lv-row">${s.levels.map((l, i) =>
          `<button class="lv ${sel.mode === 'level' && sel.level === i ? 'on' : ''}" data-lv="${i}"><b>LV ${i + 1}</b><small>${esc(l.kind || (i === 0 ? '기본' : '업그레이드'))}</small>${l.video ? `<span class="pv">${ART.play}영상</span>` : ''}</button>`).join('')}</div></div>` : ''}
        ${s.chromas.length > 1 ? `<div class="picker"><span class="tag lbl">// VARIANTS · 색상</span><div class="sw-row">${s.chromas.map((c, i) =>
          `<button class="sw ${sel.mode === 'chroma' && sel.chroma === i ? 'on' : ''}" data-ch="${i}" title="${esc(c.name)}">${c.swatch ? `<img src="${esc(c.swatch)}" alt="">` : `<span class="noimg">${i === 0 ? '기본' : i}</span>`}</button>`).join('')}</div></div>` : ''}
      </div>
      <div class="d-info">
        <span class="tag">// ${esc(s.category)}</span>
        <h2>${esc(s.name)}</h2>
        <div class="sub">${esc(s.weapon)}</div>
        ${tier ? `<div class="d-tier" style="--tc:${tc};border-color:${tc}"><img src="${esc(tier.icon)}" alt=""><b>${esc(tier.name)} 에디션</b></div>` : ''}
        <div class="d-kv">
          ${item && item.price ? `<div><span>가격</span><b><span class="price" style="font-size:16px;color:var(--text)">${item.base > item.price ? `<span class="was" style="color:var(--muted)">${num(item.base)}</span>` : ''}<img class="cur" src="${CUR.vp}" alt="">${num(item.price)}</span></b></div>` : ''}
          ${where ? `<div><span>지금</span><b style="color:var(--red)">${esc(accName(a))}의 ${where}에 있어요</b></div>` : ''}
          ${item ? `<div><span>내 계정</span><b>${item.owned ? '✅ 이미 보유 중' : '아직 없어요'}</b></div>` : ''}
          <div><span>업그레이드</span><b>${s.levels.length}단계</b></div>
          <div><span>색상</span><b>${s.chromas.length}가지</b></div>
        </div>
        <div class="d-actions">
          <button class="btn ${wish ? 'on' : ''}" id="dWish">${ART.heart}${wish ? '위시리스트에 담겨 있어요' : '위시리스트에 담기'}</button>
          <small class="note">${wish ? '이 스킨이 내 계정 상점에 뜨면 알림을 보내 드려요.' : '담아 두면 상점 · 야시장 · 번들에 뜰 때 알려 드려요.'}</small>
        </div>
      </div>
    </div>`;
    $$('video', body).forEach((v) => v.addEventListener('error', () => {
      // 영상을 못 틀면 사진으로 바꿔요
      const img = document.createElement('img');
      img.src = v.dataset.still; v.replaceWith(img);
    }));
    $$('[data-lv]', body).forEach((b) => (b.onclick = () => { sel.mode = 'level'; sel.level = +b.dataset.lv; draw(); }));
    $$('[data-ch]', body).forEach((b) => (b.onclick = () => { sel.mode = 'chroma'; sel.chroma = +b.dataset.ch; draw(); }));
    $('#dWish', body).onclick = async () => { await toggleWish(s.uuid, null); draw(); };
  };
  draw();
  openModal({ title: '스킨 자세히 보기', body, wide: true, onClose: () => { $$('video', body).forEach((v) => v.pause()); } });
}

// ════════════════════════════════════════════════ 첫 화면 · 만료 화면
function welcomeHTML() {
  return `<div class="empty">
    <div class="art">${ART.welcome}</div>
    <span class="tag">// WELCOME, AGENT</span>
    <h2>게임을 켜지 않고 오늘의 상점을 확인해요</h2>
    <p>Riot 계정으로 한 번만 로그인하면 다음부터는 자동으로 로그인돼요. 계정은 3개까지 저장할 수 있어요.</p>
    <button class="btn big" id="welcomeAdd">+ 첫 계정 추가하기</button>
    <div class="feats">
      <div class="feat"><i>01</i><b>계정 3개 한 번에</b><small>왼쪽에서 계정을 누르기만 하면 그 계정의 상점이 바로 보여요.</small></div>
      <div class="feat"><i>02</i><b>야시장 · 번들까지</b><small>할인율, 남은 시간, 잔액, 이미 가진 스킨도 함께 보여 줘요.</small></div>
      <div class="feat"><i>03</i><b>위시리스트 알림</b><small>갖고 싶은 스킨이 상점에 뜨면 매일 오전 9시에 알려 드려요.</small></div>
    </div>
  </div>`;
}
function bindWelcome() { const b = $('#welcomeAdd'); if (b) b.onclick = () => openLogin(''); }

function expiredHTML(a) {
  return `<div class="empty"><div class="art" style="width:220px">${ART.expired}</div>
    <h2>${esc(accName(a))} 계정은 다시 로그인이 필요해요</h2>
    <p>Riot 쪽에서 로그인 상태가 끝났어요. (오래 쓰지 않았거나 비밀번호를 바꾸면 이렇게 돼요) 한 번만 다시 로그인하면 또 자동으로 유지돼요.</p>
    <button class="btn big" data-relogin>다시 로그인</button></div>`;
}
function bindExpired(a) { const b = $('[data-relogin]'); if (b) b.onclick = () => openLogin(a.id); }

// ════════════════════════════════════════════════ 로그인
const L = { open: false, startedFor: null, sig: '' };
async function openLogin(relogin) {
  if (!relogin && S.accounts.length >= (S.settings?.maxAccounts || 3)) {
    toast('계정은 3개까지 저장할 수 있어요. 하나를 지운 뒤 추가해 주세요.');
    return;
  }
  L.open = true; L.relogin = relogin; L.sig = '';
  const body = document.createElement('div');
  body.id = 'loginBody';
  openModal({ title: relogin ? '다시 로그인' : '계정 추가', body, onClose: onLoginClose });
  renderLogin({ stage: 'opening' });
  try {
    const st = await api('/api/accounts/login', { relogin });
    renderLogin(st);
  } catch (e) { renderLogin({ stage: 'error', error: e.message }); }
}
function onLoginClose() {
  L.open = false;
  const st = S.poll?.login;
  if (st && ['opening', 'waiting', 'login', 'finishing'].includes(st.stage)) api('/api/accounts/login/cancel', {}).catch(() => {});
  else api('/api/accounts/login/dismiss', {}).catch(() => {});
}

function renderLogin(st) {
  const body = $('#loginBody');
  if (!body || !st) return;
  const sig = JSON.stringify([st.stage, st.error, st.code]);
  if (sig === L.sig) return;
  L.sig = sig;
  const bname = st.browser || S.poll?.browser?.name || '크롬';
  if (st.code === 'nobrowser') {
    body.innerHTML = `<div class="login-art">${ART.login}</div>
      <p class="note" style="font-size:14px">Riot 로그인은 <b>구글 크롬</b> 창에서 해요. (엣지 · 브레이브 · 웨일도 돼요)<br>이 맥에서 크롬을 찾지 못했어요. 크롬을 설치한 뒤 다시 눌러 주세요.</p>`;
    setModalActions([{ label: '닫기', onClick: closeModal }, { label: '크롬 받으러 가기', primary: true, onClick: () => api('/api/open-url', { url: 'https://www.google.com/chrome/' }) }]);
    return;
  }
  let status = '';
  switch (st.stage) {
    case 'opening': status = `<div class="login-status"><span class="radar"></span><div><b>로그인 창을 여는 중…</b><small>${esc(bname)} 창이 잠시 뒤 열려요.</small></div></div>`; break;
    case 'waiting': case 'login':
      status = `<div class="login-status"><span class="radar"></span><div><b>${esc(bname)} 창에서 Riot 계정으로 로그인해 주세요</b><small>로그인이 끝나면 창이 저절로 닫히고 여기로 돌아와요. 창이 안 보이면 Dock에서 ${esc(bname)} 아이콘을 눌러 보세요.</small></div></div>`; break;
    case 'finishing': status = `<div class="login-status"><span class="radar"></span><div><b>로그인 완료! 계정 정보를 가져오는 중…</b><small>거의 다 됐어요.</small></div></div>`; break;
    case 'error': status = `<div class="login-status err"><div><b>로그인하지 못했어요</b><small>${esc(st.error || '')}</small></div></div>`; break;
    case 'cancelled': status = `<div class="login-status err"><div><b>로그인 창이 닫혔어요</b><small>다시 시도하려면 아래 버튼을 눌러 주세요.</small></div></div>`; break;
  }
  body.innerHTML = `<div class="login-art">${ART.login}</div>
    <div class="steps">
      <div class="step"><i>1</i><div><b>${esc(bname)} 창에서 Riot 로그인</b><small>평소처럼 Riot ID · 비밀번호(또는 구글 · 애플 로그인)로 로그인해요. 비밀번호는 ValoROAD가 볼 수 없어요.</small></div></div>
      <div class="step"><i>2</i><div><b>‘로그인 상태 유지’를 꼭 체크</b><small>체크해야 다음부터 다시 로그인하지 않아도 자동으로 로그인돼요.</small></div></div>
      <div class="step"><i>3</i><div><b>끝! 창은 저절로 닫혀요</b><small>2단계 인증 코드를 물어보면 이메일로 받은 코드를 넣어 주세요.</small></div></div>
    </div>
    ${status}`;
  const busy = ['opening', 'waiting', 'login', 'finishing'].includes(st.stage);
  setModalActions(busy
    ? [{ label: '취소', onClick: closeModal }]
    : [{ label: '닫기', onClick: closeModal }, { label: '다시 시도', primary: true, onClick: () => { closeModal(); setTimeout(() => openLogin(L.relogin || ''), 50); } }]);
}

function onLoginState(st) {
  if (!L.open) return;
  if (!st) return;
  if (st.stage === 'done') {
    L.open = false;
    closeModal();
    api('/api/accounts/login/dismiss', {}).catch(() => {});
    toast(L.relogin ? '✅ 다시 로그인했어요' : '✅ 계정을 추가했어요! 상점을 불러올게요');
    if (st.accountId) {
      delete S.storeErr[st.accountId];
      S.sel = st.accountId;
      S.tab = 'daily';
      setTimeout(() => selectAccount(st.accountId), 50);
    }
    return;
  }
  renderLogin(st);
}

// ════════════════════════════════════════════════ 설정
function sw(id, on) { return `<label class="switch"><input type="checkbox" id="${id}" ${on ? 'checked' : ''}><span></span></label>`; }
function openSettings() {
  const body = document.createElement('div');
  const render = () => {
    const st = S.settings;
    body.innerHTML = `
      <div class="set-group"><span class="tag">// DISPLAY</span>
        <div class="form-row"><div class="t"><b>화면 모드</b><small>기본은 맥의 설정(라이트/다크)을 따라가요.</small></div>
          <div class="seg" id="setTheme">${['system', 'light', 'dark'].map((v) => `<button data-v="${v}" class="${st.appearance === v ? 'on' : ''}">${{ system: '시스템', light: '라이트', dark: '다크' }[v]}</button>`).join('')}</div></div>
      </div>
      <div class="set-group"><span class="tag">// ALERTS</span>
        <div class="form-row"><div class="t"><b>위시리스트 알림</b><small>위시리스트 스킨이 상점 · 야시장 · 번들에 뜨면 맥 알림으로 알려 드려요.</small></div><button class="mini-btn" id="testNotify">알림 테스트</button>${sw('setNotify', st.notify)}</div>
        <div class="form-row"><div class="t"><b>매일 ${esc(st.dailyTime)} 자동 확인</b><small>상점이 바뀌는 시각에 앱이 꺼져 있어도 저장된 계정들을 조용히 확인해요. 맥이 잠자기 중이면 깨어날 때 확인해요.</small></div>${sw('setDaily', st.dailyCheck)}</div>
      </div>
      <div class="set-group"><span class="tag">// UPDATE</span>
        <div class="form-row"><div class="t"><b>자동 업데이트</b><small>새 버전이 나오면 앱이 알아서 받아요. 지금 버전: <b>v${esc(st.build)}</b></small></div><button class="mini-btn" id="setUpd">업데이트 확인</button>${sw('setAuto', st.autoUpdate)}</div>
      </div>
      <div class="set-group"><span class="tag">// NOTICE</span>
        <ul class="warn-list">
          <li>ValoROAD는 Riot Games가 만든 앱이 아니에요. 게임이 쓰는 비공식 방식으로 상점을 읽어 와요.</li>
          <li>비밀번호는 저장하지 않아요. Riot이 준 로그인 정보만 맥의 키체인에 안전하게 보관해요.</li>
          <li>상점을 <b>보기만</b> 하고, 구매 · 설정 변경은 하지 않아요. 그래도 비공식 앱이라 Riot 정책이 바뀌면 동작이 멈출 수 있어요.</li>
          <li>Riot 계정에 2단계 인증을 켜 두는 걸 권장해요. 개인적인 용도로만 사용해 주세요.</li>
        </ul>
      </div>
      <div class="form-row" style="border:0"><div class="t"><small>ValoROAD v${esc(st.build)} · made by. Nevertheless_D</small></div><button class="mini-btn danger" id="setQuit">ValoROAD 종료</button></div>`;
    $$('#setTheme button', body).forEach((b) => (b.onclick = async () => { applyTheme(b.dataset.v); S.settings = await api('/api/settings', { appearance: b.dataset.v }); render(); }));
    $('#setNotify', body).onchange = async (e) => { S.settings = await api('/api/settings', { notify: e.target.checked }); toast(e.target.checked ? '🔔 위시리스트 알림을 켰어요' : '위시리스트 알림을 껐어요'); };
    $('#setDaily', body).onchange = async (e) => {
      S.settings = await api('/api/settings', { dailyCheck: e.target.checked });
      if (S.settings.agentError) toast('⚠️ ' + S.settings.agentError);
      else toast(e.target.checked ? `⏰ 매일 ${S.settings.dailyTime}에 자동으로 확인할게요` : '매일 자동 확인을 껐어요');
      renderSideCards();
    };
    $('#setAuto', body).onchange = async (e) => { S.settings = await api('/api/settings', { autoUpdate: e.target.checked }); toast(e.target.checked ? '✅ 자동 업데이트를 켰어요' : '자동 업데이트를 껐어요'); };
    $('#testNotify', body).onclick = async () => { await api('/api/notify/test', {}).catch(() => {}); toast('🔔 알림을 보냈어요. 화면 오른쪽 위를 확인해 보세요'); };
    $('#setUpd', body).onclick = () => { closeModal(); openUpdate(true); };
    $('#setQuit', body).onclick = quitApp;
  };
  render();
  openModal({ title: 'ValoROAD 설정', body });
}
$('#settingsBtn').onclick = openSettings;

// ════════════════════════════════════════════════ 버전 · 업데이트
const U = { open: false, sig: '', restarting: false, toldReady: '' };
const updBody = document.createElement('div');

function onUpdateState(st) {
  if (!st) return;
  S.update = st;
  renderVersion();
  const sig = JSON.stringify(st);
  if (U.open && sig !== U.sig) renderUpdate();
  U.sig = sig;
  if (st.status === 'ready' && st.installed && U.toldReady !== st.installed && !U.open) {
    U.toldReady = st.installed;
    toast(`✨ ValoROAD ${st.installed} 준비 완료! 오른쪽 위 버전 표시를 눌러 다시 시작하세요.`, 6000);
  }
}

function renderVersion() {
  const st = S.update;
  if (!st) return;
  let txt = 'v' + st.current, isNew = false;
  if (st.status === 'available') { txt = `새 버전 ${st.latest.version}`; isNew = true; }
  else if (st.status === 'downloading') { txt = `받는 중 ${Math.round(st.progress * 100)}%`; isNew = true; }
  else if (st.status === 'installing') { txt = '설치 중…'; isNew = true; }
  else if (st.status === 'ready') { txt = '다시 시작해서 업데이트'; isNew = true; }
  $('#verText').textContent = txt;
  $('#verBadge').classList.toggle('new', isNew);
}

function renderUpdate() {
  const st = S.update;
  if (!st) return;
  const Lr = st.latest;
  let t1 = '', t2 = '', showVers = false, acts = [];
  switch (st.status) {
    case 'checking': t1 = '새 버전을 확인하고 있어요…'; t2 = '잠시만 기다려 주세요.'; break;
    case 'available':
      t1 = '새 버전이 나왔어요! 🎉'; t2 = '받는 데 몇 초면 끝나요.'; showVers = true;
      acts = [{ label: '지금 업데이트', primary: true, onClick: installUpdate }]; break;
    case 'downloading': t1 = '새 버전을 받고 있어요…'; t2 = `${Math.round(st.progress * 100)}%`; showVers = true; break;
    case 'installing': t1 = '설치하고 있어요…'; t2 = '잠시만 기다려 주세요.'; showVers = true; break;
    case 'ready':
      t1 = `ValoROAD ${st.installed} 준비 완료 ✨`; t2 = '다시 시작하면 새 버전으로 바뀌어요. (몇 초 걸려요)';
      acts = [{ label: '지금 다시 시작', primary: true, onClick: restartApp }]; break;
    case 'uptodate': t1 = '최신 버전을 쓰고 있어요 👍'; t2 = `ValoROAD ${st.current}`; break;
    case 'error': t1 = '업데이트를 확인하지 못했어요'; t2 = '인터넷 연결을 확인한 뒤 다시 시도해 주세요.'; break;
    default: t1 = `ValoROAD ${st.current}`; t2 = '버튼을 눌러 새 버전이 있는지 확인해 보세요.';
  }
  const busy = ['checking', 'downloading', 'installing'].includes(st.status);
  if (!['available', 'ready'].includes(st.status)) acts = [{ label: '업데이트 확인', primary: true, disabled: busy, onClick: checkUpdate }];
  const notes = ['available', 'downloading', 'installing', 'ready'].includes(st.status) && Lr?.notes?.length
    ? `<div class="card-box"><span class="tag">// ${esc(Lr.version)}에서 바뀐 점</span><ul class="upd-notes">${Lr.notes.map((n) => `<li>${esc(n)}</li>`).join('')}</ul></div>` : '';
  updBody.innerHTML = `
    <div class="upd-hero"><img src="/icon.png" alt=""><div><div class="t1">${t1}</div><div class="t2">${esc(t2)}</div></div></div>
    ${showVers && Lr ? `<div class="upd-vers"><div class="upd-ver"><small>지금 버전</small><b>${esc(st.current)}</b></div><span class="upd-arrow">➜</span><div class="upd-ver new"><small>새 버전</small><b>${esc(Lr.version)}</b></div></div>` : ''}
    ${busy && st.status !== 'checking' ? `<div class="upd-bar"><i style="width:${st.status === 'installing' ? 100 : Math.round(st.progress * 100)}%"></i></div>` : ''}
    ${st.status === 'error' && st.error ? `<div class="upd-err">${esc(st.error)}</div>` : ''}
    ${notes}
    <div class="card-box">
      <div class="form-row" style="padding:6px 0"><div class="t"><b>자동 업데이트</b><small>새 버전이 나오면 앱이 알아서 받아 둬요.</small></div>${sw('updAuto', st.autoUpdate)}</div>
      <div class="kv"><span>지금 버전</span><b>ValoROAD ${esc(st.current)}</b></div>
      <div class="kv"><span>마지막 확인</span><b>${fmtWhen(st.lastCheck)}</b></div>
      ${st.canRollback ? `<div class="kv"><span>문제가 생겼다면 이전 버전(${esc(st.backupVersion)})으로</span><button class="mini-btn" id="updRollback">되돌리기</button></div>` : ''}
    </div>`;
  $('#updAuto', updBody).onchange = async (e) => {
    S.settings = await api('/api/settings', { autoUpdate: e.target.checked });
    S.update.autoUpdate = e.target.checked;
    toast(e.target.checked ? '✅ 자동 업데이트를 켰어요' : '자동 업데이트를 껐어요. 필요할 때 직접 확인하세요.');
  };
  const rb = $('#updRollback', updBody);
  if (rb) rb.onclick = rollbackUpdate;
  setModalActions(acts);
}

function openUpdate(check = false) {
  U.open = true;
  openModal({ title: '버전 · 업데이트', body: updBody, onClose: () => { U.open = false; } });
  renderUpdate();
  if (check && !['available', 'ready', 'downloading', 'installing'].includes(S.update?.status)) checkUpdate();
}
$('#verBadge').onclick = () => openUpdate(true);

async function checkUpdate() {
  if (S.update) { S.update.status = 'checking'; renderUpdate(); renderVersion(); }
  try { onUpdateState(await api('/api/update/check')); } catch (e) { toast('⚠️ ' + e.message); }
}
async function installUpdate() {
  try { await api('/api/update/install', {}); } catch (e) { toast('⚠️ ' + e.message); }
  if (S.update) { S.update.status = 'downloading'; S.update.progress = 0; renderUpdate(); }
}
async function rollbackUpdate() {
  const ok = await confirmBox('이전 버전으로 되돌리기', `이전 버전(${esc(S.update.backupVersion)})으로 되돌릴까요?<br>되돌리면 자동 업데이트는 꺼져요. (설정에서 다시 켤 수 있어요)`, '되돌리기');
  if (!ok) { openUpdate(false); return; }
  try { await api('/api/update/rollback', {}); } catch (e) { toast('⚠️ ' + e.message); return; }
  toast('↩︎ 이전 버전을 준비했어요. 다시 시작하면 바뀌어요.');
  openUpdate(false);
  poll();
}

async function restartApp() {
  const from = S.update?.current;
  U.restarting = true;
  closeModal();
  const veil = document.createElement('div');
  veil.className = 'restart-veil';
  veil.innerHTML = '<img src="/icon.png" alt="">새 버전으로 다시 시작하고 있어요…<span>창을 닫지 말고 잠시만 기다려 주세요.</span>';
  document.body.append(veil);
  try { await api('/api/update/restart', {}); } catch (e) { veil.remove(); U.restarting = false; toast('⚠️ ' + e.message); return; }
  const t0 = Date.now();
  const tick = async () => {
    try {
      const j = await api('/api/ping');
      if (j.build && j.build !== from) { location.reload(); return; }
    } catch { /* 아직 다시 켜지는 중 */ }
    if (Date.now() - t0 > 60000) { showEnded('다시 시작에 시간이 걸리고 있어요. ValoROAD 앱을 다시 실행해 주세요.'); return; }
    setTimeout(tick, 600);
  };
  setTimeout(tick, 1200);
}

// ════════════════════════════════════════════════ 종료
let ended = false;
function showEnded(msg) {
  ended = true;
  document.body.innerHTML = `<div class="ended"><img src="/icon.png" alt=""><b>${msg}</b><span>이 창은 닫아도 돼요. 다시 쓰려면 ValoROAD 앱을 실행하세요.</span></div>`;
}
async function quitApp() {
  const ok = await confirmBox('ValoROAD 종료', 'ValoROAD를 종료할까요?<br>매일 자동 확인을 켜 두었다면 종료해도 위시리스트 알림은 계속 받아요.', '종료');
  if (!ok) return;
  await api('/api/quit', {}).catch(() => {});
  showEnded('ValoROAD를 종료했어요 👋');
  setTimeout(() => window.close(), 400);
}
$('#quitBtn').onclick = quitApp;

// ════════════════════════════════════════════════ 남은 시간 표시
function tickTimers() {
  const now = Date.now();
  const a = selAcc();
  const v = a && S.stores[a.id];
  const ends = v && v.dailyEndsAt > now ? v.dailyEndsAt : nextUtcMidnight();
  $('#resetTimer').textContent = fmtLeft(ends - now);
  let tt = '';
  if (a && S.tab === 'daily') tt = `교체까지 <b>${fmtLeft(ends - now)}</b>`;
  else if (a && S.tab === 'night' && v?.nightEndsAt) tt = `야시장 종료까지 <b>${fmtLeft(v.nightEndsAt - now, true)}</b>`;
  else if (a && S.tab === 'bundle' && v?.bundles?.[bundleIdx]) tt = `번들 종료까지 <b>${fmtLeft(v.bundles[bundleIdx].endsAt - now, true)}</b>`;
  $('#tabTimer').innerHTML = tt;
  $$('[data-ends]').forEach((el) => { el.textContent = fmtLeft(+el.dataset.ends - now, el.dataset.long === '1'); });
  if (a && !S.loading[a.id]) {
    const t = fmtAgo(v?.fetchedAt);
    if ($('#heroUpdated').textContent !== t) $('#heroUpdated').textContent = t;
  }
}
setInterval(tickTimers, 1000);

// ════════════════════════════════════════════════ 단축키
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') { if (menuEl) { closeMenu(); return; } if (M.open) { e.preventDefault(); closeModal(); } return; }
  if (M.open) return;
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'r') { e.preventDefault(); if (S.sel) loadStore(S.sel, true); return; }
  if (e.target.tagName === 'INPUT') return;
  const tabs = { 1: 'daily', 2: 'night', 3: 'bundle', 4: 'wish' };
  if (tabs[e.key] && S.sel) setTab(tabs[e.key]);
});

// ════════════════════════════════════════════════ 주기적 갱신
let fails = 0;
async function poll() {
  if (ended || U.restarting) return;
  let p;
  try { p = await api('/api/poll'); fails = 0; } catch { if (++fails >= 6) showEnded('ValoROAD 엔진이 종료되었어요'); return; }
  const prev = S.poll;
  S.poll = p;
  const accSig = JSON.stringify(p.accounts.map((a) => [a.id, a.name, a.status, a.busy, a.error, a.level, a.card, a.vp, a.lastRefresh])) + JSON.stringify(p.hits);
  const prevSig = prev ? JSON.stringify(prev.accounts.map((a) => [a.id, a.name, a.status, a.busy, a.error, a.level, a.card, a.vp, a.lastRefresh])) + JSON.stringify(prev.hits) : '';
  S.accounts = p.accounts;
  if (S.sel && !S.accounts.find((a) => a.id === S.sel)) S.sel = null;
  if (!S.sel && S.accounts.length) {
    const last = S.settings?.lastAccount;
    S.sel = (S.accounts.find((a) => a.id === last) || S.accounts[0]).id;
    renderAccounts();
    renderAll();
    loadStore(S.sel, false);
  }
  if (accSig !== prevSig || (prev?.login?.stage !== p.login?.stage)) {
    renderAccounts();
    // 다른 곳(매일 확인 · 자동 새로 고침)에서 상점이 바뀌었으면 다시 읽어요
    for (const a of S.accounts) {
      const v = S.stores[a.id];
      if (a.lastRefresh && v && a.lastRefresh > v.fetchedAt && !S.loading[a.id] && !a.busy && S.tried[a.id] !== a.lastRefresh) {
        S.tried[a.id] = a.lastRefresh;
        loadStore(a.id, false);
      }
    }
    if (!prev || prev.accounts.length !== p.accounts.length) renderAll();
    else renderHero();
  }
  if (p.wishCount !== S.wish.size) await loadWish();
  onLoginState(p.login);
  onUpdateState(p.update);
}

async function loadWish() {
  try {
    const j = await api('/api/wishlist');
    S.wish = new Set(j.items.map((x) => x.skin));
  } catch { /* ignore */ }
  renderSideCards();
}

// ════════════════════════════════════════════════ 시작
(async function init() {
  paintArt();
  if (!T) { showEnded('ValoROAD 앱에서 열어 주세요'); return; }
  try { S.settings = await api('/api/settings'); } catch { showEnded('앱 엔진에 연결하지 못했어요'); return; }
  applyTheme(S.settings.appearance);
  $('#verText').textContent = 'v' + S.settings.build;
  await loadWish();
  await poll();
  renderAccounts();
  renderAll();
  tickTimers();
  setInterval(poll, 1200);
})();
