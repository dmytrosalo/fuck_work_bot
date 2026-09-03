package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

// pokerTmpl renders the Mini App page served at GET /poker/{id}. It is
// deliberately state-free: the page is served unauthenticated (anyone with
// the URL can request it), so the only thing templated in is the table id —
// every bit of table state (seats, board, pot, hole cards, ...) reaches the
// client only after it authenticates via /api/poker/{id}/join with Telegram
// initData. {{.TableID}} sits inside a <script> block as a bare JS value
// (`const TABLE={{.TableID}};`, no surrounding quotes in the template
// source); html/template's contextual autoescaper recognizes that position
// as a JS value context and emits a fully quoted, escaped JS string literal
// for it, which is the reason this must be html/template and not
// text/template.
var pokerTmpl = template.Must(template.New("poker").Parse(`<!doctype html>
<html lang="uk"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>Покер</title>
<script src="https://telegram.org/js/telegram-web-app.js"></script>
<style>
:root{color-scheme:dark}
*{box-sizing:border-box}
body{margin:0;background:#0a0e17;color:#e6edf7;font:14px -apple-system,"Segoe UI",sans-serif}
#bar{display:flex;justify-content:space-between;padding:6px 12px;background:#151c2b;color:#7d8aa3;font-size:11px}
#felt{position:relative;height:52vh;min-height:280px;
 background:radial-gradient(ellipse at 50% 45%,#1e7350,#124b35 70%,#0d3626)}
.seat{position:absolute;width:74px;margin-left:-37px;margin-top:-20px;text-align:center;font-size:11px;
 transition:opacity .2s}
.seat .nm{font-weight:700;color:#fff;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.seat .st{color:#7ddba5}
.seat.folded{opacity:.35}
.seat.act{outline:2px solid #ffd166;border-radius:8px;padding:2px;background:#1d2740}
.seat.act .nm{color:#ffd166}
.cd{display:block;margin-top:2px;background:#ffd166;color:#2b1d05;border-radius:8px;
 padding:0 5px;font-size:10px;font-weight:700}
.chip{display:inline-block;background:#e8a33d;color:#3a2708;border-radius:8px;padding:0 5px;
 font-size:10px;font-weight:700;margin-top:1px}
.oppHole{margin:2px 0}
.oppHole .card{width:16px;height:22px;line-height:22px;font-size:9px;margin:0 1px;border-radius:3px}
.card.back{background:linear-gradient(135deg,#2b4a7a,#1a2d4d);border:1px solid #3f6199}
#centre{position:absolute;top:44%;left:0;right:0;text-align:center}
.card{display:inline-block;background:#fff;border-radius:4px;width:26px;height:36px;line-height:36px;
 text-align:center;font-size:14px;font-weight:700;margin:0 2px;color:#111;box-shadow:0 1px 3px rgba(0,0,0,.5)}
.card.red{color:#d62828}
#pot{color:#ffd166;font-weight:700;margin-top:8px}
#mine{display:flex;justify-content:space-between;align-items:center;padding:10px 14px;background:#0d1220}
#me{font-weight:700;color:#fff}
#stack{color:#7ddba5;font-size:12px;margin-left:6px}
#acts{display:flex;gap:6px;padding:10px;background:#121927}
button{flex:1;padding:12px 0;border:0;border-radius:8px;font-weight:700;font-size:13px;
 background:#243147;color:#c9d5e8}
button.pri{background:#e8a33d;color:#2b1d05}
button.dng{background:#3a2029;color:#e08a9a}
button:disabled{opacity:.35}
#msg{text-align:center;padding:8px;color:#9fb0c9;font-size:12px;min-height:18px}
</style></head><body>
<div id="bar"><span>♠ Покер</span><span id="stage"></span></div>
<div id="felt"><div id="centre"><div id="board"></div><div id="pot"></div></div></div>
<div id="mine"><span><span id="me"></span><span id="stack"></span></span><span id="hole"></span></div>
<div id="acts">
  <button id="btn-fold" class="dng" disabled>Пас</button>
  <button id="btn-check" disabled>Чек</button>
  <button id="btn-call" disabled>Колл</button>
  <button id="btn-raise" class="pri" disabled>Рейз</button>
</div>
<div id="msg"></div>
<script>
const TABLE={{.TableID}};
const tg=(window.Telegram&&window.Telegram.WebApp)||null;
if(tg){tg.ready();tg.expand()}
const INIT=(tg&&tg.initData)||"";

const msgEl=document.getElementById("msg");
function setMsg(t){msgEl.textContent=t||""}

// Sticky error state: tick() runs every second and would otherwise stomp a
// just-set error (connection lost, action rejected, ...) with the ordinary
// countdown/turn line within a second. errorMsg takes precedence in tick()
// until the next successful render() or a reconnected SSE stream clears it.
let errorMsg=null;
function setError(t){errorMsg=t;setMsg(t)}

const SUITS={h:"♥",d:"♦",c:"♣",s:"♠"};
const STAGE_UA={waiting:"Очікування",preflop:"Префлоп",flop:"Флоп",turn:"Терн",river:"Рівер",showdown:"Шоудаун"};

// Card strings from the server are rank+suit-letter, e.g. "Ah","Td" — never
// suit symbols, so this is purely a display transform. It only ever reads
// rank/suit LETTERS out of a server-controlled card string, never anything
// player-supplied, so building its markup via string concatenation is safe.
function card(s){
  if(!s)return "";
  const suitLetter=s.slice(-1);
  const rank=s.slice(0,-1).replace("T","10");
  const red=suitLetter==="h"||suitLetter==="d";
  return '<span class="card'+(red?' red':'')+'">'+rank+(SUITS[suitLetter]||suitLetter)+'</span>';
}
// Face-down card back markup, built once from a fixed constant — never from
// anything derived from another player's data.
const CARD_BACK='<span class="card back"></span>';
const CARD_BACKS=CARD_BACK+CARD_BACK;

function clip(n){n=n||"";return n.length>10?n.slice(0,10)+"…":n}

const actionBtns=["btn-fold","btn-check","btn-call","btn-raise"].map(id=>document.getElementById(id));
function setActsBusy(busy){actionBtns.forEach(b=>b.disabled=busy)}

let state=null;
// Highest seq rendered so far. A broadcast can land between SSE subscriber
// registration and its own initial snapshot fetch, arriving with a LOWER
// seq than what's already on screen — anything not strictly greater than
// this is dropped so the table never flickers backwards.
let highestSeq=-1;
// The server computes pot by summing seats' committed chips and settlement
// zeroes them, so at showdown pot arrives as 0 — exactly when it's most
// wanted on screen. Remembered here and shown in its place until a new hand
// (stage transitions into "preflop") starts.
let lastPot=0;
let lastStage=null;

// Recomputes the action row (call amount, enabled/disabled) from the last
// known state. Called from render() on every fresh snapshot, and also from
// act()'s failure paths — a request forcibly disables the buttons while in
// flight (see act()), and a failure produces no fresh render() to restore
// them, so this must be callable on its own too.
function applyButtons(){
  if(!state){setActsBusy(true);return}
  const live=state.stage!=="waiting"&&state.stage!=="showdown";
  const seats=state.seats||[];
  const me=state.you_seat>=0?seats[state.you_seat]:null;
  const myTurn=live&&!!(me&&me.to_act);
  const highBet=Math.max(0,...seats.map(s=>s.bet||0));
  const toCall=me?Math.max(0,highBet-(me.bet||0)):0;
  document.getElementById("btn-call").textContent=toCall>0?("Колл "+toCall):"Колл";
  document.getElementById("btn-fold").disabled=!myTurn;
  document.getElementById("btn-check").disabled=!myTurn||toCall>0;
  document.getElementById("btn-call").disabled=!myTurn||toCall<=0;
  document.getElementById("btn-raise").disabled=!myTurn;
}

function render(v){
  if(v.seq<=highestSeq)return;
  highestSeq=v.seq;
  state=v;
  errorMsg=null; // a fresh snapshot means we're caught up; stop overriding the countdown/turn line

  // t.ToAct defaults to seat 0 before any hand is ever dealt and is never
  // reset between hands, so a seat can carry to_act=true while the table is
  // merely "waiting" or sitting at "showdown". Only trust to_act while a
  // hand is actually live.
  const live=v.stage!=="waiting"&&v.stage!=="showdown";
  // TableView.Seats is appended onto a nil slice server-side, so a
  // zero-seat table marshals as "seats":null.
  const seats=v.seats||[];

  if(v.stage==="preflop"&&lastStage!=="preflop")lastPot=0;
  lastStage=v.stage;
  if(v.pot>0)lastPot=v.pot;
  const potShown=v.pot>0?v.pot:lastPot;

  document.getElementById("stage").textContent=STAGE_UA[v.stage]||v.stage;
  document.getElementById("board").innerHTML=v.board.map(card).join("");
  document.getElementById("pot").textContent="🪙 Банк "+potShown;

  const felt=document.getElementById("felt");
  felt.querySelectorAll(".seat").forEach(e=>e.remove());
  const n=seats.length,cx=50,cy=42,rx=38,ry=30;
  const left=Math.max(0,v.deadline-Math.floor(Date.now()/1000));
  seats.forEach((s,i)=>{
    const ang=(-Math.PI/2)+(2*Math.PI*i/n);
    const isActive=live&&s.to_act;
    const d=document.createElement("div");
    d.className="seat"+(s.folded?" folded":"")+(isActive?" act":"");
    d.style.left=(cx+rx*Math.cos(ang))+"%";
    d.style.top=(cy+ry*Math.sin(ang))+"%";

    // Display name is player-controlled (it comes straight from the
    // player's own Telegram profile) — set via textContent, never
    // innerHTML/string-concatenation. clip() truncating to 10 chars is
    // NOT what makes this safe; textContent is.
    const nm=document.createElement("div");
    nm.className="nm";
    nm.textContent=clip(s.name);
    d.appendChild(nm);

    // Opponents' hole cards: "hole" is populated only at showdown for
    // non-folded, in-hand seats (server-enforced isolation, view.go) — show
    // the real cards when the server sent them, otherwise a face-down back
    // for anyone still holding cards. The back markup is the fixed
    // CARD_BACKS constant, never anything derived from s, so this loop
    // structurally cannot leak another player's hand.
    if(!s.folded&&v.stage!=="waiting"){
      const hole=document.createElement("div");
      hole.className="oppHole";
      hole.innerHTML=s.hole?s.hole.map(card).join(""):CARD_BACKS;
      d.appendChild(hole);
    }

    // Stack/bet/countdown are Go ints and JS numbers, not player-controlled
    // text, so textContent here is just for consistency, not a safety
    // requirement.
    const st=document.createElement("div");
    st.className="st";
    st.textContent=s.stack;
    d.appendChild(st);

    if(s.bet){
      const chip=document.createElement("span");
      chip.className="chip";
      chip.textContent=s.bet;
      d.appendChild(chip);
    }
    if(isActive){
      const cd=document.createElement("div");
      cd.className="cd";
      cd.textContent=left+"с";
      d.appendChild(cd);
    }
    felt.appendChild(d);
  });

  const me=v.you_seat>=0?seats[v.you_seat]:null;
  document.getElementById("me").textContent=me?clip(me.name):"";
  document.getElementById("stack").textContent=me?me.stack:"";
  document.getElementById("hole").innerHTML=me&&me.hole?me.hole.map(card).join(""):"";

  applyButtons();
  tick();
}

function tick(){
  if(!state)return;
  if(errorMsg){setMsg(errorMsg);return} // a pending error outranks the countdown/turn line
  const live=state.stage!=="waiting"&&state.stage!=="showdown";
  const left=Math.max(0,state.deadline-Math.floor(Date.now()/1000));
  const cd=document.querySelector(".seat.act .cd");
  if(cd)cd.textContent=left+"с";
  const seats=state.seats||[];
  const me=state.you_seat>=0?seats[state.you_seat]:null;
  setMsg(live&&me&&me.to_act?("Твій хід · "+left+"с"):"");
}
setInterval(tick,1000);

async function act(a,amount){
  // Disable every action button for the duration of the request: without
  // this a double-tap fires two requests carrying the same seq, and the
  // second one 409s and reports "Хід уже пройшов" for a move that actually
  // went through.
  setActsBusy(true);
  try{
    const r=await fetch("/api/poker/"+TABLE+"/action",{
      method:"POST",
      headers:{"Content-Type":"application/json","X-Telegram-Init-Data":INIT},
      body:JSON.stringify({action:a,amount:amount||0,seq:state?state.seq:0})
    });
    if(r.status===409){setError("Хід уже пройшов, оновлюю…");applyButtons();return}
    if(!r.ok){setError(await r.text());applyButtons();return}
    render(await r.json());
  }catch(e){setError("Зʼєднання втрачено…");applyButtons()}
}

document.getElementById("btn-fold").onclick=()=>act("fold");
document.getElementById("btn-check").onclick=()=>act("check");
document.getElementById("btn-call").onclick=()=>act("call");
document.getElementById("btn-raise").onclick=()=>{
  const seats=state?(state.seats||[]):[];
  const highBet=Math.max(0,...seats.map(s=>s.bet||0));
  const input=window.prompt("Сума рейзу (всього на цій вулиці):",String(highBet+100));
  if(input===null)return;
  const amt=parseInt(input,10);
  if(!Number.isFinite(amt))return;
  act("raise",amt);
};

(async()=>{
  if(!tg){setMsg("Відкрий через кнопку в чаті Telegram");return}
  let j;
  try{
    j=await fetch("/api/poker/"+TABLE+"/join",{method:"POST",headers:{"X-Telegram-Init-Data":INIT}});
  }catch(e){setMsg("Зʼєднання втрачено…");return}
  if(!j.ok){setMsg(await j.text());return}
  render(await j.json());

  const es=new EventSource("/api/poker/"+TABLE+"/stream?init_data="+encodeURIComponent(INIT));
  es.onmessage=e=>{try{render(JSON.parse(e.data))}catch(err){}};
  // A recovered connection must not leave a stale "connection lost" message
  // sitting on screen — errorMsg is also cleared by the next successful
  // render(), but onopen fires as soon as the socket reconnects, before any
  // data has necessarily arrived.
  es.onopen=()=>{errorMsg=null};
  es.onerror=()=>setError("Зʼєднання втрачено…");
})();
</script></body></html>`))

