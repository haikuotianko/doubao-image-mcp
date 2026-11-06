import './style.css';

import {EventsOn} from '../wailsjs/runtime/runtime';
import {
    ChooseOutDir,
    LoginDialog,
    OpenOutDir,
    RemoveAccount,
    ReorderAccounts,
    SaveImageAs,
    SaveSettings,
    SetAccountEnabled,
    ShowInFolder,
    StartHTTP,
    Status,
    StopHTTP,
    TestImage,
    UseAccount,
    ViewSession,
} from '../wailsjs/go/main/App';

const state = {
    status: null,
    busy: false,
};

const el = (id) => document.getElementById(id);

document.querySelector('#app').innerHTML = `
<header>
  <h1>豆包生图 MCP</h1>
  <span class="badge off" id="accountBadge">未登录</span>
</header>

<main>
  <div class="col">
    <section class="card">
      <div class="card-head">
        <h2>账号</h2>
        <button class="primary sm" id="btnLogin">扫码登录</button>
      </div>
      <div class="accounts" id="accounts"><div class="empty">还没有账号，点右上角「扫码登录」。</div></div>
      <div class="hint">可以登录多个账号：每点一次「扫码登录」都会清掉浏览器旧登录态、重新出二维码。拖动账号可调整顺序，生图按顺序排队到空闲的已启用账号。</div>
    </section>

    <section class="card">
      <h2>设置</h2>
      <div class="grid2">
        <label>输出目录</label>
        <div class="row">
          <input type="text" id="outDir" readonly />
          <button id="btnChooseDir">选择</button>
          <button id="btnOpenDir">打开</button>
        </div>
        <label>默认模型</label>
        <select id="defaultModel"></select>
        <label>默认比例</label>
        <select id="defaultRatio"></select>
        <label>默认风格</label>
        <select id="defaultStyle"></select>
        <label>默认账号</label>
        <select id="defaultAccount"></select>
        <label>比例修正</label>
        <label style="color:var(--text)">
          <input type="checkbox" id="fitRatio" style="width:auto;margin-right:6px" />
          结果图裁剪/补边到请求比例
        </label>
        <label>HTTP 地址</label>
        <input type="text" id="httpAddr" placeholder="127.0.0.1:8770" />
        <label>自动启动</label>
        <label style="color:var(--text)">
          <input type="checkbox" id="autoStartHttp" style="width:auto;margin-right:6px" />
          GUI 启动时自动开启 MCP HTTP 服务
        </label>
      </div>
      <div class="row" style="margin-top:12px">
        <button class="primary" id="btnSave">保存设置</button>
      </div>
    </section>

    <section class="card">
      <h2>MCP 服务</h2>
      <div class="row">
        <button id="btnHttp">启动 HTTP</button>
        <span class="badge off" id="httpBadge">HTTP 未启动</span>
      </div>
      <div class="hint">默认账号留空就用列表里第一个启用的账号。生图请求按「默认账号优先、其余按列表顺序」排队到空闲账号。</div>
      <div class="hint">stdio 模式：把 command 指向本程序、args 加 <code>-mcp</code> 即可（下面的路径按当前机器自动生成）；日志写到配置目录的 mcp.log。</div>
      <code class="box" id="mcpJson"></code>
    </section>
  </div>

  <div class="col right">
    <section class="card">
      <h2>测试生图</h2>
      <textarea id="prompt" placeholder="画面描述，例如：赛博朋克风格的重庆夜景，霓虹灯、雨夜、电影感"></textarea>
      <div class="row" style="margin-top:10px">
        <select id="testAccount" style="flex:1 1 100%"></select>
      </div>
      <div class="row" style="margin-top:8px">
        <select id="testRatio" style="width:120px"></select>
        <select id="testModel" style="width:220px"></select>
        <select id="testStyle" style="width:180px"></select>
        <button class="primary" id="btnTest">生成</button>
      </div>
      <div class="hint" id="testMsg"></div>
      <div class="thumbs" id="thumbs"></div>
    </section>

    <section class="card grow">
      <h2>日志</h2>
      <div class="logs" id="logs"></div>
    </section>
  </div>
</main>
`;

