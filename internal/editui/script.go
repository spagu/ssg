package editui

// The browser half of `ssg serve --edit` (GO-102, phase 1).
//
// One file, no framework, no bundler — the same shape as the live-reload
// script it sits beside. It is injected only by the dev server in edit mode, so
// nothing here can reach a published page.
//
// What it does: reads the marker the generator put in the head to learn which
// document is on screen, asks for that document's frontmatter and the form for
// it, and lets a click on an element the theme marked
// `data-ssg-edit="frontmatter:<key>"` open that one field. Saving posts one
// field; the server writes it, commits it, and the live-reload stream brings
// the rebuilt page back.

import "strings"

// Script returns the injected editor, with the session token baked in.
//
// The token is placed in the page rather than fetched, for the same reason the
// source marker is: the page is only ever served by the local dev server in
// edit mode, and a fetch would need a token to authorise it.
func Script(token string) string {
	return strings.ReplaceAll(editScript, "__SSG_EDIT_TOKEN__", jsString(token))
}

// jsString quotes a value for embedding in the script.
func jsString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "<", `\x3c`, ">", `\x3e`, "&", `\x26`)
	return `"` + r.Replace(s) + `"`
}

// editScript is the panel. Kept deliberately small and legible: it is served
// as source, and the person debugging it is the person who owns the site.
const editScript = `<style>
#ssg-edit-bar{position:fixed;left:0;right:0;bottom:0;z-index:2147483646;background:#1f2937;color:#f9fafb;
font:14px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;padding:10px 16px;display:flex;
align-items:center;gap:12px;box-shadow:0 -2px 12px rgba(0,0,0,.35)}
#ssg-edit-bar button{font:inherit;background:#374151;color:#f9fafb;border:1px solid #4b5563;border-radius:6px;
padding:4px 12px;cursor:pointer}
#ssg-edit-bar button:hover{background:#4b5563}
#ssg-edit-bar .ssg-edit-note{margin-left:auto;opacity:.85}
#ssg-edit-panel{position:fixed;right:0;top:0;bottom:0;width:min(420px,92vw);z-index:2147483647;background:#111827;
color:#f9fafb;font:14px/1.6 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;padding:20px;overflow:auto;
box-shadow:-4px 0 24px rgba(0,0,0,.45)}
#ssg-edit-panel h2{font-size:16px;margin:0 0 4px}
#ssg-edit-panel .ssg-edit-path{opacity:.7;font-size:12px;word-break:break-all;margin-bottom:16px}
#ssg-edit-panel label{display:block;margin:14px 0 4px;font-weight:600}
#ssg-edit-panel input,#ssg-edit-panel textarea,#ssg-edit-panel select{width:100%;box-sizing:border-box;
font:inherit;padding:7px 9px;border-radius:6px;border:1px solid #374151;background:#1f2937;color:#f9fafb}
#ssg-edit-panel textarea{min-height:90px;resize:vertical}
#ssg-edit-panel .ssg-edit-hint{opacity:.65;font-size:12px;font-weight:400}
#ssg-edit-panel .ssg-edit-actions{display:flex;gap:8px;margin-top:20px}
#ssg-edit-panel button{font:inherit;border-radius:6px;padding:7px 14px;cursor:pointer;border:1px solid #374151;
background:#374151;color:#f9fafb}
#ssg-edit-panel button.ssg-primary{background:#2563eb;border-color:#2563eb}
#ssg-edit-panel .ssg-edit-msg{margin-top:14px;padding:9px 11px;border-radius:6px;background:#374151;
white-space:pre-wrap}
#ssg-edit-panel .ssg-edit-msg.ssg-bad{background:#7f1d1d}
[data-ssg-edit]{outline:1px dashed rgba(37,99,235,.5);outline-offset:2px;cursor:pointer}
[data-ssg-edit]:hover{outline:2px solid #2563eb}
</style>
<script>(function(){
var TOKEN=__SSG_EDIT_TOKEN__;
var meta=function(n){var m=document.querySelector('meta[name="'+n+'"]');return m?m.getAttribute("content"):""};
var SOURCE=meta("ssg:source");
var el=function(tag,props){var e=document.createElement(tag);for(var k in props){e[k]=props[k]}return e};
var api=function(path,opts){
  opts=opts||{};opts.headers=opts.headers||{};opts.headers["X-SSG-Edit-Token"]=TOKEN;
  if(opts.body){opts.headers["Content-Type"]="application/json"}
  return fetch(path,opts).then(function(r){return r.json().then(function(b){
    if(!r.ok){throw new Error(b.error||("HTTP "+r.status))}return b})})};

function bar(note){
  var b=document.getElementById("ssg-edit-bar");
  if(!b){b=el("div",{id:"ssg-edit-bar"});document.body.appendChild(b)}
  b.innerHTML="";
  var label=el("strong",{textContent:"✎ edit mode"});
  b.appendChild(label);
  if(SOURCE){
    var all=el("button",{type:"button",textContent:"All fields"});
    all.onclick=function(){openPanel(null)};
    b.appendChild(all);
  }
  b.appendChild(el("span",{className:"ssg-edit-note",textContent:note||(SOURCE?SOURCE:"this page has no single source file")}));
}

function control(f){
  var input;
  if(f.kind==="text"){input=el("textarea",{})}
  else if(f.kind==="bool"){input=el("select",{});
    ["true","false"].forEach(function(v){input.appendChild(el("option",{value:v,textContent:v}))})}
  else if(f.kind==="enum"&&f.values){input=el("select",{});
    f.values.forEach(function(v){input.appendChild(el("option",{value:v,textContent:v}))})}
  else{input=el("input",{type:f.kind==="date"?"date":(f.kind==="int"?"number":"text")})}
  input.value=f.value||"";
  input.name=f.name;
  return input;
}

function openPanel(onlyKey){
  if(!SOURCE){return}
  api("/__edit/doc?path="+encodeURIComponent(SOURCE)).then(function(doc){
    var old=document.getElementById("ssg-edit-panel");if(old){old.remove()}
    var p=el("div",{id:"ssg-edit-panel"});
    p.appendChild(el("h2",{textContent:onlyKey?("Edit "+onlyKey):("Frontmatter · "+doc.type)}));
    p.appendChild(el("div",{className:"ssg-edit-path",textContent:doc.path}));
    var inputs=[];
    doc.fields.forEach(function(f){
      if(onlyKey&&f.name!==onlyKey){return}
      var l=el("label",{textContent:f.name});
      if(f.required){l.appendChild(el("span",{className:"ssg-edit-hint",textContent:" · required"}))}
      else if(f.kind==="list"){l.appendChild(el("span",{className:"ssg-edit-hint",textContent:" · comma separated"}))}
      p.appendChild(l);
      var input=control(f);p.appendChild(input);inputs.push({field:f,input:input});
    });
    if(!inputs.length){
      p.appendChild(el("div",{className:"ssg-edit-msg",textContent:"This page has no field called "+onlyKey+"."}));
    }
    var msg=el("div",{className:"ssg-edit-msg"});msg.style.display="none";
    var actions=el("div",{className:"ssg-edit-actions"});
    var save=el("button",{type:"button",className:"ssg-primary",textContent:"Save"});
    var close=el("button",{type:"button",textContent:"Close"});
    close.onclick=function(){p.remove()};
    save.onclick=function(){
      var changed=inputs.filter(function(i){return i.input.value!==(i.field.value||"")});
      if(!changed.length){show(msg,"Nothing changed.",false);return}
      save.disabled=true;
      var chain=Promise.resolve();
      changed.forEach(function(i){
        chain=chain.then(function(){
          return api("/__edit/frontmatter",{method:"POST",body:JSON.stringify(
            {path:doc.path,key:i.field.name,value:i.input.value})})
        }).then(function(r){i.field.value=i.input.value;show(msg,(r.git||"saved"),false)})
      });
      chain.catch(function(e){show(msg,String(e.message||e),true)})
           .then(function(){save.disabled=false});
    };
    actions.appendChild(save);actions.appendChild(close);
    p.appendChild(actions);p.appendChild(msg);
    document.body.appendChild(p);
    var first=p.querySelector("input,textarea,select");if(first){first.focus()}
  }).catch(function(e){bar("edit: "+(e.message||e))});
}

function show(node,text,bad){node.style.display="block";node.textContent=text;
  node.className="ssg-edit-msg"+(bad?" ssg-bad":"")}

document.addEventListener("click",function(ev){
  var target=ev.target.closest?ev.target.closest("["+"data-ssg-edit"+"]"):null;
  if(!target){return}
  var spec=target.getAttribute("data-ssg-edit")||"";
  if(spec.indexOf("frontmatter:")!==0){return}
  ev.preventDefault();
  openPanel(spec.slice("frontmatter:".length));
},true);

if(document.readyState==="loading"){document.addEventListener("DOMContentLoaded",function(){bar()})}else{bar()}
})();</script>`