// sseKeepaliveInterval is how often an idle SSE stream sends a comment-only
// ping frame. Fly.io's proxy (and some browsers/corporate proxies) can
// silently buffer or drop a connection that goes quiet, so a stream with no
// real updates still needs to write *something* periodically.
const sseKeepaliveInterval = 20 * time.Second

// subscriber is one open SSE connection watching a table.
type subscriber struct {
	userID string
	ch     chan poker.TableView
	// done is closed by sweepOnce when its table is reclaimed for being
	// idle, so a still-connected SSE goroutine watching a now-deleted table
	// exits instead of idling forever on a channel nothing will ever send
	// to again.
	done chan struct{}
}

// PokerHub owns every live poker table and the SSE subscribers watching
// them. It is the only thing allowed to mutate a poker.Table from outside
// the poker package: the engine itself is lock-free (so it stays trivially
// testable in isolation), so this hub is responsible for serializing access
// to each table via table.Lock()/Unlock() around every Sit/Act/Showdown/
// ViewFor call.
//
// Lock ordering: a table's own lock is always the OUTER lock, h.mu is
// always the INNER lock. broadcast() takes h.mu while the caller already
// holds the table lock — never the reverse.
type PokerHub struct {
	db    *storage.DB
	bot   *tele.Bot
	token string

	// isMember checks whether userID is a member of the Telegram chat
	// chatID. It defaults to a bot-backed check (see defaultIsMember) but
	// is a field, not a hardcoded call, so tests can stub it instead of
	// relying on a nil bot to skip the check.
	isMember func(chatID, userID int64) (bool, error)

	mu     sync.Mutex
	tables map[string]*poker.Table
	subs   map[string][]*subscriber

	// seatedAt maps a userID to the single table they currently hold a seat
	// (or a pending seat attempt) at, hub-wide. It exists so one bankroll
	// can't back simultaneous buy-ins at several tables: the engine settles
	// per hand rather than escrowing at buy-in (deliberately, so a mid-hand
	// redeploy can't eat a real buy-in), so nothing else stops a user from
	// sitting at N tables at once for N times their real balance. Guarded by
	// h.mu, same as subs/tables.
	seatedAt map[string]string

	// lastActivity records, per table id, the last time a join or an action
	// touched that table — deliberately PLAYER-INITIATED events only. A
	// sweeper-fired forced timeout or a sweeper-started hand must NOT
	// refresh this: those are evidence of absence (nobody is there to act),
	// not activity, and touching it there would mean two permanently AFK
	// but still-funded players keep their table (and their seatedAt claims)
	// alive forever, exactly what item 3's reclamation exists to prevent.
	// It lives on the hub rather than on poker.Table so idle bookkeeping
	// never has to touch the engine. A table idle beyond idleTableTimeout
	// is reclaimed by the sweeper — see sweepOnce. Guarded by h.mu, same as
	// tables/subs/seatedAt.
	lastActivity map[string]time.Time

	// showdownAt records, per table id, the wall-clock time settle() last
	// transitioned that table into StageShowdown. sweepOnce's auto-deal
	// branch requires at least one full sweepInterval to have elapsed since
	// this timestamp before dealing the next hand — without it, a hand that
	// settled a millisecond before a sweep pass began would deal instantly,
	// giving players ~0 seconds to see the showdown reveal (the only moment
	// it exists for). Guarded by h.mu, same as lastActivity/seatedAt/subs.
	showdownAt map[string]time.Time

	// membershipCache maps a (chatID, userID) pair to the wall-clock time
	// its last POSITIVE Telegram chat-membership check succeeded. auth()
	// consults it before ever calling isMember again, so a network blip or
	// a Telegram rate-limit cannot strand an already-verified player
	// mid-hand by forcing a fresh Telegram round-trip on literally every
	// fold/call/raise and every SSE reconnect. Only positive results are
	// ever cached (see auth) — a negative or errored check leaves no trace
	// here, so an unknown or currently-failing user can never be admitted
	// from a stale entry. Guarded by h.mu, same as the other hub maps.
	membershipCache map[membershipKey]time.Time
}

