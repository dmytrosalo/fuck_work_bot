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
// (`const TABLE={{.TableID}}||...`, no surrounding quotes in the template
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
#bar{display:flex;justify-content:space-between;align-items:center;gap:8px;
 padding:6px 10px;background:#151c2b;color:#7d8aa3;font-size:11px}
#bar span{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
#blinds{color:#c9a25a;font-weight:700}
#blinds.rising{color:#ffd166}
#felt{position:relative;height:48vh;min-height:340px;
 background:radial-gradient(ellipse at 50% 45%,var(--f1),var(--f2) 70%,var(--f3))}
:root{--f1:#1e7350;--f2:#124b35;--f3:#0d3626}
/* Felt themes. Purely cosmetic and per-player: the choice lives in this
   browser only and is never sent anywhere, so two players at one table can
   pick different colours without either seeing the other's. */
.felt-green {--f1:#1e7350;--f2:#124b35;--f3:#0d3626}
.felt-blue  {--f1:#1d5f86;--f2:#123f5c;--f3:#0c2b41}
.felt-purple{--f1:#5a3d80;--f2:#3b2757;--f3:#281a3c}
.felt-red   {--f1:#8a2f3a;--f2:#5c1f27;--f3:#3f151b}
.felt-slate {--f1:#3c4756;--f2:#28303b;--f3:#1b2129}
#themebtn{background:none;border:0;padding:0 2px;font-size:13px;cursor:pointer;flex:0 0 auto}
#themes{display:none;gap:6px;padding:6px 10px;background:#151c2b;justify-content:center}
#themes.on{display:flex}
#themes button{flex:0 0 30px;height:24px;padding:0;border-radius:6px;border:2px solid transparent}
#themes button.sel{border-color:#ffd166}
/* Quick reactions: one tap sends the emoji as an ordinary chat message. */
#quick{display:flex;gap:5px;padding:0 8px 6px;overflow-x:auto}
#quick button{flex:0 0 auto;min-width:40px;padding:6px 0;font-size:18px;
 background:#1b2536;border-radius:8px}
.seat{position:absolute;width:104px;margin-left:-52px;margin-top:-20px;text-align:center;font-size:11px;
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
.oppHole .card{width:26px;height:36px;line-height:36px;font-size:15px;margin:0 1px;border-radius:5px}
.card.back{background:linear-gradient(135deg,#2b4a7a,#1a2d4d);border:1px solid #3f6199}
/* Director Bo hides behind a pair of diamond jacks. */
.card.back.bo{background:#fff;border:1px solid #d9c48a;color:#d62828;
 font-weight:800;box-shadow:0 0 6px rgba(217,196,138,.4)}
/* Android God: gold leaf with the droid on it. */
.card.back.droid{background:linear-gradient(150deg,#f6d879,#c99a2e 60%,#8f6a12);
 border:1px solid #ffe9a8;display:inline-flex;align-items:center;justify-content:center;
 box-shadow:0 0 8px rgba(246,216,121,.45)}
.card.back.droid svg{width:64%;height:64%;fill:#3a2a05}
#centre{position:absolute;top:34%;left:0;right:0;text-align:center}
.card{display:inline-block;background:#fff;border-radius:6px;width:43px;height:59px;line-height:59px;
 text-align:center;font-size:22px;font-weight:700;margin:0 3px;color:#111;box-shadow:0 1px 3px rgba(0,0,0,.5)}
/* Target is a 1024px-wide viewport and up, where five board cards clear the
   side seats comfortably. Below that they cannot, so step back down rather
   than let the board and the seats collide. */
@media (max-width:760px){
 .card{width:34px;height:47px;line-height:47px;font-size:18px;margin:0 2px}
 .oppHole .card{width:21px;height:29px;line-height:29px;font-size:12px}
 .seat{width:74px;margin-left:-37px}
}
/* A card that is part of your current best five. */
.card.made{outline:2px solid #7ddba5;box-shadow:0 0 10px rgba(125,219,165,.55)}
.card.red{color:#d62828}
#pot{display:inline-block;color:#ffd166;font-weight:700;margin-top:10px;
 background:rgba(4,20,12,.55);border-radius:12px;padding:2px 12px}
#mine{display:flex;justify-content:space-between;align-items:center;padding:10px 14px;background:#0d1220}
#handline{text-align:center;padding:5px 10px;background:#0d1220;font-size:13px;
 color:#7ddba5;font-weight:700;min-height:19px}
#handline.result{color:#ffd166}
#me{font-weight:700;color:#fff}
#stack{color:#7ddba5;font-size:12px;margin-left:6px}
#acts{display:flex;gap:6px;padding:10px;background:#121927}
button{flex:1;padding:12px 0;border:0;border-radius:8px;font-weight:700;font-size:13px;
 background:#243147;color:#c9d5e8}
button.pri{background:#e8a33d;color:#2b1d05}
button.dng{background:#3a2029;color:#e08a9a}
button:disabled{opacity:.35}
#msg{text-align:center;padding:8px;color:#9fb0c9;font-size:12px;min-height:18px}
/* Pre-action row. Occupies the same slot as #acts and only one of the two is
   ever displayed, so the controls never move under the player's thumb. */
/* Deliberately NOT styled like #acts. The first version looked identical to
   the live action row, so players read the missing Рейз as buttons
   disappearing rather than as a different row doing a different job. */
#prewrap{display:none;padding:6px 10px 10px;background:#121927}
#prewrap.on{display:block}
#prehint{color:#6f7f99;font-size:10px;text-transform:uppercase;letter-spacing:.06em;
 padding:0 2px 5px}
#pre{display:flex;gap:6px}
#pre button{background:transparent;color:#8fa1bd;border:1px dashed #34435c;padding:10px 0}
#pre button.armed{background:#2f4462;color:#ffd166;border:1px solid #ffd166}
#pre button:disabled{opacity:.3}
.waitTag{display:block;margin-top:2px;color:#8fa1bd;font-size:9px;font-style:italic}
/* Table chat */
#chat{background:#0d1220;border-top:1px solid #1a2233}
#chatlog{height:89px;overflow-y:auto;padding:7px 12px;font-size:14px;line-height:1.45}
.cline{margin:1px 0;word-break:break-word}
.cwho{color:#7ddba5;font-weight:700}
#chatrow{display:flex;gap:6px;padding:6px 8px 8px}
#chatinput{flex:1;min-width:0;padding:11px 12px;border:0;border-radius:8px;
 background:#1b2536;color:#e6edf7;font-size:16px}
#chatinput::placeholder{color:#5d6b83}
#chatsend{flex:0 0 52px;padding:9px 0}
/* Raise controls. window.prompt() is unavailable inside Telegram's webview —
   it returns null without ever showing a dialog — so the raise amount has to
   be chosen with real in-page controls. */
#raisebox{display:none;padding:10px;background:#121927;border-top:1px solid #1d2740}
#raisebox.on{display:block}
#raiseval{text-align:center;color:#ffd166;font-weight:700;font-size:15px;margin-bottom:8px}
#raiserange{width:100%;accent-color:#e8a33d;margin:0 0 10px}
.rrow{display:flex;gap:6px;margin-bottom:6px}
.rrow button{padding:9px 0;font-size:12px}
/* Win banner: one shot per hand, purely decorative and pointer-transparent so
   it can never swallow a tap meant for the table underneath. */
#win{position:absolute;inset:0;display:flex;align-items:center;justify-content:center;
 pointer-events:none;opacity:0}
#win.go{animation:winpop 2s ease-out}
#win b{background:rgba(4,10,6,.62);color:#ffd166;font-size:32px;font-weight:800;
 padding:10px 22px;border-radius:14px;box-shadow:0 6px 24px rgba(0,0,0,.5)}
@keyframes winpop{
 0%{opacity:0;transform:scale(.55)}
 18%{opacity:1;transform:scale(1.14)}
 32%{transform:scale(1)}
 78%{opacity:1;transform:scale(1)}
 100%{opacity:0;transform:scale(1.02)}}
</style></head><body>
<div id="bar"><span id="session">♠ Покер</span><span id="blinds"></span>
 <button id="themebtn" title="Колір столу">🎨</button><span id="stage"></span></div>
<div id="themes">
  <button data-felt="felt-green"  style="background:#176b48"></button>
  <button data-felt="felt-blue"   style="background:#1d5f86"></button>
  <button data-felt="felt-purple" style="background:#5a3d80"></button>
  <button data-felt="felt-red"    style="background:#8a2f3a"></button>
  <button data-felt="felt-slate"  style="background:#3c4756"></button>
</div>
<div id="felt"><div id="centre"><div id="board"></div><div id="pot"></div></div><div id="win"><b></b></div></div>
<div id="mine"><span><span id="me"></span><span id="stack"></span></span><span id="hole"></span></div>
<div id="handline"></div>
<div id="acts">
  <button id="btn-fold" class="dng" disabled>Пас</button>
  <button id="btn-check" disabled>Чек</button>
  <button id="btn-call" disabled>Колл</button>
  <button id="btn-raise" class="pri" disabled>Рейз</button>
</div>
<div id="raisebox">
  <div id="raiseval"></div>
  <input id="raiserange" type="range" min="0" max="100" step="10" value="0">
  <div class="rrow">
    <button data-preset="min">Мін</button>
    <button data-preset="half">½ банку</button>
    <button data-preset="pot">Банк</button>
    <button data-preset="all">Ва-банк</button>
  </div>
  <div class="rrow">
    <button id="raise-cancel">Скасувати</button>
    <button id="raise-ok" class="pri">Підтвердити</button>
  </div>
</div>
<div id="prewrap">
  <div id="prehint">Ходить інший гравець · обери дію наперед</div>
  <div id="pre">
    <button data-pre="fold">Пас</button>
    <button data-pre="checkfold">Чек/Пас</button>
    <button data-pre="call">Колл</button>
  </div>
</div>
<div id="msg"></div>
<div id="chat">
  <div id="chatlog"></div>
  <div id="quick"></div>
  <div id="chatrow">
    <input id="chatinput" type="text" maxlength="200" placeholder="Напиши…" autocomplete="off">
    <button id="chatsend">→</button>
  </div>
</div>
<script>
const tg=(window.Telegram&&window.Telegram.WebApp)||null;
if(tg){tg.ready();tg.expand()}
const INIT=(tg&&tg.initData)||"";
// Two ways in. Opened from a group, Telegram serves the fixed @BotFather URL
// with no id in the path and carries the table in startapp, so start_param
// is the only source. Opened at /poker/{id} directly, the templated id wins.
const TABLE={{.TableID}}||((tg&&tg.initDataUnsafe&&tg.initDataUnsafe.start_param)||"");

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
function card(s,made){
  if(!s)return "";
  const suitLetter=s.slice(-1);
  const rank=s.slice(0,-1).replace("T","10");
  const red=suitLetter==="h"||suitLetter==="d";
  return '<span class="card'+(red?' red':'')+(made?' made':'')+'">'+rank+(SUITS[suitLetter]||suitLetter)+'</span>';
}
// The five cards making the viewer's current best hand, as a lookup. Server
// sends them for the VIEWER'S OWN seat only, so highlighting can never
// reveal anything about an opponent.
let madeSet=new Set();
function cardMaybeMade(s){return card(s,madeSet.has(s))}
// Face-down card backs. Each is a FIXED constant; backFor picks between
// them using only the seat's user_id, never anything derived from that
// seat's cards, so a face-down hand still cannot leak through its artwork.
const CARD_BACK='<span class="card back"></span>';
const CARD_BACK_BO='<span class="card back bo">J\u2666</span>';
const DROID='<svg viewBox="0 0 24 18" aria-hidden="true">'+
  '<path d="M5.6 4.2 4.3 2.1a.5.5 0 0 1 .85-.5L6.5 3.8a8.7 8.7 0 0 1 7 0l1.35-2.2a.5.5 0 0 1 .85.5l-1.3 2.1A7.6 7.6 0 0 1 18 10.2H2A7.6 7.6 0 0 1 5.6 4.2Z"/>'+
  '<circle cx="7" cy="7.2" r=".95" fill="#f6d879"/><circle cx="13" cy="7.2" r=".95" fill="#f6d879"/>'+
  '<rect x="2" y="11.2" width="16" height="5.4" rx="1.6"/></svg>';
const CARD_BACK_DROID='<span class="card back droid">'+DROID+'</span>';
function backFor(userID){
  if(userID==="bot:1")return CARD_BACK_BO;
  if(userID==="bot:2")return CARD_BACK_DROID;
  return CARD_BACK;
}
function backsFor(userID){const b=backFor(userID);return b+b}

// Seats are 104px wide at the 1024px+ target, so full bot names fit.
function clip(n){n=n||"";return n.length>17?n.slice(0,17)+"…":n}

function mmss(total){
  total=Math.max(0,Math.floor(total));
  const m=Math.floor(total/60),sec=total%60;
  return m+":"+(sec<10?"0":"")+sec;
}
// Ukrainian needs three plural forms, not two: 1 роздача, 2 роздачі,
// 5 роздач. Picking one and appending "s"-style would read as broken.
function plural(n,one,few,many){
  const m10=n%10,m100=n%100;
  if(m10===1&&m100!==11)return one;
  if(m10>=2&&m10<=4&&(m100<12||m100>14))return few;
  return many;
}

// The bar's two clocks tick locally between snapshots. The server sends
// DURATIONS (elapsed, next_blind_in) rather than timestamps, so a device
// with a wrong clock still counts correctly; these anchors convert them
// back into something tickable.
let clockBase=0,clockAnchor=0,blindBase=-1,handsSeen=0;
function renderBar(v){
  clockBase=v.elapsed||0;
  clockAnchor=Date.now();
  blindBase=(typeof v.next_blind_in==="number")?v.next_blind_in:-1;
  handsSeen=v.hands||0;
  document.getElementById("blinds").textContent=
    "Блайнди "+(v.small_blind||0)+"/"+(v.big_blind||0);
  paintBar();
}
function paintBar(){
  const drift=Math.floor((Date.now()-clockAnchor)/1000);
  const secs=clockBase+(clockAnchor?drift:0);
  document.getElementById("session").textContent=
    "♠ "+mmss(secs)+" · "+handsSeen+" "+plural(handsSeen,"роздача","роздачі","роздач");
  const el=document.getElementById("blinds");
  if(blindBase<0){
    el.classList.remove("rising");
    return;
  }
  const left=Math.max(0,blindBase-drift);
  el.textContent=el.textContent.split(" ↑")[0]+" ↑"+mmss(left);
  // Highlight the last minute before the stakes double.
  el.classList.toggle("rising",left<=60);
}

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
// Latch so the win banner plays once per hand — see render().
let winShown=false;

// Queued action to play the moment it becomes our turn: null | "fold" |
// "checkfold" | "call". preAmt records the call price AT ARM TIME so a raise
// arriving in between cancels the call instead of silently committing to a
// bigger number than the player agreed to.
let pre=null,preAmt=0;
function clearPre(){
  pre=null;preAmt=0;
  document.querySelectorAll("#pre button").forEach(b=>b.classList.remove("armed"));
}

// Renders the chat log. Every message part goes in via textContent — names
// and text are player-controlled, so this must never touch innerHTML.
function renderChat(msgs){
  const box=document.getElementById("chatlog");
  // Only auto-scroll when already pinned to the bottom, so reading back
  // through the log is not yanked away by someone else's message.
  const pinned=box.scrollTop+box.clientHeight>=box.scrollHeight-8;
  box.textContent="";
  (msgs||[]).forEach(m=>{
    const line=document.createElement("div");
    line.className="cline";
    const who=document.createElement("span");
    who.className="cwho";
    who.textContent=(m.name||"?")+": ";
    line.appendChild(who);
    line.appendChild(document.createTextNode(m.text||""));
    box.appendChild(line);
  });
  if(pinned)box.scrollTop=box.scrollHeight;
}

// Replays the CSS animation from the start: removing the class alone is not
// enough, the reflow between remove and add is what restarts it.
function showWin(n){
  const el=document.getElementById("win");
  el.firstElementChild.textContent="+"+n+" 🪙";
  el.classList.remove("go");
  void el.offsetWidth;
  el.classList.add("go");
}

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
  // Never leave the sizing panel open past our own turn: the bounds it was
  // built from are stale the moment the street or the high bet moves, and
  // confirming from a stale panel just earns an ErrRaiseTooLow.
  if(!myTurn)closeRaise();

  // Pre-action row is offered only while we are in a live hand and it is
  // someone else's turn. A folded or all-in seat has nothing left to queue.
  // Not dealt in means nothing to pre-select: the queued action could only
  // fire on a LATER hand, against a price that no longer exists.
  const canQueue=live&&!!me&&me.in_hand&&!myTurn&&!me.folded&&!me.all_in;
  document.getElementById("prewrap").classList.toggle("on",canQueue);
  document.getElementById("acts").style.display=canQueue?"none":"flex";
  if(!canQueue&&!myTurn)clearPre();
  const preCall=document.querySelector('#pre button[data-pre="call"]');
  preCall.textContent=toCall>0?("Колл "+toCall):"Колл";
  // Nothing to call yet: arming it would be meaningless, and Чек/Пас
  // already covers the free-to-check case.
  preCall.disabled=toCall<=0;
}

// Plays a queued action once it is genuinely our turn. The intent is
// re-evaluated against the CURRENT state, never the state it was armed in:
// «Чек/Пас» folds if a bet appeared, and «Колл» cancels outright if the
// price moved, so a pre-action can never commit more chips than the player
// saw when they tapped it.
function maybeFirePre(){
  if(!pre||!state)return;
  const seats=state.seats||[];
  const me=state.you_seat>=0?seats[state.you_seat]:null;
  const live=state.stage!=="waiting"&&state.stage!=="showdown";
  if(!live||!me||me.folded){clearPre();return}
  if(!me.to_act)return;
  const toCall=Math.max(0,(state.high_bet||0)-(me.bet||0));
  const choice=pre,armed=preAmt;
  clearPre(); // clear BEFORE acting: act() renders again, and a still-armed
              // pre would re-enter this function from that render.
  if(choice==="fold")act("fold");
  else if(choice==="checkfold")act(toCall>0?"fold":"check");
  else if(choice==="call"){
    if(toCall===armed)act("call");
    else setError("Ставка змінилась — колл скасовано");
  }
}

function render(v){
  // Chat first, and NOT behind the seq gate below. A chat message does not
  // mutate the table, so it never bumps seq — gating it would mean chat
  // only ever appeared alongside an unrelated game action. Bumping seq on
  // chat instead was rejected: seq is the action-ordering token, and moving
  // it would 409 every player's pending action each time someone typed.
  renderChat(v.chat);
  // Also outside the gate: a chat-only broadcast carries no new seq, and
  // the bar should still re-anchor its clocks from it.
  renderBar(v);
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
  madeSet=new Set(v.hand_cards||[]);
  document.getElementById("board").innerHTML=v.board.map(cardMaybeMade).join("");
  document.getElementById("pot").textContent="🪙 Банк "+potShown;

  const felt=document.getElementById("felt");
  felt.querySelectorAll(".seat").forEach(e=>e.remove());
  // cy/ry are tuned against #centre's top: the viewer's seat sits at
  // cy+ry, and the board+pot block ends around top+61px. They collided
  // when the felt shrank to make room for the chat log, hiding the pot
  // behind the bottom seat.
  const n=seats.length,cx=50,cy=44,rx=38,ry=32;
  const left=Math.max(0,v.deadline-Math.floor(Date.now()/1000));
  // Seats are placed RELATIVE to the viewer, who always sits at the bottom
  // of the oval (+PI/2) with everyone else running clockwise from there —
  // the orientation every poker client uses. Seat order round the table is
  // preserved because only the starting offset changes. A spectator with no
  // seat (you_seat < 0) falls back to seat 0 at the bottom.
  const meIdx=v.you_seat>=0?v.you_seat:0;
  const myUserID=v.you_seat>=0?seats[v.you_seat].user_id:null;
  seats.forEach((s,i)=>{
    const ang=(Math.PI/2)+(2*Math.PI*((i-meIdx+n)%n)/n);
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
    // Face-down backs belong ONLY to a seat actually holding cards. A
    // player who sat down mid-hand has in_hand=false and no hole cards;
    // drawing backs for them showed cards that do not exist and made the
    // empty hand look like a bug rather than a wait.
    if(s.in_hand&&!s.folded&&v.stage!=="waiting"){
      const hole=document.createElement("div");
      hole.className="oppHole";
      hole.innerHTML=s.hole?s.hole.map(c=>card(c,s.user_id===myUserID&&madeSet.has(c))).join(""):backsFor(s.user_id);
      d.appendChild(hole);
    }else if(!s.in_hand&&v.stage!=="waiting"){
      const wait=document.createElement("div");
      wait.className="waitTag";
      wait.textContent="чекає";
      d.appendChild(wait);
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
  // Showdown result outranks the running hand read: at showdown the point
  // is who won, not what you were holding.
  const handEl=document.getElementById("handline");
  if(v.stage==="showdown"&&v.winners&&v.winners.length){
    handEl.classList.add("result");
    handEl.textContent=v.winners.join(", ")+
      (v.winners.length>1?" ділять банк":" виграє")+
      (v.win_hand?" · "+v.win_hand:"");
  }else if(v.hand_name&&live){
    handEl.classList.remove("result");
    handEl.textContent="У тебе: "+v.hand_name;
  }else{
    handEl.classList.remove("result");
    handEl.textContent="";
  }

  // Showdown can render more than once (the action response and a broadcast
  // both carry it, and later bookkeeping bumps seq again), so the banner is
  // latched and only rearmed once the next hand leaves showdown.
  if(v.stage!=="showdown")winShown=false;
  else if(!winShown){
    winShown=true;
    const won=me?(me.won||0):0;
    if(won>0)showWin(won);
  }
  document.getElementById("me").textContent=me?clip(me.name):"";
  document.getElementById("stack").textContent=me?me.stack:"";
  const holeEl=document.getElementById("hole");
  if(me&&me.hole&&me.hole.length){
    holeEl.innerHTML=me.hole.map(cardMaybeMade).join("");
  }else if(me&&!me.in_hand&&live){
    // Sat down after the deal: explain the empty hand instead of leaving a
    // blank space that reads as a failure to load.
    holeEl.innerHTML="";
    holeEl.textContent="Чекаєш наступної роздачі";
  }else{
    holeEl.innerHTML="";
  }

  applyButtons();
  tick();
  // Last, so it acts on a fully rendered, current snapshot.
  maybeFirePre();
}

function tick(){
  paintBar(); // the session clock keeps running even between hands
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
// Raise sizing. The server's "amount" is the TOTAL this seat should have
// committed on the current street, not the increment, so every number here
// is a street total.
//
// window.prompt() used to collect it. Telegram's webview does not implement
// prompt — it returns null immediately without ever showing a dialog — so
// the old handler silently did nothing on every tap. These controls replace
// it; nothing else about the raise request changed.
const raiseBox=document.getElementById("raisebox");
const raiseRange=document.getElementById("raiserange");
const raiseVal=document.getElementById("raiseval");

// min_raise and high_bet come from the server: min_raise is the smallest
// legal increment over high_bet and widens after a raise, so deriving it
// client-side as "+BigBlind" produced amounts the engine rejected with
// ErrRaiseTooLow.
function raiseBounds(){
  if(!state)return null;
  const seats=state.seats||[];
  const me=state.you_seat>=0?seats[state.you_seat]:null;
  if(!me)return null;
  const high=state.high_bet||0;
  const cap=me.stack+me.bet;              // going all-in, as a street total
  // A stack too short for a full raise can still shove: the engine allows
  // amount == stack+bet even when that is under high+min_raise.
  const min=Math.min(high+(state.min_raise||0),cap);
  return {min:min,max:cap,pot:state.pot||0,high:high};
}
function setRaise(v){
  const b=raiseBounds();
  if(!b)return;
  v=Math.round(Math.max(b.min,Math.min(b.max,v)));
  raiseRange.value=String(v);
  raiseVal.textContent="Рейз до "+v+" 🪙"+(v>=b.max?" (ва-банк)":"");
}
function openRaise(){
  const b=raiseBounds();
  if(!b)return;
  // A seat with nothing behind cannot raise at all; max would equal the
  // current bet and the slider would have no range.
  if(b.max<=b.high){setError("Нема на що рейзити");return}
  raiseRange.min=String(b.min);
  raiseRange.max=String(b.max);
  raiseRange.step="10";
  setRaise(b.min);
  raiseBox.classList.add("on");
}
function closeRaise(){raiseBox.classList.remove("on")}

raiseRange.oninput=()=>setRaise(parseInt(raiseRange.value,10)||0);
document.querySelectorAll("#raisebox [data-preset]").forEach(btn=>{
  btn.onclick=()=>{
    const b=raiseBounds();
    if(!b)return;
    const p=btn.getAttribute("data-preset");
    if(p==="min")setRaise(b.min);
    else if(p==="half")setRaise(b.high+Math.round(b.pot/2));
    else if(p==="pot")setRaise(b.high+b.pot);
    else setRaise(b.max);
  };
});
document.getElementById("raise-cancel").onclick=closeRaise;
document.getElementById("raise-ok").onclick=()=>{
  const amt=parseInt(raiseRange.value,10);
  closeRaise();
  if(Number.isFinite(amt))act("raise",amt);
};
document.getElementById("btn-raise").onclick=openRaise;

document.querySelectorAll("#pre button").forEach(btn=>{
  btn.onclick=()=>{
    const choice=btn.getAttribute("data-pre");
    const armed=(pre===choice);
    clearPre();
    if(armed)return; // second tap on the armed option cancels it
    pre=choice;
    if(choice==="call"){
      const seats=state?(state.seats||[]):[];
      const me=state&&state.you_seat>=0?seats[state.you_seat]:null;
      preAmt=me?Math.max(0,(state.high_bet||0)-(me.bet||0)):0;
    }
    btn.classList.add("armed");
  };
});

const chatInput=document.getElementById("chatinput");
async function sendChat(){
  const text=chatInput.value.trim();
  if(!text)return;
  chatInput.value="";
  try{
    const r=await fetch("/api/poker/"+TABLE+"/chat",{
      method:"POST",
      headers:{"Content-Type":"application/json","X-Telegram-Init-Data":INIT},
      body:JSON.stringify({text:text})
    });
    if(!r.ok){setError(await r.text());return}
    render(await r.json());
  }catch(e){setError("Зʼєднання втрачено…")}
}
// Quick reactions. Each is an ordinary chat message, so it goes through the
// same endpoint, the same length cap and the same per-user cooldown — a
// tap-happy player cannot outrun the limit any more than a typing one.
const QUICK=["\ud83d\ude02","\ud83e\udd21","\ud83d\udd25","\ud83d\ude2d","\ud83d\ude21","\ud83e\udd14","\ud83d\udc4d","\ud83d\udca9"];
const quickRow=document.getElementById("quick");
QUICK.forEach(e=>{
  const b=document.createElement("button");
  b.type="button";
  b.textContent=e;             // textContent, not innerHTML — same rule as chat
  b.onclick=()=>{chatInput.value=e;sendChat()};
  quickRow.appendChild(b);
});

// Felt colour. Stored in this browser only; nothing about it is sent to the
// server, so it is a personal preference rather than a table setting.
const FELT_KEY="poker.felt";
const themesRow=document.getElementById("themes");
function applyFelt(name){
  document.body.className=name||"felt-green";
  themesRow.querySelectorAll("button").forEach(b=>
    b.classList.toggle("sel",b.getAttribute("data-felt")===(name||"felt-green")));
}
function savedFelt(){
  // Private-mode webviews can throw on storage access rather than return
  // null, so a failure here must fall back to the default, not break the
  // page before the table ever loads.
  try{return localStorage.getItem(FELT_KEY)}catch(e){return null}
}
themesRow.querySelectorAll("button").forEach(b=>{
  b.onclick=()=>{
    const name=b.getAttribute("data-felt");
    applyFelt(name);
    try{localStorage.setItem(FELT_KEY,name)}catch(e){}
    themesRow.classList.remove("on");
  };
});
document.getElementById("themebtn").onclick=()=>themesRow.classList.toggle("on");
applyFelt(savedFelt());

document.getElementById("chatsend").onclick=sendChat;
chatInput.onkeydown=e=>{if(e.key==="Enter"){e.preventDefault();sendChat()}};

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
	ch     chan tableEnvelope
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

	// chat holds each table's recent messages, newest last, capped at
	// chatHistory. Ephemeral by design: it shares the table's lifetime and
	// is dropped when the sweeper reclaims one. Guarded by h.mu.
	chat map[string][]chatMsg

	// lastChatAt is the per-user cooldown clock for chat. Keyed by userID
	// rather than (table, user) so a spammer cannot dodge it by opening a
	// second table. Guarded by h.mu.
	lastChatAt map[string]time.Time
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
		chat:            map[string][]chatMsg{},
		lastChatAt:      map[string]time.Time{},
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
		case "chat":
			h.handleChat(w, r, tbl, uid, firstName, username)
		default:
			http.NotFound(w, r)
		}
	})
	// Registered under both spellings so the URL configured with @BotFather
	// works with or without its trailing slash. With only the "/poker/"
	// subtree pattern, a request to "/poker" gets a 301 from the mux — and a
	// redirect can drop the "#tgWebAppData=..." fragment that carries
	// initData, leaving the app unauthenticated for a reason nothing logs.
	page := func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/poker"), "/")
		// An empty id is the normal case for a Mini App opened from a
		// group: Telegram serves the fixed URL registered with @BotFather
		// and delivers the table id as initDataUnsafe.start_param instead,
		// which only the client can read. Serve the shell and let it
		// resolve the id — every /api/poker route still authorizes the id
		// the client ends up using, so this is not a way in.
		if id != "" && h.table(id) == nil {
			http.Error(w, "Стіл закрито", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pokerTmpl.Execute(w, map[string]string{"TableID": id})
	}
	mux.HandleFunc("/poker", page)
	mux.HandleFunc("/poker/", page)
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
		view := h.envelope(tbl, userID)
		tbl.Unlock()
		h.touch(tbl.ID) // reconnecting is real player-initiated activity
		writeJSON(w, view)
		return
	}
	tbl.Unlock()

	// A claim pointing at some OTHER table is usually stale rather than
	// real: the player ran /poker again, which creates a fresh table, while
	// their old one sits idle for up to thirty minutes before the sweeper
	// reclaims it. Without this they are locked out of their own new table
	// for that whole window with no way to clear it — the "Ти вже за іншим
	// столом" dead end. Genuinely being mid-hand elsewhere still blocks.
	h.releaseStaleClaim(userID, tbl.ID)

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
	// Make room for a genuinely new human at a full table by evicting one
	// bot, so bots occupying every seat can never silently defeat "humans
	// get priority". This runs only between hands (Stage is StageWaiting —
	// this table has never dealt a hand — or StageShowdown, right after a
	// hand's chips have already been distributed by settle()): removing a
	// seat mid-hand would both disturb the Button/ToAct indices the engine
	// derives from seat position AND drop that seat's still-live Committed
	// chips out of the pot BuildPots computes at showdown, silently
	// destroying real money. A reconnecting player never reaches this point
	// (see the early-return above), so this can only ever cost a BOT its
	// seat, never a human who is already seated.
	if (tbl.Stage == poker.StageWaiting || tbl.Stage == poker.StageShowdown) && len(tbl.Seats) >= poker.MaxSeats {
		h.evictOneBot(tbl)
	}
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
	// ensureBots runs BEFORE the SeatedCount() >= 2 guard below (not folded
	// into it): a lone human is one seat, and gating ensureBots itself on
	// >= 2 would mean it never runs in exactly the case bots exist for.
	if tbl.Stage == poker.StageWaiting {
		h.ensureBots(tbl)
		if tbl.SeatedCount() >= 2 {
			_ = tbl.StartHand()
		}
	}
	view := h.envelope(tbl, userID)
	h.broadcast(tbl) // called with the table lock held, per lock ordering
	tbl.Unlock()
	h.touch(tbl.ID)

	writeJSON(w, view)
}

// evictOneBot removes one bot seat to make room for an arriving human at a
// full table. It repairs both tbl.Button and tbl.ToAct so they stay valid
// indices into the shortened Seats slice: if the evicted seat is the one
// tbl.ToAct points at — a stale index left over from the just-finished
// hand's last action, still a valid Seats index at StageShowdown — ToAct is
// reset to -1 rather than skipping that seat, since at StageWaiting and
// StageShowdown (the only stages this ever runs in — see handleJoin) ToAct
// is never read for money; the money-relevant path is Act/ForceTimeout
// during live betting, which this function never touches. Leaving a lone
// evictable bot un-evicted just because it happened to sit at a stale
// ToAct would otherwise reject a sixth human with "стіл заповнений" even
// though a seat is free to take. The caller MUST hold the table lock and
// must only call this between hands. Reports whether a bot was removed.
func (h *PokerHub) evictOneBot(tbl *poker.Table) bool {
	for i, s := range tbl.Seats {
		if !isBotUser(s.UserID) {
			continue
		}
		tbl.Seats = append(tbl.Seats[:i], tbl.Seats[i+1:]...)
		if tbl.Button >= i {
			tbl.Button--
		}
		if tbl.Button < 0 {
			tbl.Button = len(tbl.Seats) - 1
		}
		switch {
		case tbl.ToAct == i:
			tbl.ToAct = -1
		case tbl.ToAct > i:
			tbl.ToAct--
		}
		tbl.Seq++
		return true
	}
	return false
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

	view := h.envelope(tbl, userID)
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
	sub := &subscriber{userID: userID, ch: make(chan tableEnvelope, 4), done: make(chan struct{})}
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
	initial := h.envelope(tbl, userID)
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

func sendView(w http.ResponseWriter, f http.Flusher, v tableEnvelope) {
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
	// Read the chat log once, under the lock we already hold. Calling
	// chatSnapshot here would try to take h.mu a second time and deadlock;
	// sync.Mutex is not reentrant. One read serves every subscriber because
	// chat, unlike the table view, is not redacted per viewer.
	msgs := h.chatLocked(tbl.ID)
	for _, s := range h.subs[tbl.ID] {
		select {
		case s.ch <- tableEnvelope{TableView: tbl.ViewFor(s.userID), Chat: msgs}:
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

// botInterval paces bot actions. It is deliberately NOT sweepInterval: bots
// used to act on the 5s housekeeping tick, one action per pass, so a hand
// with two bots spent roughly ten seconds per betting round — about
// forty-five seconds of a hand with nothing happening. The pause should read
// as deliberation, not as a hang.
const botInterval = 1200 * time.Millisecond

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
	// Bots run on their own, much faster clock. Housekeeping (idle
	// reclamation, forced timeouts, dealing the next hand) stays on the 5s
	// sweep, where its cost is irrelevant and its pauses are wanted.
	go func() {
		for range time.Tick(botInterval) {
			h.botTickOnce()
		}
	}()
}

// botTickOnce gives one bot per table its turn. Same two-phase locking as
// sweepOnce: snapshot the table list under h.mu, release it, then take each
// table lock separately — never the reverse, which would invert the
// documented ordering against broadcast().
func (h *PokerHub) botTickOnce() {
	h.mu.Lock()
	tables := make([]*poker.Table, 0, len(h.tables))
	for _, t := range h.tables {
		tables = append(tables, t)
	}
	h.mu.Unlock()

	for _, tbl := range tables {
		tbl.Lock()
		h.botStep(tbl)
		tbl.Unlock()
	}
}

// botStep lets a single bot act, settling and broadcasting if that action
// ended the hand. Caller must hold tbl's lock and not h.mu.
//
// Both tickers funnel through here on purpose: the settle-on-transition
// condition is a money rule, and having it written twice is how the two
// copies eventually disagree.
func (h *PokerHub) botStep(tbl *poker.Table) {
	// Nothing to do between hands, and acting during the showdown pause
	// would cut short the only moment the reveal exists for.
	if tbl.Stage == poker.StageWaiting || tbl.Stage == poker.StageShowdown {
		return
	}
	prev := tbl.Stage
	if !h.actBots(tbl) {
		return
	}
	if tbl.Stage == poker.StageShowdown && prev != poker.StageShowdown {
		h.settle(tbl)
	}
	h.broadcast(tbl)
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
		case tbl.Stage == poker.StageShowdown && h.showdownReady(id):
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
			// reveal, the only moment it exists for.
			//
			// ensureBots runs BEFORE the SeatedCount() >= 2 check below, not
			// folded into this case's own guard: if both bots busted out
			// last hand, SeatedCount() can drop to 1 with a lone human left,
			// and gating entry to this case on >= 2 would stop ensureBots
			// from ever running to rebuy/reseat them — wedging the table in
			// exactly the situation bots exist to prevent.
			h.ensureBots(tbl)
			// hasActiveHuman guards against a bot-only hand: a solo human
			// who busts to 0 still occupies a seat (ensureBots' humans
			// count includes them, so it makes no changes), but the two
			// bots they were playing against still hold chips, so
			// SeatedCount() alone stays >= 2 forever. That's money-safe —
			// the busted human is excluded from settlement and the bots'
			// deltas cancel — but it deals a pointless bot-only hand every
			// sweep interval until the 30-minute idle reclaim. Requiring at
			// least one non-bot seat with chips closes that off without
			// touching the (separately reviewed) money path.
			if tbl.SeatedCount() >= 2 && hasActiveHuman(tbl) {
				// Also deliberately NOT h.touch(id): an auto-started hand
				// with nobody acting is still just the sweeper auto-folding
				// forever, same reasoning as the ForceTimeout case above —
				// it must not keep an abandoned table alive.
				if err := tbl.StartHand(); err == nil {
					h.broadcast(tbl)
				}
			}
		}

		// Let a bot to act take its turn, independent of the two cases
		// above: a bot may be first to act immediately after StartHand()
		// just dealt above, or simply be mid-hand on a later pass where
		// neither case applied this tick. Guarded to active betting stages
		// only, both because actBots has nothing to do between hands and to
		// avoid a pointless re-broadcast during the showdown-reveal pause.
		// One call per pass — see actBots's own doc comment — so a bot's
		// turn resolves within a sweep interval rather than looping to
		// completion inside this lock hold.
		// Kept here as well as on the bot ticker: a hand dealt by the
		// StartHand branch just above may have a bot first to act, and this
		// lets it move without waiting for the next bot tick. botStep is
		// idempotent when no bot is to act, so the two callers cannot
		// double-act.
		h.botStep(tbl)

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
		h.dropChat(id) // chat shares the table's lifetime; don't leak the log
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

// releaseStaleClaim drops userID's hub-wide seat claim when the table it
// points at no longer has real money riding on them, and stands them up
// from that table so the abandoned seat cannot keep paying blinds into
// settlements against a bankroll that has moved on.
//
// Locking follows the hub's rule the same way sweepOnce does: h.mu alone to
// resolve the old table, released, then that table's lock, released, then
// h.mu alone again to mutate the claim. h.mu is never held across a table
// lock, which would invert the ordering broadcast() depends on.
//
// The claim is re-checked under the final lock before deleting: between the
// two h.mu sections the player may have joined somewhere else entirely, and
// clobbering that newer claim would hand them two funded seats — exactly
// what seatedAt exists to prevent.
func (h *PokerHub) releaseStaleClaim(userID, wantTableID string) {
	h.mu.Lock()
	existing, has := h.seatedAt[userID]
	var old *poker.Table
	if has && existing != wantTableID {
		old = h.tables[existing]
	}
	h.mu.Unlock()

	if !has || existing == wantTableID {
		return
	}

	if old != nil {
		old.Lock()
		if old.HasLiveStake(userID) {
			old.Unlock()
			return // really is mid-hand elsewhere: the 409 is correct
		}
		stoodUp := old.StandUp(userID)
		if stoodUp {
			h.broadcast(old)
		}
		old.Unlock()
	}

	h.mu.Lock()
	if h.seatedAt[userID] == existing {
		delete(h.seatedAt, userID)
	}
	h.mu.Unlock()
}
