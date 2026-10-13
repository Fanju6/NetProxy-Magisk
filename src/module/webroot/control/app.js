// NetProxy HTTP API v1 同源图形化客户端。零外部依赖，凭据仅驻留当前页面内存。
const API = '/api/v1';
const $ = (id) => document.getElementById(id);
const views = { overview:'概览', nodes:'节点管理', subscriptions:'订阅管理', configs:'配置编辑', logs:'运行日志', diagnostics:'诊断工具' };
const state = { secret:'', view:'overview', status:null, info:null, catalog:[], snapshot:null, mode:null,
  subs:[], currentGroup:'', configTarget:'module', configRevision:'', configContent:'', logKind:'service', logLines:200,
  sse:null, reconnect:null, reconnectAttempt:0, renderEpoch:0, inFlight:false };
const safe = (s) => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const idq = (s) => encodeURIComponent(String(s ?? ''));
const fmtSize = (n) => { let v=Number(n)||0; const u=['B','KiB','MiB','GiB','TiB'];let i=0;while(Math.abs(v)>=1024&&i<u.length-1){v/=1024;i++}return v.toFixed(i===0?0:1)+' '+u[i] };
const time = (n) => { n=Number(n)||0; if(!n)return '—';return new Date(n*1000).toLocaleString('zh-CN',{hour12:false}) };
const duration = (n) => { n=Number(n)||0; const h=Math.floor(n/3600),m=Math.floor(n%3600/60);return h? h+'小时 '+m+'分钟' : m+'分钟' };
const array = (n) => Array.isArray(n)?n:[];
const humanError = (e) => { let s=e?.message||String(e);if(e?.code)s+=' ['+e.code+']';if(e?.persisted)s+='；变更已保存，请刷新核实，不要重复提交';return s };
let toastTimer;
function toast(message,isError=false){let e=$('toast');e.textContent=message;e.className='toast visible'+(isError?' error':'');clearTimeout(toastTimer);toastTimer=setTimeout(()=>e.className='toast',5500)}
async function api(path,method='GET',body,signal){
  if(!state.secret)throw new Error('尚未连接管理 API');
  const headers={Authorization:'Bearer '+state.secret};
  if(body!==undefined)headers['Content-Type']='application/json';
  const response=await fetch(API+path,{method,headers,body:body===undefined?undefined:JSON.stringify(body),
    cache:'no-store',credentials:'omit',referrerPolicy:'no-referrer',signal});
  const type=response.headers.get('content-type')||'';
  if(!type.includes('application/json'))throw new Error('接口未返回 JSON，请确认 NetProxy 版本和本地监听地址');
  const r=await response.json();
  if(r.schema!==1)throw new Error('管理接口 schema 不兼容，期望 1');
  if(!response.ok||!r.ok){
    const error=new Error(r.message||'接口请求失败');
    error.code=r.code||'api.error';
    error.persisted=!!r.persisted||!!r.data?.persisted;
    error.data=r.data;
    error.status=response.status;
    throw error;
  }
  return r.data;
}
async function fetchSafe(path){try{return await api(path)}catch(e){return { _error:humanError(e) }}}
async function init(){
  $('auth-form').addEventListener('submit',async(e)=>{
    e.preventDefault(); const value=$('secret').value.trim(),button=$('connect-button');
    $('auth-error').textContent='';
    if(!/^[0-9a-fA-F]{64}$/.test(value)){$('auth-error').textContent='请输入 64 位十六进制 secret（api access 的 data.secret）';return}
    button.disabled=true;button.textContent='正在验证本地 API…';state.secret=value;
    try{
      state.info=await api('/info');$('auth-layer').hidden=true;$('secret').value='';
      $('api-version').textContent='v'+(state.info.api_version??'1')+' · '+(state.info.version??'');
      setConnection(true); await loadView(); startEvents();
    }catch(err){state.secret='';$('auth-error').textContent=humanError(err);setConnection(false)}
    finally{button.disabled=false;button.innerHTML='建立安全连接 <span>→</span>'}
  });
  $('disconnect').addEventListener('click',logout);
  $('refresh').addEventListener('click',()=>void loadView(true));
  $('mobile-nav').addEventListener('click',() => document.querySelector('.sidebar').classList.toggle('open'));
  document.querySelectorAll('nav [data-view]').forEach(b=>b.addEventListener('click',()=>{
    state.view=b.dataset.view;document.querySelector('.sidebar').classList.remove('open');void loadView();
  }));
  $('dialog-close').onclick=()=>$('dialog').close();
  $('dialog-cancel').onclick=()=>$('dialog').close();
  document.addEventListener('visibilitychange',()=>{if(document.hidden)stopEvents();else if(state.secret){startEvents();void loadView(true)}});
  window.addEventListener('pagehide',stopEvents);
  $('content').addEventListener('click',event=>{const b=event.target.closest('[data-action]');if(b)void act(b.dataset.action,b.dataset)});
  $('content').addEventListener('change',event=>{
    if(event.target.id==='group-select'){state.currentGroup=event.target.value;void loadView()}
    if(event.target.id==='config-target'){state.configTarget=event.target.value;void loadConfig()}
    if(event.target.id==='log-kind'){state.logKind=event.target.value;void loadView()}
    if(event.target.id==='log-lines'){state.logLines=Number(event.target.value)||200;void loadView()}
    if(event.target.id==='mode-select')void updateMode(event.target.value);
  });
}
function setConnection(connected){
  const e=$('side-connection');e.classList.toggle('connected',connected);
  e.querySelector('strong').textContent=connected?'本地已连接':'未连接';
  setLive(connected?'实时事件连接中':'等待连接',false)
}
function setLive(message,live){$('live-indicator').classList.toggle('live',live);$('live-indicator').lastChild.textContent=message}
function logout(){
  stopEvents();state.secret='';state.info=null;state.status=null;state.snapshot=null;state.catalog=[];
  state.subs=[];state.configContent='';state.configRevision='';$('content').replaceChildren();
  $('auth-layer').hidden=false;setConnection(false);$('secret').focus();
}
function title(description=''){
  $('page-title').textContent=views[state.view];$('breadcrumb').textContent=views[state.view];
  document.querySelectorAll('nav [data-view]').forEach(b=>b.classList.toggle('active',b.dataset.view===state.view));
  return '<div class="content-title"><div><h2>'+safe(views[state.view])+'</h2><p>'+safe(description)+'</p></div>';
}
function loading(){ $('content').innerHTML='<div class="loading"><div class="spinner"></div>正在获取实际运行数据…</div>' }
function errorView(e){ $('content').innerHTML='<div class="panel"><div class="empty"><span>!</span>'+safe(humanError(e))+
  '<div style="margin-top:16px"><button type="button" class="secondary" data-action="retry">重试</button></div></div></div>' }