// ---------------------------------------------------------------- 渲染

function renderAccounts(status) {
  const box = el('accounts');
  const badge = el('accountBadge');
  const accounts = status.accounts || [];
  const enabledCount = accounts.filter((a) => a.enabled).length;

  if (status.activeAccount) {
    const active = accounts.find((a) => a.name === status.activeAccount);
    badge.textContent = `默认账号：${(active && (active.nickname || active.name)) || status.activeAccount}`;
    badge.className = 'badge on';
  } else if (enabledCount > 0) {
    badge.textContent = `${enabledCount} 个账号可调度`;
    badge.className = 'badge on';
  } else {
    badge.textContent = '未登录';
    badge.className = 'badge off';
  }
  if (status.waiting > 0) {
    badge.textContent += ` · 排队 ${status.waiting}`;
  }

  if (accounts.length === 0) {
    box.innerHTML = '<div class="empty">还没有账号，点右上角「扫码登录」（可以连续加多个）。</div>';
    return;
  }
  box.innerHTML = '';
  for (const acc of accounts) {
    const row = document.createElement('div');
    row.className = 'account' + (acc.active ? ' active' : '') + (acc.enabled ? '' : ' disabled');
    row.draggable = true;
    row.dataset.name = acc.name;

    // 拖拽排序：顺序就是生图时的优先级
    row.ondragstart = (e) => {
      dragName = acc.name;
      row.classList.add('dragging');
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', acc.name);
    };
    row.ondragend = () => {
      dragName = null;
      row.classList.remove('dragging');
    };
    row.ondragover = (e) => {
      if (!dragName || dragName === acc.name) return;
      e.preventDefault();
      row.classList.add('drag-over');
    };
    row.ondragleave = () => row.classList.remove('drag-over');
    row.ondrop = async (e) => {
      e.preventDefault();
      row.classList.remove('drag-over');
      if (!dragName || dragName === acc.name) return;
      const names = accounts.map((a) => a.name);
      const from = names.indexOf(dragName);
      const to = names.indexOf(acc.name);
      if (from < 0 || to < 0) return;
      names.splice(to, 0, names.splice(from, 1)[0]);
      dragName = null;
      try {
        await ReorderAccounts(names);
      } catch (err) {
        appendLog('调整顺序失败：' + (err && err.message ? err.message : err));
      }
      refresh();
    };

    const handle = document.createElement('span');
    handle.className = 'drag-handle';
    handle.textContent = '⠿';
    handle.title = '拖动调整顺序';
    row.appendChild(handle);

    // 头像：有图用图，没有就用昵称首字兜底
    const avatar = document.createElement('div');
    avatar.className = 'avatar';
    if (acc.avatar) {
      const img = document.createElement('img');
      img.src = acc.avatar;
      img.alt = '';
      img.draggable = false;
      avatar.appendChild(img);
    } else {
      avatar.textContent = (acc.nickname || acc.name || '?').trim().slice(0, 1).toUpperCase();
    }
    row.appendChild(avatar);

    const name = acc.nickname || acc.name;
    const tags = [];
    if (acc.active) tags.push('<span class="tag on">默认</span>');
    if (acc.busy) tags.push('<span class="tag busy">生成中</span>');
    if (!acc.enabled) tags.push('<span class="tag off">已停用</span>');

    const info = document.createElement('div');
    info.className = 'grow';
    info.innerHTML = `
      <div class="name">${escapeHtml(name)} ${tags.join(' ')}</div>
      <div class="meta">${escapeHtml(acc.name)}${acc.cookieCount ? ` · ${acc.cookieCount} 项 cookie` : ''} · 添加于 ${escapeHtml(acc.addedAt || '未知')}</div>`;
    row.appendChild(info);

    // 启用开关：停用后不参与自动排队
    const toggle = document.createElement('label');
    toggle.className = 'switch';
    toggle.title = acc.enabled ? '点击停用（不参与自动排队）' : '点击启用';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = !!acc.enabled;
    cb.onchange = () => act(() => SetAccountEnabled(acc.name, cb.checked), cb.checked ? '已启用' : '已停用');
    toggle.appendChild(cb);
    const knob = document.createElement('span');
    knob.className = 'knob';
    toggle.appendChild(knob);
    row.appendChild(toggle);

    const viewBtn = document.createElement('button');
    viewBtn.className = 'sm';
    viewBtn.textContent = '会话';
    viewBtn.title = '用这个账号的 cookie 打开豆包，看登录态还在不在';
    viewBtn.onclick = () => openSession(acc.name);
    row.appendChild(viewBtn);

    const delBtn = document.createElement('button');
    delBtn.className = 'danger sm';
    delBtn.textContent = '删除';
    delBtn.onclick = () => act(() => RemoveAccount(acc.name), '已删除账号');
    row.appendChild(delBtn);
    box.appendChild(row);
  }
}

