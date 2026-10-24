const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];
const paths = {
  overview: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  cube: '<path d="m12 3 9 5v9l-9 5-9-5V8zM3 8l9 5 9-5M12 13v9M7.5 5.5l9 5"/>',
  layers: '<path d="m12 3 10 5-10 5L2 8zM2 12l10 5 10-5M2 16l10 5 10-5"/>',
  nodes: '<rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6.5h.01M7 17.5h.01M11 6.5h6M11 17.5h6"/>',
  network: '<circle cx="12" cy="5" r="3"/><circle cx="5" cy="18" r="3"/><circle cx="19" cy="18" r="3"/><path d="m10 7-4 8m8-8 4 8M8 18h8"/>',
  storage: '<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 4 18 4 18 0V5M3 12c0 4 18 4 18 0"/>',
  settings: '<path d="m9 3-.5 3L6 7 3 6l-2 4 2 2v3l-2 2 2 4 3-1 2.5 1L9 24h5l.5-3 2.5-1 3 1 2-4-2-2v-3l2-2-2-4-3 1-2.5-1L14 3z" transform="translate(1 -1) scale(.9)"/><circle cx="12" cy="12" r="3"/>',
  events: '<path d="M13 2 4 14h7l-1 8 10-12h-7z"/>',
  terminal: '<rect x="2" y="3" width="20" height="18" rx="3"/><path d="m6 8 4 4-4 4m7 0h5"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',
  refresh: '<path d="M20 7a9 9 0 1 0 1 9M20 2v6h-6"/>',
  bell: '<path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4"/>',
  cluster: '<path d="m12 2 9 5v10l-9 5-9-5V7zM12 7v10M7 9l10 6M7 15l10-6"/><circle cx="12" cy="12" r="3"/>',
  cpu: '<rect x="6" y="6" width="12" height="12" rx="2"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v4m6-4v4m-6 12v4m6-4v4M2 9h4m-4 6h4m12-6h4m-4 6h4"/>',
  memory: '<rect x="3" y="6" width="18" height="12" rx="2"/><path d="M7 10v4m5-4v4m5-4v4M7 18v3m5-3v3m5-3v3"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  warning: '<path d="m12 3 10 18H2zM12 9v5m0 3h.01"/>',
};
const icon = name => `<svg viewBox="0 0 24 24" aria-hidden="true">${paths[name] || paths.cube}</svg>`;
$$('[data-icon]').forEach(el => el.innerHTML = icon(el.dataset.icon));
const esc = value => String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const pods = [
  ['api-gateway-7d8f9c6b5-x2k9m','production','Running','1/1',0,42,128,'worker-eu-01','2d 14h'],
  ['api-gateway-7d8f9c6b5-r7v3p','production','Running','1/1',0,38,124,'worker-eu-02','2d 14h'],
  ['auth-service-6c4d8b9f2-m8n4k','production','Running','1/1',0,24,96,'worker-eu-01','5d 3h'],
  ['payment-worker-5f9c7d8b6-h3j2l','production','Running','1/1',1,67,256,'worker-eu-03','1d 8h'],
  ['redis-master-0','default','Running','1/1',0,12,64,'worker-eu-02','12d 6h'],
  ['postgres-primary-0','production','Running','1/1',0,89,512,'worker-eu-03','18d 2h'],
  ['coredns-76f75df574-k9w2r','kube-system','Running','1/1',0,8,32,'worker-eu-01','32d 14h'],
  ['prometheus-server-0','monitoring','Running','2/2',0,145,768,'worker-eu-04','7d 11h'],
  ['grafana-5b7d6c8f9-p2q4s','monitoring','Running','1/1',0,18,112,'worker-eu-04','7d 11h'],
  ['image-processor-8c6f5b7d4-t6y9u','production','Pending','0/1',0,0,0,'unassigned','4m'],
  ['notification-worker-4d7f8b6c5-z1a3b','production','CrashLoopBackOff','0/1',7,3,24,'worker-eu-02','23m'],
  ['metrics-server-6d94bc8694-v3n8x','kube-system','Running','1/1',0,14,48,'worker-eu-03','32d 14h'],
].map((p,i) => ({id:i,name:p[0],namespace:p[1],status:p[2],ready:p[3],restarts:p[4],cpu:p[5],memory:p[6],node:p[7],age:p[8]}));
const nodes = Array.from({length:4},(_,i)=>({id:100+i,name:`worker-eu-0${i+1}`,namespace:'—',status:'Ready',ready:'Ready',cpu:[32,28,46,33][i],memory:[48,42,61,57][i],node:'eu-west-1'+['a','b','c','a'][i],age:'32d 14h',restarts:0,role:'worker'}));
const deployments = ['api-gateway','auth-service','payment-worker','image-processor','notification-worker','grafana'].map((name,i)=>({id:200+i,name,namespace:i===5?'monitoring':'production',status:i>2&&i<5?'Progressing':'Available',ready:i===0?'2/2':i>2&&i<5?'0/1':'1/1',cpu:pods[i].cpu,memory:pods[i].memory,node:'RollingUpdate',age:pods[i].age,restarts:0}));
const services = ['api-gateway','auth-service','postgres','redis','prometheus','grafana'].map((name,i)=>({id:300+i,name,namespace:i===3?'default':i>3?'monitoring':'production',status:'Active',ready:i===0?'LoadBalancer':'ClusterIP',cpu:0,memory:0,node:`10.100.${i+1}.${23+i*7}`,port:[443,8080,5432,6379,9090,3000][i],age:'18d 2h',restarts:0}));
const events = [
  {id:400,name:'Deployment successfully rolled out',namespace:'production',status:'Normal',ready:'Deployment',node:'api-gateway',age:'2m',reason:'DeploymentComplete'},
  {id:401,name:'Container restarted',namespace:'production',status:'Warning',ready:'Pod',node:'payment-worker',age:'8m',reason:'BackOff'},
  {id:402,name:'Node is ready',namespace:'kube-system',status:'Normal',ready:'Node',node:'worker-eu-04',age:'14m',reason:'NodeReady'},
  {id:403,name:'Insufficient CPU to schedule pod',namespace:'production',status:'Warning',ready:'Pod',node:'image-processor',age:'4m',reason:'FailedScheduling'},
  {id:404,name:'Liveness probe failed',namespace:'production',status:'Warning',ready:'Pod',node:'notification-worker',age:'1m',reason:'Unhealthy'},
];
const datasets = {Pods:pods,Deployments:deployments,Services:services,Nodes:nodes,Events:events};
let currentTab='Pods',currentPage='Overview',selected=-1,currentResource=null,detailTab='Overview',range='1h',cluster='production-eu';
const navItems=[['Overview','overview',''],['Workloads','cube','18'],['Nodes','nodes','4'],['Networking','network','6'],['Storage','storage','3'],['Configuration','settings',''],['Events','events','5']];
function renderNav(){
  $('#navigation').innerHTML=navItems.map(([n,i,c],idx)=>`${idx===6?'<div class="nav-separator"></div>':''}<button class="nav-item ${currentPage===n?'active':''}" data-page="${n}">${icon(i)}<span>${n}</span>${c?`<span class="nav-count">${c}</span>`:''}</button>`).join('');
  $$('.nav-item').forEach(b=>b.onclick=()=>navigate(b.dataset.page));
}
function navigate(page){
  if(page==='Storage'||page==='Configuration'){openUtility(page);return;}
  currentPage=page;
  const pageInfo={Overview:['Cluster overview','A little clarity in your Kubernetes universe.'],Workloads:['Workloads','Your applications, from deployment to the last container.'],Nodes:['Cluster nodes','Keep a pulse on the machines behind your workloads.'],Networking:['Services & networking','Explore how your applications connect.'],Events:['Cluster events','Every change has a story. Follow it here.']};
  $('#page-title').textContent=pageInfo[page][0];$('#page-description').textContent=pageInfo[page][1];$('#breadcrumb-page').textContent=page;
  $('#charts-section').hidden=page!=='Overview';
  if(page==='Overview'||page==='Workloads')currentTab='Pods';if(page==='Nodes')currentTab='Nodes';if(page==='Networking')currentTab='Services';if(page==='Events')currentTab='Events';
  $('#resource-search').value='';$('#namespace-filter').value='all';$('#status-filter').value='all';renderNav();renderTabs();renderTable();
}
function renderMetrics(){
  const metrics=[['Total pods','12','/ 26 capacity','10 healthy','2 need attention','cube','', 'M1 25 8 20 15 22 22 10 29 14 36 9 43 14 50 4 57 10 64 6 71 8'],['Nodes ready','4','/ 4 nodes','All nodes ready','across 3 zones','nodes','blue','M1 17 12 17 12 10 24 10 24 15 37 15 37 8 49 8 49 11 61 11 61 5 71 5'],['CPU usage','34.8','%','5.6 / 16 cores','plenty of headroom','cpu','','M1 23 8 19 15 22 22 13 29 17 36 11 43 14 50 7 57 11 64 5 71 9'],['Memory usage','52.1','%','16.7 / 32 GiB','within limits','memory','purple','M1 24 8 22 15 25 22 17 29 19 36 12 43 14 50 8 57 11 64 5 71 7']];
  $('#metrics').innerHTML=metrics.map(([label,value,unit,foot,note,i,c,path])=>`<article class="metric"><div class="metric-label">${label}${icon(i)}</div><div class="metric-value">${value}<small>${unit}</small></div><div class="metric-foot"><span class="dot green"></span><em>${foot}</em><span>· ${note}</span></div><div class="metric-spark ${c}"><svg viewBox="0 0 73 30"><path d="${path}"/></svg></div></article>`).join('');
}
function renderChart(){
  const width=600,height=154,left=32,bottom=132;
  const count=55;
  const makePoints=(base,amplitude,phase)=>Array.from({length:count},(_,i)=>[left+i*(width-left-12)/(count-1),bottom-(base+Math.sin(i*.67+phase)*amplitude+Math.sin(i*1.79+phase)*amplitude*.4+Math.cos(i*.17)*amplitude*.6)*(bottom-12)/100]);
  const phase=range==='1h'?0:range==='6h'?2:5;
  const cpu=makePoints(35,5,phase),memory=makePoints(53,3,phase+2);
  const path=points=>'M'+points.map(p=>p.map(n=>n.toFixed(1)).join(',')).join(' L');
  const labels=range==='1h'?['10:00','10:10','10:20','10:30','10:40','10:50','11:00']:range==='6h'?['05:00','06:00','07:00','08:00','09:00','10:00','11:00']:['11:00','15:00','19:00','23:00','03:00','07:00','11:00'];
  $('#resource-chart').innerHTML=`<svg viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" role="img" aria-label="Demo CPU and memory utilization over ${range}"><defs><linearGradient id="cpu-fill" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#b6ee83" stop-opacity=".09"/><stop offset="1" stop-color="#b6ee83" stop-opacity="0"/></linearGradient><linearGradient id="memory-fill" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#a798da" stop-opacity=".1"/><stop offset="1" stop-color="#a798da" stop-opacity="0"/></linearGradient></defs>${[0,25,50,75,100].map(v=>{let y=bottom-v*(bottom-12)/100;return `<line class="grid" x1="${left}" y1="${y}" x2="590" y2="${y}"/><text x="0" y="${y+3}">${v}%</text>`}).join('')}<path d="${path(memory)} L588,132 L32,132 Z" fill="url(#memory-fill)"/><path d="${path(cpu)} L588,132 L32,132 Z" fill="url(#cpu-fill)"/><path class="plot" d="${path(memory)}" stroke="#a798da"/><path class="plot" d="${path(cpu)}" stroke="#b6ee83"/>${labels.map((l,i)=>`<text x="${left+i*93}" y="151" text-anchor="${i===6?'end':'start'}">${l}</text>`).join('')}</svg><div class="chart-hover" hidden></div>`;
  $('#resource-chart').onmousemove=e=>{const rect=e.currentTarget.getBoundingClientRect();const i=Math.max(0,Math.min(count-1,Math.round((e.clientX-rect.left)/rect.width*(count-1))));const tip=$('.chart-hover');tip.hidden=false;tip.style.left=Math.min(65,Math.max(5,(e.clientX-rect.left)/rect.width*100))+'%';tip.textContent=`CPU ${((bottom-cpu[i][1])*100/(bottom-12)).toFixed(1)}% · Memory ${((bottom-memory[i][1])*100/(bottom-12)).toFixed(1)}%`;};
  $('#resource-chart').onmouseleave=()=>$('.chart-hover').hidden=true;
}
function renderActivity(){
  $('#activity').innerHTML=events.slice(0,3).map((e,i)=>`<div class="activity-item"><span class="activity-icon ${e.status==='Warning'?'warning':''}">${icon(e.status==='Warning'?'refresh':i===2?'nodes':'check')}</span><div><strong>${e.name}</strong><p>${e.node} <span>· ${e.namespace}</span></p></div><time>${e.age} ago</time></div>`).join('');
}
function renderTabs(){
  $('#resource-tabs').innerHTML=Object.keys(datasets).map(t=>`<button data-tab="${t}" class="${t===currentTab?'active':''}">${t}${t==='Pods'?'<span class="tab-count">12</span>':''}</button>`).join('');
  $$('#resource-tabs button').forEach(b=>b.onclick=()=>{currentTab=b.dataset.tab;selected=-1;$('#status-filter').value='all';renderTabs();renderTable();});
  const statuses=[...new Set(datasets[currentTab].map(r=>r.status))];
  $('#status-filter').innerHTML='<option value="all">All statuses</option>'+statuses.map(s=>`<option>${s}</option>`).join('');
}
function filtered(){const q=$('#resource-search').value.toLowerCase();return datasets[currentTab].filter(r=>(!q||Object.values(r).join(' ').toLowerCase().includes(q))&&($('#namespace-filter').value==='all'||r.namespace===$('#namespace-filter').value)&&($('#status-filter').value==='all'||r.status===$('#status-filter').value));}
function statusPill(status){return `<span class="status-pill ${['Pending','Progressing','Warning'].includes(status)?'warning':status==='CrashLoopBackOff'?'danger':''}"><i class="dot"></i>${esc(status)}</span>`;}
function renderTable(){
  const rows=filtered();selected=Math.min(selected,rows.length-1);
  const heads=currentTab==='Services'?['NAME','NAMESPACE','STATUS','TYPE','CLUSTER IP','PORT','AGE','']:currentTab==='Events'?['MESSAGE','NAMESPACE','TYPE','OBJECT','REASON','LAST SEEN','']:currentTab==='Nodes'?['NAME','STATUS','CPU','MEMORY','ZONE','AGE','']:['NAME','NAMESPACE','STATUS','READY','RESTARTS','CPU','MEMORY','AGE',''];
  $('#table-head').innerHTML='<tr>'+heads.map(h=>`<th>${h}${h==='NAME'?' <span style="color:#b0bca9">↓</span>':''}</th>`).join('')+'</tr>';
  $('#resource-body').innerHTML=rows.length?rows.map((r,i)=>{
    const name=`<td class="name-cell">${icon(currentTab==='Nodes'?'nodes':currentTab==='Events'?'events':'cube')}${esc(r.name)}</td>`;
    const ns=`<td><span class="namespace-pill ${r.namespace}">${esc(r.namespace)}</span></td>`;
    const status=`<td>${statusPill(r.status)}</td>`;
    const cpu=`<td><span class="tiny-bar"><i><b style="width:${Math.min(r.cpu,100)}%"></b></i>${r.cpu}${currentTab==='Nodes'?'%':'m'}</span></td>`;
    const mem=`<td><span class="tiny-bar memory-bar"><i><b style="width:${Math.min(currentTab==='Nodes'?r.memory:r.memory/8,100)}%"></b></i>${r.memory}${currentTab==='Nodes'?'%':'Mi'}</span></td>`;
    let cells=currentTab==='Services'?name+ns+status+`<td>${r.ready}</td><td>${r.node}</td><td>${r.port}/TCP</td><td>${r.age}</td>`:currentTab==='Events'?name+ns+status+`<td>${r.node}</td><td>${r.reason}</td><td>${r.age} ago</td>`:currentTab==='Nodes'?name+status+cpu+mem+`<td>${r.node}</td><td>${r.age}</td>`:name+ns+status+`<td>${r.ready}</td><td style="${r.restarts?'color:#d4b778':''}">${r.restarts}</td>`+cpu+mem+`<td>${r.age}</td>`;
    return `<tr data-row="${i}" tabindex="0" aria-label="Inspect ${esc(r.name)}" class="${selected===i?'selected':''}">${cells}<td class="row-more">···</td></tr>`;
  }).join(''):`<tr><td colspan="${heads.length}" class="empty-state">No resources match your filters. Try another namespace or search.</td></tr>`;
  $('#table-count').textContent=`Showing ${rows.length} of ${datasets[currentTab].length} ${currentTab.toLowerCase()}`;$('#resource-total').textContent=`${datasets[currentTab].length} resources`;
  $$('#resource-body tr[data-row]').forEach(row=>{row.onclick=()=>{selected=Number(row.dataset.row);openDetail(rows[selected]);};row.onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();openDetail(rows[Number(row.dataset.row)]);}};});
}
function openDetail(r){currentResource={...r,kind:currentTab};detailTab='Overview';$('#detail-drawer').hidden=false;$('#drawer-backdrop').hidden=false;renderDetail();$('.drawer-close').focus();}
function closeDrawer(){$('#detail-drawer').hidden=true;$('#drawer-backdrop').hidden=true;currentResource=null;}
function renderDetail(){
  const r=currentResource;
  $('#detail-drawer').innerHTML=`<div class="drawer-top"><span>${esc(r.kind.toUpperCase())} / RESOURCE DETAILS</span><button class="icon-button drawer-close" aria-label="Close details">×</button></div><h2>${esc(r.name)}</h2>${statusPill(r.status)}<div class="detail-tabs">${['Overview','Logs','YAML'].map(t=>`<button class="${detailTab===t?'active':''}" data-detail-tab="${t}">${t}</button>`).join('')}</div><div id="detail-content"></div>`;
  $('.drawer-close').onclick=closeDrawer;$$('[data-detail-tab]').forEach(b=>b.onclick=()=>{detailTab=b.dataset.detailTab;renderDetail();});
  if(detailTab==='Overview'){
    const facts=[['Namespace',r.namespace],['Created',`${r.age} ago`],['Ready',r.ready],['Node / target',r.node],['CPU',`${r.cpu||0}${r.kind==='Nodes'?'%':' millicores'}`],['Memory',`${r.memory||0}${r.kind==='Nodes'?'%':' MiB'}`]];
    $('#detail-content').innerHTML=`<div class="detail-facts">${facts.map(([k,v])=>`<div><span>${k}</span><strong>${esc(v)}</strong></div>`).join('')}</div><h3 class="detail-subtitle">Labels</h3><pre class="code-block">app.kubernetes.io/name: ${esc(r.name.split('-').slice(0,2).join('-'))}\napp.kubernetes.io/managed-by: helm\nenvironment: ${esc(cluster)}</pre>${r.kind==='Pods'?'<div class="detail-actions"><button class="button" id="restart-resource">'+icon('refresh')+' Restart demo pod</button><button class="button" id="copy-name">Copy name</button></div>':'<div class="detail-actions"><button class="button" id="copy-name">Copy name</button></div>'}<p class="detail-note">Demo environment · Resource data and actions are simulated locally. No Kubernetes cluster is connected.</p>`;
    $('#copy-name').onclick=async()=>{try{await navigator.clipboard.writeText(r.name);toast('Resource name copied');}catch{toast('Clipboard unavailable. Select the resource name to copy it.');}};
    if($('#restart-resource'))$('#restart-resource').onclick=()=>{const pod=pods.find(p=>p.id===r.id);pod.restarts++;pod.status='Running';pod.ready='1/1';Object.assign(currentResource,pod);renderTable();renderDetail();toast('Demo pod restarted · restart count updated');};
  }else if(detailTab==='Logs'){
    const isPod=r.kind==='Pods';
    $('#detail-content').innerHTML=isPod?`<div class="detail-facts"><div><span>Container</span><strong>${esc(r.name.split('-').slice(0,2).join('-'))}</strong></div><div><span>Source</span><strong>Demo log snapshot</strong></div></div><pre class="code-block" style="margin-top:22px">${esc(logsFor(r))}</pre><button class="button" id="refresh-logs">${icon('refresh')} Refresh demo logs</button>`:'<p class="detail-note">Container logs are available for pods. Open the Pods tab and select a pod to inspect its logs.</p>';
    if(isPod)$('#refresh-logs').onclick=()=>{renderDetail();toast('Demo log snapshot refreshed');};
  }else{
    const kind={Pods:'Pod',Deployments:'Deployment',Nodes:'Node',Services:'Service',Events:'Event'}[r.kind];
    const yaml=`# Demo resource manifest\napiVersion: ${kind==='Deployment'?'apps/v1':'v1'}\nkind: ${kind}\nmetadata:\n  name: ${r.name}\n${r.kind==='Nodes'?'':`  namespace: ${r.namespace}\n`}  labels:\n    environment: ${cluster}\n${kind==='Pod'?`spec:\n  nodeName: ${r.node==='unassigned'?'null':r.node}\n  containers:\n    - name: ${r.name.split('-')[0]}\n      image: example/app:1.4.2\n      resources:\n        requests:\n          cpu: 100m\n          memory: 128Mi\nstatus:\n  phase: ${r.status==='Running'?'Running':r.status==='Pending'?'Pending':'Running'}`:kind==='Service'?`spec:\n  type: ${r.ready}\n  clusterIP: ${r.node}\n  ports:\n    - port: ${r.port}\n      protocol: TCP`:kind==='Node'?`spec:\n  podCIDR: 10.244.${r.id-100}.0/24\nstatus:\n  conditions:\n    - type: Ready\n      status: "True"`:kind==='Event'?`reason: ${r.reason}\ntype: ${r.status}\nmessage: ${r.name}`:`spec:\n  replicas: ${r.name==='api-gateway'?2:1}\n  selector:\n    matchLabels:\n      app: ${r.name}\n  template:\n    metadata:\n      labels:\n        app: ${r.name}\n    spec:\n      containers:\n        - name: ${r.name}\n          image: example/${r.name}:1.4.2`}`;
    $('#detail-content').innerHTML=`<pre class="code-block">${esc(yaml)}</pre><button class="button" id="download-yaml">Download YAML</button>`;
    $('#download-yaml').onclick=()=>{const url=URL.createObjectURL(new Blob([yaml],{type:'text/yaml'}));const a=document.createElement('a');a.href=url;a.download=r.name+'.yaml';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);toast('Demo manifest downloaded');};
  }
}
function logsFor(r){const now=new Date().toISOString().slice(0,19);return `${now}Z INFO  Starting ${r.name.split('-').slice(0,2).join('-')}\n${now}Z INFO  Configuration loaded\n${now}Z INFO  Connecting to service dependencies\n${now}Z INFO  Health endpoint listening on :8080\n${now}Z INFO  GET /healthz 200 0.4ms\n${now}Z INFO  GET /api/v1/status 200 12ms\n${now}Z ${r.status==='CrashLoopBackOff'?'ERROR Connection refused: dependency unavailable\n'+now+'Z WARN  Retrying connection in 5s':r.status==='Pending'?'WARN  Waiting for available CPU on a node':'INFO  Readiness probe passed\n'+now+'Z INFO  Request completed successfully'}\n\n# Simulated logs — no live cluster connection`;}
function openUtility(page){
  const storage=page==='Storage';$('#detail-drawer').hidden=false;$('#drawer-backdrop').hidden=false;
  $('#detail-drawer').innerHTML=`<div class="drawer-top"><span>${page.toUpperCase()}</span><button class="icon-button drawer-close" aria-label="Close">×</button></div><h2>${storage?'Persistent volumes':'Cluster configuration'}</h2><p class="detail-note">Demo environment · Example ${storage?'storage resources':'configuration'} for ${cluster}.</p><pre class="code-block">${storage?'postgres-data-primary-0\n  Status: Bound · 100 GiB · gp3\n\nredis-data-master-0\n  Status: Bound · 20 GiB · gp3\n\nprometheus-server-data\n  Status: Bound · 50 GiB · gp3':`Context: aws-${cluster}\nNamespace: all namespaces\nKubernetes: v1.31.4\nRegion: eu-west-1\nAuthentication: demo mode\nPermissions: simulated administrator`}</pre>`;
  $('.drawer-close').onclick=closeDrawer;$('.drawer-close').focus();
}
let toastTimer;
function toast(message){clearTimeout(toastTimer);$('#toast').textContent=message;$('#toast').hidden=false;toastTimer=setTimeout(()=>$('#toast').hidden=true,4000);}
const commands=[...navItems.map(([n,i])=>({label:n,icon:i,action:()=>navigate(n),hint:'Go to page'})),{label:'Refresh resources',icon:'refresh',action:refresh,hint:'Action'},{label:'Switch demo cluster',icon:'cluster',action:switchCluster,hint:'Context'},{label:'Keyboard shortcuts',icon:'terminal',action:()=>$('#help-dialog').showModal(),hint:'Help'}];
function renderCommands(){const q=$('#command-search').value.toLowerCase();const matches=commands.filter(c=>c.label.toLowerCase().includes(q));$('#command-results').innerHTML=matches.length?matches.map(c=>`<button class="command-result" data-command="${commands.indexOf(c)}">${icon(c.icon)}<span>${c.label}</span><span>${c.hint} ↵</span></button>`).join(''):'<p class="empty-state">No matching commands</p>';$$('[data-command]').forEach(b=>b.onclick=()=>{$('#command-dialog').close();commands[Number(b.dataset.command)].action();});}
function openCommands(){$('#command-search').value='';renderCommands();$('#command-dialog').showModal();$('#command-search').focus();}
function refresh(){renderChart();renderActivity();renderTable();$('#last-updated').textContent='Updated just now';toast('Demo resource snapshot refreshed');setTimeout(()=>$('#last-updated').textContent='Live updates',5000);}
function switchCluster(){cluster=cluster==='production-eu'?'staging-eu':'production-eu';$('#cluster-name').textContent=cluster;$('#banner-name').textContent=cluster;$('#context-name').textContent='aws-'+cluster;$('.breadcrumb').childNodes[2].textContent=' '+cluster+' ';$('.banner-tag').textContent=cluster==='production-eu'?'PRODUCTION':'STAGING';$('.page-footer>span:first-child').innerHTML=`<i class="dot green"></i> Connected to ${cluster} <span class="footer-demo">· Demo environment</span>`;toast('Switched demo context to '+cluster);}
$('#command-button').onclick=openCommands;$('#command-search').oninput=renderCommands;
$('#command-search').onkeydown=e=>{if(e.key==='ArrowDown'){e.preventDefault();$('.command-result')?.focus();}if(e.key==='Enter')$('.command-result')?.click();};
$('#command-results').onkeydown=e=>{if(!['ArrowDown','ArrowUp'].includes(e.key))return;e.preventDefault();const results=$$('.command-result');const i=results.indexOf(document.activeElement);results[(i+(e.key==='ArrowDown'?1:-1)+results.length)%results.length]?.focus();};
$('#help-button').onclick=()=>$('#help-dialog').showModal();$('.close-dialog').onclick=()=>$('#help-dialog').close();
$$('dialog').forEach(d=>d.addEventListener('click',e=>{if(e.target===d){const rect=d.getBoundingClientRect();if(e.clientX<rect.left||e.clientX>rect.right||e.clientY<rect.top||e.clientY>rect.bottom)d.close();}}));
$('#cluster-switch').onclick=switchCluster;$('#refresh-button').onclick=refresh;
$('#view-events').onclick=()=>navigate('Events');$('#notification-button').onclick=()=>{navigate('Events');$('#status-filter').value='Warning';renderTable();};
$('#resource-search').oninput=()=>{selected=-1;renderTable();};$('#namespace-filter').onchange=()=>{selected=-1;renderTable();};$('#status-filter').onchange=()=>{selected=-1;renderTable();};
$('#drawer-backdrop').onclick=closeDrawer;
$$('[data-range]').forEach(b=>b.onclick=()=>{range=b.dataset.range;$$('[data-range]').forEach(el=>el.classList.toggle('active',el===b));renderChart();});
document.addEventListener('keydown',e=>{
  const typing=['INPUT','TEXTAREA','SELECT'].includes(document.activeElement.tagName);
  if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==='k'){e.preventDefault();if($('#command-dialog').open)$('#command-dialog').close();else openCommands();return;}
  if(e.key==='Escape'){closeDrawer();return;}
  if(typing||$$('dialog[open]').length||!$('#detail-drawer').hidden)return;
  if(e.key==='/'){e.preventDefault();$('#resource-search').focus();}
  if(e.key==='?'){$('#help-dialog').showModal();}
  if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();const rows=filtered();if(!rows.length)return;selected=Math.max(0,Math.min(rows.length-1,selected+(e.key==='ArrowDown'?1:-1)));renderTable();$(`#resource-body tr[data-row="${selected}"]`)?.scrollIntoView({block:'nearest'});}
  if(e.key==='Enter'&&selected>=0&&document.activeElement===document.body){openDetail(filtered()[selected]);}
});
if (!location.pathname.startsWith('/k9s')) { renderNav();renderMetrics();renderChart();renderActivity();renderTabs();renderTable(); }
if (location.pathname.startsWith('/k9s')) {
// Live adapter for the authenticated /k9s/ deployment. Never falls back to demo data.
let liveSnapshot=null, liveBusy=false, liveError='', detailSequence=0;
const liveHistory=[];
for(const list of Object.values(datasets)) list.splice(0);
const ageOf=value=>{const minutes=Math.max(0,Math.floor((Date.now()-Date.parse(value))/60000));return !Number.isFinite(minutes)?'—':minutes<1?'now':minutes<60?minutes+'m':minutes<1440?Math.floor(minutes/60)+'h':Math.floor(minutes/1440)+'d';};
function quantity(value){
  if(value===undefined||value===null)return null;
  const match=String(value).match(/^([\d.+-]+(?:e[+-]?\d+)?)([a-zA-Z]*)$/i);if(!match)return null;
  const powers={n:1e-9,u:1e-6,m:1e-3,'':1,k:1e3,K:1e3,M:1e6,G:1e9,T:1e12,P:1e15,E:1e18,Ki:1024,Mi:1024**2,Gi:1024**3,Ti:1024**4,Pi:1024**5,Ei:1024**6};
  return Object.hasOwn(powers,match[2])?Number(match[1])*powers[match[2]]:null;
}
async function api(path){const response=await fetch('/k9s/api/'+path,{credentials:'same-origin',cache:'no-store'});let body;try{body=await response.json();}catch{throw new Error('The server has not loaded the live dashboard API. Install the prepared server update and restart the service.');}if(!response.ok)throw new Error(body.error||'Kubernetes request failed ('+response.status+').');return body;}
function replaceData(snapshot){
  const d=snapshot.data, pm=new Map((d.podMetrics||[]).map(p=>[p.metadata.namespace+'/'+p.metadata.name,p]));
  const nm=new Map((d.nodeMetrics||[]).map(n=>[n.metadata.name,n]));
  function base(r){return {id:r.metadata.uid,name:r.metadata.name,resourceName:r.metadata.name,namespace:r.metadata.namespace||'—',age:ageOf(r.metadata.creationTimestamp),cpu:null,memory:null,restarts:0,raw:r};}
  pods.splice(0,pods.length,...d.Pods.map(p=>{
    const statuses=p.status?.containerStatuses||[], waiting=statuses.find(c=>c.state?.waiting?.reason),metric=pm.get(p.metadata.namespace+'/'+p.metadata.name);
    const r=base(p), sum=key=>metric?.containers?.reduce((n,c)=>n+(quantity(c.usage?.[key])||0),0);
    return {...r,status:p.metadata.deletionTimestamp?'Terminating':waiting?.state.waiting.reason||p.status?.phase||'Unknown',ready:statuses.filter(c=>c.ready).length+'/'+(p.spec?.containers?.length||0),restarts:statuses.reduce((n,c)=>n+c.restartCount,0),cpu:metric?Math.round(sum('cpu')*1000):null,memory:metric?Math.round(sum('memory')/1024**2):null,node:p.spec?.nodeName||'unassigned'};
  }));
  nodes.splice(0,nodes.length,...d.Nodes.map(n=>{
    const metric=nm.get(n.metadata.name),cap=n.status?.allocatable||{}, ready=n.status?.conditions?.some(c=>c.type==='Ready'&&c.status==='True');
    return {...base(n),status:ready?'Ready':'NotReady',ready:ready?'Ready':'NotReady',cpu:metric&&quantity(cap.cpu)>0?Math.round(quantity(metric.usage.cpu)/quantity(cap.cpu)*100):null,memory:metric&&quantity(cap.memory)>0?Math.round(quantity(metric.usage.memory)/quantity(cap.memory)*100):null,node:n.metadata.labels?.['topology.kubernetes.io/zone']||'—'};
  }));
  deployments.splice(0,deployments.length,...d.Deployments.map(r=>({...base(r),status:(r.status?.observedGeneration||0)>=(r.metadata.generation||0)&&(r.status?.availableReplicas||0)>=(r.spec?.replicas??1)?'Available':'Progressing',ready:(r.status?.readyReplicas||0)+'/'+(r.spec?.replicas??1),node:r.spec?.strategy?.type||'—'})));
  services.splice(0,services.length,...d.Services.map(r=>({...base(r),status:'Active',ready:r.spec?.type||'ClusterIP',node:r.spec?.clusterIP||'—',port:(r.spec?.ports||[]).map(p=>p.port+'/'+(p.protocol||'TCP')).join(', ')})));
  events.splice(0,events.length,...d.Events.slice().sort((a,b)=>Date.parse(b.lastTimestamp||b.eventTime||b.metadata.creationTimestamp)-Date.parse(a.lastTimestamp||a.eventTime||a.metadata.creationTimestamp)).slice(0,100).map(r=>({...base(r),name:r.message||r.reason||r.metadata.name,status:r.type||'Normal',ready:r.involvedObject?.kind||'—',node:r.involvedObject?.name||'—',reason:r.reason||'—',age:ageOf(r.lastTimestamp||r.eventTime||r.metadata.creationTimestamp)})));
}
function setConnection(text,good){
  $('#cluster-switch small').innerHTML=`<i class="dot" style="background:${good?'var(--green)':'#d9af72'}"></i>${esc(text)}`;
  $('.cluster-health').innerHTML=`<span class="dot" style="background:${good?'var(--green)':'#d9af72'}"></span>${esc(text)}`;
  $('#last-updated').textContent=liveBusy?'Refreshing…':good?'Polling every 15s':'Connection unavailable';
  $('.page-footer>span:first-child').textContent=(liveSnapshot?'Context: '+liveSnapshot.context+' · ':'')+text;
}
function updateIdentity(){
  cluster=liveSnapshot?.context||'Not connected';$('#cluster-name').textContent=cluster;$('#banner-name').textContent=cluster;$('#context-name').textContent=cluster;
  $('.breadcrumb').childNodes[2].textContent=' '+cluster+' ';
  const version=[...new Set(nodes.map(n=>n.raw.status?.nodeInfo?.kubeletVersion).filter(Boolean))].join(', ')||'Version unavailable';
  const regions=[...new Set(nodes.map(n=>n.raw.metadata.labels?.['topology.kubernetes.io/region']).filter(Boolean))].join(', ');
  $('.banner-title>span').textContent=(liveSnapshot?.connected?'Live data':'Not connected')+' · '+version+(regions?' · '+regions:'');
  $('.banner-meta>span:nth-child(2)').innerHTML='LAST SUCCESS<strong>'+esc(liveSnapshot?new Date(liveSnapshot.observedAt).toLocaleTimeString():'—')+'</strong>';
  $('.banner-tag').textContent='READ ONLY';
  navItems.find(n=>n[0]==='Workloads')[2]=String(pods.length+deployments.length);navItems.find(n=>n[0]==='Nodes')[2]=String(nodes.length);navItems.find(n=>n[0]==='Networking')[2]=String(services.length);navItems.find(n=>n[0]==='Events')[2]=String(events.length);navItems.find(n=>n[0]==='Storage')[2]=String(liveSnapshot?.data.Storage.length||0);
  renderNav();
}
function utilization(){
  const measured=nodes.filter(n=>n.cpu!==null&&n.memory!==null);
  if(!measured.length)return null;
  let cpu=0,mem=0,capCpu=0,capMem=0;
  const metrics=new Map(liveSnapshot.data.nodeMetrics.map(n=>[n.metadata.name,n]));
  for(const n of measured){const usage=metrics.get(n.name).usage;cpu+=quantity(usage.cpu);mem+=quantity(usage.memory);capCpu+=quantity(n.raw.status.allocatable.cpu);capMem+=quantity(n.raw.status.allocatable.memory);}
  return {cpu:cpu/capCpu*100,memory:mem/capMem*100,count:measured.length,cores:cpu,gib:mem/1024**3};
}
renderMetrics=function(){
  const usage=liveSnapshot&&utilization();
  const metrics=[['Total pods',liveSnapshot?pods.length:'—','',pods.filter(p=>p.status==='Running').length+' running','cube'],['Nodes ready',liveSnapshot?nodes.filter(n=>n.status==='Ready').length:'—','/ '+nodes.length+' nodes','Observed node readiness','nodes'],['CPU usage',usage?usage.cpu.toFixed(1):'—',usage?'%':'',usage?usage.cores.toFixed(2)+' cores · '+usage.count+'/'+nodes.length+' nodes':'Metrics unavailable','cpu'],['Memory usage',usage?usage.memory.toFixed(1):'—',usage?'%':'',usage?usage.gib.toFixed(2)+' GiB · '+usage.count+'/'+nodes.length+' nodes':'Metrics unavailable','memory']];
  $('#metrics').innerHTML=metrics.map(([label,value,unit,note,i])=>`<article class="metric"><div class="metric-label">${label}${icon(i)}</div><div class="metric-value">${value}<small>${unit}</small></div><div class="metric-foot">${esc(note)}</div></article>`).join('');
};
renderTabs=function(){
  const previous=$('#status-filter').value;
  $('#resource-tabs').innerHTML=Object.keys(datasets).map(t=>`<button data-tab="${t}" class="${t===currentTab?'active':''}">${t}<span class="tab-count">${datasets[t].length}</span></button>`).join('');
  $$('#resource-tabs button').forEach(b=>b.onclick=()=>{currentTab=b.dataset.tab;selected=-1;$('#status-filter').value='all';renderTabs();renderTable();});
  const statuses=[...new Set(datasets[currentTab].map(r=>r.status))];$('#status-filter').innerHTML='<option value="all">All statuses</option>'+statuses.map(s=>`<option>${esc(s)}</option>`).join('');if(statuses.includes(previous))$('#status-filter').value=previous;
};
statusPill=function(status){const warning=['Pending','Progressing','Warning','Terminating','Unknown'].includes(status),bad=['CrashLoopBackOff','NotReady','Failed','ImagePullBackOff','ErrImagePull','Error'].includes(status);return `<span class="status-pill ${bad?'danger':warning?'warning':''}"><i class="dot"></i>${esc(status)}</span>`;};
renderTable=function(){
  const rows=filtered();selected=Math.min(selected,rows.length-1);
  const columns=currentTab==='Services'?['NAME','NAMESPACE','STATUS','TYPE','CLUSTER IP','PORT','AGE']:currentTab==='Events'?['MESSAGE','NAMESPACE','TYPE','OBJECT','REASON','LAST SEEN']:currentTab==='Nodes'?['NAME','STATUS','CPU','MEMORY','ZONE','AGE']:['NAME','NAMESPACE','STATUS','READY','RESTARTS','CPU','MEMORY','AGE'];
  $('#table-head').innerHTML='<tr>'+columns.map(c=>'<th>'+c+'</th>').join('')+'</tr>';
  const metric=(value,unit)=>value===null?'—':esc(value)+unit;
  const error=liveError||liveSnapshot?.errors[currentTab];
  $('#resource-body').innerHTML=rows.map((r,i)=>{
    const cells=currentTab==='Services'?[r.name,r.namespace,{status:r.status},r.ready,r.node,r.port,r.age]:currentTab==='Events'?[r.name,r.namespace,{status:r.status},r.node,r.reason,r.age]:currentTab==='Nodes'?[r.name,{status:r.status},metric(r.cpu,'%'),metric(r.memory,'%'),r.node,r.age]:[r.name,r.namespace,{status:r.status},r.ready,r.restarts,metric(r.cpu,'m'),metric(r.memory,'Mi'),r.age];
    return `<tr data-row="${i}" tabindex="0" class="${selected===i?'selected':''}" aria-label="Inspect ${esc(r.name)}">${cells.map((c,j)=>`<td>${c&&typeof c==='object'?statusPill(c.status):esc(c)}</td>`).join('')}</tr>`;
  }).join('')||`<tr><td colspan="${columns.length}" class="empty-state">${esc(error||(!liveSnapshot?'Connecting to your cluster…':'No resources match your filters.'))}</td></tr>`;
  $('#table-count').textContent=`${rows.length} of ${datasets[currentTab].length} ${currentTab.toLowerCase()}${liveError?' · STALE DATA':''}`;$('#resource-total').textContent=liveSnapshot?'Observed '+new Date(liveSnapshot.observedAt).toLocaleTimeString():'';
  $$('#resource-body tr[data-row]').forEach(row=>{row.onclick=()=>{selected=Number(row.dataset.row);openDetail(rows[selected]);};row.onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();row.click();}};});
};
renderActivity=function(){
  $('#activity').innerHTML=events.slice(0,3).map(e=>`<div class="activity-item"><span class="activity-icon ${e.status==='Warning'?'warning':''}">${icon(e.status==='Warning'?'warning':'check')}</span><div><strong>${esc(e.name)}</strong><p>${esc(e.node)} · ${esc(e.namespace)}</p></div><time>${esc(e.age)}</time></div>`).join('')||'<p class="detail-note" style="padding:20px">No events available.</p>';
  $('.activity-foot').textContent='Read-only snapshot · refreshes every 15 seconds';
};
renderChart=function(){
  const usage=liveSnapshot&&utilization();
  $$('.chart-legend strong').forEach((el,i)=>el.textContent=usage?(i?usage.memory:usage.cpu).toFixed(1)+'%':'—');
  $('.chart-panel .panel-heading p').textContent='Observed since this page was opened. No historical data is fabricated.';
  const span=range==='1h'?3600000:range==='6h'?21600000:86400000;
  const points=liveHistory.filter(p=>Date.now()-p.time<span);
  if(points.length<2){$('#resource-chart').innerHTML='<div class="empty-state">'+(usage?'Collecting live samples. The chart will appear after the next refresh.':'Utilization unavailable. A working Kubernetes Metrics API and permission to read metrics are required.')+'</div>';return;}
  const min=points[0].time,max=points.at(-1).time;
  const line=key=>'M'+points.map(p=>`${32+(p.time-min)/(max-min)*550},${130-Math.min(100,Math.max(0,p[key]))*1.1}`).join(' L');
  $('#resource-chart').innerHTML=`<svg viewBox="0 0 600 155" preserveAspectRatio="none" role="img" aria-label="Observed CPU and memory usage"><line class="grid" x1="32" y1="75" x2="582" y2="75"/><line class="grid" x1="32" y1="130" x2="582" y2="130"/><text x="0" y="78">50%</text><text x="0" y="133">0%</text><path class="plot" stroke="#b6ee83" d="${line('cpu')}"/><path class="plot" stroke="#a798da" d="${line('memory')}"/><text x="32" y="151">${new Date(min).toLocaleTimeString()}</text><text x="580" y="151" text-anchor="end">${new Date(max).toLocaleTimeString()}</text></svg>`;
  $('#resource-chart').onmousemove=null;$('#resource-chart').onmouseleave=null;
};
renderDetail=async function(){
  const seq=++detailSequence,r=currentResource;if(!r)return;
  $('#detail-drawer').innerHTML=`<div class="drawer-top"><span>${esc(r.kind.toUpperCase())} / LIVE RESOURCE</span><button class="icon-button drawer-close" aria-label="Close details">×</button></div><h2>${esc(r.name)}</h2>${statusPill(r.status)}<div class="detail-tabs">${['Overview',...(r.kind==='Pods'?['Logs']:[]),'YAML'].map(t=>`<button class="${detailTab===t?'active':''}" data-detail-tab="${t}">${t}</button>`).join('')}</div><div id="detail-content"></div>`;
  $('.drawer-close').onclick=closeDrawer;$$('[data-detail-tab]').forEach(b=>b.onclick=()=>{detailTab=b.dataset.detailTab;renderDetail();});
  if(detailTab==='Overview'){
    $('#detail-content').innerHTML=`<div class="detail-facts">${[['Namespace',r.namespace],['Age',r.age],['Ready / type',r.ready],['Node / target',r.node],['CPU',r.cpu===null?'Unavailable':r.cpu+(r.kind==='Nodes'?'%':'m')],['Memory',r.memory===null?'Unavailable':r.memory+(r.kind==='Nodes'?'%':'Mi')]].map(([k,v])=>`<div><span>${k}</span><strong>${esc(v)}</strong></div>`).join('')}</div><h3 class="detail-subtitle">Labels</h3><pre class="code-block">${esc(JSON.stringify(r.raw.metadata.labels||{},null,2))}</pre><p class="detail-note">Live Kubernetes resource · Read-only dashboard. No restart, deletion, or configuration changes are performed.</p>`;return;
  }
  $('#detail-content').textContent='Loading from Kubernetes…';
  try{
    const query=new URLSearchParams({kind:r.kind,name:r.resourceName,namespace:r.namespace});
    const data=await api((detailTab==='Logs'?'logs':'manifest')+'?'+query);
    if(seq!==detailSequence||currentResource!==r||$('#detail-drawer').hidden)return;
    $('#detail-content').innerHTML=`<pre class="code-block">${esc(data.text)}</pre>${detailTab==='Logs'?'<button class="button" id="reload-detail">Refresh logs</button>':'<button class="button" id="download-yaml">Download YAML</button>'}`;
    if($('#reload-detail'))$('#reload-detail').onclick=()=>renderDetail();
    if($('#download-yaml'))$('#download-yaml').onclick=()=>{const url=URL.createObjectURL(new Blob([data.text],{type:'text/yaml'}));const a=document.createElement('a');a.href=url;a.download=r.resourceName+'.yaml';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);};
  }catch(e){if(seq===detailSequence&&currentResource===r&&!$('#detail-drawer').hidden)$('#detail-content').textContent=e.message;}
};
openUtility=function(page){
  $('#detail-drawer').hidden=false;$('#drawer-backdrop').hidden=false;
  const storage=page==='Storage',items=liveSnapshot?.data.Storage||[];
  const text=storage?(items.map(r=>`${r.metadata.namespace}/${r.metadata.name}\n  ${r.status?.phase||'Unknown'} · ${r.status?.capacity?.storage||'—'} · ${r.spec?.storageClassName||'default class'}`).join('\n\n')||liveSnapshot?.errors.Storage||'No persistent volume claims available.'):`Context: ${cluster}\nMode: read only\nRefresh: every 15 seconds\nLast snapshot: ${liveSnapshot?.observedAt||'Not connected'}\n\n${Object.entries(liveSnapshot?.errors||{}).map(([key,v])=>key+': '+v).join('\n')||'No resource errors reported.'}`;
  $('#detail-drawer').innerHTML=`<div class="drawer-top"><span>${storage?'PERSISTENT VOLUME CLAIMS':'CONNECTION DETAILS'}</span><button class="icon-button drawer-close" aria-label="Close">×</button></div><h2>${storage?'Storage':'Cluster connection'}</h2><pre class="code-block">${esc(text)}</pre>`;$('.drawer-close').onclick=closeDrawer;$('.drawer-close').focus();
};
refresh=async function(){
  if(liveBusy)return;liveBusy=true;setConnection('Refreshing cluster data',false);
  try{
    const snapshot=await api('snapshot');
    if(!snapshot.connected)throw new Error(Object.values(snapshot.errors)[0]||'Cluster connection failed.');
    if(liveSnapshot&&snapshot.context!==liveSnapshot.context){liveHistory.splice(0);closeDrawer();}
    liveSnapshot=snapshot;liveError='';replaceData(snapshot);updateIdentity();
    const oldNamespace=$('#namespace-filter').value,ns=[...new Set([...pods,...deployments,...services,...events].map(r=>r.namespace))].sort();
    $('#namespace-filter').innerHTML='<option value="all">All namespaces</option>'+ns.map(n=>`<option>${esc(n)}</option>`).join('');if(ns.includes(oldNamespace))$('#namespace-filter').value=oldNamespace;
    const usage=utilization();if(usage&&liveHistory.at(-1)?.time!==Date.parse(snapshot.observedAt)){liveHistory.push({time:Date.parse(snapshot.observedAt),cpu:usage.cpu,memory:usage.memory});if(liveHistory.length>5760)liveHistory.shift();}
    const partial=Object.keys(snapshot.errors).some(k=>!k.endsWith('Metrics'));
    setConnection(partial?'Connected · some resources unavailable':'Connected · live Kubernetes data',true);
  }catch(e){liveError=e.message;setConnection('Not connected · '+(liveSnapshot?'showing last snapshot':'no cluster data'),false);}
  finally{
    liveBusy=false;$('#last-updated').textContent=liveError?'Connection unavailable':'Refreshes every 15s';
    let banner=$('#connection-message');if(!banner){banner=document.createElement('div');banner.id='connection-message';banner.className='connection-message';$('.cluster-banner').after(banner);}
    const errors=liveSnapshot?Object.entries(liveSnapshot.errors).filter(([key])=>!key.endsWith('Metrics')).map(([key,message])=>key+': '+message):[];
    banner.hidden=!liveError&&!errors.length;banner.textContent=liveError||errors.join(' · ');
    renderMetrics();renderTabs();renderTable();renderActivity();renderChart();
  }
};
switchCluster=function(){openUtility('Configuration');};
$('#cluster-switch').onclick=switchCluster;$('#refresh-button').onclick=()=>refresh();
commands.find(c=>c.label==='Refresh resources').action=()=>refresh();
const switchCommand=commands.find(c=>c.label==='Switch demo cluster');switchCommand.label='Cluster connection';switchCommand.action=switchCluster;
$('.profile strong').textContent='Cluster observer';$('.profile small').textContent='Host Kubernetes login';$$('.avatar,.small-avatar').forEach(el=>el.textContent='K8');
$('#page-description').textContent='Live Kubernetes resources from your configured context.';
updateIdentity();renderMetrics();renderTabs();renderTable();renderActivity();renderChart();
refresh();setInterval(()=>{if(!document.hidden)refresh();},15000);

}