function endTitle(actions=''){return '<div class="actions">'+actions+'</div></div>'}
function btn(action,label,cls='secondary',extra=''){return '<button type="button" class="'+cls+'" data-action="'+safe(action)+'" '+extra+'>'+safe(label)+'</button>'}
function empty(label){return '<div class="empty"><span>◇</span>'+safe(label)+'</div>'}
function field(label,name,value='',type='text',placeholder=''){
  return '<div class="field"><label for="field-'+safe(name)+'">'+safe(label)+'</label><input id="field-'+safe(name)+'" name="'+safe(name)+'" type="'+type+'" value="'+safe(value)+'" placeholder="'+safe(placeholder)+'"></div>' }
function textarea(label,name,value='',placeholder=''){
  return '<div class="field"><label for="field-'+safe(name)+'">'+safe(label)+'</label><textarea id="field-'+safe(name)+'" name="'+safe(name)+'" placeholder="'+safe(placeholder)+'">'+safe(value)+'</textarea></div>' }
function combo(label,name,items,value=''){
  return '<div class="field"><label for="field-'+safe(name)+'">'+safe(label)+'</label><select id="field-'+safe(name)+'" name="'+safe(name)+'">'+items.map(item=>{
    let [v,txt]=Array.isArray(item)?item:[item,item];return '<option value="'+safe(v)+'"'+(String(v)===String(value)?' selected':'')+'>'+safe(txt)+'</option>'
  }).join('')+'</select></div>' }
