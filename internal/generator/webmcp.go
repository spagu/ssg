// Package generator — WebMCP (#224): the built site describes itself to an
// agent running in the visitor's browser.
//
// This is not `ssg mcp`. That server runs on the author's machine, writes files
// and runs git; this is a property of the OUTPUT — a small script the published
// site carries so an agent that opens it can call declared tools instead of
// scraping the DOM. The site already knows its own index at build time and
// today throws that knowledge away.
//
// WebMCP is a W3C Community Group Draft, implemented so far only by Chrome. So
// the script feature-detects and does nothing at all when the API is absent,
// which is currently almost every visit, and the whole feature is off unless
// asked for.
package generator

import (
	"strings"

	ssgi18n "github.com/spagu/ssg/internal/i18n"
	"github.com/spagu/ssg/internal/models"
)

// webmcpRuntime is the script every page carries when webmcp is enabled.
//
// It registers four tools. Three read; one acts. All of them answer from
// search-index.json, which the build already writes — so this adds a script,
// not a second copy of the site's data.
//
// The index is fetched on the FIRST tool call, never at page load: a visitor
// without an agent must not pay for a feature they cannot use.
//
// __SSG_INDEX__ is replaced with the index URL for the page's language.
const webmcpRuntime = `(function(){
var mc=navigator.modelContext;
if(!mc||typeof mc.registerTool!=="function"){return}
var INDEX=__SSG_INDEX__,pending=null;
function load(){
if(pending){return pending}
pending=fetch(INDEX).then(function(r){
if(!r.ok){throw new Error("search index unavailable: "+r.status)}
return r.json()}).then(function(d){return Array.isArray(d)?d:[]});
return pending}
function text(v){return{content:[{type:"text",text:JSON.stringify(v)}]}}
function brief(d){return{title:d.title,url:d.url,excerpt:d.excerpt,tags:d.tags||[]}}
function score(d,q){
var t=(d.title||"").toLowerCase(),b=(d.excerpt||"")+" "+(d.text||"");
var n=0,i=b.toLowerCase().indexOf(q);
if(t.indexOf(q)>=0){n+=10}
while(i>=0){n++;i=b.toLowerCase().indexOf(q,i+1)}
return n}
mc.registerTool({name:"searchPosts",
description:"Search this site's posts and pages by keyword. Returns matching titles, URLs and excerpts.",
inputSchema:{type:"object",properties:{query:{type:"string",description:"Words to look for"},
limit:{type:"integer",description:"Maximum results (default 10)"}},required:["query"]},
execute:function(a){return load().then(function(docs){
var q=String(a.query||"").toLowerCase().trim();
if(!q){return text([])}
var hits=docs.map(function(d){return{d:d,n:score(d,q)}}).filter(function(h){return h.n>0});
hits.sort(function(x,y){return y.n-x.n});
return text(hits.slice(0,a.limit>0?a.limit:10).map(function(h){return brief(h.d)}))})}});
mc.registerTool({name:"listByTag",
description:"List this site's documents carrying a given tag.",
inputSchema:{type:"object",properties:{tag:{type:"string"}},required:["tag"]},
execute:function(a){return load().then(function(docs){
var t=String(a.tag||"").toLowerCase();
return text(docs.filter(function(d){
return (d.tags||[]).some(function(x){return String(x).toLowerCase()===t})}).map(brief))})}});
mc.registerTool({name:"getDocument",
description:"Fetch one document of this site by its URL, as published.",
inputSchema:{type:"object",properties:{url:{type:"string"}},required:["url"]},
execute:function(a){return load().then(function(docs){
var d=docs.filter(function(x){return x.url===a.url})[0];
return text(d?{title:d.title,url:d.url,excerpt:d.excerpt,tags:d.tags||[],lang:d.lang,text:d.text}:null)})}});
mc.registerTool({name:"navigate",
description:"Open one of this site's own documents in the current tab.",
inputSchema:{type:"object",properties:{url:{type:"string"}},required:["url"]},
execute:function(a){return load().then(function(docs){
var d=docs.filter(function(x){return x.url===a.url})[0];
if(!d){return text({navigated:false,reason:"not a document of this site"})}
location.href=d.url;
return text({navigated:true,url:d.url})})}});
})();`

// webmcpIndexURL is the search index the script reads for a given page. Without
// i18n there is one index at the root; with it, each language has its own, and
// a page must read the one in its own language or the agent gets the site in a
// language the reader did not ask for.
func (g *Generator) webmcpIndexURL(page *models.Page) string {
	if !g.config.I18n.Enabled {
		return "/search-index.json"
	}
	lang := g.currentLang
	if page != nil && page.Lang != "" {
		lang = page.Lang
	}
	prefix := ssgi18n.Prefix(lang, g.config.DefaultLanguage, g.config.I18n)
	if prefix == "" {
		return "/search-index.json"
	}
	return "/" + strings.Trim(prefix, "/") + "/search-index.json"
}

// injectWebMCP splices the runtime in before </body>, the same seam mermaid and
// KaTeX use. A document already carrying it is left alone, so a theme that
// ships its own registration keeps it rather than getting two.
func injectWebMCP(html, indexURL string) string {
	if strings.Contains(html, "navigator.modelContext") {
		return html
	}
	// jsStringLiteral already does this for the mermaid theme name: quote, and
	// escape <, > and & along with the obvious characters, which is what makes
	// a value safe to inline into script source.
	body := `<script>` + strings.Replace(webmcpRuntime, "__SSG_INDEX__", jsStringLiteral(indexURL), 1) + `</script>`
	if i := strings.LastIndex(html, "</body>"); i >= 0 {
		return html[:i] + body + "\n" + html[i:]
	}
	return html + body
}
