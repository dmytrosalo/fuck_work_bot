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
#felt{position:relative;height:54vh;min-height:420px;overflow:hidden;
 background:radial-gradient(ellipse at 50% 45%,#12202b,#0b141c 70%,#080f15)}
/* The table is now a real oval sitting inside the felt area, so the
   background photo or colour shows around it as the room rather than being
   the table surface itself. */
#oval{position:absolute;left:6%;right:6%;top:12%;bottom:14%;border-radius:50%;
 background:radial-gradient(ellipse at 50% 42%,var(--f1),var(--f2) 68%,var(--f3));
 border:10px solid #2a1c10;
 box-shadow:0 0 0 3px #4a3520 inset,0 0 0 6px rgba(0,0,0,.35),0 14px 34px rgba(0,0,0,.55)}
#oval::after{content:"";position:absolute;inset:10px;border-radius:50%;
 box-shadow:0 0 26px rgba(0,0,0,.4) inset;pointer-events:none}
:root{--f1:#1e7350;--f2:#124b35;--f3:#0d3626}
/* Felt themes. Purely cosmetic and per-player: the choice lives in this
   browser only and is never sent anywhere, so two players at one table can
   pick different colours without either seeing the other's. */
.felt-green {--f1:#1e7350;--f2:#124b35;--f3:#0d3626}
.felt-blue  {--f1:#1d5f86;--f2:#123f5c;--f3:#0c2b41}
.felt-purple{--f1:#5a3d80;--f2:#3b2757;--f3:#281a3c}
.felt-red   {--f1:#8a2f3a;--f2:#5c1f27;--f3:#3f151b}
.felt-slate {--f1:#3c4756;--f2:#28303b;--f3:#1b2129}
#themebtn,#sndbtn,#radiobtn,#avbtn,#histbtn,#leavebtn{background:none;border:0;padding:0 2px;
 font-size:13px;cursor:pointer;flex:0 0 auto}
#history{display:none;padding:8px 10px;background:#151c2b;max-height:210px;overflow-y:auto}
#history.open{display:block}
.hand{padding:6px 2px 7px;border-bottom:1px solid #1d2740}
.hand:last-child{border-bottom:0}
.hhead{display:flex;align-items:center;gap:8px;font-size:12px}
.hhead .hno{color:#5d6b83;flex:0 0 40px;font-size:11px}
.hhead .hb{flex:1;display:flex;gap:2px}
.hhead .hp{color:#ffd166;font-weight:700;flex:0 0 auto}
.hb span,.hpc span{background:#fff;color:#111;border-radius:3px;padding:0 3px;font-size:11px;font-weight:700}
.hb span.red,.hpc span.red{color:#d62828}
.hpl{display:flex;align-items:center;gap:7px;padding:3px 0 0 40px;font-size:11px}
.hpl.folded{opacity:.5}
.hpl .hn{flex:0 0 110px;color:#e6edf7;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.hpl.won .hn{color:#ffd166;font-weight:700}
.hpl .hpc{flex:0 0 auto;display:flex;gap:2px}
.hpl .hcm{flex:1;color:#7ddba5}
.hpl .hd{flex:0 0 auto;font-weight:700}
.hpl .hd.up{color:#7ddba5}
.hpl .hd.dn{color:#e08a9a}
.hnoboard{color:#5d6b83;font-size:11px;font-style:italic}
#history .empty{color:#5d6b83;font-size:12px;text-align:center;padding:8px}
#radiobtn.on{filter:drop-shadow(0 0 5px #ffd166)}
#radio{display:none;padding:8px 10px;background:#151c2b}
#radio.open{display:block}
#radio .stations{display:flex;flex-wrap:wrap;gap:6px}
#radio button{flex:1 1 auto;min-width:132px;padding:9px 10px;background:#243147;color:#c9d5e8;
 border-radius:8px;font-size:12px}
#radio button.playing{background:#2f4462;color:#ffd166;outline:1px solid #ffd166}
#radio .credit{color:#5d6b83;font-size:10px;padding-top:7px;text-align:center}
#avatars{display:none;flex-wrap:wrap;gap:8px;justify-content:center;padding:9px 10px;background:#151c2b}
#avatars.open{display:flex}
#avatars button{flex:0 0 46px;height:46px;padding:0;font-size:24px;border-radius:50%;
 background:#1b2536;border:2px solid #46536b}
#avatars button.sel{border-color:#ffd166;box-shadow:0 0 10px rgba(255,209,102,.5)}
#sndbtn.off{opacity:.4}
#themes{display:none;gap:6px;padding:6px 10px;background:#151c2b;justify-content:center}
#themes.on{display:flex}
#themes button{flex:0 0 30px;height:24px;padding:0;border-radius:6px;border:2px solid transparent}
#themes button.sel{border-color:#ffd166}
#feltrandom{background:#243147;color:#e6edf7;font-size:13px;line-height:1}
/* Quick reactions: one tap sends the emoji as an ordinary chat message. */
#quick{display:flex;gap:5px;padding:0 8px 6px;overflow-x:auto}
/* Buy-in chooser, shown over the felt before the first sit. */
#buyin{position:absolute;inset:0;display:none;flex-direction:column;
 align-items:center;justify-content:center;gap:12px;background:rgba(6,14,10,.86);z-index:5}
#buyin.on{display:flex}
#buyin h3{margin:0;font-size:16px;color:#e6edf7}
#buyin .opts{display:flex;flex-wrap:wrap;gap:8px;justify-content:center;max-width:88%}
#buyin button{flex:0 0 auto;min-width:96px;padding:12px 14px;background:#e8a33d;color:#2b1d05;
 border-radius:10px;font-size:14px}
#buyin button.alt{background:#243147;color:#c9d5e8}
#buyin .bal{color:#8fa1bd;font-size:12px}
#quick button{flex:0 0 auto;min-width:40px;padding:6px 0;font-size:18px;
 background:#1b2536;border-radius:8px}
.seat{position:absolute;width:104px;margin-left:-52px;margin-top:-34px;text-align:center;font-size:11px;
 transition:opacity .2s}
.seat .av{width:44px;height:44px;margin:0 auto -12px;border-radius:50%;
 background:#1b2536;border:2px solid #46536b;display:flex;align-items:center;
 justify-content:center;font-size:24px;position:relative;z-index:2}
.seat .plaque{background:rgba(10,16,26,.92);border:1px solid #33415a;border-radius:9px;
 padding:13px 5px 5px;position:relative;z-index:1}
.seat .nm{font-weight:700;color:#fff;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;
 font-size:11px;line-height:1.2}
.seat .st{color:#7ddba5;font-size:12px;font-weight:700}
.seat.folded{opacity:.4}
.seat.act .av{border-color:#ffd166;box-shadow:0 0 12px rgba(255,209,102,.6)}
.seat.act .plaque{border-color:#ffd166;background:#1d2740}
.seat.act .nm{color:#ffd166}
.seat .allin{display:block;color:#ff9d6b;font-size:9px;font-weight:700;letter-spacing:.04em}
.cd{display:block;margin-top:2px;background:#ffd166;color:#2b1d05;border-radius:8px;
 padding:0 5px;font-size:10px;font-weight:700}
/* A bet sits out on the cloth between its player and the pot, the way it
   does on a real table — inside the seat plaque it read as part of the
   stack rather than as chips already committed. */
.bet{position:absolute;transform:translate(-50%,-50%);z-index:3;
 background:rgba(8,16,12,.72);color:#ffd166;border:1px solid rgba(255,209,102,.45);
 border-radius:11px;padding:2px 8px;font-size:11px;font-weight:700;white-space:nowrap;
 box-shadow:0 2px 6px rgba(0,0,0,.45)}
.bet i{font-style:normal;margin-right:3px}
.oppHole{margin:2px 0}
.oppHole .card{width:35px;height:49px;line-height:49px;font-size:20px;margin:0 2px;border-radius:6px}
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
.card{display:inline-block;background:#fff;border-radius:7px;width:58px;height:80px;line-height:80px;
 text-align:center;font-size:30px;font-weight:700;margin:0 3px;color:#111;box-shadow:0 2px 5px rgba(0,0,0,.5)}
/* Target is a 1024px-wide viewport and up, where five board cards clear the
   side seats comfortably. Below that they cannot, so step back down rather
   than let the board and the seats collide. */
@media (max-width:760px){
 .card{width:34px;height:47px;line-height:47px;font-size:18px;margin:0 2px}
 .oppHole .card{width:21px;height:29px;line-height:29px;font-size:12px}
 .seat{width:74px;margin-left:-37px}
 /* #felt clips (overflow:hidden) and bottoms out at min-height:420px, which
    leaves only ~92px above a top-row seat. At the desktop size the bubble
    lost its top edge to that clip, so it shrinks here to stay whole. */
 .blame{width:84px;height:84px}
 .blame b{font-size:10px;line-height:1.1}
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
/* Data Android God keeps a hookah at his seat, and puffs it when he takes a
   pot. Both layers are decoration only: pointer-transparent and stacked
   under the cards, so neither can swallow a tap meant for the table --
   the same guarantee #win documents above. */
.hookah{position:absolute;right:-42px;bottom:-2px;width:48px;height:48px;
 opacity:.95;pointer-events:none;z-index:2;
 filter:drop-shadow(0 2px 4px rgba(0,0,0,.5))}
.hookah svg{width:100%;height:100%;display:block}
/* Two puffs off the bowl, the second offset so they read as a rhythm
   rather than one blob. They hang off the seat, which render() rebuilds
   every snapshot -- see smokeUntil for why that is safe. */
.hookah::before,.hookah::after{content:"";position:absolute;left:17px;top:-6px;
 width:14px;height:14px;border-radius:50%;background:rgba(226,238,255,.6);
 /* The blur is what turns these from two grey dots into smoke. */
 filter:blur(3px);opacity:0;pointer-events:none}
.seat.smoking .hookah::before{animation:hookahpuff 3.4s ease-out}
.seat.smoking .hookah::after{animation:hookahpuff 3.4s ease-out .8s}
@keyframes hookahpuff{
 0%{opacity:0;transform:translate(0,0) scale(.4)}
 20%{opacity:.8}
 100%{opacity:0;transform:translate(-13px,-54px) scale(3.2)}}
/* The haze is one wide, very soft band drifting across the felt. It is
   static markup rather than a per-render node so it survives the seat
   rebuild, and sits directly after #oval so it floats over the table but
   under the board cards and the seat plaques. */
/* Five overlapping blobs rather than one centred wash: a single gradient
   reads as "the table got slightly lighter", while lumpy overlapping ones
   read as actual smoke. The blur is what fuses them into cloud. */
.haze{position:absolute;inset:0;pointer-events:none;opacity:0;filter:blur(11px);
 background:
  radial-gradient(26% 32% at 28% 44%,rgba(232,244,255,.62),transparent 70%),
  radial-gradient(32% 38% at 56% 62%,rgba(216,234,255,.58),transparent 72%),
  radial-gradient(22% 28% at 74% 40%,rgba(236,246,255,.50),transparent 70%),
  radial-gradient(38% 28% at 44% 76%,rgba(212,230,255,.46),transparent 74%),
  radial-gradient(20% 26% at 86% 66%,rgba(226,240,255,.40),transparent 72%)}
.haze.go{animation:hookahhaze 6.5s ease-out}
/* Opacity holds a plateau from 14% to 68% instead of touching 1 for an
   instant. The earlier curve peaked and immediately fell away, which on a
   phone read as nothing having happened at all. */
@keyframes hookahhaze{
 0%{opacity:0;transform:translate(-15%,8%) scale(.7)}
 14%{opacity:1}
 68%{opacity:1}
 100%{opacity:0;transform:translate(13%,-6%) scale(1.42)}}
/* When a hand goes badly a bot blames someone who is not at the table.
   The bubble is decoration like the smoke: pointer-transparent, under the
   cards, and rationed so it stays a joke rather than a ticker. It is filled
   rather than left as bare outline -- the source icon is a stroke-only
   shape, and unfilled it is unreadable over the felt. */
.blame{position:absolute;left:50%;bottom:calc(100% - 8px);
 transform:translateX(-50%);width:116px;height:116px;
 pointer-events:none;z-index:4;opacity:0;color:#cfe0f5;
 filter:drop-shadow(0 3px 7px rgba(0,0,0,.55))}
.blame svg{position:absolute;inset:0;width:100%;height:100%;display:block}
/* The icon's box runs 4..20 by 4..16 in a 24x24 viewBox, so the text sits
   in that band and never spills onto the tail. */
.blame b{position:absolute;left:16.7%;right:16.7%;top:16.7%;height:50%;
 display:flex;align-items:center;justify-content:center;text-align:center;
 font-size:12px;font-weight:700;line-height:1.15;color:#eaf2ff;padding:0 3px}
.seat.blaming .blame{animation:blamepop 3.6s ease-out}
@keyframes blamepop{
 0%{opacity:0;transform:translateX(-50%) translateY(7px) scale(.82)}
 12%{opacity:1;transform:translateX(-50%) translateY(0) scale(1.05)}
 20%{transform:translateX(-50%) translateY(0) scale(1)}
 84%{opacity:1;transform:translateX(-50%) translateY(0) scale(1)}
 100%{opacity:0;transform:translateX(-50%) translateY(-7px) scale(1)}}
@media (prefers-reduced-motion:reduce){
 .seat.smoking .hookah::before,.seat.smoking .hookah::after{animation:none}
 .haze.go{animation:none}
 .seat.blaming .blame{animation:none}}
</style></head><body>
<div id="bar"><span id="session">♠ Покер</span><span id="blinds"></span>
 <button id="themebtn" title="Колір столу">🎨</button>
 <button id="sndbtn" title="Звук">🔊</button>
 <button id="radiobtn" title="Лоу-фай радіо">📻</button>
 <button id="avbtn" title="Аватар">🙂</button>
 <button id="histbtn" title="Останні роздачі">🕘</button>
 <button id="leavebtn" title="Встати з-за столу">🚪</button><span id="stage"></span></div>
<div id="history"></div>
<div id="avatars"></div>
<div id="radio">
  <div class="stations"></div>
  <div class="credit">потік: SomaFM · listener-supported</div>
</div>
<div id="themes">
  <button data-felt="felt-green"  style="background:#176b48"></button>
  <button data-felt="felt-blue"   style="background:#1d5f86"></button>
  <button data-felt="felt-purple" style="background:#5a3d80"></button>
  <button data-felt="felt-red"    style="background:#8a2f3a"></button>
  <button data-felt="felt-slate"  style="background:#3c4756"></button>
  <button data-felt="" id="feltrandom" title="Випадкове фото">🎲</button>
</div>
<div id="felt"><div id="oval"></div><div id="haze" class="haze"></div><div id="centre"><div id="board"></div><div id="pot"></div></div><div id="win"><b></b></div>
 <div id="buyin"><h3>Скільки береш за стіл?</h3><div class="opts"></div><div class="bal"></div></div></div>
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
// Filenames of the background photos compiled into the binary. Rendered by
// html/template as a JS array literal, escaped as data.
const BGS={{.Backgrounds}};

// ---- Sound -------------------------------------------------------------
// Every sound is synthesised with the Web Audio API rather than loaded from
// a file: nothing to embed in the binary, nothing to fetch, and no CSP
// surface. The context is created lazily on the first real gesture because
// mobile browsers refuse to start audio before one.
const SND_KEY="poker.sound";
let audio=null,soundOn=true;
try{soundOn=localStorage.getItem(SND_KEY)!=="0"}catch(e){}

function audioCtx(){
  if(!soundOn)return null;
  const AC=window.AudioContext||window.webkitAudioContext;
  if(!AC)return null;
  if(!audio)audio=new AC();
  // Backgrounding the app suspends the context; resume or everything after
  // that is silent with no error to notice.
  if(audio.state==="suspended")audio.resume();
  return audio;
}

// One shaped tone. Gain is ramped rather than switched so notes do not
// click, which on short blips is louder than the note itself.
function tone(freq,start,dur,vol,type){
  const ac=audioCtx();
  if(!ac)return;
  const t=ac.currentTime+start;
  const o=ac.createOscillator(),g=ac.createGain();
  o.type=type||"sine";
  o.frequency.setValueAtTime(freq,t);
  g.gain.setValueAtTime(0,t);
  g.gain.linearRampToValueAtTime(vol,t+0.012);
  g.gain.exponentialRampToValueAtTime(0.0001,t+dur);
  o.connect(g);g.connect(ac.destination);
  o.start(t);o.stop(t+dur+0.02);
}

// Filtered noise burst — cards sliding, chips landing.
function noise(start,dur,vol,freq){
  const ac=audioCtx();
  if(!ac)return;
  const t=ac.currentTime+start;
  const n=Math.floor(ac.sampleRate*dur);
  const buf=ac.createBuffer(1,n,ac.sampleRate);
  const d=buf.getChannelData(0);
  for(let i=0;i<n;i++)d[i]=(Math.random()*2-1)*(1-i/n);
  const src=ac.createBufferSource();src.buffer=buf;
  const bp=ac.createBiquadFilter();bp.type="bandpass";bp.frequency.value=freq||2000;
  const g=ac.createGain();g.gain.value=vol;
  src.connect(bp);bp.connect(g);g.connect(ac.destination);
  src.start(t);
}

const SFX={
  deal(){noise(0,0.12,0.22,2600);noise(0.07,0.12,0.18,2200)},
  chip(){tone(1180,0,0.05,0.10,"triangle");tone(1560,0.045,0.06,0.08,"triangle")},
  fold(){tone(190,0,0.13,0.12,"sine")},
  check(){tone(520,0,0.07,0.09,"sine")},
  turn(){tone(740,0,0.13,0.13);tone(988,0.11,0.20,0.13)},
  win(){[523,659,784,1047].forEach((f,i)=>tone(f,i*0.09,0.34,0.15,"triangle"))},
  lose(){[440,349,262].forEach((f,i)=>tone(f,i*0.10,0.28,0.11,"sine"))}
};

// ---- Lofi radio --------------------------------------------------------
// A plain <audio> pointed at a public SomaFM stream. It is NOT routed
// through our server on purpose: proxying would push every listener's
// bandwidth through the Fly instance. The consequence is that each player's
// browser connects to SomaFM directly, so SomaFM sees their IP the same way
// any web radio would.
const RADIO_KEY="poker.radio";
const radioBox=document.getElementById("radio");
const radioBtn=document.getElementById("radiobtn");
let radioEl=null,radioStations=null,radioNow="";

function paintRadio(){
  radioBtn.classList.toggle("on",!!radioNow);
  radioBox.querySelectorAll("button").forEach(b=>
    b.classList.toggle("playing",b.getAttribute("data-st")===radioNow));
}

function stopRadio(){
  if(radioEl){radioEl.pause();radioEl.src="";radioEl=null}
  radioNow="";
  try{localStorage.removeItem(RADIO_KEY)}catch(e){}
  paintRadio();
}

function playRadio(st){
  if(radioNow===st.id){stopRadio();return}
  stopRadio();
  radioEl=new Audio(st.url);
  radioEl.volume=0.35;   // it is background music, not the main event
  radioEl.play().then(()=>{
    radioNow=st.id;
    try{localStorage.setItem(RADIO_KEY,st.id)}catch(e){}
    paintRadio();
  }).catch(()=>{
    // Autoplay refusal or a dead stream. Say so rather than leaving a
    // button that looks armed and plays nothing.
    setError("Радіо не запустилось");
    stopRadio();
  });
}

async function loadRadio(){
  if(radioStations)return radioStations;
  try{
    const r=await fetch("/poker/radio");
    radioStations=r.ok?await r.json():[];
  }catch(e){radioStations=[]}
  const row=radioBox.querySelector(".stations");
  row.textContent="";
  if(!radioStations.length){
    const p=document.createElement("div");
    p.className="credit";
    p.textContent="Станції недоступні";
    row.appendChild(p);
    return radioStations;
  }
  radioStations.forEach(st=>{
    const b=document.createElement("button");
    b.type="button";
    b.setAttribute("data-st",st.id);
    b.textContent=st.title;      // textContent: the title comes off the wire
    b.onclick=()=>playRadio(st);
    row.appendChild(b);
  });
  paintRadio();
  return radioStations;
}

radioBtn.onclick=async()=>{
  radioBox.classList.toggle("open");
  if(radioBox.classList.contains("open"))await loadRadio();
};

// Resume last night's station, but only on a gesture — browsers refuse to
// start audio otherwise, and a refused play() would just log an error.
(function(){
  let want=null;
  try{want=localStorage.getItem(RADIO_KEY)}catch(e){}
  if(!want)return;
  const resume=async()=>{
    document.removeEventListener("pointerdown",resume);
    const list=await loadRadio();
    const st=list.find(x=>x.id===want);
    if(st)playRadio(st);
  };
  document.addEventListener("pointerdown",resume,{once:true});
})();

const sndBtn=document.getElementById("sndbtn");
function paintSound(){
  sndBtn.textContent=soundOn?"🔊":"🔇";
  sndBtn.classList.toggle("off",!soundOn);
}
sndBtn.onclick=()=>{
  soundOn=!soundOn;
  try{localStorage.setItem(SND_KEY,soundOn?"1":"0")}catch(e){}
  paintSound();
  if(soundOn)SFX.check(); // confirm it is actually audible
};
paintSound();

// Telegram's own haptics, paired with the turn chime: with a 90s clock you
// will be looking elsewhere, and a buzz reaches you when a sound may not.
function buzz(kind){
  try{
    const hf=tg&&tg.HapticFeedback;
    if(!hf)return;
    if(kind==="turn")hf.impactOccurred("medium");
    else hf.notificationOccurred(kind);
  }catch(e){}
}

// Chooses sounds by diffing two consecutive snapshots. Driven by state
// rather than by our own clicks so other players' and the bots' actions are
// audible too — that is the whole point, since they act while you watch.
function playTransition(prev,v){
  if(!soundOn||!prev)return;
  const live=v.stage!=="waiting"&&v.stage!=="showdown";
  const seats=v.seats||[],old=prev.seats||[];
  const byId={};old.forEach(s=>byId[s.user_id]=s);

  if(v.board&&prev.board&&v.board.length>prev.board.length)SFX.deal();

  let acted=false;
  seats.forEach(s=>{
    const o=byId[s.user_id];
    if(!o)return;
    if(!o.folded&&s.folded){SFX.fold();acted=true}
    else if((s.bet||0)>(o.bet||0)){SFX.chip();acted=true}
  });

  const me=v.you_seat>=0?seats[v.you_seat]:null;
  const wasMine=prev.you_seat>=0&&old[prev.you_seat]&&old[prev.you_seat].to_act;
  if(live&&me&&me.to_act&&!wasMine){SFX.turn();buzz("turn")}

  if(v.stage==="showdown"&&prev.stage!=="showdown"&&me){
    const won=me.won||0;
    if(won>0){SFX.win();buzz("success")}
    else if(won<0)SFX.lose();
  }
  return acted;
}

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
// The hookah standing at Data Android God's seat. Full-colour icon from
// SVG Repo, inlined so the page stays self-contained -- the Mini App
// ships as one template with no static asset route behind it.
const HOOKAH='<svg viewBox="0 0 512 512" aria-hidden="true">'+
  '<path fill="#E6E6E6" d="M143.689,304.14c-10.762,0-24.01-2.373-36.786-10.934c-32.074-21.494-29.09'+
  '5-61.117-28.953-62.794 c2.108-24.763,17.813-41.76,24.468-47.919c26.449-24.473,58.97-19.741,96.62'+
  '7-14.263c35.723,5.197,76.212,11.088,123.953-4.751 c17.366-5.763,49.691-16.488,72.482-47.314c22.7'+
  '58-30.782,25.353-66.145,23.522-90.388c-0.357-4.727,3.185-8.848,7.913-9.205 c4.729-0.358,8.849,3.'+
  '186,9.205,7.913c1.679,22.224,0.425,65.014-26.836,101.886c-26.025,35.2-63.074,47.493-80.88,53.401'+
  ' c-51.597,17.119-96.084,10.646-131.83,5.447c-36.031-5.242-62.065-9.031-82.496,9.875c-5.187,4.799'+
  '-17.424,17.993-19.023,36.774 c-0.026,0.31-2.359,31.154,21.404,47.077c18.458,12.37,39.126,6.989,4'+
  '3.081,5.787c4.538-1.381,9.331,1.178,10.711,5.712 '+
  'c1.38,4.536-1.178,9.33-5.713,10.71C161.041,302.219,153.371,304.14,143.689,304.14z"/><path '+
  'fill="#FFDB6C" '+
  'd="M221.786,145.074c-4.74,0-8.584-3.842-8.584-8.584v-27.669c0-4.742,3.843-8.584,8.584-8.584 '+
  'c4.74,0,8.584,3.842,8.584,8.584v27.669C230.369,141.232,226.526,145.074,221.786,145.074z"/><path '+
  'fill="#634E9B" d="M337.031,473.087H107.218c-17.577,0-31.825,14.249-31.825,31.825l0,0c0,3.898,3.1'+
  '9,7.088,7.088,7.088 h279.289c3.898,0,7.088-3.19,7.088-7.088l0,0C368.857,487.336,354.608,473.087,'+
  '337.031,473.087z"/><path fill="#FFDB6C" '+
  'd="M148.12,473.092c-14.728-16.686-23.665-38.604-23.665-62.61c0-52.275,42.377-94.65,94.65-94.65 '+
  's94.65,42.377,94.65,94.65c0,24.006-8.936,45.924-23.665,62.609"/><path fill="#FFB04C" '+
  'd="M219.106,315.831c-6.519,0-12.884,0.66-19.033,1.915c43.152,8.809,75.619,46.981,75.619,92.736 c'+
  '0,24.006-8.936,45.924-23.665,62.609H148.119l0,0l141.973-0.001c14.727-16.686,23.665-38.604,23.665'+
  '-62.609 C313.757,358.208,271.381,315.831,219.106,315.831z"/><path fill="#6EAECD" d="M259.321,315'+
  '.831H184.93c-12.642,0-22.89-10.248-22.89-22.89l0,0c0-12.642,10.248-22.89,22.89-22.89 h74.392c12.'+
  '642,0,22.89,10.248,22.89,22.89l0,0C282.21,305.583,271.962,315.831,259.321,315.831z"/><path '+
  'fill="#5388B4" d="M259.32,270.051h-28.231c12.642,0,22.89,10.248,22.89,22.89l0,0c0,12.642-10.248,'+
  '22.89-22.89,22.89 h28.231c12.642,0,22.89-10.248,22.89-22.89l0,0C282.21,280.3,271.962,270.051,259'+
  '.32,270.051z"/><path fill="#634E9B" '+
  'd="M276.203,108.822H168.048c-12.642,0-22.89-10.248-22.89-22.89v-4.359 c0-2.649,2.147-4.797,4.797'+
  '-4.797h144.341c2.65,0,4.797,2.147,4.797,4.797v4.359C299.091,98.574,288.843,108.822,276.203,108.8'+
  '22z"/><path fill="#4C3A7A" d="M269.813,76.776v9.156c0,12.642-10.248,22.89-22.89,22.89h29.279c12.'+
  '642,0,22.89-10.248,22.89-22.89 v-4.359c0-2.649-2.147-4.797-4.797-4.797H269.813z"/><path '+
  'fill="#EE3446" '+
  'd="M183.784,270.051c0-21.175,17.165-38.34,38.34-38.34s38.34,17.165,38.34,38.34"/><path '+
  'fill="#BA2C53" d="M222.124,231.711c-4.119,0-8.084,0.657-11.803,1.86c15.398,4.979,26.538,19.425,2'+
  '6.538,36.481h23.605 C260.465,248.877,243.3,231.711,222.124,231.711z"/><g><circle fill="#42A555" '+
  'cx="221.78" cy="207.828" r="23.885"/><circle fill="#42A555" cx="222.306" cy="160.057" '+
  'r="23.885"/></g><g><path fill="#427451" '+
  'd="M221.786,183.94c-3.327,0-6.493,0.683-9.371,1.912c8.533,3.644,14.516,12.109,14.516,21.974 s-5.'+
  '982,18.33-14.516,21.974c2.877,1.229,6.044,1.912,9.371,1.912c13.191,0,23.885-10.694,23.885-23.887'+
  ' C245.672,194.634,234.978,183.94,221.786,183.94z"/><path fill="#427451" '+
  'd="M222.306,136.168c-3.327,0-6.493,0.683-9.371,1.912c8.533,3.644,14.516,12.109,14.516,21.974 s-5'+
  '.982,18.33-14.516,21.973c2.877,1.229,6.044,1.912,9.371,1.912c13.191,0,23.885-10.694,23.885-23.88'+
  '5 C246.192,146.862,235.498,136.168,222.306,136.168z"/></g><path fill="#FFDB6C" '+
  'd="M245.672,76.776H197.9l-23.563-66.217C172.505,5.41,176.322,0,181.787,0h76.957 '+
  'c5.328,0,9.131,5.161,7.552,10.249L245.672,76.776z"/><path fill="#FFB04C" d="M244.052,0L220.25,76'+
  '.776h25.421l20.625-66.528C267.874,5.161,264.071,0,258.743,0H244.052z"/><path fill="#4C3A7A" '+
  'd="M337.031,473.087h-47.196c17.455,0,31.605,14.149,31.605,31.605V512h40.33 '+
  'c3.914,0,7.088-3.174,7.088-7.088l0,0C368.857,487.336,354.608,473.087,337.031,473.087z"/>'+
  '</svg>';
// The speech bubble a bot blames from. Stroke-only in the source icon, so
// it takes a fill here to stay readable over the felt.
const BUBBLE='<svg viewBox="0 0 24 24" fill="none" aria-hidden="true">'+
  '<path d="M20 4H4V16H7V21L12 16H20V4Z" fill="rgba(9,17,27,.86)" '+
  'stroke="currentColor" stroke-width="1.5" stroke-linecap="round" '+
  'stroke-linejoin="round"/></svg>';
// What each bot blames when it loses a pot. Keyed by seat user_id for the
// reason pokerbots.go documents about the card backs: botNames is editable
// prose, and renaming a bot must not move its line onto another seat.
const BLAME={"bot:1":"Це все Делна!","bot:2":"Це все бекенд!"};
function backFor(userID){
  if(userID==="bot:1")return CARD_BACK_BO;
  if(userID==="bot:2")return CARD_BACK_DROID;
  return CARD_BACK;
}
function backsFor(userID){const b=backFor(userID);return b+b}

// Seats are 104px wide at the 1024px+ target, so full bot names fit.
function clip(n){n=n||"";return n.length>17?n.slice(0,17)+"…":n}

// Ten avatars, addressed by index. The server stores and bounds the index
// and knows nothing about the emoji, so this pool can change without a
// migration — only the order matters.
const AVATARS=["\ud83e\udd8a","\ud83d\udc3a","\ud83d\udc3b","\ud83e\udd81","\ud83d\udc38",
               "\ud83d\udc19","\ud83e\udd89","\ud83d\udc37","\ud83d\udc35","\ud83d\udc7d"];
// Bots wear their own faces rather than a slot from the pool, matching the
// card backs, which are also chosen from user_id.
function avatarFor(s){
  if(s.user_id==="bot:1")return "\ud83c\udfa9";
  if(s.user_id==="bot:2")return "\ud83e\udd16";
  return AVATARS[(s.avatar|0)%AVATARS.length];
}

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

// Data Android God's hookah smoke. render() rebuilds every .seat from
// scratch on each snapshot, so a class dropped straight onto his seat would
// vanish on the next one mid-puff. smokeUntil is the state instead: render()
// re-applies .smoking for as long as it is still in the future, the same way
// it re-derives .folded and .act rather than remembering them. The haze layer
// is static markup and survives the rebuild, so it only needs replaying.
let smokeUntil=0;
const SMOKE_MS=4200;

// He takes a lot of pots, so the smoke is rationed rather than automatic.
// Same shape and same reasoning as tauntChance/tauntCooldown in
// pokerbots.go: the probability is for variety, but the cooldown is what
// actually prevents two puffs on consecutive hands, which is the burst that
// would read as noise. The gate is much shorter than the taunt one: a
// drifting haze is far quieter than a chat line, and TurnTimeout is only
// the ceiling on ONE turn, so real hands finish well inside it -- at 90s
// the cooldown was swallowing wins faster than the roll was granting them,
// which made the effect rarer than the 35% suggests.
//
// The roll is per-client, so two people at the same table can disagree about
// whether he smoked this hand. That is deliberate: it keeps the effect
// entirely in the page and costs the protocol nothing. Moving it to the
// server would take one bool on TableView if we ever want it shared.
const SMOKE_CHANCE=0.35;
const SMOKE_COOLDOWN_MS=45*1000;
let lastSmokeAt=0;
function maybeHookahSmoke(){
  const now=Date.now();
  if(now-lastSmokeAt<SMOKE_COOLDOWN_MS)return;
  if(Math.random()>=SMOKE_CHANCE)return;
  lastSmokeAt=now;
  startHookahSmoke();
}
// Blame bubbles are rationed exactly like the smoke, but on their own
// clocks: one bot's bubble must not ration the other's, and both bots can
// lose the same hand. blameUntil mirrors smokeUntil -- render() rebuilds
// every .seat, so the class has to be re-derived rather than remembered.
const BLAME_CHANCE=0.35;
const BLAME_COOLDOWN_MS=45*1000;
const BLAME_MS=3600;
const lastBlameAt={},blameUntil={};
function maybeBlame(userID){
  const now=Date.now();
  if(now-(lastBlameAt[userID]||0)<BLAME_COOLDOWN_MS)return;
  if(Math.random()>=BLAME_CHANCE)return;
  lastBlameAt[userID]=now;
  blameUntil[userID]=now+BLAME_MS;
  const seat=document.querySelector('.seat[data-bot="'+userID+'"]');
  if(seat)seat.classList.add("blaming");
}
function startHookahSmoke(){
  smokeUntil=Date.now()+SMOKE_MS;
  // The seat for this snapshot is already in the DOM by the time the
  // showdown block runs, so light it now; later renders read smokeUntil.
  const seat=document.querySelector(".seat.droidseat");
  if(seat)seat.classList.add("smoking");
  const hz=document.getElementById("haze");
  // Same reflow-between-remove-and-add restart trick showWin() documents.
  hz.classList.remove("go");
  void hz.offsetWidth;
  hz.classList.add("go");
}

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
  const prevSnapshot=state; // captured before state is replaced, for the sound diff
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
  felt.querySelectorAll(".seat,.bet").forEach(e=>e.remove());
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
    // Keyed off user_id, never the display name — botNames is editable
    // prose and his card back and avatar already key off "bot:2" for the
    // same reason (see pokerbots.go).
    const blameLine=BLAME[s.user_id];
    if(blameLine){
      d.dataset.bot=s.user_id;
      if(Date.now()<(blameUntil[s.user_id]||0))d.classList.add("blaming");
      const bl=document.createElement("div");
      bl.className="blame";
      bl.innerHTML=BUBBLE;
      const t=document.createElement("b");
      // Fixed strings from BLAME, but set as text anyway so the bubble can
      // never become an injection point if a line ever comes from data.
      t.textContent=blameLine;
      bl.appendChild(t);
      d.appendChild(bl);
    }
    if(s.user_id==="bot:2"){
      d.classList.add("droidseat");
      if(Date.now()<smokeUntil)d.classList.add("smoking");
      const hk=document.createElement("div");
      hk.className="hookah";
      hk.innerHTML=HOOKAH;
      d.appendChild(hk);
    }
    d.style.left=(cx+rx*Math.cos(ang))+"%";
    d.style.top=(cy+ry*Math.sin(ang))+"%";

    // Display name is player-controlled (it comes straight from the
    // player's own Telegram profile) — set via textContent, never
    // innerHTML/string-concatenation. clip() truncating to 10 chars is
    // NOT what makes this safe; textContent is.
    const av=document.createElement("div");
    av.className="av";
    av.textContent=avatarFor(s);
    d.appendChild(av);

    const plaque=document.createElement("div");
    plaque.className="plaque";
    const nm=document.createElement("div");
    nm.className="nm";
    nm.textContent=clip(s.name);
    plaque.appendChild(nm);
    const st=document.createElement("div");
    st.className="st";
    st.textContent=s.stack;
    plaque.appendChild(st);
    if(s.all_in){
      const ai=document.createElement("span");
      ai.className="allin";
      ai.textContent="ВА-БАНК";
      plaque.appendChild(ai);
    }
    d.appendChild(plaque);

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
    // The bet is drawn on the cloth, not in the plaque — see below.
    if(isActive){
      const cd=document.createElement("div");
      cd.className="cd";
      cd.textContent=left+"с";
      d.appendChild(cd);
    }
    felt.appendChild(d);

    // The committed chips, placed on the line from this seat toward the
    // pot at 52% of the way in — far enough off the plaque to read as
    // being on the cloth, short enough not to collide with the board.
    if(s.bet){
      const chip=document.createElement("div");
      chip.className="bet";
      // Seats on the vertical axis — the viewer at the bottom, and the
      // top seat at even seat counts — would drop their chips straight
      // onto the pot, which sits dead centre. Nudge those sideways,
      // proportional to how vertical the seat is, so the amount stays
      // beside the pot instead of on top of it. Side seats barely move.
      const vertical=1-Math.abs(Math.cos(ang));
      chip.style.left=(cx+rx*0.52*Math.cos(ang)+13*vertical)+"%";
      chip.style.top=(cy+ry*0.52*Math.sin(ang))+"%";
      const coin=document.createElement("i");
      coin.textContent="🪙";
      chip.appendChild(coin);
      chip.appendChild(document.createTextNode(String(s.bet)));
      felt.appendChild(chip);
    }
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
    // Independent of the banner: he can take a pot in a hand you also
    // won a piece of, and both should play.
    const droid=seats.find(s=>s.user_id==="bot:2");
    if(droid&&(droid.won||0)>0)maybeHookahSmoke();
    // A blind posted and folded is not a loss worth blaming anyone for,
    // so this wants a real dent rather than any negative result at all.
    for(const s of seats){
      if(BLAME[s.user_id]&&(s.won||0)< -(v.big_blind||0))maybeBlame(s.user_id);
    }
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
  playTransition(prevSnapshot,v);
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

// Avatar picker. Unlike the felt and the sound toggle this is NOT a local
// preference: everyone at the table sees it, so it goes to the server and
// comes back on the seat like any other shared state.
const avBox=document.getElementById("avatars");
AVATARS.forEach((emoji,i)=>{
  const b=document.createElement("button");
  b.type="button";
  b.textContent=emoji;
  b.setAttribute("data-av",String(i));
  b.onclick=async()=>{
    try{
      const r=await fetch("/api/poker/"+TABLE+"/avatar",{
        method:"POST",
        headers:{"Content-Type":"application/json","X-Telegram-Init-Data":INIT},
        body:JSON.stringify({idx:i})
      });
      if(!r.ok){setError(await r.text());return}
      render(await r.json());
      avBox.classList.remove("open");
    }catch(e){setError("Зʼєднання втрачено…")}
  };
  avBox.appendChild(b);
});
document.getElementById("avbtn").onclick=()=>{
  avBox.classList.toggle("open");
  // Mark the one currently worn, read from our own seat rather than
  // remembered locally — the server is the source of truth.
  const me=state&&state.you_seat>=0?state.seats[state.you_seat]:null;
  const mine=me?(me.avatar|0):-1;
  avBox.querySelectorAll("button").forEach(b=>
    b.classList.toggle("sel",Number(b.getAttribute("data-av"))===mine));
};

// Recent hands, fetched on demand. Deliberately not carried on the state
// payload: ten boards on every broadcast would cost far more than a list
// nobody looks at most of the time.
const histBox=document.getElementById("history");
function miniCard(c){
  const su=c.slice(-1),rank=c.slice(0,-1).replace("T","10");
  const el=document.createElement("span");
  if(su==="h"||su==="d")el.className="red";
  el.textContent=rank+(SUITS[su]||su);
  return el;
}
async function loadHistory(){
  histBox.textContent="";
  let rows=[];
  try{
    const r=await fetch("/api/poker/"+TABLE+"/history",{headers:{"X-Telegram-Init-Data":INIT}});
    if(r.ok)rows=await r.json();
  }catch(e){}
  if(!rows||!rows.length){
    const p=document.createElement("div");
    p.className="empty";
    p.textContent="Ще жодної роздачі";
    histBox.appendChild(p);
    return;
  }
  rows.forEach(h=>{
    const box=document.createElement("div");
    box.className="hand";

    const head=document.createElement("div");
    head.className="hhead";
    const no=document.createElement("div");
    no.className="hno";
    no.textContent="#"+h.hand;
    head.appendChild(no);

    const board=document.createElement("div");
    board.className="hb";
    if((h.board||[]).length){
      h.board.forEach(c=>board.appendChild(miniCard(c)));
    }else{
      // No board at all means the hand ended before the flop.
      const p=document.createElement("span");
      p.className="hnoboard";
      p.style.background="none";
      p.textContent="до флопу";
      board.appendChild(p);
    }
    head.appendChild(board);

    const pot=document.createElement("div");
    pot.className="hp";
    pot.textContent=h.pot+" 🪙";
    head.appendChild(pot);
    box.appendChild(head);

    (h.players||[]).forEach(p=>{
      const row=document.createElement("div");
      row.className="hpl"+(p.folded?" folded":"")+(p.won?" won":"");

      // Names come from Telegram profiles: textContent, as everywhere.
      const nm=document.createElement("div");
      nm.className="hn";
      nm.textContent=(p.won?"🏆 ":"")+p.name;
      row.appendChild(nm);

      const hole=document.createElement("div");
      hole.className="hpc";
      (p.hole||[]).forEach(c=>hole.appendChild(miniCard(c)));
      row.appendChild(hole);

      const combo=document.createElement("div");
      combo.className="hcm";
      combo.textContent=p.folded?"пас":(p.combo||"");
      row.appendChild(combo);

      const d=document.createElement("div");
      d.className="hd "+(p.delta>0?"up":(p.delta<0?"dn":""));
      d.textContent=p.delta>0?("+"+p.delta):String(p.delta||0);
      row.appendChild(d);

      box.appendChild(row);
    });

    histBox.appendChild(box);
  });
}
document.getElementById("histbtn").onclick=async()=>{
  histBox.classList.toggle("open");
  if(histBox.classList.contains("open"))await loadHistory();
};

// Leaving the table. Frees the seat for someone else and releases the
// hub-wide claim, so the next open offers a fresh buy-in rather than
// dropping you back into the seat you just left.
document.getElementById("leavebtn").onclick=async()=>{
  let r;
  try{
    r=await fetch("/api/poker/"+TABLE+"/leave",{
      method:"POST",headers:{"X-Telegram-Init-Data":INIT}});
  }catch(e){setError("Зʼєднання втрачено…");return}
  if(!r.ok){setError(await r.text());return}

  const res=await r.json();
  if(res.pending){
    // Chips still in a live pot: the server stands us up when the hand
    // ends. Say so instead of opening a buy-in we cannot take yet.
    setError("Встанеш, щойно закінчиться роздача");
    return;
  }

  // Re-enter from the top: the join with no amount asks what we may buy in
  // for, and the picker takes it from there.
  let j;
  try{j=await join(0)}catch(e){setMsg("Зʼєднання втрачено…");return}
  if(!j.ok){setMsg(await j.text());return}
  let view=await j.json();
  if(view.choose_buy_in){
    view=await pickBuyIn(view);
    if(!view)return;
  }
  render(view);
};

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
// No stored choice means "surprise me": a photo picked at random from the
// embedded set, re-rolled each time the app opens. Picking a colour is an
// explicit opt out of that; picking 🎲 opts back in.
function applyFelt(name){
  const felt=document.getElementById("felt");
  if(!name&&BGS&&BGS.length){
    const pick=BGS[Math.floor(Math.random()*BGS.length)];
    document.body.className="felt-green";
    // The dark layer is composited into the same background property
    // rather than an overlay element, so it can never sit above the seats
    // or swallow a tap meant for the table.
    felt.style.backgroundImage=
      "linear-gradient(rgba(4,18,12,.58),rgba(4,18,12,.58)),url('/poker/bg/"+encodeURIComponent(pick)+"')";
    felt.style.backgroundSize="cover";
    felt.style.backgroundPosition="center";
  }else{
    document.body.className=name||"felt-green";
    felt.style.backgroundImage="";
  }
  themesRow.querySelectorAll("button").forEach(b=>
    b.classList.toggle("sel",b.getAttribute("data-felt")===name));
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
    // An empty choice clears the preference rather than storing one, so
    // the next open rolls a fresh photo instead of pinning this one.
    try{name?localStorage.setItem(FELT_KEY,name):localStorage.removeItem(FELT_KEY)}catch(e){}
    themesRow.classList.remove("on");
  };
});
document.getElementById("themebtn").onclick=()=>themesRow.classList.toggle("on");
applyFelt(savedFelt());

document.getElementById("chatsend").onclick=sendChat;
chatInput.onkeydown=e=>{if(e.key==="Enter"){e.preventDefault();sendChat()}};

function join(amount){
  return fetch("/api/poker/"+TABLE+"/join",{
    method:"POST",
    headers:{"Content-Type":"application/json","X-Telegram-Init-Data":INIT},
    body:JSON.stringify({buy_in:amount||0})
  });
}

// Offers the stack sizes this player can actually afford and resolves with
// the seated view. The server bounds the amount again on the way back in —
// these buttons are a convenience, not the check.
function pickBuyIn(info){
  return new Promise(resolve=>{
    const box=document.getElementById("buyin");
    const opts=box.querySelector(".opts");
    box.querySelector(".bal").textContent="Баланс: "+info.balance+" 🪙";
    opts.textContent="";

    if(info.max<info.min){
      box.querySelector("h3").textContent="Замало богдудіків — треба щонайменше "+info.min;
      box.classList.add("on");
      resolve(null);
      return;
    }

    const steps=[1000,2500,5000,10000].filter(v=>v>=info.min&&v<info.max);
    steps.push(info.max); // always offer everything available, last
    steps.forEach((v,i)=>{
      const b=document.createElement("button");
      b.type="button";
      if(i<steps.length-1)b.className="alt";
      b.textContent=(i===steps.length-1?"Все: ":"")+v;
      b.onclick=async()=>{
        box.classList.remove("on");
        let r;
        try{r=await join(v)}catch(e){setMsg("Зʼєднання втрачено…");resolve(null);return}
        if(!r.ok){setMsg(await r.text());resolve(null);return}
        resolve(await r.json());
      };
      opts.appendChild(b);
    });
    box.classList.add("on");
  });
}

(async()=>{
  if(!tg){setMsg("Відкрий через кнопку в чаті Telegram");return}
  let j;
  try{
    j=await join(0); // no amount: ask what the options are
  }catch(e){setMsg("Зʼєднання втрачено…");return}
  if(!j.ok){setMsg(await j.text());return}
  let view=await j.json();
  if(view.choose_buy_in){
    view=await pickBuyIn(view);
    if(!view)return; // nothing chosen, or too poor to sit
  }
  render(view);

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

	// super holds the pending Супер гра per table id, if any. Guarded by
	// h.mu like the other per-table maps. Deliberately NOT part of the
	// table snapshot: a deploy mid-game drops it, which is the only
	// outcome that cannot half-apply money.
	super map[string]*superGame
	// superLast is the last time each user actually played a Супер гра,
	// keyed by user id. Only a game that was offered updates it, so a
	// missed chance roll does not start the cooldown.
	superLast map[string]time.Time
	// superRoll returns a number in [0,1) for the chance gate. A field so
	// tests can make the roll deterministic; production leaves it nil and
	// falls back to rand.Float64.
	superRoll func() float64

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

	// lastTauntAt is the per-table floor between bot taunts. Keyed by table
	// rather than by bot so two bots cannot alternate and defeat it.
	// Guarded by h.mu; dropped with the table by dropChat's caller.
	lastTauntAt map[string]time.Time

	// savedSeq records the table Seq last written to the database, so the
	// sweeper only persists tables that actually moved rather than
	// rewriting every table every five seconds. Guarded by h.mu.
	savedSeq map[string]uint64

	// leaving maps a userID to the table they asked to leave while they
	// still had chips in a live pot. settle() stands them up once the hand
	// ends. Guarded by h.mu.
	leaving map[string]string

	// history holds each table's recent finished hands, newest last,
	// capped at historyDepth. Served on demand rather than broadcast.
	// Guarded by h.mu; dropped with the table.
	history map[string][]handResult
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
		super:           map[string]*superGame{},
		superLast:       map[string]time.Time{},
		membershipCache: map[membershipKey]time.Time{},
		chat:            map[string][]chatMsg{},
		lastChatAt:      map[string]time.Time{},
		lastTauntAt:     map[string]time.Time{},
		savedSeq:        map[string]uint64{},
		history:         map[string][]handResult{},
		leaving:         map[string]string{},
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
// CreateOrGet returns the chat's existing live table if it has one, and
// only creates a new one otherwise.
//
// Every /poker used to mint a fresh table, which left the older button
// sitting in the chat pointing at a table that still existed but that
// nobody was at — and, after a restart, at one that no longer existed at
// all. Reusing the live table means the button in the chat keeps working
// for as long as the table does, and everyone who taps any of them lands at
// the same table instead of being scattered across several.
func (h *PokerHub) CreateOrGet(chatID int64) *poker.Table {
	h.mu.Lock()
	for _, t := range h.tables {
		if t.ChatID == chatID {
			h.mu.Unlock()
			return t
		}
	}
	h.mu.Unlock()
	return h.Create(chatID)
}

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
			http.Error(w, "Стіл закрито — напиши /poker у чаті, щоб створити новий", http.StatusNotFound)
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
			h.handleJoin(w, r, tbl, uid, firstName, username)
		case "stream":
			h.handleStream(w, r, tbl, uid)
		case "action":
			h.handleAction(w, r, tbl, uid)
		case "chat":
			h.handleChat(w, r, tbl, uid, firstName, username)
		case "avatar":
			h.handleAvatar(w, r, tbl, uid)
		case "history":
			h.handleHistory(w, tbl)
		case "leave":
			h.handleLeave(w, tbl, uid)
		case "super":
			h.handleSuper(w, r, tbl, uid)
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
			http.Error(w, "Стіл закрито — напиши /poker у чаті, щоб створити новий", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pokerTmpl.Execute(w, map[string]any{"TableID": id, "Backgrounds": bgNames()})
	}
	mux.HandleFunc("/poker/bg/", h.serveBackground)
	mux.HandleFunc("/poker/radio", h.handleRadio)
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
func (h *PokerHub) handleJoin(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64, firstName, username string) {
	userID := fmt.Sprintf("%d", uid)
	name := resolveTarget(firstName, username)

	tbl.Lock()
	if idx := tbl.SeatIndexOf(userID); idx >= 0 {
		// A seat with chips — or with none but still contesting a live pot,
		// i.e. all-in — is a genuine reconnect: return the table as it is.
		if tbl.Seats[idx].Stack > 0 || tbl.HasLiveStake(userID) {
			view := h.envelope(tbl, userID)
			tbl.Unlock()
			h.touch(tbl.ID) // reconnecting is real player-initiated activity
			writeJSON(w, view)
			return
		}
		// Busted. The seat survives settlement with zero chips, and
		// StartHand only deals to seats with a stack, so without this the
		// player is pinned to a dead seat forever: the fast path above
		// would keep short-circuiting every reopen before any buy-in could
		// run, leaving them watching hands they can never be dealt into.
		// Standing them up drops them through to the ordinary join below,
		// which offers a fresh buy-in.
		tbl.StandUp(userID)
		h.broadcast(tbl)
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
		http.Error(w, "Стіл закрито — напиши /poker у чаті, щоб створити новий", http.StatusNotFound)
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
	// The largest stack this player could sit with right now.
	maxBuy := balance
	if maxBuy > poker.MaxBuyIn {
		maxBuy = poker.MaxBuyIn
	}

	// A join carrying no amount is the client asking what it may offer,
	// not a request to be seated. Answering with the range keeps this on
	// one endpoint: the client has no other way to learn the balance, and
	// the server must be the one to bound it either way, since a crafted
	// request could otherwise name any number.
	var req struct {
		BuyIn int `json:"buy_in"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.BuyIn <= 0 {
		if fresh {
			h.releaseSeatClaim(userID, tbl.ID)
		}
		writeJSON(w, map[string]any{
			"choose_buy_in": true,
			"balance":       balance,
			"min":           poker.MinBuyIn,
			"max":           maxBuy,
		})
		return
	}

	buyIn := req.BuyIn
	if buyIn > maxBuy {
		buyIn = maxBuy
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
	avatar := 0
	if h.db != nil {
		avatar = h.db.GetPokerAvatar(userID)
	}
	if err := tbl.Sit(userID, name, buyIn); err != nil {
		tbl.Unlock()
		if fresh {
			h.releaseSeatClaim(userID, tbl.ID)
		}
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	tbl.SetAvatar(userID, avatar)
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
		if err := h.db.SettlePoker(entries, "poker"); err != nil {
			log.Printf("[poker] settle tx failed for table %s: %v", tbl.ID, err)
		}
	}

	h.recordHistory(tbl, deltas)
	// The hand is over: anyone who asked to leave mid-hand goes now.
	h.applyPendingLeaves(tbl)
	h.botTaunt(tbl, deltas)

	// Offered only after the hand has fully settled, so the stake is a
	// balance the player already holds and the payout is a clean second
	// transaction rather than an edit to the settlement.
	h.offerSuper(tbl, deltas)

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
	// A pending Супер гра holds the showdown open past the usual interval
	// so the winner can decide and everyone can watch the result.
	if h.superHolds(tableID) {
		return false
	}
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
		http.Error(w, "Стіл закрито — напиши /poker у чаті, щоб створити новий", http.StatusNotFound)
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
	// Same reasoning as chatLocked above: superView would try to take h.mu
	// a second time and deadlock, so read the super game once here under
	// the lock we already hold via superViewLocked. Building the envelope
	// this way -- not via envelope() -- also guarantees a broadcast and a
	// fresh SSE snapshot never disagree about what "super" says.
	super := h.superViewLocked(tbl.ID)
	for _, s := range h.subs[tbl.ID] {
		select {
		case s.ch <- tableEnvelope{TableView: tbl.ViewFor(s.userID), Chat: msgs, Super: super}:
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
		// StageWaiting is reached two ways: a brand-new table nobody has
		// joined yet, and — the case this exists for — a table restored
		// after a restart, whose players are already seated. handleJoin's
		// auto-start cannot help there: a seated player takes the reconnect
		// fast path and returns before ever reaching it, so without this
		// the table sits at "Очікування" forever with nobody to deal it.
		// hasActiveHuman keeps a fresh empty table from seating bots to
		// play against themselves.
		case tbl.Stage == poker.StageWaiting && hasActiveHuman(tbl),
			tbl.Stage == poker.StageShowdown && h.showdownReady(id):
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

		// Persist while the table lock is still held, and only when the
		// table has moved since the last write — Seq changes on every
		// action, deal and settlement, so it is exactly the right trigger.
		h.mu.Lock()
		changed := h.savedSeq[id] != tbl.Seq
		if changed {
			h.savedSeq[id] = tbl.Seq
		}
		h.mu.Unlock()
		if changed {
			h.persistTable(tbl)
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
		h.dropChat(id) // chat shares the table's lifetime; don't leak the log
		delete(h.lastTauntAt, id)
		delete(h.savedSeq, id)
		delete(h.history, id)
		for uid, tid := range h.leaving {
			if tid == id {
				delete(h.leaving, uid)
			}
		}
		if h.db != nil {
			// Reclaimed deliberately: its snapshot must go too, or the next
			// restart would resurrect a table nobody is at.
			h.db.DeletePokerTable(id)
		}
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