function box(label,value,note){return '<div class="card"><div class="card-label">'+safe(label)+'</div><div class="card-value">'+safe(value)+'</div><div class="card-foot">'+safe(note)+'</div></div>'}
function detail(name,value){return '<div class="detail"><dt>'+safe(name)+'</dt><dd>'+safe(value)+'</dd></div>'}
function panel(h,content,tools=''){return '<div class="panel"><div class="panel-head"><h3>'+safe(h)+'</h3>'+tools+'</div>'+content+'</div>'}
function options(items,selected){return items.map(([value,label])=>'<option value="'+safe(value)+'"'+(String(value)===String(selected)?' selected':'')+'>'+safe(label)+'</option>').join('')}
async function loadView(silent=false){
  if(!state.secret)return;
  const epoch=++state.renderEpoch;if(!silent)loading();
  $('page-title').textContent=views[state.view];$('breadcrumb').textContent=views[state.view];
  document.querySelectorAll('nav [data-view]').forEach(b=>b.classList.toggle('active',b.dataset.view===state.view));
  try{
    let html='';
    if(state.view==='overview')html=await overview();
    if(state.view==='nodes')html=await nodes();
    if(state.view==='subscriptions')html=await subscriptions();
    if(state.view==='configs')html=await configs();
    if(state.view==='logs')html=await logs();
    if(state.view==='diagnostics')html=await diagnostics();
    if(epoch===state.renderEpoch) $('content').innerHTML=html;
  }catch(e){if(epoch===state.renderEpoch)errorView(e)}
}
async function overview(){
  const [service,cat,mode]=await Promise.all([fetchSafe('/service'),fetchSafe('/catalog'),fetchSafe('/mode')]);
  if(!service._error)state.status=service;
  if(Array.isArray(cat))state.catalog=cat;
  if(!mode._error)state.mode=mode;
  const s=state.status||{},running=s.state==='ready',active=s.active_group_name||s.active_group_id||'未选择';
  const modes=array(state.mode?.available).length?state.mode.available:array(s.available_outbound_modes);
  const modeName=s.outbound_mode||s.configured_outbound_mode||state.mode?.mode||'—';
  const modeSelect='<select id="mode-select" aria-label="切换出站模式">'+options(modes.map(x=>[x,x]),modeName)+'</select>';
  let output=title('管理状态、运行时与最近的服务操作')
    +endTitle(btn('refresh','刷新状态')+btn('service-check','检查配置'));
  if(service._error)output+='<div class="notice">服务状态暂不可用：'+safe(service._error)+'。仍可以通过配置页面修复损坏配置。</div>';
  output+='<div class="kpi-grid">'+box('代理服务',running?'运行中':s.state||'未知',running?'sing-box 已就绪':(s.error||'等待启动'))
    +box('当前模式',modeName,'内核原生出站模式')
    +box('活动分组',active,s.active_group_node_count?String(s.active_group_node_count)+' 个节点':'本地持久配置')
    +box('已下载流量',fmtSize(s.download_total),'↑ '+fmtSize(s.upload_total))+'</div>';
  output+='<div class="split">';
  output+=panel('服务控制','<div class="panel-body"><div class="status-hero"><div class="hero-icon'+(running?'':' offline')+'">'+(running?'●':'○')+
    '</div><div><strong>'+safe(running?'连接正常':s.state==='failed'?'启动失败':'当前未运行')+'</strong><p>'+
    safe(running?'透明代理运行中，可执行重载或重启':s.error||'管理 API 独立运行，仍可编辑配置')+
    '</p></div></div><div class="service-actions">'+
    btn(running?'service-stop':'service-start',running?'停止服务':'启动服务',running?'danger':'primary')+
    btn('service-restart','重启核心')+btn('service-reload','重载配置')+'</div></div>');
  output+=panel('设备与运行信息','<div class="panel-body"><dl>'+
    detail('运行时入站',s.active_backend||'未激活')+detail('配置入站',s.configured_backend||'—')+
    detail('运行时间',running?duration(s.uptime_seconds):'—')+detail('sing-box PID',s.pid??'—')+
    detail('后台 Worker',s.worker_state||'—')+detail('当前选择',s.runtime_selected||s.selected_node_ref||'—')+
    detail('内存占用',fmtSize(s.memory_bytes))+
    detail('连接数',String(s.connections_in??0)+' 入 / '+String(s.connections_out??0)+' 出')+
    '</dl></div>');
  output+='</div>';
  output+='<div class="split">'+panel('出站模式','<div class="panel-body"><p class="help">选择后将通过原生 /mode API 保存并应用。仅显示当前配置支持的模式。</p><div style="margin-top:15px">'+(modes.length?modeSelect:empty('当前配置未返回可用模式'))+'</div></div>')+
    panel('模块信息','<div class="panel-body"><dl>'+
    detail('模块版本',state.info?.version||'—')+detail('API 版本',state.info?.api_version||'—')+
    detail('实例 PID',state.info?.pid||'—')+detail('平台',state.info?.platform||'—')+
    detail('后台实例',state.info?.instance_id||'—')+'</dl></div>')+'</div>';
  return output;
}
async function nodes(){
  const snapshot=await api('/nodes/snapshot');state.snapshot=snapshot;
  const groups=array(snapshot?.groups);
  if(!groups.find(g=>g.group?.id===state.currentGroup))state.currentGroup=groups.find(g=>g.group?.active)?.group?.id||groups[0]?.group?.id||'default';
  const current=groups.find(g=>g.group?.id===state.currentGroup);
  let output=title('查看持久节点，手动选用、测速和编辑')+
    endTitle(btn('node-add','添加节点','primary')+btn('node-import','批量导入')+btn('node-delay-all','整组测速'))+
    '<div class="panel"><div class="panel-head"><h3>当前节点分组</h3><select id="group-select" style="max-width:270px" aria-label="节点分组">'+
    options(groups.map(g=>[g.group.id,(g.group.name||g.group.id)+' ('+g.group.node_count+')']),state.currentGroup)+
    '</select></div><div class="panel-body"><dl>'+
    detail('分组 ID',current?.group?.id||state.currentGroup)+
    detail('选择模式',snapshot?.selection?.selector_mode||'—')+
    detail('运行时节点',snapshot?.selection?.runtime_selected||'—')+
    '</dl></div></div>';
  if(!current?.nodes?.length)return output+panel('节点列表',empty('该分组没有可用节点，可添加或导入'));
  const selected=snapshot?.selection?.selected_node_ref||'';
  output+=panel('节点列表','<div class="rows">'+array(current.nodes).map(n=>{
    const tag=n.tag||'',isSelected=(current.group.id+'/'+tag)===selected||snapshot?.selection?.runtime_selected===tag;
    const extra=' data-group="'+safe(current.group.id)+'" data-tag="'+safe(tag)+'"';
    return '<div class="row-card"><div class="flex-grow"><h4>'+safe(tag)+(isSelected?'<span class="pill active">当前</span>':'')+
      '</h4><p>'+safe(n.protocol||'未知协议')+' · '+safe(n.server||'地址未展示')+'</p></div><div class="actions">'+
      btn('node-use','选择','primary',extra)+btn('node-delay','测速','secondary',extra)+
      btn('node-edit','编辑','secondary',extra)+btn('node-export','导出','secondary',extra)+btn('node-delete','删除','danger',extra)+
      '</div></div>';
  }).join('')+'</div>',btn('node-auto','使用 Auto','secondary'));
  return output;
}
async function subscriptions(){
  const data=await api('/subscriptions');state.subs=array(data);
  let output=title('管理 URL 订阅和定时更新策略')+
    endTitle(btn('subscription-add','添加订阅','primary')+btn('subscription-update-all','全部更新'));
  output+=panel('已配置订阅',state.subs.length?'<div class="rows">'+state.subs.map(g=>{
    const id=' data-id="'+safe(g.id)+'"';
    return '<div class="row-card"><div class="flex-grow"><h4>'+safe(g.name||g.id)+(g.active?'<span class="pill active">当前分组</span>':'')+
      '</h4><p>'+safe(g.id)+' · '+safe(g.node_count??0)+' 个节点 · '+(g.auto_update?'自动更新':'手动更新')+
      '</p><p>上次成功：'+safe(g.last_success_at||'未更新')+(g.last_error?' · '+safe(g.last_error):'')+'</p></div><div class="actions">'+
      btn('subscription-update','更新','primary',id)+btn('subscription-edit','设置','secondary',id)+
      btn('subscription-progress','进度','secondary',id)+btn('subscription-delete','删除','danger',id)+
      '</div></div>';
  }).join('')+'</div>':empty('还没有 URL 订阅，请先添加订阅'));
  return output;
}
async function configs(){
  const targets=await api('/configs');
  let entries=Array.isArray(targets)?targets:array(targets?.targets||targets?.configs);
  let list=entries.map(e=>typeof e==='string'?e:(e.id||e.target||e.name||e.path)).filter(Boolean);
  // 这些完整目标是离线恢复的基础；即使 configs 不可用也允许尝试读取。
  list=[...new Set(['module','inbound','singbox/config.json',...list])];
  if(!list.includes(state.configTarget))state.configTarget='module';
  await readConfig();
  let output=title('支持原文读取、候选校验和带 revision 的安全保存')+
    endTitle(btn('config-validate','校验候选')+btn('config-save','保存配置','primary'));
  output+='<div class="panel"><div class="panel-head"><h3>JSON 编辑器</h3><select id="config-target" style="max-width:270px" aria-label="配置目标">'+
    options(list.map(x=>[x,x]),state.configTarget)+'</select></div><div class="panel-body">'+
    '<div class="config-info">目标：<strong>'+safe(state.configTarget)+'</strong><br>Revision：<span class="mono">'+safe(state.configRevision)+'</span><br>服务停止或配置损坏时仍可编辑完整配置。遇到 revision 冲突必须重新读取，不会自动覆盖其他修改。</div>'+
    '<label for="config-editor">配置原文</label><textarea class="editor" id="config-editor" spellcheck="false" autocomplete="off">'+safe(state.configContent)+'</textarea>'+
    '<div class="actions" style="margin-top:15px">'+btn('config-format','格式化 JSON')+btn('config-reset','重新读取')+'</div></div></div>';
  return output;
}
async function readConfig(){
  const data=await api('/config?target='+idq(state.configTarget));
  state.configRevision=data.revision;
  state.configContent=typeof data.content==='string'?data.content:JSON.stringify(data.content??{},null,2);
}
async function loadConfig(){try{loading();await loadView()}catch(e){errorView(e)}}
async function logs(){
  const content=await api('/logs?kind='+idq(state.logKind)+'&lines='+idq(state.logLines));
  const logText=typeof content==='string'?content:
    (typeof content?.content==='string'?content.content:JSON.stringify(content,null,2));
  return title('查看并清理脱敏运行日志')+endTitle(btn('log-clear','清空当前日志','danger')+btn('refresh','刷新日志'))+
    '<div class="panel"><div class="panel-head"><h3>实时日志快照</h3><div class="inline-fields"><select id="log-kind" aria-label="日志类型">'+
    options([['service','模块日志'],['core','sing-box 日志']],state.logKind)+
    '</select><select id="log-lines" aria-label="日志行数">'+options([[100,'100 行'],[200,'200 行'],[500,'500 行'],[1000,'1000 行'],[2000,'2000 行']],state.logLines)+
    '</select></div></div><div class="panel-body"><pre class="log-area">'+safe(logText||'暂无日志')+'</pre></div></div>';
}
async function diagnostics(){
  return title('导出脱敏诊断包，检查 eBPF 环境和接口能力')+
    endTitle(btn('diagnostics-export','导出诊断包','primary'))+
    '<div class="split">'+
    panel('eBPF 能力检查','<div class="panel-body"><p class="help">通过真实模块 API 检测设备能力，无需 Root 命令输入。</p><div class="service-actions">'+
    btn('ebpf-configured','当前配置')+btn('ebpf-all','全部能力')+btn('ebpf-local','本机网络')+btn('ebpf-shared','共享网络')+
    '</div><pre id="diagnostic-result" class="log-area" style="min-height:180px;margin-top:16px">选择检查类型开始</pre></div>')+
    panel('接口与安全','<div class="panel-body"><dl>'+detail('API 入口',location.origin+'/api/v1')+
    detail('协议版本','schema=1')+detail('鉴权','独立 Bearer Token')+
    detail('凭据存储','仅当前页面内存')+detail('事件来源','SSE /api/v1/events')+
    detail('下载内容','脱敏 tar.gz')+'</dl></div>')+'</div>';
}
async function dialog(titleText,markup,onSave,confirmLabel='保存'){
  const d=$('dialog'),form=$('dialog-form');
  $('dialog-title').textContent=titleText;$('dialog-body').innerHTML=markup;
  $('dialog-confirm').textContent=confirmLabel;$('dialog-confirm').disabled=false;
  form.onsubmit=async e=>{
    e.preventDefault();
    $('dialog-confirm').disabled=true;
    try{const values=Object.fromEntries(new FormData(form).entries());await onSave(values);d.close()}
    catch(e){toast(humanError(e),true)}
    finally{$('dialog-confirm').disabled=false}
  };
  d.showModal();
}
async function mutate(path,method='POST',body){
  if(state.inFlight)return;
  state.inFlight=true;
  try{let data=await api(path,method,body);toast('操作成功');await loadView(true);return data}
  catch(e){toast(humanError(e),true);if(e.persisted)void loadView(true);throw e}
  finally{state.inFlight=false}
}
async function updateMode(value){
  const confirmed=window.confirm('确定将出站模式切换为 '+value+' 吗？');
  if(!confirmed){void loadView(true);return}
  try{await mutate('/mode','PUT',{mode:value})}catch{void loadView(true)}
}
async function downloadDiagnostics(){
  const response=await fetch(API+'/diagnostics/export',{headers:{Authorization:'Bearer '+state.secret},cache:'no-store',credentials:'omit',referrerPolicy:'no-referrer'});
  if(!response.ok){
    const r=await response.json().catch(()=>null);
    throw new Error(r?.message||'诊断下载失败 ('+response.status+')');
  }
  if(!response.headers.get('content-type')?.includes('application/gzip'))throw new Error('诊断包格式不是 gzip');
  const blob=await response.blob();if(!blob.size)throw new Error('下载内容为空');
  const url=URL.createObjectURL(blob),a=document.createElement('a');
  a.href=url;a.download='NetProxy-Diagnostics-'+new Date().toISOString().slice(0,10)+'.tar.gz';
  document.body.append(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(url),1000);
  toast('诊断包已下载，请注意保护日志隐私');
}
async function act(action,d){
  if(!state.secret)return;
  const group=d.group||state.currentGroup||'default',tag=d.tag||'',ident=d.id||'';
  const path='/nodes?group_id='+idq(group)+'&node_tag='+idq(tag);
  try{
    switch(action){
      case 'retry':case 'refresh':await loadView();break;
      case 'service-start':case 'service-stop':case 'service-restart':case 'service-reload':case 'service-check':{
        const op=action.slice(8);if(op==='stop'||op==='restart'){if(!confirm('确定'+(op==='stop'?'停止代理服务':'重启核心')+'吗？'))break}
        await mutate('/service/'+op,'POST');break;
      }
      case 'node-use':await mutate('/selection','PUT',{group_id:group,node_tag:tag});break;
      case 'node-auto':await mutate('/selection','PUT',{group_id:state.currentGroup||'default',node_tag:''});break;
      case 'node-delay':await mutate('/nodes/delay','POST',{group_id:group,node_tag:tag});break;
      case 'node-delay-all':await mutate('/nodes/delay','POST',{group_id:group,node_tag:''});break;
      case 'node-add':await dialog('添加本地节点',
        field('分组 ID','group_id',state.currentGroup||'default')+
        textarea('节点内容 / 节点分享链接','content','', '支持模块识别的原生节点内容')+
        combo('允许不安全 TLS','allow_insecure',[['false','否（推荐）'],['true','是']],'false'),
        async v=>{await mutate('/nodes','POST',{group_id:v.group_id,content:v.content,allow_insecure:v.allow_insecure==='true'})},'添加');break;
      case 'node-import':await dialog('批量导入节点',
        textarea('订阅或节点文本','content','','粘贴节点链接或原始配置内容')+
        combo('允许不安全 TLS','allow_insecure',[['false','否（推荐）'],['true','是']],'false'),
        async v=>{await mutate('/nodes/import','POST',{content:v.content,allow_insecure:v.allow_insecure==='true'})},'导入');break;
      case 'node-edit':{
        const value=await api('/nodes/get?group_id='+idq(group)+'&node_tag='+idq(tag));
        await dialog('编辑节点 · '+tag,
          '<div class="config-info">分组 '+safe(group)+' / '+safe(tag)+'</div>'+
          textarea('原生节点配置','content',typeof value==='string'?value:JSON.stringify(value,null,2))+
          combo('允许不安全 TLS','allow_insecure',[['false','否（推荐）'],['true','是']],'false'),
          async v=>{await mutate('/nodes','PUT',{group_id:group,node_tag:tag,content:v.content,allow_insecure:v.allow_insecure==='true'})});break;
      }
      case 'node-export':{
        const value=await api('/nodes/export?group_id='+idq(group)+'&node_tag='+idq(tag));
        const text=typeof value==='string'?value:JSON.stringify(value,null,2);
        await dialog('导出节点 · '+tag,textarea('分享内容','content',text),async()=>{try{await navigator.clipboard.writeText(text);toast('已复制到剪贴板')}catch{toast('请手动选中复制',true)}},'复制');break;
      }
      case 'node-delete':if(confirm('确定删除节点 '+tag+' 吗？此操作会修改持久配置。'))await mutate(path,'DELETE');break;
      case 'subscription-add':await dialog('添加 URL 订阅',
        field('订阅名称（可选）','name')+field('订阅 URL','url','','url','https://')+
        field('更新周期（分钟，至少 15）','minutes','1440','number')+
        combo('下载方式','update_via_proxy',[['auto','自动'],['always','代理'],['never','直连']],'auto')+
        combo('自动更新','auto_update',[['true','开启'],['false','关闭']],'true')+
        field('User-Agent（可选）','user_agent')+
        field('HWID（可选）','hwid'),
        async v=>{await mutate('/subscriptions','POST',{name:v.name,url:v.url,update_interval:Math.max(900,Number(v.minutes)*60),update_via_proxy:v.update_via_proxy,auto_update:v.auto_update==='true',user_agent:v.user_agent,hwid:v.hwid})},'添加订阅');break;
      case 'subscription-update-all':await mutate('/subscriptions/update-all','POST');break;
      case 'subscription-update':await mutate('/subscriptions/'+idq(ident)+'/update','POST');break;
      case 'subscription-edit':{
        const meta=await api('/subscriptions/'+idq(ident)+'?private=true');
        await dialog('订阅设置 · '+(meta.name||ident),
          field('订阅名称','name',meta.name)+field('订阅 URL','url',meta.url,'url')+
          field('更新周期（分钟）','minutes',Math.round(Number(meta.update_interval||86400)/60),'number')+
          combo('下载方式','update_via_proxy',[['auto','自动'],['always','代理'],['never','直连']],meta.update_via_proxy||'auto')+
          combo('自动更新','auto_update',[['true','开启'],['false','关闭']],String(!!meta.auto_update))+
          field('User-Agent','user_agent',meta.user_agent)+field('HWID','hwid',meta.hwid)+
          field('节点包含过滤','include',meta.include)+field('节点排除过滤','exclude',meta.exclude)+
          field('下载超时（秒）','download_timeout',meta.download_timeout??60,'number'),
          async v=>{await mutate('/subscriptions/'+idq(ident),'PUT',{name:v.name,url:v.url,update_interval:Number(v.minutes)*60,
            update_via_proxy:v.update_via_proxy,auto_update:v.auto_update==='true',user_agent:v.user_agent,
            hwid:v.hwid,include:v.include,exclude:v.exclude,download_timeout:Number(v.download_timeout)})});break;
      }
      case 'subscription-progress':{
        const [progress,history]=await Promise.all([
          fetchSafe('/subscriptions/'+idq(ident)+'/progress'),fetchSafe('/subscriptions/'+idq(ident)+'/history')]);
        await dialog('订阅状态 · '+ident,
          '<p>更新进度与最近历史，来自实际管理 API：</p><pre class="log-area">'+
          safe(JSON.stringify({progress,history},null,2))+'</pre>',
          async()=>{},'关闭');break;
      }
      case 'subscription-delete':if(confirm('确定删除订阅 '+ident+' 吗？若正在使用该分组，请先选用其他分组。'))await mutate('/subscriptions/'+idq(ident),'DELETE');break;
      case 'config-format':{
        const editor=$('config-editor');editor.value=JSON.stringify(JSON.parse(editor.value),null,2);toast('JSON 已格式化（尚未保存）');break;
      }
      case 'config-reset':if(confirm('放弃未保存的更改并重新读取？'))await loadView();break;
      case 'config-validate':case 'config-save':{
        const raw=$('config-editor').value;
        const target=state.configTarget;
        if(!state.configRevision)throw new Error('未获取 revision，禁止盲目写入');
        if(action==='config-save'&&!confirm('确定将此配置保存到 '+target+' 吗？'))break;
        const url='/config'+(action==='config-validate'?'/validate':'')+'?target='+idq(target);
        try{
          const r=await api(url,action==='config-validate'?'POST':'PUT',{revision:state.configRevision,content:raw});
          if(action==='config-save')state.configRevision=r.revision||state.configRevision;
          toast(action==='config-save'?'配置已保存':'候选配置校验通过');
          if(action==='config-save')await loadView(true);
        }catch(err){
          if(err.persisted)await loadView(true);
          if(err.code?.includes('conflict'))toast('配置 revision 已变化。草稿没有覆盖设备，请备份草稿后重新读取',true);
          throw err;
        }
        break;
      }
      case 'log-clear':if(confirm('确定清空 '+state.logKind+' 日志吗？'))await mutate('/logs?kind='+idq(state.logKind),'DELETE');break;
      case 'diagnostics-export':await downloadDiagnostics();break;
      default:
        if(action.startsWith('ebpf-')){
          const mode=action.slice(5);
          const result=await api('/diagnostics/ebpf','POST',{mode,raw:false});
          const container=$('diagnostic-result');if(container)container.textContent=JSON.stringify(result,null,2);
          toast('eBPF 预检完成');break;
        }
    }
  }catch(e){toast(humanError(e),true)}
}
function startEvents(){
  if(!state.secret||document.hidden||state.sse)return;
  clearTimeout(state.reconnect);const controller=new AbortController();state.sse=controller;
  void (async()=>{
    try{
      const res=await fetch(API+'/events?topics=status,groups,subscriptions',{
        headers:{Authorization:'Bearer '+state.secret,Accept:'text/event-stream'},
        cache:'no-store',credentials:'omit',referrerPolicy:'no-referrer',signal:controller.signal
      });
      if(!res.ok||!res.body)throw new Error('SSE 状态 '+res.status);
      setLive('实时同步中',true);state.reconnectAttempt=0;
      const reader=res.body.getReader(),decoder=new TextDecoder();let buffer='';
      for(;;){
        const {value,done}=await reader.read();if(done)break;
        buffer+=decoder.decode(value,{stream:true}).replace(/\r\n/g,'\n');
        if(buffer.length>1024*1024)throw new Error('事件帧超过大小限制');
        let i;while((i=buffer.indexOf('\n\n'))!==-1){
          const block=buffer.slice(0,i);buffer=buffer.slice(i+2);
          let name='message',data=[];
          for(const line of block.split('\n')){if(line.startsWith('event:'))name=line.slice(6).trim();else if(line.startsWith('data:'))data.push(line.slice(5).trimStart())}
          if(data.length)onEvent(name,data.join('\n'));
        }
      }
    }catch(e){if(!controller.signal.aborted)setLive('事件流已断开，重连中',false)}
    finally{
      if(state.sse===controller)state.sse=null;
      if(!controller.signal.aborted&&state.secret&&!document.hidden){
        const delay=Math.min(15000,1000*2**Math.min(state.reconnectAttempt++,4));
        state.reconnect=setTimeout(()=>{void loadView(true);startEvents()},delay);
      }
    }
  })();
}
let eventRefresh;
function onEvent(name,raw){
  let r;try{r=JSON.parse(raw)}catch{return}
  if(r.schema!==1||!r.ok)return;
  if(name==='status')state.status=r.data;
  if(name==='groups')state.snapshot=r.data;
  if(name==='subscriptions')state.subs=array(r.data);
  // 避免事件频繁重绘编辑器，保留用户输入和滚动位置。
  if(['overview','nodes','subscriptions'].includes(state.view)){
    clearTimeout(eventRefresh);eventRefresh=setTimeout(()=>void loadView(true),400);
  }
}
function stopEvents(){
  clearTimeout(state.reconnect);clearTimeout(eventRefresh);
  if(state.sse){state.sse.abort();state.sse=null}
  if(state.secret)setLive('实时订阅已暂停',false)
}
void init();