// dragName 记录正在拖的账号（HTML5 拖拽本身带 dataTransfer，这里图省事）。
let dragName = null;

// openSession 打开某个账号的会话窗口（name 为空则用当前账号）。
async function openSession(name) {
  try {
    await ViewSession(name || '');
  } catch (err) {
    appendLog('打开会话窗口失败：' + (err && err.message ? err.message : err));
  } finally {
    refresh();
  }
}

function fillSelect(select, options, value) {
  select.innerHTML = '';
  for (const opt of options) {
    const o = document.createElement('option');
    o.value = opt.value;
    o.textContent = opt.label;
    select.appendChild(o);
  }
  if (value !== undefined && value !== null) select.value = value;
}

function renderStatus(status) {
  state.status = status;
  renderAccounts(status);

  el('outDir').value = status.outDir || '';
  el('httpAddr').value = status.httpAddr || '';
  el('fitRatio').checked = !!status.fitRatio;
  el('autoStartHttp').checked = !!status.autoStartHttp;

  const models = (status.models || []).map((m) => ({
    value: m.name,
    label: m.name + (m.needUpgrade ? '（需订阅）' : ''),
  }));
  const ratios = (status.ratios || []).map((r) => ({value: r, label: r}));
  const styles = [{value: '', label: '不指定风格'}].concat(
      (status.styles || []).map((s) => ({value: s.key, label: s.name})));

  // 刷新时保留当前选择：否则切换账号触发的刷新会把你在测试面板挑的模型/比例打回默认。
  const keep = (id) => el(id).value;

  fillSelect(el('defaultModel'), models, keep('defaultModel') || status.defaultModel);
  fillSelect(el('defaultRatio'), ratios, keep('defaultRatio') || status.defaultRatio);
  fillSelect(el('defaultStyle'), styles, keep('defaultStyle') || status.defaultStyle || '');
  fillSelect(el('testModel'), models, keep('testModel') || status.defaultModel);
  fillSelect(el('testRatio'), ratios, keep('testRatio') || status.defaultRatio);
  fillSelect(el('testStyle'), styles, keep('testStyle') || status.defaultStyle || '');

  // 默认账号 / 测试账号都只列启用中的账号：停用的账号不参与调度，选了也没用。
  const enabledAccounts = (status.accounts || []).filter((a) => a.enabled);
  const accountOptions = [{value: '', label: '（留空 = 用列表里第一个启用的账号）'}].concat(
      enabledAccounts.map((a) => ({value: a.name, label: a.nickname || a.name})));
  fillSelect(el('defaultAccount'), accountOptions, status.activeAccount || '');

  const testAccountOptions = [{value: '', label: '自动（任意空闲账号）'}].concat(
      enabledAccounts.map((a) => ({value: a.name, label: `用账号：${a.nickname || a.name}`})));
  fillSelect(el('testAccount'), testAccountOptions, keep('testAccount') || '');

  renderHTTP(status.httpRunning);
  renderLogs(status.logs || []);
}