// membershipKey identifies one (chat, user) pair for membershipCache.
type membershipKey struct {
	chatID int64
	userID int64
}

// membershipCacheTTL is how long a positive membership check stays cached
// before auth() will re-verify with Telegram. Named as a constant per the
// review: long enough that ordinary in-hand fold/call/raise traffic and SSE
// reconnects never re-hit Telegram, short enough that someone who is
// actually removed from the chat is re-checked on a human timescale rather
// than never.
const membershipCacheTTL = 5 * time.Minute

func NewPokerHub(db *storage.DB, bot *tele.Bot, token string) *PokerHub {
	return &PokerHub{
		db:              db,
		bot:             bot,
		token:           token,
		isMember:        defaultIsMember(bot),
		tables:          map[string]*poker.Table{},
		subs:            map[string][]*subscriber{},
		seatedAt:        map[string]string{},
		lastActivity:    map[string]time.Time{},
		showdownAt:      map[string]time.Time{},
		membershipCache: map[membershipKey]time.Time{},
	}
}

// claimSeat atomically reserves userID's single hub-wide seat for tableID.
// live is false if tableID is no longer present in h.tables — the caller
// (a request that resolved its *poker.Table pointer before the sweeper
// reclaimed the table out from under it, a TOCTOU window between Register's
// existence check and this call) must refuse the join outright and must
// NOT fall through to the ok check below: writing h.seatedAt for a dead
// table id would resurrect a hub-map entry the sweeper will never see
// again (it only ever iterates the current h.tables), permanently
// stranding that claim exactly like the bug item 3 was written to fix.
// ok is false if userID already holds a claim on a *different*, still-live
// table, in which case the caller must reject the join outright. fresh is
// true when this call created the claim (nothing existed before) — the
// caller must remember that, because only a fresh claim should be released
// if the subsequent Sit() fails; a pre-existing claim for this same table
// means the user may already have a live seat here, and releasing it out
// from under them on an unrelated Sit failure would let a concurrent join
// steal their spot at another table.
func (h *PokerHub) claimSeat(userID, tableID string) (fresh, ok, live bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, tableLive := h.tables[tableID]; !tableLive {
		return false, false, false
	}
	if existing, exists := h.seatedAt[userID]; exists {
		return false, existing == tableID, true
	}
	h.seatedAt[userID] = tableID
	return true, true, true
}

