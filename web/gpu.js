/* NVIDIA DCGM page, composed with the existing read-only k9s UI. */
(() => {
  'use strict';
  const q = s => document.querySelector(s);
  const escape = v => String(v ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const number = (v, digits = 1) => typeof v === 'number' && Number.isFinite(v) ? v.toFixed(digits) : '—';
  const sum = xs => xs.reduce((a,b) => a+b,0);
  const finite = v => typeof v === 'number' && Number.isFinite(v);
  let snapshot = null, failure = '', busy = false, pageOpen = false;
  const summary = document.createElement('section');
  summary.id = 'gpu-overview'; summary.className = 'panel gpu-overview';
  summary.innerHTML = '<div><strong>GPU utilization</strong><span id="gpu-summary">Connecting to GPU metrics…</span></div><button class="button" id="gpu-open">View GPUs ↗</button>';
  q('#metrics').after(summary);
  const panel = document.createElement('section');
  panel.id = 'gpu-page'; panel.hidden = true;
  panel.innerHTML = '<div class="gpu-cards" id="gpu-cards"></div><div class="gpu-status" role="status" id="gpu-status"></div><article class="panel"><div class="panel-heading"><div><h2>GPU devices</h2><p>Live device measurements · VRAM excludes driver-reserved memory</p></div><label class="select-wrap">Node <select id="gpu-node" aria-label="Filter GPUs by node"><option value="">All nodes</option></select></label></div><div class="table-scroll"><table class="gpu-table"><thead><tr><th>Device / model</th><th>Node</th><th>GPU utilization</th><th>VRAM used / usable</th><th>Assigned workload</th></tr></thead><tbody id="gpu-rows"></tbody></table></div><div class="gpu-note">Utilization measures GPU activity; allocated memory can remain high while utilization is 0%. Workload labels identify device assignments, not per-process utilization.</div></article>';
  summary.after(panel);
  navItems.splice(3, 0, ['GPUs','cpu','']);
  const baseNavigate = navigate;
  navigate = function(page) {
    if (page === 'GPUs') {
      pageOpen = true; currentPage = page; closeDrawer();
      document.body.classList.add('gpu-view'); panel.hidden = false;
      q('#page-title').textContent = 'GPU utilization';
      q('#page-description').textContent = 'Live GPU activity and memory usage across your cluster.';
      q('#breadcrumb-page').textContent = 'GPUs';
      renderNav(); render(); poll();
      return;
    }
    if (page !== 'Configuration' && page !== 'Storage') {
      pageOpen = false; document.body.classList.remove('gpu-view'); panel.hidden = true;
    }
    baseNavigate(page);
  };
  commands.push({label:'GPUs', icon:'cpu', action:()=>navigate('GPUs'), hint:'Live GPU utilization'});
  q('#gpu-open').onclick = () => navigate('GPUs');
  q('#gpu-node').onchange = render;
  renderNav();
  function meter(value) {
    return '<div class="gpu-meter"><span style="width:'+ (finite(value) ? Math.max(0,Math.min(100,value)) : 0) +'%"></span></div>';
  }
  function render() {
    const all = snapshot?.devices || [];
    const utilization = all.map(d => d.utilization).filter(finite);
    const memory = all.filter(d => finite(d.memoryUsedMiB));
    const avg = utilization.length ? sum(utilization) / utilization.length : null;
    const used = memory.length ? sum(memory.map(d=>d.memoryUsedMiB))/1024 : null;
    const incomplete = snapshot && (snapshot.stale || Object.keys(snapshot.errors || {}).length > 0);
    const label = failure || snapshot?.stale ? 'Last known GPU measurements' : incomplete ? 'Partial GPU measurements' : 'GPU utilization';
    q('#gpu-overview strong').textContent = label;
    q('#gpu-summary').textContent = snapshot ? `${all.length} GPUs · ${number(avg)}${avg === null ? '' : '% average'} · ${number(used)} GiB VRAM used` : failure || 'Connecting to GPU metrics…';
    navItems.find(x=>x[0]==='GPUs')[2] = all.length ? String(all.length) : '';
    renderNav();
    const nodes = [...new Set(all.map(d=>d.node))].sort();
    const selection = q('#gpu-node').value;
    q('#gpu-node').innerHTML = '<option value="">All nodes</option>' + nodes.map(n=>`<option value="${escape(n)}">${escape(n)}</option>`).join('');
    if (nodes.includes(selection)) q('#gpu-node').value = selection;
    const selectedNode = q('#gpu-node').value;
    const rows = all.filter(d=>!selectedNode || d.node===selectedNode);
    const cards = [
      ['GPU devices',all.length || (snapshot ? 0 : '—'), `${nodes.length} nodes reporting`],
      ['Average GPU activity',number(avg)+(avg===null?'':'%'),`${utilization.length}/${all.length} devices measured`],
      ['VRAM used',number(used)+(used===null?'':' GiB'),`${memory.length}/${all.length} devices measured`],
      ['Exporters reporting',snapshot ? `${snapshot.exportersHealthy}/${snapshot.exportersExpected}` : '—','Refreshes every 15 seconds']
    ];
    q('#gpu-cards').innerHTML = cards.map(([title,value,note])=>`<article class="metric"><div class="metric-label">${escape(title)}</div><div class="metric-value">${escape(value)}</div><div class="metric-foot">${escape(note)}</div></article>`).join('');
    const issues = Object.entries(snapshot?.errors || {}).map(([source,message])=>source+': '+message);
    const time = snapshot?.observedAt ? new Date(snapshot.observedAt).toLocaleTimeString() : null;
    q('#gpu-status').textContent = [failure, snapshot?.stale ? 'Stale data — exporter refresh failed.' : '', issues.join(' · '), time ? `Observed ${time} · NVIDIA DCGM` : 'Waiting for GPU measurements'].filter(Boolean).join(' ');
    q('#gpu-status').classList.toggle('gpu-warning', Boolean(failure || incomplete));
    q('#gpu-rows').innerHTML = rows.length ? rows.map(d => {
      const device = 'GPU '+d.gpu+(d.instance ? ' / MIG '+d.instance : '');
      const used = finite(d.memoryUsedMiB) ? d.memoryUsedMiB/1024 : null;
      const usable = finite(d.memoryUsableMiB) ? d.memoryUsableMiB/1024 : null;
      const workloads = d.workloads.length ? d.workloads.map(w=>`<div class="gpu-workload">${escape(w.pod)}<small>${escape(w.namespace)} · ${escape(w.container)}</small></div>`).join('') : '<span class="gpu-muted">No workload label reported</span>';
      return `<tr><td><strong>${escape(device)}</strong><small>${escape(d.model)}</small><small title="${escape(d.uuid)}">${escape(d.uuid)}</small></td><td>${escape(d.node)}</td><td><strong>${finite(d.utilization)?number(d.utilization)+'%':'Unavailable'}</strong>${meter(d.utilization)}</td><td><strong>${number(used)} / ${number(usable)} GiB</strong>${meter(d.memoryPercent)}<small>${finite(d.memoryPercent)?number(d.memoryPercent)+'% used':'Memory measurement unavailable'}</small></td><td>${workloads}</td></tr>`;
    }).join('') : '<tr><td colspan="5" class="empty-state">'+(snapshot && !issues.length ? 'No GPU devices reported.' : 'GPU measurements unavailable. Check the exporter status above.')+'</td></tr>';
  }
  async function poll() {
    if (busy) return;
    busy = true;
    const controller = new AbortController(), timeout = setTimeout(()=>controller.abort(),30000);
    try {
      const response = await fetch('/k9s/api/gpu',{credentials:'same-origin',cache:'no-store',signal:controller.signal});
      if (!response.ok) throw new Error('GPU metrics request failed (HTTP '+response.status+').');
      snapshot = await response.json(); failure = '';
    } catch (error) { failure = error.name === 'AbortError' ? 'GPU metrics refresh timed out.' : error.message; }
    finally { clearTimeout(timeout); busy = false; render(); }
  }
  q('#refresh-button').addEventListener('click',poll);
  document.addEventListener('keydown',event=>{
    if (pageOpen && ['ArrowDown','ArrowUp','Enter','/'].includes(event.key) && event.target===document.body) event.stopImmediatePropagation();
  },true);
  document.addEventListener('visibilitychange',()=>{if(!document.hidden)poll();});
  render(); poll(); setInterval(()=>{if(!document.hidden)poll();},15000);
})();