function renderHTTP(running) {
  const badge = el('httpBadge');
  const addr = (state.status && state.status.httpAddr) || '127.0.0.1:8770';
  badge.textContent = running ? `HTTP 运行中 · ${addr}` : 'HTTP 未启动';
  badge.className = 'badge ' + (running ? 'on' : 'off');
  el('btnHttp').textContent = running ? '停止 HTTP' : '启动 HTTP';
  el('btnHttp').classList.toggle('primary', !running);

  // 路径必须取当前这台机器上真实的 exe 位置，写死会让别人的配置指向不存在的文件。
  // 后端用 os.Executable() 取，Windows 是 ...\doubao-image-mcp.exe，macOS/Linux 是对应二进制。
  const exePath = (state.status && state.status.exePath) || 'doubao-image-mcp.exe';
  el('mcpJson').textContent =
      '// HTTP 方式\n' + JSON.stringify({
        mcpServers: {
          doubao: {
            type: 'http',
            url: `http://${addr}/mcp`,
          },
        },
      }, null, 2) + '\n\n// stdio 方式\n' + JSON.stringify({
        mcpServers: {
          doubao: {command: exePath, args: ['-mcp']},
        },
      }, null, 2);
}

function renderLogs(lines) {
  el('logs').textContent = lines.join('\n');
  el('logs').scrollTop = el('logs').scrollHeight;
}

function appendLog(line) {
  const box = el('logs');
  box.textContent += (box.textContent ? '\n' : '') + line;
  box.scrollTop = box.scrollHeight;
}

function escapeHtml(text) {
  return String(text == null ? '' : text).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}

// ---------------------------------------------------------------- 交互

async function run(fn, okMessage) {
  try {
    const result = await fn();
    if (okMessage) appendLog(okMessage);
    return result;
  } catch (err) {
    appendLog('错误：' + (err && err.message ? err.message : err));
    return undefined;
  }
}

async function refresh() {
  const status = await run(() => Status());
  if (status) renderStatus(status);
}

// act 用于「改动类」操作：执行成功后立刻刷新界面。
// 开关、删除、默认账号这些必须马上反映到卡片和下拉，否则会看到过期的灰色状态。
async function act(fn, okMessage) {
  const result = await run(fn, okMessage);
  await refresh();
  return result;
}

el('btnLogin').onclick = async () => {
  el('btnLogin').disabled = true;
  el('btnLogin').textContent = '等待扫码…';
  try {
    const result = await LoginDialog();
    if (result && result.verified) {
      appendLog('登录成功：' + result.account);
    } else if (result && result.message) {
      appendLog('登录完成但校验未通过：' + result.message);
    }
  } catch (err) {
    appendLog('登录失败：' + (err && err.message ? err.message : err));
  } finally {
    el('btnLogin').disabled = false;
    el('btnLogin').textContent = '扫码登录';
    refresh();
  }
};

// 默认账号切换：留空表示按列表顺序取第一个启用的账号。
el('defaultAccount').onchange = () => act(() => UseAccount(el('defaultAccount').value), '默认账号已更新');