// releaseSeatClaim drops userID's hub-wide seat claim, but only if it still
// points at tableID — never clobber a claim a later, unrelated join already
// moved elsewhere.
func (h *PokerHub) releaseSeatClaim(userID, tableID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.seatedAt[userID] == tableID {
		delete(h.seatedAt, userID)
	}
}

// defaultIsMember returns the production chat-membership checker backed by
// bot. If bot is nil, membership can never be verified, so the checker
// fails CLOSED: it always reports "not a member" rather than silently
// granting access, unlike the old behaviour of skipping the check entirely.
func defaultIsMember(bot *tele.Bot) func(chatID, userID int64) (bool, error) {
	if bot == nil {
		return func(chatID, userID int64) (bool, error) {
			return false, nil
		}
	}
	return func(chatID, userID int64) (bool, error) {
		m, err := bot.ChatMemberOf(&tele.Chat{ID: chatID}, &tele.User{ID: userID})
		if err != nil {
			return false, err
		}
		switch m.Role {
		case tele.Creator, tele.Administrator, tele.Member, tele.Restricted:
			return true, nil
		default:
			return false, nil
		}
	}
}

// Create allocates a new table for the given chat and registers it in the
// hub under a fresh random id.
func (h *PokerHub) Create(chatID int64) *poker.Table {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	tbl := poker.NewTable(id, chatID)
	h.mu.Lock()
	h.tables[id] = tbl
	h.lastActivity[id] = time.Now()
	h.mu.Unlock()
	return tbl
}

func (h *PokerHub) table(id string) *poker.Table {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tables[id]
}