// 大图查看：点缩略图弹出，Esc / 点背景 / 关闭按钮退出，可另存或定位文件。
function showLightbox(img) {
  const overlay = document.createElement('div');
  overlay.className = 'lightbox';

  const inner = document.createElement('div');
  inner.className = 'lb-inner';

  const pic = document.createElement('img');
  pic.src = img.url || '';
  pic.alt = '';
  inner.appendChild(pic);

  const bar = document.createElement('div');
  bar.className = 'lb-bar';

  const meta = document.createElement('span');
  meta.className = 'lb-meta';
  const size = img.width && img.height ? `${img.width}×${img.height}` : '尺寸未知';
  meta.textContent = `${size}${img.format ? ' · ' + img.format : ''}`;
  bar.appendChild(meta);

  const spacer = document.createElement('span');
  spacer.className = 'spacer';
  bar.appendChild(spacer);

  const btnSave = document.createElement('button');
  btnSave.textContent = '另存到…';
  btnSave.onclick = async () => {
    btnSave.disabled = true;
    try {
      const saved = await SaveImageAs(img.path || '');
      if (saved) appendLog('已另存为 ' + saved);
    } catch (err) {
      appendLog('另存失败：' + (err && err.message ? err.message : err));
    } finally {
      btnSave.disabled = false;
    }
  };
  bar.appendChild(btnSave);

  const btnFolder = document.createElement('button');
  btnFolder.textContent = '所在文件夹';
  btnFolder.onclick = async () => {
    try {
      await ShowInFolder(img.path || '');
    } catch (err) {
      appendLog('打开文件夹失败：' + (err && err.message ? err.message : err));
    }
  };
  bar.appendChild(btnFolder);

  const btnClose = document.createElement('button');
  btnClose.className = 'primary';
  btnClose.textContent = '关闭';
  btnClose.onclick = () => overlay.remove();
  bar.appendChild(btnClose);

  inner.appendChild(bar);
  overlay.appendChild(inner);
  overlay.onclick = (e) => {
    if (e.target === overlay) overlay.remove();
  };
  document.body.appendChild(overlay);
}

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') {
    const lb = document.querySelector('.lightbox');
    if (lb) lb.remove();
  }
});

el('btnChooseDir').onclick = async () => {
  const dir = await run(() => ChooseOutDir());
  if (dir) el('outDir').value = dir;
};

el('btnOpenDir').onclick = () => run(() => OpenOutDir());

el('btnSave').onclick = async () => {
  const payload = {
    outDir: el('outDir').value,
    defaultModel: el('defaultModel').value,
    defaultRatio: el('defaultRatio').value,
    defaultStyle: el('defaultStyle').value,
    fitRatio: el('fitRatio').checked,
    httpAddr: el('httpAddr').value,
    autoStartHttp: el('autoStartHttp').checked,
  };
  await run(() => SaveSettings(payload), '设置已保存');
  refresh();
};

el('btnHttp').onclick = async () => {
  const running = state.status && state.status.httpRunning;
  if (running) {
    await run(() => StopHTTP(), 'HTTP 已停止');
  } else {
    await run(() => StartHTTP(), 'HTTP 已启动');
  }
  refresh();
};

el('btnTest').onclick = async () => {
  const prompt = el('prompt').value.trim();
  if (!prompt) {
    appendLog('请先填画面描述');
    return;
  }
  el('btnTest').disabled = true;
  el('testMsg').textContent = '生成中，通常 10-40 秒…';
  el('thumbs').innerHTML = '';
  try {
    const result = await TestImage(
        prompt,
        el('testRatio').value,
        el('testModel').value,
        el('testStyle').value,
        el('testAccount').value);
    const images = (result.images || []).filter((img) => img.url);
    el('testMsg').textContent = `${result.message || ''}${images.length > 1 ? `（点任意一张看大图）` : ''}`;
    for (const img of images) {
      const im = document.createElement('img');
      im.src = img.url;
      im.title = `点击查看大图${img.path ? '\n' + img.path : ''}`;
      im.loading = 'lazy';
      im.onclick = () => showLightbox(img);
      el('thumbs').appendChild(im);
    }
  } catch (err) {
    el('testMsg').textContent = '失败：' + (err && err.message ? err.message : err);
  } finally {
    el('btnTest').disabled = false;
  }
};

EventsOn('log', (line) => appendLog(line));

refresh();