// tableIDFrom extracts the table id and sub-action from /api/poker/{id}/{action}.
func tableIDFrom(path string) (id, action string) {
	rest := strings.TrimPrefix(path, "/api/poker/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// auth verifies initData and chat membership, returning the authenticated
// Telegram user id and profile fields. On failure it returns a non-zero
// HTTP status code the caller should respond with.
//
// The raw initData string is handed to verifyInitData untouched — it is
// never parsed here first, and never re-parsed afterwards. A second parser
// with different duplicate-key semantics is the only way a known,
// currently-unexploitable duplicate-key issue in initData parsing becomes
// an actual auth bypass.
//
// Membership is re-verified with Telegram on every join/action/SSE
// reconnect, so it distinguishes two very different failures rather than
// collapsing both into 403: a DEFINITIVE "not a member" (isMember returned
// ok=false with no error) still returns 403, but a checker ERROR — a
// network blip, a Telegram rate-limit — must never read as "you're not in
// this chat" (that discards whatever the player was mid-hand and the
// sweeper folds them 90s later), so it returns 503 instead and admits
// nothing. A successful positive check is cached for membershipCacheTTL
// (see cachedMember/cacheMember) so ordinary in-hand traffic isn't gated on
// a Telegram round-trip every time; failures are never cached, keeping the
// fail-closed property — an unknown or currently-failing user is never
// admitted from a stale cache entry.
func (h *PokerHub) auth(r *http.Request, tbl *poker.Table) (uid int64, firstName, username string, status int) {
	initData := r.Header.Get("X-Telegram-Init-Data")
	if initData == "" {
		initData = r.URL.Query().Get("init_data")
	}
	if initData == "" {
		return 0, "", "", http.StatusUnauthorized
	}
	uid, firstName, username, err := verifyInitData(initData, h.token, 24*time.Hour)
	if err != nil {
		return 0, "", "", http.StatusUnauthorized
	}

	if h.cachedMember(tbl.ChatID, uid) {
		return uid, firstName, username, 0
	}
	ok, err := h.isMember(tbl.ChatID, uid)
	if err != nil {
		return 0, "", "", http.StatusServiceUnavailable
	}
	if !ok {
		return 0, "", "", http.StatusForbidden
	}
	h.cacheMember(tbl.ChatID, uid)
	return uid, firstName, username, 0
}

// cachedMember reports whether (chatID, userID) has a still-fresh positive
// membership result cached. A stale entry is evicted on read rather than
// left to leak forever.
func (h *PokerHub) cachedMember(chatID, userID int64) bool {
	key := membershipKey{chatID: chatID, userID: userID}
	h.mu.Lock()
	defer h.mu.Unlock()
	at, ok := h.membershipCache[key]
	if !ok {
		return false
	}
	if time.Since(at) > membershipCacheTTL {
		delete(h.membershipCache, key)
		return false
	}
	return true
}

// cacheMember records a fresh positive membership result for (chatID,
// userID). Only ever called after isMember has returned ok=true with no
// error — see auth.
func (h *PokerHub) cacheMember(chatID, userID int64) {
	key := membershipKey{chatID: chatID, userID: userID}
	h.mu.Lock()
	h.membershipCache[key] = time.Now()
	h.mu.Unlock()
}

// Register wires the poker HTTP surface into mux: the join/stream/action
// API under /api/poker/{id}/{action} and the mini-app page under
// /poker/{id}.
func (h *PokerHub) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/poker/", func(w http.ResponseWriter, r *http.Request) {
		id, action := tableIDFrom(r.URL.Path)
		tbl := h.table(id)
		if tbl == nil {
			http.Error(w, "Стіл закрито", http.StatusNotFound)
			return
		}
		uid, firstName, username, status := h.auth(r, tbl)
		if status != 0 {
			msg := "Відкрий через кнопку в чаті"
			switch status {
			case http.StatusForbidden:
				msg = "Ти не з цього чату"
			case http.StatusServiceUnavailable:
				// A transient Telegram error, not a definitive membership
				// answer — see auth's doc comment. Never say "not a
				// member" for this.
				msg = "Телеграм не відповідає, спробуй ще раз"
			}
			http.Error(w, msg, status)
			return
		}
		switch action {
		case "join":
			h.handleJoin(w, tbl, uid, firstName, username)
		case "stream":
			h.handleStream(w, r, tbl, uid)
		case "action":
			h.handleAction(w, r, tbl, uid)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/poker/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/poker/")
		if h.table(id) == nil {
			http.Error(w, "Стіл закрито", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pokerTmpl.Execute(w, map[string]string{"TableID": id})
	})
}

// handleJoin seats the authenticated user with min(balance, MaxBuyIn) chips.
// The engine itself rejects a buy-in below MinBuyIn.
//
// A user may hold only one seat across the whole hub at a time — see
// seatedAt — so a single bankroll can't back simultaneous buy-ins at
// several tables. The claim is taken before Sit() (and released again if
// Sit() then fails) rather than debited from the balance, because
// settlement happens per hand precisely so a mid-hand redeploy can't eat a
// real buy-in; escrowing chips at sit-down would undo that.
//
// A player who ALREADY occupies a seat at THIS table is reconnecting — the
// Mini App was closed and reopened, an iOS webview got backgrounded, they
// switched device — not joining fresh. Sit() would always fail for them
// (ErrAlreadySat, or an unrelated ErrBuyInTooLow/ErrTableFull depending on
// Sit's own error precedence — none of which describes their situation),
// stranding them at a dead-end error screen that never opens the SSE
// stream while their chips stay seated and the sweeper bleeds their blinds
// every 90s until the 30-minute reclaim. So this is checked FIRST, directly
// against tbl.Seats, and short-circuits straight to success: no re-seating,
// no re-reading their balance, no resetting their stack. Checked against
// tbl.Seats rather than h.seatedAt deliberately: a player busted to 0 chips
// has their h.seatedAt claim released at settlement (see settle) so they
// can join a DIFFERENT table, but they still occupy a Seat row here, so
// h.seatedAt alone would wrongly treat their reconnect to THIS table as a
// fresh join (and then wrongly 409 them as "already at another table" once
// they've claimed elsewhere).
func (h *PokerHub) handleJoin(w http.ResponseWriter, tbl *poker.Table, uid int64, firstName, username string) {
	userID := fmt.Sprintf("%d", uid)
	name := resolveTarget(firstName, username)

	tbl.Lock()
	if tbl.SeatIndexOf(userID) >= 0 {
		view := tbl.ViewFor(userID)
		tbl.Unlock()
		h.touch(tbl.ID) // reconnecting is real player-initiated activity
		writeJSON(w, view)
		return
	}
	tbl.Unlock()

	fresh, ok, live := h.claimSeat(userID, tbl.ID)
	if !live {
		// The sweeper reclaimed this table in the window between Register's
		// existence check and here — same response an unknown table
		// already produces, since as far as the hub's bookkeeping is
		// concerned that's exactly what this now is.
		http.Error(w, "Стіл закрито", http.StatusNotFound)
		return
	}
	if !ok {
		http.Error(w, "Ти вже за іншим столом", http.StatusConflict)
		return
	}

	balance := 0
	if h.db != nil {
		balance = h.db.GetBalance(userID, name)
	}
	buyIn := balance
	if buyIn > poker.MaxBuyIn {
		buyIn = poker.MaxBuyIn
	}

	tbl.Lock()
	if err := tbl.Sit(userID, name, buyIn); err != nil {
		tbl.Unlock()
		if fresh {
			h.releaseSeatClaim(userID, tbl.ID)
		}
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	// Auto-start once a second player is seated. Without this, hands never
	// begin: nothing else ever calls StartHand on a freshly created table.
	if tbl.Stage == poker.StageWaiting && tbl.SeatedCount() >= 2 {
		_ = tbl.StartHand()
	}
	view := tbl.ViewFor(userID)
	h.broadcast(tbl) // called with the table lock held, per lock ordering
	tbl.Unlock()
	h.touch(tbl.ID)

	writeJSON(w, view)
}

// handleAction applies one player action, settles the hand exactly once if
// it just reached showdown, and returns the actor's fresh view.
//
// amount is untrusted client input. It is passed straight through to
// Act, which is the sole authority on whether it is legal — this handler
// neither clamps nor otherwise "sanitizes" it, since doing so could turn an
// invalid action into a valid one.
func (h *PokerHub) handleAction(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64) {
	var body struct {
		Action string `json:"action"`
		Amount int    `json:"amount"`
		Seq    uint64 `json:"seq"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Некоректний запит", http.StatusBadRequest)
		return
	}
	userID := fmt.Sprintf("%d", uid)

	tbl.Lock()

	if body.Seq != tbl.Seq {
		tbl.Unlock()
		http.Error(w, "Застаріла дія, онови стан", http.StatusConflict)
		return
	}

	prevStage := tbl.Stage
	if err := tbl.Act(userID, poker.Action(body.Action), body.Amount); err != nil {
		tbl.Unlock()
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	// Settle exactly once: only on the single transition into showdown, not
	// merely "whenever we currently observe StageShowdown". Act() itself
	// refuses any further action once the hand is over (StageWaiting or
	// StageShowdown), so this branch cannot be entered twice for the same
	// hand even without the engine's own internal settled guard.
	if tbl.Stage == poker.StageShowdown && prevStage != poker.StageShowdown {
		h.settle(tbl)
	}

	view := tbl.ViewFor(userID)
	h.broadcast(tbl)
	tbl.Unlock()
	h.touch(tbl.ID)

	// Written outside the table lock, like handleJoin: with no server
	// WriteTimeout, a client that stops reading here must not be able to
	// stall the whole table for everyone else.
	writeJSON(w, view)
}

// settle writes each player's showdown delta to the currency database in
// one atomic transaction, records when this hand settled (so the sweeper
// can guarantee at least one full sweep interval before dealing the next
// hand — see showdownAt/showdownReady), and releases the hub-wide seatedAt
// claim of any seat busted to 0 chips so they are not locked out of every
// OTHER table until this table itself goes 30 minutes idle. The caller
// must already hold tbl.Lock().
func (h *PokerHub) settle(tbl *poker.Table) {
	deltas := tbl.Showdown()

	h.mu.Lock()
	h.showdownAt[tbl.ID] = time.Now()
	h.mu.Unlock()

	if h.db != nil {
		entries := make([]storage.PokerDelta, 0, len(tbl.Seats))
		bankDelta := 0
		for _, s := range tbl.Seats {
			d, ok := deltas[s.UserID]
			if !ok || d == 0 {
				continue
			}
			// A bot has no balance of its own: its win or loss belongs to
			// the house. Netting every bot into one bank entry reduces the
			// per-hand transaction row count from one-per-bot to exactly one,
			// while keeping humans + bank summing to zero.
			if isBotUser(s.UserID) {
				bankDelta += d
				continue
			}
			entries = append(entries, storage.PokerDelta{UserID: s.UserID, Name: s.Name, Amount: d})
		}
		if bankDelta != 0 {
			entries = append(entries, storage.PokerDelta{UserID: bankUserID, Name: "Банк", Amount: bankDelta})
		}
		// A crash or SIGTERM mid-settlement is the only place in the system
		// that could otherwise break the zero-sum invariant across players
		// (some credited, some not) — SettlePoker wraps every entry in one
		// database transaction so it commits all-or-nothing. See
		// internal/storage/sqlite.go.
		if err := h.db.SettlePoker(entries); err != nil {
			log.Printf("[poker] settle tx failed for table %s: %v", tbl.ID, err)
		}
	}

	// A seat busted to 0 chips in this hand must not stay locked out of
	// every OTHER table hub-wide until this table itself goes 30 minutes
	// idle (idleTableTimeout). releaseSeatClaim takes h.mu, the INNER lock
	// relative to the table lock the caller already holds — the correct
	// direction per the lock-ordering rule.
	for _, s := range tbl.Seats {
		if s.Stack <= 0 {
			h.releaseSeatClaim(s.UserID, tbl.ID)
		}
	}
}

// showdownReady reports whether at least one full sweepInterval has passed
// since tableID's hand last settled into showdown, per h.showdownAt. A
// table with no recorded showdown time is treated as ready — this should
// not happen via any production code path (settle() always records one on
// every transition into StageShowdown), so a missing timestamp must never
// wedge a table in showdown forever.
func (h *PokerHub) showdownReady(tableID string) bool {
	h.mu.Lock()
	at, ok := h.showdownAt[tableID]
	h.mu.Unlock()
	if !ok {
		return true
	}
	return time.Since(at) >= sweepInterval
}

func (h *PokerHub) handleStream(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Стрімінг не підтримується", http.StatusInternalServerError)
		return
	}
	userID := fmt.Sprintf("%d", uid)

	// Registered — and its liveness checked — before any SSE-specific
	// header is written, so a table the sweeper already reclaimed (the same
	// TOCTOU window claimSeat/touch guard against: this handler resolved
	// tbl via Register's existence check before the reclaim) can still
	// cleanly 404 instead of resurrecting a h.subs entry the sweeper will
	// never see again.
	sub := &subscriber{userID: userID, ch: make(chan poker.TableView, 4), done: make(chan struct{})}
	if !h.registerSubscriber(tbl.ID, sub) {
		http.Error(w, "Стіл закрито", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Tells Fly.io's proxy (nginx-compatible) not to buffer this response;
	// without it an idle stream can sit in the proxy's buffer indefinitely.
	w.Header().Set("X-Accel-Buffering", "no")

	defer func() {
		h.mu.Lock()
		list := h.subs[tbl.ID]
		for i, s := range list {
			if s == sub {
				h.subs[tbl.ID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		h.mu.Unlock()
	}()

	// The hub mutex is released above before we ever touch the table lock:
	// register-then-release, then take the table lock separately for the
	// initial snapshot. Nesting them the other way risks deadlock against
	// broadcast(), which takes h.mu while holding the table lock.
	tbl.Lock()
	initial := tbl.ViewFor(userID)
	tbl.Unlock()
	sendView(w, flusher, initial)

	keepalive := time.NewTicker(sseKeepaliveInterval)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-sub.done:
			// Table reclaimed as idle by the sweeper: nothing will ever be
			// sent on sub.ch again, so exit rather than idle here forever.
			return
		case v := <-sub.ch:
			sendView(w, flusher, v)
		case <-keepalive.C:
			// Comment-only SSE frame: keeps the connection warm through
			// proxies/browsers that drop or buffer a silent stream, without
			// being interpreted as a data event by the client.
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func sendView(w http.ResponseWriter, f http.Flusher, v poker.TableView) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", raw)
	f.Flush()
}

// broadcast pushes a fresh, individually-redacted snapshot to every viewer
// of tbl. The caller must already hold tbl.Lock() — broadcast takes h.mu
// as the inner lock, never the other way around.
//
// A subscriber's channel send never blocks: a full channel means a slow
// consumer, so that update is dropped and the next snapshot repairs it.
func (h *PokerHub) broadcast(tbl *poker.Table) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.subs[tbl.ID] {
		select {
		case s.ch <- tbl.ViewFor(s.userID):
		default: // slow consumer: drop, the next snapshot repairs it
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// sweepInterval is how often the sweeper checks every table for an expired
// turn clock, a hand ready to auto-start, or idleness.
const sweepInterval = 5 * time.Second

// idleTableTimeout is how long a table may go without a PLAYER-INITIATED
// join or action (sweeper-fired timeouts and sweeper-started hands do not
// count — see lastActivity) before the sweeper reclaims it: removed from
// h.tables, every seatedAt claim pointing at it released, and its
// subscriber list dropped so any still-connected SSE goroutines exit.
// Reclaimed regardless of whether it still has seats — an abandoned table
// with seated players is exactly the case that would otherwise strand their
// hub-wide seat claim until the process restarts.
//
// 30 minutes comfortably exceeds a realistic hand length even with every
// street forced to its full TurnTimeout (90s): a hand has at most 4 streets
// (preflop/flop/turn/river) each with at most MaxSeats-1 live actors, so the
// pathological worst case is on the order of a few minutes, not 30 — a
// table where players are merely thinking between actions is never at risk
// of being reclaimed mid-hand.
const idleTableTimeout = 30 * time.Minute

// touch records activity on tableID now — but only if tableID is still
// present in h.tables. Without that guard, a handler that resolved its
// *poker.Table pointer before the sweeper reclaimed the table (the same
// TOCTOU window claimSeat guards against) would silently resurrect a
// h.lastActivity entry for a dead table id: the sweeper only ever iterates
// the current h.tables, so that entry would never be cleaned up again —
// the exact unbounded growth item 4 exists to bound. A no-op here is
// intentionally silent (not an error the caller need act on): by the time
// touch runs, the real mutation (Sit/Act) already succeeded against the
// table object itself, which stays valid even once orphaned from the hub.
//
// Guarded by h.mu; safe to call either standalone or while holding a
// table's own lock, since h.mu is always the inner lock relative to a
// table lock.
func (h *PokerHub) touch(tableID string) {
	h.mu.Lock()
	if _, live := h.tables[tableID]; live {
		h.lastActivity[tableID] = time.Now()
	}
	h.mu.Unlock()
}

// activitySince returns how long it has been since tableID last saw
// activity, per lastActivity. A table with no recorded activity (should not
// happen — Create seeds it) is treated as maximally idle.
func (h *PokerHub) activitySince(tableID string) time.Duration {
	h.mu.Lock()
	last, ok := h.lastActivity[tableID]
	h.mu.Unlock()
	if !ok {
		return idleTableTimeout + 1
	}
	return time.Since(last)
}

// registerSubscriber appends sub to tableID's subscriber list, but only if
// tableID is still present in h.tables — the same TOCTOU guard as
// claimSeat's live check, applied to h.subs instead of h.seatedAt. Reports
// whether it registered. Guarded by h.mu.
func (h *PokerHub) registerSubscriber(tableID string, sub *subscriber) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, live := h.tables[tableID]; !live {
		return false
	}
	h.subs[tableID] = append(h.subs[tableID], sub)
	return true
}

// StartSweeper starts the background goroutine that enforces turn deadlines,
// auto-starts hands, and reclaims idle tables across the whole hub. Without
// it: a player who closes Telegram stalls their table forever with real
// chips committed, a table that fills up never actually deals a hand, and
// h.tables/h.subs grow without bound for the life of the process. Call once,
// after Register, from main — it runs for the life of the process.
func (h *PokerHub) StartSweeper() {
	go func() {
		for range time.Tick(sweepInterval) {
			h.sweepOnce()
		}
	}()
}

// sweepOnce runs a single sweep pass over every table. Split out from
// StartSweeper's ticker loop so tests can drive one deterministic pass
// directly, without a real background goroutine (and its ticker) to leak
// across tests.
//
// Lock ordering: a table's own lock is always OUTER, h.mu is always INNER
// (see the PokerHub doc comment). This snapshots the table list under h.mu
// and releases it before ever touching a table lock — the same two-phase
// shape broadcast() relies on, just run in the opposite direction: broadcast
// is called while already holding a table lock and takes h.mu inside; here
// we take h.mu first, alone, then take each table lock separately afterward.
// Reclaiming idle tables happens in a third pass, back under h.mu alone,
// once every table lock has already been released.
func (h *PokerHub) sweepOnce() {
	h.mu.Lock()
	tables := make(map[string]*poker.Table, len(h.tables))
	for id, t := range h.tables {
		tables[id] = t
	}
	h.mu.Unlock()

	var idleIDs []string
	for id, tbl := range tables {
		tbl.Lock()
		prevStage := tbl.Stage
		switch {
		case tbl.ForceTimeout():
			// Settle exactly as handleAction does: only on the single
			// transition into showdown, reusing the same helper so the
			// money rules never diverge between the two call sites.
			if tbl.Stage == poker.StageShowdown && prevStage != poker.StageShowdown {
				h.settle(tbl)
			}
			h.broadcast(tbl)
			// Deliberately NOT h.touch(id) here: a sweeper-fired forced
			// timeout is evidence of absence, not activity. Refreshing
			// lastActivity on it would mean two permanently-AFK-but-funded
			// players never go idle (the sweeper would keep auto-folding
			// them forever, forever renewing their claim), which strands
			// their seatedAt claims exactly as item 3 was written to fix.
			// Only a real join or a real player action (handleJoin,
			// handleAction) may refresh idleness.
		case tbl.Stage == poker.StageShowdown && tbl.SeatedCount() >= 2 && h.showdownReady(id):
			// At least one full sweepInterval has passed since settle()
			// transitioned this table into showdown (see
			// showdownAt/showdownReady) — checking merely "prevStage was
			// already StageShowdown at the top of THIS pass" is NOT the
			// same thing and was the bug here: prevStage is captured fresh
			// on every call, so it is equally true one tick after a real
			// prior-pass showdown and one MILLISECOND after this same
			// settle() call above finished (e.g. via the ForceTimeout case
			// on some earlier pass, or a player action moments ago) —
			// either way giving players ~0 seconds to see the showdown
			// reveal, the only moment it exists for. The table still has
			// 2+ players with chips, so deal the next hand rather than
			// leaving it stuck after exactly one hand.
			//
			// Also deliberately NOT h.touch(id): an auto-started hand with
			// nobody acting is still just the sweeper auto-folding forever,
			// same reasoning as the ForceTimeout case above — it must not
			// keep an abandoned table alive.
			if err := tbl.StartHand(); err == nil {
				h.broadcast(tbl)
			}
		}
		idle := h.activitySince(id) > idleTableTimeout
		tbl.Unlock()

		if idle {
			idleIDs = append(idleIDs, id)
		}
	}

	if len(idleIDs) == 0 {
		return
	}
	h.mu.Lock()
	for _, id := range idleIDs {
		delete(h.tables, id)
		delete(h.lastActivity, id)
		delete(h.showdownAt, id)
		for _, sub := range h.subs[id] {
			close(sub.done)
		}
		delete(h.subs, id)
		for uid, tid := range h.seatedAt {
			if tid == id {
				delete(h.seatedAt, uid)
			}
		}
	}
	h.mu.Unlock()
}
